-- +goose Up
-- Add custom-format scoring to audio quality profiles (Lidarr-style):
-- format_items maps custom-format IDs to scores; min_format_score rejects
-- releases whose aggregate format score falls below the threshold.
ALTER TABLE audio_quality_profiles ADD COLUMN format_items TEXT NOT NULL DEFAULT '[]';
ALTER TABLE audio_quality_profiles ADD COLUMN min_format_score INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE audio_quality_profiles DROP COLUMN min_format_score;
ALTER TABLE audio_quality_profiles DROP COLUMN format_items;
