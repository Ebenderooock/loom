-- +goose Up

-- user_invites backs self-service onboarding: an admin generates a single-use
-- invite link (token) with a pre-assigned role, and the recipient redeems it to
-- create their own account. used_at/used_by record redemption for the audit
-- trail; a NULL used_at means the invite is still claimable (subject to expiry).
CREATE TABLE IF NOT EXISTS user_invites (
    id         TEXT PRIMARY KEY,
    token      TEXT NOT NULL UNIQUE,
    email      TEXT NOT NULL DEFAULT '',
    role       TEXT NOT NULL DEFAULT 'user',
    created_by INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT,
    used_by    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_user_invites_token ON user_invites (token);

-- +goose Down

DROP INDEX IF EXISTS idx_user_invites_token;
DROP TABLE IF EXISTS user_invites;
