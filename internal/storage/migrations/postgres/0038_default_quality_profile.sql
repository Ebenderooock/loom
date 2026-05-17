-- +goose Up
ALTER TABLE media_preferences ADD COLUMN default_quality_profile_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE media_preferences DROP COLUMN default_quality_profile_id;
