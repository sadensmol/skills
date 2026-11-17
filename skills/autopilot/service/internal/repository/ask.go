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

const askColumns = `id, issue_key, question, options, status, answer, answered_by, dialog_id, created_at, closed_at`

type Ask struct {
	db *sql.DB
}

func NewAsk(db *sql.DB) *Ask {
	return &Ask{db: db}
}

type CreateAskData struct {
	IssueKey domain.IssueKey
	Question string
	Options  []domain.AskOption
	Now      time.Time
}

// Create stores an ask as open when the run has no open ask, else as queued behind it.
// One statement decides, and a unique index backs it, so two racing callers cannot both open.
func (a *Ask) Create(ctx context.Context, d CreateAskData) (domain.Ask, error) {
	options, err := json.Marshal(d.Options)
	if err != nil {
		return domain.Ask{}, fmt.Errorf("cannot encode options of ask for run %s: %w", d.IssueKey, err)
	}
	if d.Options == nil {
		options = []byte("[]")
	}
	var id int64
	err = a.db.QueryRowContext(ctx, `
INSERT INTO ask (issue_key, question, options, status, created_at)
VALUES (?, ?, ?,
    CASE WHEN EXISTS (SELECT 1 FROM ask WHERE issue_key = ? AND status = 'open') THEN 'queued' ELSE 'open' END,
    ?)
RETURNING id`, d.IssueKey, d.Question, string(options), d.IssueKey, formatTime(d.Now)).Scan(&id)
	if err != nil {
		return domain.Ask{}, fmt.Errorf("cannot create ask for run %s: %w", d.IssueKey, err)
	}
	return a.Find(ctx, domain.AskID(id))
}

func (a *Ask) Find(ctx context.Context, id domain.AskID) (domain.Ask, error) {
	ask, err := scanAsk(a.db.QueryRowContext(ctx, `SELECT `+askColumns+` FROM ask WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ask{}, fmt.Errorf("ask %d: %w", id, domain.ErrNotFound)
	}
	if err != nil {
		return domain.Ask{}, fmt.Errorf("cannot find ask %d: %w", id, err)
	}
	return ask, nil
}

func (a *Ask) FindOpen(ctx context.Context, key domain.IssueKey) (domain.Ask, error) {
	ask, err := scanAsk(a.db.QueryRowContext(ctx, `SELECT `+askColumns+` FROM ask WHERE issue_key = ? AND status = 'open'`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Ask{}, fmt.Errorf("open ask of run %s: %w", key, domain.ErrNotFound)
	}
	if err != nil {
		return domain.Ask{}, fmt.Errorf("cannot find open ask of run %s: %w", key, err)
	}
	return ask, nil
}

// SetDialog stores the id of the agterm dialog that shows the ask.
func (a *Ask) SetDialog(ctx context.Context, id domain.AskID, dialogID string) error {
	result, err := a.db.ExecContext(ctx, `UPDATE ask SET dialog_id = ? WHERE id = ?`, dialogID, id)
	if err != nil {
		return fmt.Errorf("cannot set dialog of ask %d: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot count rows of ask %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("ask %d: %w", id, domain.ErrNotFound)
	}
	return nil
}

// Answer closes an open ask and opens the oldest queued ask of the same run.
func (a *Ask) Answer(ctx context.Context, id domain.AskID, answer domain.AskAnswer, now time.Time) error {
	return a.close(ctx, id, `UPDATE ask SET status = 'answered', answer = ?, answered_by = ?, closed_at = ?
WHERE id = ? AND status = 'open'`, answer.Text, answer.By, formatTime(now), id)
}

// Cancel closes an open ask without an answer and opens the oldest queued ask of the same run.
func (a *Ask) Cancel(ctx context.Context, id domain.AskID, now time.Time) error {
	return a.close(ctx, id, `UPDATE ask SET status = 'cancelled', closed_at = ? WHERE id = ? AND status = 'open'`, formatTime(now), id)
}

func (a *Ask) close(ctx context.Context, id domain.AskID, query string, args ...any) (err error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot start close of ask %d: %w", id, err)
	}
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				err = fmt.Errorf("%w (rollback also failed)", err)
			}
		}
	}()

	var key domain.IssueKey
	if scanErr := tx.QueryRowContext(ctx, `SELECT issue_key FROM ask WHERE id = ?`, id).Scan(&key); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return fmt.Errorf("ask %d: %w", id, domain.ErrNotFound)
		}
		return fmt.Errorf("cannot find ask %d: %w", id, scanErr)
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("cannot close ask %d: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot count rows of ask %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("ask %d: %w", id, domain.ErrNotOpen)
	}
	_, err = tx.ExecContext(ctx, `
UPDATE ask SET status = 'open'
WHERE id = (SELECT MIN(id) FROM ask WHERE issue_key = ? AND status = 'queued')`, key)
	if err != nil {
		return fmt.Errorf("cannot open next ask of run %s: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit close of ask %d: %w", id, err)
	}
	return nil
}

func scanAsk(row rowScanner) (domain.Ask, error) {
	var ask domain.Ask
	var options, createdAt string
	var closedAt sql.NullString
	err := row.Scan(&ask.ID, &ask.IssueKey, &ask.Question, &options, &ask.Status, &ask.Answer, &ask.AnsweredBy, &ask.DialogID, &createdAt, &closedAt)
	if err != nil {
		return domain.Ask{}, err
	}
	if err := json.Unmarshal([]byte(options), &ask.Options); err != nil {
		return domain.Ask{}, fmt.Errorf("cannot decode options of ask %d: %w", ask.ID, err)
	}
	if ask.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Ask{}, err
	}
	if ask.ClosedAt, err = parseOptionalTime(closedAt); err != nil {
		return domain.Ask{}, err
	}
	return ask, nil
}
