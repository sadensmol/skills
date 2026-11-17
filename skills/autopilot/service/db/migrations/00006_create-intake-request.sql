-- +goose Up
-- One row per owner message that intake took, written before the issue exists.
-- The primary key is the dedupe: a redelivery finds the row and resumes or stops.
CREATE TABLE intake_request (
    channel_id TEXT NOT NULL,
    request_ts TEXT NOT NULL,
    state TEXT NOT NULL,
    issue_id TEXT NOT NULL DEFAULT '',
    issue_key TEXT NOT NULL DEFAULT '',
    issue_title TEXT NOT NULL DEFAULT '',
    issue_url TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (channel_id, request_ts)
);

-- +goose Down
DROP TABLE intake_request;
