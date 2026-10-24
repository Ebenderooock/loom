-- +goose Up
-- Libraries are currently SQLite-backed; reserve the same schema version.
CREATE TABLE library_health_reports (
    library_id TEXT PRIMARY KEY,
    report TEXT NOT NULL
);
CREATE TABLE library_health_dismissals (
    library_id TEXT NOT NULL,
    finding_id TEXT NOT NULL,
    PRIMARY KEY (library_id, finding_id)
);

-- +goose Down
DROP TABLE library_health_dismissals;
DROP TABLE library_health_reports;
