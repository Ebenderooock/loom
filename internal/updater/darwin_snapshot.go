//go:build darwin || windows

package updater

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"

	_ "modernc.org/sqlite"
)

// Runs only in the credential-dropped snapshot child. Separated for native
// unprivileged WAL/space-in-path tests, never exposed as a root path-taking API.
func vacuumToStream(ctx context.Context, source, dest string, out io.Writer) error {
	db, err := sql.Open("sqlite", "file:"+source+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return err
	}
	f, err := os.Open(dest) // #nosec G304 -- private child-generated snapshot path, never root SQLite input
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(out, io.LimitReader(f, (64<<30)+1))
	if n > 64<<30 {
		return errors.New("SQLite snapshot exceeds 64 GiB")
	}
	return err
}
