-- +goose Up
-- Phase 2c: cache the most recent caps document for each indexer so a
-- restart doesn't blank-state every kind that does network discovery
-- (Newznab/Torznab `t=caps`).
ALTER TABLE indexer_health ADD COLUMN last_caps_json JSONB;

-- +goose Down
ALTER TABLE indexer_health DROP COLUMN last_caps_json;
