package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog/log"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/sadensmol/autopilot/db"
	"github.com/sadensmol/autopilot/internal/domain"
)

// DBFileName is the state file inside the settings folder.
const DBFileName = "autopilot.db"

func Open(ctx context.Context, path string) (*sql.DB, error) {
	// _txlock=immediate takes the write lock at BEGIN, so busy_timeout waits for it; a deferred
	// tx that reads first fails at once with SQLITE_BUSY_SNAPSHOT when another writer commits.
	pragmas := url.Values{
		"_pragma": {"journal_mode(WAL)", "busy_timeout(5000)", "foreign_keys(1)"},
		"_txlock": {"immediate"},
	}
	dsn := (&url.URL{Scheme: "file", Opaque: url.PathEscape(path), RawQuery: pragmas.Encode()}).String()
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot open sqlite %s: %w", path, err)
	}
	if err := migrate(ctx, sqlDB); err != nil {
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.Error().Ctx(ctx).Err(closeErr).Msgf("cannot close sqlite %s after failed migration", path)
		}
		return nil, err
	}
	return sqlDB, nil
}

func migrate(ctx context.Context, sqlDB *sql.DB) error {
	migrations, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("cannot find embedded migrations: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations)
	if err != nil {
		return fmt.Errorf("cannot create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("cannot apply migrations: %w", err)
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatOptionalTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(t), Valid: true}
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot parse time %q: %w", s, err)
	}
	return t, nil
}

func parseOptionalTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return parseTime(s.String)
}

func isUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}

func requireAffected(result sql.Result, what string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot count rows of %s: %w", what, err)
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", what, domain.ErrNotFound)
	}
	return nil
}

// decodeStrings reads a JSON string list; an empty list is nil, as a list that was never set.
func decodeStrings(raw string) ([]string, error) {
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("cannot decode %q: %w", raw, err)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}

// encodeList writes a list as JSON; a nil list is an empty one.
func encodeList(list any) (string, error) {
	body, err := json.Marshal(list)
	if err != nil {
		return "", err
	}
	if string(body) == "null" {
		return "[]", nil
	}
	return string(body), nil
}
