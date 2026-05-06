// Package migrate provides tooling to import data from Radarr, Sonarr, and
// Prowlarr SQLite databases into Loom's own database.
package migrate

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/mattn/go-sqlite3"
)

// Importer reads from external *arr databases and writes into Loom's DB.
type Importer struct {
	loomDB *sql.DB
	logger *slog.Logger
}

// NewImporter creates an Importer that writes into the supplied Loom database.
func NewImporter(loomDB *sql.DB, logger *slog.Logger) *Importer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Importer{loomDB: loomDB, logger: logger}
}

// openSourceDB opens a source *arr SQLite database in read-only mode.
func openSourceDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	return sql.Open("sqlite3", dsn)
}

// loomID converts an external integer ID into a Loom-style string ID.
func loomID(prefix string, id int64) string {
	return fmt.Sprintf("%s-%d", prefix, id)
}
