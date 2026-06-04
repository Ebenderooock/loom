-- +goose Up

-- media_requests records user-submitted requests for movies or series that an
-- admin approves or rejects. On approval the media is added (monitored) and a
-- search-and-grab is triggered. Status transitions:
--   pending   -> submitted, awaiting admin decision
--   approving -> admin accepted; fulfillment (add + grab) in progress (lock)
--   approved  -> media added to the library (grab runs asynchronously)
--   rejected  -> admin declined; reason captured
--   failed    -> fulfillment errored; reason captured (re-requestable)
--   available -> reserved for a future "downloaded & imported" transition
-- Requester identity (user_id, username) is denormalized so request history
-- survives user deletion.
CREATE TABLE IF NOT EXISTS media_requests (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL DEFAULT '',
    username     TEXT NOT NULL DEFAULT '',
    media_type   TEXT NOT NULL,
    tmdb_id      TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    year         INTEGER NOT NULL DEFAULT 0,
    poster_path  TEXT NOT NULL DEFAULT '',
    overview     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    reason       TEXT NOT NULL DEFAULT '',
    media_id     TEXT NOT NULL DEFAULT '',
    decided_by   TEXT NOT NULL DEFAULT '',
    decided_at   TIMESTAMP,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_media_requests_status ON media_requests (status);
CREATE INDEX IF NOT EXISTS idx_media_requests_user ON media_requests (user_id);

-- Prevent duplicate open or fulfilled requests for the same media. Only one
-- request per (media_type, tmdb_id) may exist in a non-terminal-decline state;
-- rejected/failed requests are re-requestable.
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_requests_open
    ON media_requests (media_type, tmdb_id)
    WHERE status IN ('pending', 'approving', 'approved', 'available');

-- +goose Down

DROP TABLE IF EXISTS media_requests;
