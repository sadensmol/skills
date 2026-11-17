-- +goose Up
CREATE TABLE run (
    issue_key TEXT PRIMARY KEY,
    product TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    request_ts TEXT NOT NULL,
    thread_ts TEXT NOT NULL,
    card_ts TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    worktree_base TEXT NOT NULL DEFAULT '',
    phase TEXT NOT NULL,
    previous_phase TEXT NOT NULL DEFAULT '',
    last_post_at TEXT,
    ended_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX run_channel_id_request_ts_uq ON run (channel_id, request_ts);

CREATE INDEX run_ended_at_idx ON run (ended_at);

-- +goose Down
DROP TABLE run;
