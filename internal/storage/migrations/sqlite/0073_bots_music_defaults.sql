-- +goose Up
-- Chat-approval defaults for music (artist) requests.
ALTER TABLE bot_config ADD COLUMN default_music_quality_profile_id TEXT NOT NULL DEFAULT '';
ALTER TABLE bot_config ADD COLUMN default_music_library_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE bot_config DROP COLUMN default_music_library_id;
ALTER TABLE bot_config DROP COLUMN default_music_quality_profile_id;
