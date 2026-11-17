-- +goose Up
CREATE TABLE verdict (
    id INTEGER PRIMARY KEY,
    issue_key TEXT NOT NULL REFERENCES run (issue_key),
    repo TEXT NOT NULL,
    sha TEXT NOT NULL,
    checks TEXT NOT NULL DEFAULT '[]',
    manual TEXT NOT NULL DEFAULT '[]',
    red_reproofs TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL
);

CREATE INDEX verdict_issue_key_repo_idx ON verdict (issue_key, repo);

-- +goose Down
DROP TABLE verdict;
