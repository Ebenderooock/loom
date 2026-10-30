-- +goose Up
CREATE TABLE library_health_reports (
    library_id TEXT PRIMARY KEY REFERENCES libraries(id) ON DELETE CASCADE,
    report TEXT NOT NULL
);
CREATE TABLE library_health_dismissals (
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    finding_id TEXT NOT NULL,
    PRIMARY KEY (library_id, finding_id)
);

-- +goose Down
DROP TABLE library_health_dismissals;
DROP TABLE library_health_reports;
