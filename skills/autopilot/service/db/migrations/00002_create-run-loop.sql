-- +goose Up
CREATE TABLE run_loop (
    issue_key TEXT NOT NULL REFERENCES run (issue_key),
    loop TEXT NOT NULL,
    count INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (issue_key, loop)
);

-- +goose Down
DROP TABLE run_loop;
