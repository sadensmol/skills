-- +goose Up
CREATE TABLE run_repo (
    issue_key TEXT NOT NULL REFERENCES run (issue_key),
    repo TEXT NOT NULL,
    red_base_sha TEXT NOT NULL DEFAULT '',
    red_tests_sha TEXT NOT NULL DEFAULT '',
    red_test_ids TEXT NOT NULL DEFAULT '[]',
    red_qa_files TEXT NOT NULL DEFAULT '[]',
    red_criteria_tests TEXT NOT NULL DEFAULT '[]',
    red_reproofs TEXT NOT NULL DEFAULT '[]',
    pr_url TEXT NOT NULL DEFAULT '',
    pr_state TEXT NOT NULL DEFAULT '',
    pr_last_comment_id TEXT NOT NULL DEFAULT '',
    pr_checked_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (issue_key, repo)
);

-- +goose Down
DROP TABLE run_repo;
