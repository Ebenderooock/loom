-- +goose Up
-- Preserve the release-group's full MusicBrainz artist credit independently
-- from the legacy album owner used for library/profile and file organization.
ALTER TABLE albums ADD COLUMN artist_credits JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE albums ALTER COLUMN artist_id DROP NOT NULL;
ALTER TABLE albums ADD COLUMN library_id TEXT;
ALTER TABLE albums ADD COLUMN quality_profile_id TEXT;
ALTER TABLE albums ADD COLUMN metadata_profile_id TEXT;

-- +goose Down
-- Refuse a downgrade that cannot represent existing standalone albums.
ALTER TABLE albums ALTER COLUMN artist_id SET NOT NULL;
ALTER TABLE albums DROP COLUMN metadata_profile_id;
ALTER TABLE albums DROP COLUMN quality_profile_id;
ALTER TABLE albums DROP COLUMN library_id;
ALTER TABLE albums DROP COLUMN artist_credits;
