-- +goose Up
ALTER TABLE run_repo ADD COLUMN deploy_retried_at TEXT;

-- +goose Down
ALTER TABLE run_repo DROP COLUMN deploy_retried_at;
