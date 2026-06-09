-- +goose Up
-- Seed a default audio quality profile and metadata profile so newly added
-- artists have sane acquisition defaults out of the box.

INSERT INTO audio_quality_profiles (id, name, items, cutoff, upgrade_allowed) VALUES
    ('aqp_standard', 'Standard Lossless', '[{"definition_id":"aq_mp3_256","allowed":true},{"definition_id":"aq_mp3_v0","allowed":true},{"definition_id":"aq_mp3_320","allowed":true},{"definition_id":"aq_aac_256","allowed":true},{"definition_id":"aq_aac_320","allowed":true},{"definition_id":"aq_flac","allowed":true},{"definition_id":"aq_flac_24","allowed":true}]', 'aq_flac', 1);

INSERT INTO metadata_profiles (id, name, primary_types, secondary_types, release_statuses) VALUES
    ('mp_standard', 'Standard', '["Album","EP"]', '[]', '["official"]');

-- +goose Down
DELETE FROM metadata_profiles WHERE id = 'mp_standard';
DELETE FROM audio_quality_profiles WHERE id = 'aqp_standard';
