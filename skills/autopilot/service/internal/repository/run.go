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

const runColumns = `issue_key, product, channel_id, request_ts, thread_ts, card_ts, session_id, worktree_base,
phase, previous_phase, last_post_at, ended_at, created_at`

type Run struct {
	db *sql.DB
}

func NewRun(db *sql.DB) *Run {
	return &Run{db: db}
}

// Create refuses a repeated issue key or a repeated (channel, request ts) with ErrAlreadyExists.
func (r *Run) Create(ctx context.Context, run domain.Run, now time.Time) (domain.Run, error) {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO run (issue_key, product, channel_id, request_ts, thread_ts, card_ts, session_id, worktree_base,
    phase, previous_phase, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.IssueKey, run.Product, run.ChannelID, run.RequestTS, run.ThreadTS, run.CardTS, run.SessionID, run.WorktreeBase,
		run.Phase, run.PreviousPhase, formatTime(now), formatTime(now))
	if isUniqueViolation(err) {
		return domain.Run{}, fmt.Errorf("run %s in channel %s at %s: %w", run.IssueKey, run.ChannelID, run.RequestTS, domain.ErrAlreadyExists)
	}
	if err != nil {
		return domain.Run{}, fmt.Errorf("cannot create run %s: %w", run.IssueKey, err)
	}
	return r.FindByKey(ctx, run.IssueKey)
}

func (r *Run) FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx, `SELECT `+runColumns+` FROM run WHERE issue_key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Run{}, fmt.Errorf("run %s: %w", key, domain.ErrNotFound)
	}
	if err != nil {
		return domain.Run{}, fmt.Errorf("cannot find run %s: %w", key, err)
	}
	return run, nil
}

func (r *Run) FindByRequest(ctx context.Context, channelID, requestTS string) (domain.Run, error) {
	run, err := scanRun(r.db.QueryRowContext(ctx,
		`SELECT `+runColumns+` FROM run WHERE channel_id = ? AND request_ts = ?`, channelID, requestTS))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Run{}, fmt.Errorf("run for request %s at %s: %w", channelID, requestTS, domain.ErrNotFound)
	}
	if err != nil {
		return domain.Run{}, fmt.Errorf("cannot find run for request %s at %s: %w", channelID, requestTS, err)
	}
	return run, nil
}

func (r *Run) ListActive(ctx context.Context) ([]domain.Run, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+runColumns+` FROM run WHERE ended_at IS NULL ORDER BY created_at, issue_key`)
	if err != nil {
		return nil, fmt.Errorf("cannot list active runs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // a read cursor; Err() below reports the failure
	var runs []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("cannot read active run: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list active runs: %w", err)
	}
	return runs, nil
}

// SetPhase moves the current phase to previous_phase and stores the new one.
func (r *Run) SetPhase(ctx context.Context, key domain.IssueKey, phase domain.Phase, now time.Time) error {
	return r.exec(ctx, key, `UPDATE run SET previous_phase = phase, phase = ?, updated_at = ? WHERE issue_key = ?`,
		phase, formatTime(now), key)
}

func (r *Run) SetCardTS(ctx context.Context, key domain.IssueKey, cardTS string, now time.Time) error {
	return r.exec(ctx, key, `UPDATE run SET card_ts = ?, updated_at = ? WHERE issue_key = ?`, cardTS, formatTime(now), key)
}

func (r *Run) SetSession(ctx context.Context, key domain.IssueKey, sessionID, worktreeBase string, now time.Time) error {
	return r.exec(ctx, key, `UPDATE run SET session_id = ?, worktree_base = ?, updated_at = ? WHERE issue_key = ?`,
		sessionID, worktreeBase, formatTime(now), key)
}

func (r *Run) SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error {
	return r.exec(ctx, key, `UPDATE run SET last_post_at = ?, updated_at = ? WHERE issue_key = ?`, formatTime(at), formatTime(at), key)
}

func (r *Run) End(ctx context.Context, key domain.IssueKey, now time.Time) error {
	return r.exec(ctx, key, `UPDATE run SET ended_at = ?, updated_at = ? WHERE issue_key = ?`, formatTime(now), formatTime(now), key)
}

// IncLoop adds one to the loop's count and returns the new count.
func (r *Run) IncLoop(ctx context.Context, key domain.IssueKey, loop domain.Loop, now time.Time) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
INSERT INTO run_loop (issue_key, loop, count, updated_at) VALUES (?, ?, 1, ?)
ON CONFLICT (issue_key, loop) DO UPDATE SET count = count + 1, updated_at = excluded.updated_at
RETURNING count`, key, loop, formatTime(now)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count %s loop of run %s: %w", loop, key, err)
	}
	return count, nil
}

// ResetLoops sets the count of the given loops back to zero.
func (r *Run) ResetLoops(ctx context.Context, key domain.IssueKey, loops ...domain.Loop) error {
	for _, loop := range loops {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM run_loop WHERE issue_key = ? AND loop = ?`, key, loop); err != nil {
			return fmt.Errorf("cannot reset %s loop of run %s: %w", loop, key, err)
		}
	}
	return nil
}

func (r *Run) Loops(ctx context.Context, key domain.IssueKey) (map[domain.Loop]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT loop, count FROM run_loop WHERE issue_key = ?`, key)
	if err != nil {
		return nil, fmt.Errorf("cannot list loops of run %s: %w", key, err)
	}
	defer rows.Close() //nolint:errcheck // a read cursor; Err() below reports the failure
	loops := map[domain.Loop]int{}
	for rows.Next() {
		var loop domain.Loop
		var count int
		if err := rows.Scan(&loop, &count); err != nil {
			return nil, fmt.Errorf("cannot read loop of run %s: %w", key, err)
		}
		loops[loop] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list loops of run %s: %w", key, err)
	}
	return loops, nil
}

// SetRedBase stores the red proof of one repo; a second red proof for the same repo replaces the first.
func (r *Run) SetRedBase(ctx context.Context, key domain.IssueKey, red domain.RedBase, now time.Time) error {
	lists := make([]string, 4)
	for i, list := range []any{red.TestIDs, red.QAFiles, red.CriteriaTests, red.ReProofs} {
		encoded, err := encodeList(list)
		if err != nil {
			return fmt.Errorf("cannot encode the red proof of %s in run %s: %w", red.Repo, key, err)
		}
		lists[i] = encoded
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO run_repo (issue_key, repo, red_base_sha, red_tests_sha, red_test_ids, red_qa_files, red_criteria_tests, red_reproofs,
    created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (issue_key, repo) DO UPDATE SET
    red_base_sha = excluded.red_base_sha, red_tests_sha = excluded.red_tests_sha, red_test_ids = excluded.red_test_ids,
    red_qa_files = excluded.red_qa_files, red_criteria_tests = excluded.red_criteria_tests, red_reproofs = excluded.red_reproofs,
    updated_at = excluded.updated_at`,
		key, red.Repo, red.BaseSHA, red.TestsSHA, lists[0], lists[1], lists[2], lists[3], formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("cannot store red base of %s in run %s: %w", red.Repo, key, err)
	}
	return nil
}

// SetPR stores the PR URL of one repo; a repo without a red proof, such as a config-only change, gets a row here.
func (r *Run) SetPR(ctx context.Context, key domain.IssueKey, repo, prURL string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO run_repo (issue_key, repo, pr_url, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (issue_key, repo) DO UPDATE SET pr_url = excluded.pr_url, updated_at = excluded.updated_at`,
		key, repo, prURL, formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("cannot store PR of %s in run %s: %w", repo, key, err)
	}
	return nil
}

// SetPRCard stores the posted card of a repo's PR.
func (r *Run) SetPRCard(ctx context.Context, key domain.IssueKey, repo string, card domain.PRCard, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE run_repo SET pr_card_ts = ?, pr_card_text = ?, updated_at = ? WHERE issue_key = ? AND repo = ?`,
		card.TS, card.Text, formatTime(now), key, repo)
	if err != nil {
		return fmt.Errorf("cannot store the PR card of %s in run %s: %w", repo, key, err)
	}
	return requireAffected(result, fmt.Sprintf("repo %s of run %s", repo, key))
}

// SetPRWatch stores the poll state of a repo's PR and stamps the row with now; a repo without a row gets one.
func (r *Run) SetPRWatch(ctx context.Context, key domain.IssueKey, repo string, w domain.PRWatch, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO run_repo (issue_key, repo, pr_state, pr_last_comment_id, pr_checked_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (issue_key, repo) DO UPDATE SET
    pr_state = excluded.pr_state, pr_last_comment_id = excluded.pr_last_comment_id,
    pr_checked_at = excluded.pr_checked_at, updated_at = excluded.updated_at`,
		key, repo, w.State, w.LastCommentID, formatOptionalTime(w.CheckedAt), formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("cannot store PR watch of %s in run %s: %w", repo, key, err)
	}
	return nil
}

// SetMerge stores the merge of a repo's PR; a repo without a row gets one.
func (r *Run) SetMerge(ctx context.Context, key domain.IssueKey, repo string, m domain.Merge, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO run_repo (issue_key, repo, branch, merge_sha, deploy_state, merged_at, deploy_retried_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (issue_key, repo) DO UPDATE SET
    branch = excluded.branch, merge_sha = excluded.merge_sha, deploy_state = excluded.deploy_state,
    merged_at = excluded.merged_at, deploy_retried_at = excluded.deploy_retried_at, updated_at = excluded.updated_at`,
		key, repo, m.Branch, m.SHA, m.DeployState, formatOptionalTime(m.At), formatOptionalTime(m.RetriedAt), formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("cannot store the merge of %s in run %s: %w", repo, key, err)
	}
	return nil
}

// SetDeployState stores where the deploy of a merged repo stands.
func (r *Run) SetDeployState(ctx context.Context, key domain.IssueKey, repo string, state domain.DeployState, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE run_repo SET deploy_state = ?, updated_at = ? WHERE issue_key = ? AND repo = ?`,
		state, formatTime(now), key, repo)
	if err != nil {
		return fmt.Errorf("cannot store the deploy state of %s in run %s: %w", repo, key, err)
	}
	return requireAffected(result, fmt.Sprintf("repo %s of run %s", repo, key))
}

// SetDeployRetried stores when the failed jobs of a merged repo's deploy were re-run.
func (r *Run) SetDeployRetried(ctx context.Context, key domain.IssueKey, repo string, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE run_repo SET deploy_retried_at = ?, updated_at = ? WHERE issue_key = ? AND repo = ?`,
		formatTime(now), formatTime(now), key, repo)
	if err != nil {
		return fmt.Errorf("cannot store the deploy retry of %s in run %s: %w", repo, key, err)
	}
	return requireAffected(result, fmt.Sprintf("repo %s of run %s", repo, key))
}

// PendingDeploys lists the merged repos whose deploy the deploy watch still waits for, with their runs.
func (r *Run) PendingDeploys(ctx context.Context) ([]domain.PendingDeploy, error) {
	keys, err := r.pendingDeployKeys(ctx)
	if err != nil {
		return nil, err
	}
	var pending []domain.PendingDeploy
	for _, key := range keys {
		run, err := r.FindByKey(ctx, key)
		if err != nil {
			return nil, err
		}
		repos, err := r.Repos(ctx, key)
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			if repo.Merge.DeployState.Watched() {
				pending = append(pending, domain.PendingDeploy{Run: run, Repo: repo})
			}
		}
	}
	return pending, nil
}

func (r *Run) pendingDeployKeys(ctx context.Context) ([]domain.IssueKey, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT issue_key FROM run_repo WHERE deploy_state IN (?, ?) ORDER BY issue_key`,
		domain.DeployPending, domain.DeployFaulted)
	if err != nil {
		return nil, fmt.Errorf("cannot list pending deploys: %w", err)
	}
	defer rows.Close() //nolint:errcheck // a read cursor; Err() below reports the failure
	var keys []domain.IssueKey
	for rows.Next() {
		var key domain.IssueKey
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("cannot read pending deploy: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list pending deploys: %w", err)
	}
	return keys, nil
}

func (r *Run) Repos(ctx context.Context, key domain.IssueKey) ([]domain.RunRepo, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT repo, red_base_sha, red_tests_sha, red_test_ids, red_qa_files, red_criteria_tests, red_reproofs, pr_url, pr_card_ts, pr_card_text, pr_state, pr_last_comment_id, pr_checked_at,
    branch, merge_sha, deploy_state, merged_at, deploy_retried_at
FROM run_repo WHERE issue_key = ? ORDER BY repo`, key)
	if err != nil {
		return nil, fmt.Errorf("cannot list repos of run %s: %w", key, err)
	}
	defer rows.Close() //nolint:errcheck // a read cursor; Err() below reports the failure
	var repos []domain.RunRepo
	for rows.Next() {
		var repo domain.RunRepo
		var checkedAt, mergedAt, retriedAt sql.NullString
		var ids, qaFiles, criteriaTests, reProofs string
		if err := rows.Scan(&repo.Repo, &repo.RedBaseSHA, &repo.RedTestsSHA, &ids, &qaFiles, &criteriaTests, &reProofs,
			&repo.PRURL, &repo.PRCard.TS, &repo.PRCard.Text, &repo.Watch.State, &repo.Watch.LastCommentID, &checkedAt,
			&repo.Merge.Branch, &repo.Merge.SHA, &repo.Merge.DeployState, &mergedAt, &retriedAt); err != nil {
			return nil, fmt.Errorf("cannot read repo of run %s: %w", key, err)
		}
		if repo.RedTestIDs, err = decodeStrings(ids); err != nil {
			return nil, fmt.Errorf("cannot read red test ids of run %s: %w", key, err)
		}
		if repo.RedQAFiles, err = decodeStrings(qaFiles); err != nil {
			return nil, fmt.Errorf("cannot read red QA files of run %s: %w", key, err)
		}
		if repo.RedReProofs, err = decodeStrings(reProofs); err != nil {
			return nil, fmt.Errorf("cannot read red re-proofs of run %s: %w", key, err)
		}
		if err := json.Unmarshal([]byte(criteriaTests), &repo.RedCriteriaTests); err != nil {
			return nil, fmt.Errorf("cannot read red criteria tests of run %s: %w", key, err)
		}
		if len(repo.RedCriteriaTests) == 0 {
			repo.RedCriteriaTests = nil
		}
		if repo.Watch.CheckedAt, err = parseOptionalTime(checkedAt); err != nil {
			return nil, fmt.Errorf("cannot read repo of run %s: %w", key, err)
		}
		if repo.Merge.At, err = parseOptionalTime(mergedAt); err != nil {
			return nil, fmt.Errorf("cannot read repo of run %s: %w", key, err)
		}
		if repo.Merge.RetriedAt, err = parseOptionalTime(retriedAt); err != nil {
			return nil, fmt.Errorf("cannot read repo of run %s: %w", key, err)
		}
		repos = append(repos, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cannot list repos of run %s: %w", key, err)
	}
	return repos, nil
}

func (r *Run) exec(ctx context.Context, key domain.IssueKey, query string, args ...any) error {
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("cannot update run %s: %w", key, err)
	}
	return requireAffected(result, fmt.Sprintf("run %s", key))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRun(row rowScanner) (domain.Run, error) {
	var run domain.Run
	var lastPostAt, endedAt sql.NullString
	var createdAt string
	err := row.Scan(&run.IssueKey, &run.Product, &run.ChannelID, &run.RequestTS, &run.ThreadTS, &run.CardTS, &run.SessionID,
		&run.WorktreeBase, &run.Phase, &run.PreviousPhase, &lastPostAt, &endedAt, &createdAt)
	if err != nil {
		return domain.Run{}, err
	}
	if run.LastPostAt, err = parseOptionalTime(lastPostAt); err != nil {
		return domain.Run{}, err
	}
	if run.EndedAt, err = parseOptionalTime(endedAt); err != nil {
		return domain.Run{}, err
	}
	if run.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Run{}, err
	}
	return run, nil
}
