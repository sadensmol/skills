-- +goose Up
CREATE TABLE ask (
    id INTEGER PRIMARY KEY,
    issue_key TEXT NOT NULL REFERENCES run (issue_key),
    question TEXT NOT NULL,
    options TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL,
    answer TEXT NOT NULL DEFAULT '',
    answered_by TEXT NOT NULL DEFAULT '',
    dialog_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    closed_at TEXT,
    CONSTRAINT ask_status_ck CHECK (status IN ('open', 'queued', 'answered', 'cancelled'))
);

-- at most one open ask per run; later asks wait as queued, oldest id first
CREATE UNIQUE INDEX ask_issue_key_open_uq ON ask (issue_key) WHERE status = 'open';

CREATE INDEX ask_issue_key_status_idx ON ask (issue_key, status);

-- +goose Down
DROP TABLE ask;
