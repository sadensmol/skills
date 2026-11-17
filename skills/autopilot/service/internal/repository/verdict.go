package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sadensmol/autopilot/internal/domain"
)

const verdictColumns = `issue_key, repo, sha, checks, manual, red_reproofs, created_at`

type Verdict struct {
	db *sql.DB
}

func NewVerdict(db *sql.DB) *Verdict {
	return &Verdict{db: db}
}

// Save appends a verdict; a rerun at the same SHA adds a newer row.
func (v *Verdict) Save(ctx context.Context, verdict domain.Verdict, now time.Time) error {
	checks, err := json.Marshal(verdict.Checks)
	if err != nil {
		return fmt.Errorf("cannot encode checks of %s in run %s: %w", verdict.Repo, verdict.IssueKey, err)
	}
	if verdict.Checks == nil {
		checks = []byte("[]")
	}
	manual, err := encodeList(verdict.Manual)
	if err != nil {
		return fmt.Errorf("cannot encode manual checks of %s in run %s: %w", verdict.Repo, verdict.IssueKey, err)
	}
	reProofs, err := encodeList(verdict.RedReProofs)
	if err != nil {
		return fmt.Errorf("cannot encode red re-proofs of %s in run %s: %w", verdict.Repo, verdict.IssueKey, err)
	}
	_, err = v.db.ExecContext(ctx, `
INSERT INTO verdict (issue_key, repo, sha, checks, manual, red_reproofs, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		verdict.IssueKey, verdict.Repo, verdict.SHA, string(checks), manual, reProofs, formatTime(now))
	if err != nil {
		return fmt.Errorf("cannot save verdict of %s in run %s: %w", verdict.Repo, verdict.IssueKey, err)
	}
	return nil
}

func (v *Verdict) Latest(ctx context.Context, key domain.IssueKey, repo string) (domain.Verdict, error) {
	verdict, err := scanVerdict(v.db.QueryRowContext(ctx, `
SELECT `+verdictColumns+` FROM verdict WHERE issue_key = ? AND repo = ? ORDER BY id DESC LIMIT 1`, key, repo))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Verdict{}, fmt.Errorf("verdict of %s in run %s: %w", repo, key, domain.ErrNotFound)
	}
	if err != nil {
		return domain.Verdict{}, fmt.Errorf("cannot find verdict of %s in run %s: %w", repo, key, err)
	}
	return verdict, nil
}

func (v *Verdict) List(ctx context.Context, key domain.IssueKey) ([]domain.Verdict, error) {
	rows, err := v.db.QueryContext(ctx, `SELECT `+verdictColumns+` FROM verdict WHERE issue_key = ? ORDER BY id`, key)
	if err != nil {
		return nil, fmt.Errorf("cannot list verdicts of run %s: %w", key, err)
	}
	defer rows.Close() //nolint:errcheck // a read cursor; Err() below reports the failure
	var verdicts []domain.Verdict
	for rows.Next() {
		verdict, err := scanVerdict(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read verdict of run %s: %w", key, err)
		}
		verdicts = append(verdicts, verdict)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list verdicts of run %s: %w", key, err)
	}
	return verdicts, nil
}

func scanVerdict(row rowScanner) (domain.Verdict, error) {
	var verdict domain.Verdict
	var checks, manual, reProofs, createdAt string
	if err := row.Scan(&verdict.IssueKey, &verdict.Repo, &verdict.SHA, &checks, &manual, &reProofs, &createdAt); err != nil {
		return domain.Verdict{}, err
	}
	if err := json.Unmarshal([]byte(checks), &verdict.Checks); err != nil {
		return domain.Verdict{}, fmt.Errorf("cannot decode checks: %w", err)
	}
	var err error
	if verdict.Manual, err = decodeStrings(manual); err != nil {
		return domain.Verdict{}, fmt.Errorf("cannot decode manual checks: %w", err)
	}
	if verdict.RedReProofs, err = decodeStrings(reProofs); err != nil {
		return domain.Verdict{}, fmt.Errorf("cannot decode red re-proofs: %w", err)
	}
	if verdict.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Verdict{}, err
	}
	return verdict, nil
}
