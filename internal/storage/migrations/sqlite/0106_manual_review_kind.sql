-- +goose Up
ALTER TABLE manual_review ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE manual_review ADD COLUMN workflow_id TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_manual_review_subject ON manual_review(kind, media_type, media_id, download_path);

-- +goose Down
DROP INDEX IF EXISTS idx_manual_review_subject;
ALTER TABLE manual_review DROP COLUMN workflow_id;
ALTER TABLE manual_review DROP COLUMN kind;
