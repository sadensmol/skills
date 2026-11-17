package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/sadensmol/autopilot/internal/domain"
)

type IntakeRequest struct {
	db *sql.DB
}

func NewIntakeRequest(db *sql.DB) *IntakeRequest {
	return &IntakeRequest{db: db}
}

// Claim records the message when it is new and returns its row either way.
func (r *IntakeRequest) Claim(ctx context.Context, channelID, requestTS string, now time.Time) (domain.IntakeRequest, error) {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO intake_request (channel_id, request_ts, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (channel_id, request_ts) DO NOTHING`,
		channelID, requestTS, domain.IntakeClaimed, formatTime(now), formatTime(now))
	if err != nil {
		return domain.IntakeRequest{}, fmt.Errorf("cannot claim request %s in %s: %w", requestTS, channelID, err)
	}
	req := domain.IntakeRequest{ChannelID: channelID, RequestTS: requestTS}
	err = r.db.QueryRowContext(ctx, `
SELECT state, issue_id, issue_key, issue_title, issue_url FROM intake_request WHERE channel_id = ? AND request_ts = ?`,
		channelID, requestTS).Scan(&req.State, &req.Issue.ID, &req.Issue.Key, &req.Issue.Title, &req.Issue.URL)
	if err != nil {
		return domain.IntakeRequest{}, fmt.Errorf("cannot read request %s in %s: %w", requestTS, channelID, err)
	}
	return req, nil
}

// SetIssue records the created issue and moves the row to IntakeIssued.
func (r *IntakeRequest) SetIssue(ctx context.Context, channelID, requestTS string, issue domain.IntakeIssue, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE intake_request SET state = ?, issue_id = ?, issue_key = ?, issue_title = ?, issue_url = ?, updated_at = ?
WHERE channel_id = ? AND request_ts = ?`,
		domain.IntakeIssued, issue.ID, issue.Key, issue.Title, issue.URL, formatTime(now), channelID, requestTS)
	if err != nil {
		return fmt.Errorf("cannot record issue of request %s in %s: %w", requestTS, channelID, err)
	}
	return requireAffected(result, fmt.Sprintf("request %s in %s", requestTS, channelID))
}

func (r *IntakeRequest) SetState(ctx context.Context, channelID, requestTS string, state domain.IntakeState, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE intake_request SET state = ?, updated_at = ? WHERE channel_id = ? AND request_ts = ?`,
		state, formatTime(now), channelID, requestTS)
	if err != nil {
		return fmt.Errorf("cannot set state of request %s in %s: %w", requestTS, channelID, err)
	}
	return requireAffected(result, fmt.Sprintf("request %s in %s", requestTS, channelID))
}
