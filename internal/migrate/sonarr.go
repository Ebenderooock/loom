package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ImportSonarr reads series, episodes, root folders, and quality profiles from
// a Sonarr SQLite database and inserts them into Loom.
func (imp *Importer) ImportSonarr(ctx context.Context, sonarrDBPath string) (*ImportResult, error) {
	start := time.Now()
	res := &ImportResult{Source: "sonarr"}

	src, err := openSourceDB(sonarrDBPath)
	if err != nil {
		return nil, fmt.Errorf("open sonarr db: %w", err)
	}
	defer src.Close()

	if err := src.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping sonarr db: %w", err)
	}

	tx, err := imp.loomDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	imp.importSonarrProfiles(ctx, src, tx, res)
	imp.importSonarrRootFolders(ctx, src, tx, res)
	imp.importSonarrSeries(ctx, src, tx, res)
	imp.importSonarrEpisodes(ctx, src, tx, res)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	res.Duration = time.Since(start)
	return res, nil
}

func (imp *Importer) importSonarrProfiles(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx, `SELECT Id, Name FROM QualityProfiles`)
	if err != nil {
		imp.logger.Warn("sonarr: could not read QualityProfiles", "err", err)
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
		lid := loomID("sonarr-qp", id)
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
			imp.logger.Info("sonarr: imported quality profile", "name", name, "id", lid)
		} else {
			res.Skipped++
		}
	}
}

func (imp *Importer) importSonarrRootFolders(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx, `SELECT Id, Path FROM RootFolders`)
	if err != nil {
		imp.logger.Warn("sonarr: could not read RootFolders", "err", err)
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
		lid := loomID("sonarr-lib", id)
		result, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO libraries (id, name, path, media_type, created_at, updated_at)
			 VALUES (?, ?, ?, 'tv', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			lid, path, path)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert library %q: %s", path, err))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			res.LibrariesAdded++
			imp.logger.Info("sonarr: imported root folder as library", "path", path, "id", lid)
		} else {
			res.Skipped++
		}
	}
}

func (imp *Importer) importSonarrSeries(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx,
		`SELECT Id, Title, COALESCE(Year, 0), COALESCE(TvdbId, 0), COALESCE(ImdbId, ''),
		        COALESCE(Overview, ''), COALESCE(Path, ''), Monitored,
		        COALESCE(QualityProfileId, 0), COALESCE(SeriesType, 'standard'),
		        COALESCE(SeasonFolder, 1)
		 FROM Series`)
	if err != nil {
		imp.logger.Warn("sonarr: could not read Series", "err", err)
		res.Errors = append(res.Errors, "read Series: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id               int64
			title            string
			year             int
			tvdbID           int64
			imdbID           string
			overview, path   string
			monitored        bool
			qualityProfileID int64
			seriesType       string
			seasonFolder     bool
		)
		if err := rows.Scan(&id, &title, &year, &tvdbID, &imdbID,
			&overview, &path, &monitored, &qualityProfileID, &seriesType, &seasonFolder); err != nil {
			res.Errors = append(res.Errors, "scan series: "+err.Error())
			continue
		}

		// Skip if tvdb_id already exists in Loom.
		if tvdbID > 0 {
			var exists int
			_ = tx.QueryRowContext(ctx,
				`SELECT 1 FROM series WHERE tvdb_id = ? LIMIT 1`, fmt.Sprint(tvdbID)).Scan(&exists)
			if exists == 1 {
				res.Skipped++
				continue
			}
		}

		lid := loomID("sonarr", id)
		monStatus := "monitored"
		if !monitored {
			monStatus = "unmonitored"
		}
		qpID := loomID("sonarr-qp", qualityProfileID)
		sfInt := 1
		if !seasonFolder {
			sfInt = 0
		}

		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO series
			 (id, title, year, tvdb_id, imdb_id, overview, series_type, monitoring_status,
			  quality_profile_id, season_folder,
			  created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
			         strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
			lid, title, year, fmt.Sprint(tvdbID), imdbID, overview,
			seriesType, monStatus, qpID, sfInt)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert series %q: %s", title, err))
			continue
		}
		res.SeriesAdded++
		imp.logger.Debug("sonarr: imported series", "title", title, "id", lid)
	}
}

func (imp *Importer) importSonarrEpisodes(ctx context.Context, src *sql.DB, tx *sql.Tx, res *ImportResult) {
	rows, err := src.QueryContext(ctx,
		`SELECT Id, SeriesId, COALESCE(SeasonNumber, 0), COALESCE(EpisodeNumber, 0),
		        COALESCE(Title, ''), COALESCE(Overview, ''),
		        COALESCE(AirDate, ''), Monitored
		 FROM Episodes`)
	if err != nil {
		imp.logger.Warn("sonarr: could not read Episodes", "err", err)
		res.Errors = append(res.Errors, "read Episodes: "+err.Error())
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id, seriesID           int64
			seasonNum, episodeNum  int
			title, overview        string
			airDate                string
			monitored              bool
		)
		if err := rows.Scan(&id, &seriesID, &seasonNum, &episodeNum,
			&title, &overview, &airDate, &monitored); err != nil {
			res.Errors = append(res.Errors, "scan episode: "+err.Error())
			continue
		}

		lid := loomID("sonarr-ep", id)
		sID := loomID("sonarr", seriesID)
		// The episodes table requires a season_id. Create a deterministic one.
		seasonID := fmt.Sprintf("sonarr-s%d-se%d", seriesID, seasonNum)

		// Ensure the season row exists (upsert).
		_, _ = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO seasons
			 (id, series_id, season_number, created_at, updated_at)
			 VALUES (?, ?, ?,
			         strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
			seasonID, sID, seasonNum)

		monInt := 1
		if !monitored {
			monInt = 0
		}

		result, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO episodes
			 (id, series_id, season_id, episode_number, title, overview, air_date, monitored,
			  created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?,
			         strftime('%Y-%m-%dT%H:%M:%SZ','now'), strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
			lid, sID, seasonID, episodeNum, title, overview, airDate, monInt)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("insert episode %q S%02dE%02d: %s", title, seasonNum, episodeNum, err))
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			res.EpisodesAdded++
		} else {
			res.Skipped++
		}
	}
}
