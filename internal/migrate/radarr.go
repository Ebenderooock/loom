package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ImportRadarr reads movies, root folders, and quality profiles from a Radarr
// SQLite database and inserts them into Loom.
func (imp *Importer) ImportRadarr(ctx context.Context, radarrDBPath string) (*ImportResult, error) {
	start := time.Now()
	res := &ImportResult{Source: "radarr"}

	src, err := openSourceDB(radarrDBPath)
	if err != nil {
		return nil, fmt.Errorf("open radarr db: %w", err)
	}
	defer src.Close()

	if err := src.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping radarr db: %w", err)
	}

	tx, err := imp.loomDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	imp.importRadarrProfiles(ctx, src, tx, res)
	imp.importRadarrRootFolders(ctx, src, tx, res)
	imp.importRadarrMovies(ctx, src, tx, res)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	res.Duration = time.Since(start)
	return res, nil
}

func (imp *Importer) importRadarrProfiles(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx, `SELECT Id, Name FROM QualityProfiles`)
	if err != nil {
		imp.logger.Warn("radarr: could not read QualityProfiles", "err", err)
		res.Errors = append(res.Errors, "read QualityProfiles: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			res.Errors = append(res.Errors, "scan profile: "+err.Error())
			continue
		}
		lid := loomID("radarr-qp", id)
		result, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO quality_profiles (id, name, cutoff, items, created_at, updated_at)
			 VALUES (?, ?, '', '[]', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			lid, name)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert profile %q: %s", name, err))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			res.ProfilesAdded++
			imp.logger.Info("radarr: imported quality profile", "name", name, "id", lid)
		} else {
			res.Skipped++
		}
	}
}

func (imp *Importer) importRadarrRootFolders(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx, `SELECT Id, Path FROM RootFolders`)
	if err != nil {
		imp.logger.Warn("radarr: could not read RootFolders", "err", err)
		res.Errors = append(res.Errors, "read RootFolders: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			res.Errors = append(res.Errors, "scan root folder: "+err.Error())
			continue
		}
		lid := loomID("radarr-lib", id)
		result, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO libraries (id, name, path, media_type, created_at, updated_at)
			 VALUES (?, ?, ?, 'movie', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			lid, path, path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert library %q: %s", path, err))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			res.LibrariesAdded++
			imp.logger.Info("radarr: imported root folder as library", "path", path, "id", lid)
		} else {
			res.Skipped++
		}
	}
}

func (imp *Importer) importRadarrMovies(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx,
		`SELECT Id, Title, Year, COALESCE(TmdbId, ''), COALESCE(ImdbId, ''),
		        COALESCE(Overview, ''), COALESCE(Path, ''), Monitored,
		        COALESCE(QualityProfileId, 0), COALESCE(MovieStatus, ''),
		        COALESCE(Images, '[]')
		 FROM Movies`)
	if err != nil {
		imp.logger.Warn("radarr: could not read Movies", "err", err)
		res.Errors = append(res.Errors, "read Movies: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id               int64
			title            string
			year             int
			tmdbID, imdbID   string
			overview, path   string
			monitored        bool
			qualityProfileID int64
			movieStatus      string
			images           string
		)
		if err := rows.Scan(&id, &title, &year, &tmdbID, &imdbID,
			&overview, &path, &monitored, &qualityProfileID, &movieStatus, &images); err != nil {
			res.Errors = append(res.Errors, "scan movie: "+err.Error())
			continue
		}

		// Skip if tmdb_id already exists in Loom.
		if tmdbID != "" && tmdbID != "0" {
			var exists int
			_ = tx.QueryRowContext(ctx,
				`SELECT 1 FROM movies WHERE tmdb_id = ? LIMIT 1`, fmt.Sprint(tmdbID)).Scan(&exists)
			if exists == 1 {
				res.Skipped++
				continue
			}
		}

		lid := loomID("radarr", id)
		monStatus := "monitored"
		if !monitored {
			monStatus = "unmonitored"
		}

		qpID := loomID("radarr-qp", qualityProfileID)

		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO movies
			 (id, title, year, tmdb_id, imdb_id, overview, status, monitoring_status,
			  quality_profile_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			lid, title, year, fmt.Sprint(tmdbID), imdbID, overview,
			movieStatus, monStatus, qpID)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert movie %q: %s", title, err))
			continue
		}
		res.MoviesAdded++
		imp.logger.Debug("radarr: imported movie", "title", title, "id", lid)
	}
}
