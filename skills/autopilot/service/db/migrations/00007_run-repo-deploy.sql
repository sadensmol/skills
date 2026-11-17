-- +goose Up
ALTER TABLE run_repo ADD COLUMN branch TEXT NOT NULL DEFAULT '';
ALTER TABLE run_repo ADD COLUMN merge_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE run_repo ADD COLUMN deploy_state TEXT NOT NULL DEFAULT '';
ALTER TABLE run_repo ADD COLUMN merged_at TEXT;

-- +goose Down
ALTER TABLE run_repo DROP COLUMN merged_at;
ALTER TABLE run_repo DROP COLUMN deploy_state;
ALTER TABLE run_repo DROP COLUMN merge_sha;
ALTER TABLE run_repo DROP COLUMN branch;
