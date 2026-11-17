-- +goose Up
ALTER TABLE run_repo ADD COLUMN pr_card_ts TEXT NOT NULL DEFAULT '';
ALTER TABLE run_repo ADD COLUMN pr_card_text TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE run_repo DROP COLUMN pr_card_text;
ALTER TABLE run_repo DROP COLUMN pr_card_ts;
