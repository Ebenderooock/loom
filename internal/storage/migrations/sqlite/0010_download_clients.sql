-- +goose Up
-- Phase 3a: download-clients core. `download_clients` carries one row
-- per configured download target (qBittorrent, Transmission, Deluge,
-- SABnzbd, NZBGet, plus the builtin/null stub). Connection details
-- (host/port/tls/username/password) are first-class columns because
-- every supported kind has them; truly kind-specific knobs land in
-- `config_json`. Credentials are stored as TEXT — the same shape
-- proxy credentials use today; encryption-at-rest is tracked in
-- ADR-0014 and will be introduced uniformly across both subsystems.
--
-- `download_client_health` is split out so health updates from the
-- 5-minute scheduler sweep don't churn the main row, mirroring
-- `indexer_health`.
CREATE TABLE download_clients (
    id                TEXT     PRIMARY KEY,
    name              TEXT     NOT NULL,
    kind              TEXT     NOT NULL,
    protocol          TEXT     NOT NULL,
    enabled           INTEGER  NOT NULL DEFAULT 1,
    priority          INTEGER  NOT NULL DEFAULT 25,
    host              TEXT     NOT NULL DEFAULT '',
    port              INTEGER  NOT NULL DEFAULT 0,
    tls               INTEGER  NOT NULL DEFAULT 0,
    username          TEXT     NOT NULL DEFAULT '',
    password          TEXT     NOT NULL DEFAULT '',
    config_json       TEXT     NOT NULL DEFAULT '{}',
    category_default  TEXT     NOT NULL DEFAULT '',
    save_path_default TEXT     NOT NULL DEFAULT '',
    remove_completed  INTEGER  NOT NULL DEFAULT 0,
    remove_failed     INTEGER  NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_download_clients_kind    ON download_clients(kind);
CREATE INDEX idx_download_clients_enabled ON download_clients(enabled);

CREATE TABLE download_client_health (
    client_id             TEXT     PRIMARY KEY REFERENCES download_clients(id) ON DELETE CASCADE,
    status                TEXT     NOT NULL DEFAULT 'unknown',
    last_checked_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_success_at       DATETIME,
    last_failure_at       DATETIME,
    last_error            TEXT     NOT NULL DEFAULT '',
    consecutive_failures  INTEGER  NOT NULL DEFAULT 0,
    last_free_space_bytes INTEGER,
    last_categories_json  TEXT     NOT NULL DEFAULT '[]'
);

-- +goose Down
DROP TABLE download_client_health;
DROP INDEX IF EXISTS idx_download_clients_enabled;
DROP INDEX IF EXISTS idx_download_clients_kind;
DROP TABLE download_clients;
