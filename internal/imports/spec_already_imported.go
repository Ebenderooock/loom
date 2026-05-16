package imports

import (
	"context"
	"database/sql"
)

// AlreadyImportedSpec rejects files whose source path has already been
// recorded as a successful import in the import_history table.
type AlreadyImportedSpec struct {
	db *sql.DB
}

// NewAlreadyImportedSpec creates a spec that checks the import_history
// table for previous imports of the same source path.
func NewAlreadyImportedSpec(db *sql.DB) *AlreadyImportedSpec {
	return &AlreadyImportedSpec{db: db}
}

func (s *AlreadyImportedSpec) Name() string { return "NotAlreadyImported" }

func (s *AlreadyImportedSpec) IsSatisfiedBy(ctx context.Context, c *ImportCandidate) *ImportRejection {
	if s.db == nil {
		return nil
	}

	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM import_history
		 WHERE source_path = ? AND status = ?`,
		c.SourcePath, string(StatusImported),
	).Scan(&count)
	if err != nil {
		// If we can't check, don't block the import.
		return nil
	}

	if count > 0 {
		return &ImportRejection{
			Reason:  RejectionAlreadyImported,
			Message: "File has already been imported from this source path",
		}
	}
	return nil
}
