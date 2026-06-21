package series

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Repository defines data access operations for series, seasons, episodes, and credits.
type Repository interface {
	ListSeries(ctx context.Context) ([]*Series, error)
	SearchSeries(ctx context.Context, query string) ([]*Series, error)
	GetSeries(ctx context.Context, id string) (*Series, error)
	CreateSeries(ctx context.Context, s *Series) error
	UpdateSeries(ctx context.Context, s *Series) error
	DeleteSeries(ctx context.Context, id string) error

	ListSeasons(ctx context.Context, seriesID string) ([]*Season, error)
	GetSeason(ctx context.Context, id string) (*Season, error)
	CreateSeason(ctx context.Context, s *Season) error
	UpdateSeason(ctx context.Context, s *Season) error

	ListEpisodes(ctx context.Context, seriesID string, seasonNum *int) ([]*Episode, error)
	GetEpisode(ctx context.Context, id string) (*Episode, error)
	CreateEpisode(ctx context.Context, e *Episode) error
	UpdateEpisode(ctx context.Context, e *Episode) error

	CreateEpisodeFile(ctx context.Context, f *EpisodeFile) error

	DeleteSeasonsBySeriesID(ctx context.Context, seriesID string) error
	DeleteEpisodesBySeriesID(ctx context.Context, seriesID string) error
	DeleteCreditsBySeriesID(ctx context.Context, seriesID string) error

	GetCredits(ctx context.Context, seriesID string) ([]*SeriesCredit, error)
	SaveCredits(ctx context.Context, seriesID string, credits []*SeriesCredit) error
	GetEpisodeStats(ctx context.Context, seriesID string) (*EpisodeStats, error)
	GetAllEpisodeStats(ctx context.Context) (map[string]*EpisodeStats, error)
	GetSeasonEpisodeStats(ctx context.Context, seriesID string) (map[string]*EpisodeStats, error)
}

// NewRepository creates a new Repository backed by the given database.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

type sqlRepo struct {
	db *sql.DB
}

const seriesColumns = `id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, network, status, series_type, metadata_provider, quality_profile_id, library_id, monitoring_status, season_folder, release_date, created_at, updated_at`

func scanSeries(scanner interface {
	Scan(dest ...interface{}) error
}) (*Series, error) {
	s := &Series{}
	var genreBytes []byte
	var createdStr, updatedStr string
	err := scanner.Scan(
		&s.ID, &s.Title, &s.Year, &s.IMDBID, &s.TMDBID, &s.TVDBID,
		&s.Overview, &genreBytes, &s.Runtime, &s.Rating,
		&s.BackdropPath, &s.PosterPath, &s.Network,
		&s.Status, &s.SeriesType, &s.MetadataProvider,
		&s.QualityProfileID, &s.LibraryID, &s.MonitoringStatus,
		&s.SeasonFolder, &s.ReleaseDate, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt = parseTime(createdStr)
	s.UpdatedAt = parseTime(updatedStr)
	if len(genreBytes) > 0 {
		_ = json.Unmarshal(genreBytes, &s.Genres)
	}
	return s, nil
}

func (r *sqlRepo) ListSeries(ctx context.Context) ([]*Series, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+seriesColumns+` FROM series ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Series
	for rows.Next() {
		s, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *sqlRepo) SearchSeries(ctx context.Context, query string) ([]*Series, error) {
	like := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+seriesColumns+` FROM series WHERE title LIKE ? ORDER BY updated_at DESC`, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Series
	for rows.Next() {
		s, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetSeries(ctx context.Context, id string) (*Series, error) {
	return scanSeries(r.db.QueryRowContext(ctx,
		`SELECT `+seriesColumns+` FROM series WHERE id = ?`, id))
}

func (r *sqlRepo) CreateSeries(ctx context.Context, s *Series) error {
	genreBytes, _ := json.Marshal(s.Genres)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO series (id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, network, status, series_type, metadata_provider, quality_profile_id, library_id, monitoring_status, season_folder, release_date, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.Year, s.IMDBID, s.TMDBID, s.TVDBID,
		s.Overview, string(genreBytes), s.Runtime, s.Rating,
		s.BackdropPath, s.PosterPath, s.Network,
		string(s.Status), string(s.SeriesType), s.MetadataProvider,
		s.QualityProfileID, s.LibraryID, string(s.MonitoringStatus),
		s.SeasonFolder, s.ReleaseDate, s.CreatedAt.Format(time.RFC3339), s.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (r *sqlRepo) UpdateSeries(ctx context.Context, s *Series) error {
	genreBytes, _ := json.Marshal(s.Genres)
	_, err := r.db.ExecContext(ctx,
		`UPDATE series SET title = ?, year = ?, imdb_id = ?, tvdb_id = ?, overview = ?, genres = ?, runtime = ?, rating = ?, backdrop_path = ?, poster_path = ?, network = ?, status = ?, series_type = ?, metadata_provider = ?, quality_profile_id = ?, library_id = ?, monitoring_status = ?, season_folder = ?, release_date = ?, updated_at = ?
		 WHERE id = ?`,
		s.Title, s.Year, s.IMDBID, s.TVDBID, s.Overview, string(genreBytes), s.Runtime, s.Rating,
		s.BackdropPath, s.PosterPath, s.Network,
		string(s.Status), string(s.SeriesType), s.MetadataProvider,
		s.QualityProfileID, s.LibraryID, string(s.MonitoringStatus),
		s.SeasonFolder, s.ReleaseDate, s.UpdatedAt.Format(time.RFC3339), s.ID,
	)
	return err
}

func (r *sqlRepo) DeleteSeries(ctx context.Context, id string) error {
	// Delete children first (belt-and-suspenders with CASCADE)
	r.db.ExecContext(ctx, `DELETE FROM episode_files WHERE episode_id IN (SELECT id FROM episodes WHERE series_id = ?)`, id)
	r.db.ExecContext(ctx, `DELETE FROM episodes WHERE series_id = ?`, id)
	r.db.ExecContext(ctx, `DELETE FROM seasons WHERE series_id = ?`, id)
	r.db.ExecContext(ctx, `DELETE FROM series_credits WHERE series_id = ?`, id)
	_, err := r.db.ExecContext(ctx, `DELETE FROM series WHERE id = ?`, id)
	return err
}

// Season operations

const seasonColumns = `id, series_id, season_number, title, overview, poster_path, monitored, episode_count, created_at, updated_at`

func scanSeason(scanner interface {
	Scan(dest ...interface{}) error
}) (*Season, error) {
	s := &Season{}
	var createdStr, updatedStr string
	err := scanner.Scan(
		&s.ID, &s.SeriesID, &s.SeasonNumber, &s.Title, &s.Overview,
		&s.PosterPath, &s.Monitored, &s.EpisodeCount, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	s.CreatedAt = parseTime(createdStr)
	s.UpdatedAt = parseTime(updatedStr)
	return s, nil
}

func (r *sqlRepo) ListSeasons(ctx context.Context, seriesID string) ([]*Season, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+seasonColumns+` FROM seasons WHERE series_id = ? ORDER BY season_number ASC`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Season
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetSeason(ctx context.Context, id string) (*Season, error) {
	return scanSeason(r.db.QueryRowContext(ctx,
		`SELECT `+seasonColumns+` FROM seasons WHERE id = ?`, id))
}

func (r *sqlRepo) CreateSeason(ctx context.Context, s *Season) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO seasons (id, series_id, season_number, title, overview, poster_path, monitored, episode_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.SeriesID, s.SeasonNumber, s.Title, s.Overview,
		s.PosterPath, s.Monitored, s.EpisodeCount, s.CreatedAt.Format(time.RFC3339), s.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (r *sqlRepo) UpdateSeason(ctx context.Context, s *Season) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE seasons SET title = ?, overview = ?, poster_path = ?, monitored = ?, episode_count = ?, updated_at = ?
		 WHERE id = ?`,
		s.Title, s.Overview, s.PosterPath, s.Monitored, s.EpisodeCount, s.UpdatedAt.Format(time.RFC3339), s.ID,
	)
	return err
}

// Episode operations

const episodeColumns = `id, series_id, season_id, episode_number, title, overview, air_date, runtime, still_path, monitored, has_file, created_at, updated_at`

func scanEpisode(scanner interface {
	Scan(dest ...interface{}) error
}) (*Episode, error) {
	e := &Episode{}
	var createdStr, updatedStr string
	err := scanner.Scan(
		&e.ID, &e.SeriesID, &e.SeasonID, &e.EpisodeNumber, &e.Title, &e.Overview,
		&e.AirDate, &e.Runtime, &e.StillPath, &e.Monitored, &e.HasFile, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	e.CreatedAt = parseTime(createdStr)
	e.UpdatedAt = parseTime(updatedStr)
	return e, nil
}

func (r *sqlRepo) ListEpisodes(ctx context.Context, seriesID string, seasonNum *int) ([]*Episode, error) {
	var rows *sql.Rows
	var err error

	if seasonNum != nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT e.id, e.series_id, e.season_id, e.episode_number, e.title, e.overview, e.air_date, e.runtime, e.still_path, e.monitored, e.has_file, e.created_at, e.updated_at FROM episodes e
			 JOIN seasons s ON e.season_id = s.id
			 WHERE e.series_id = ? AND s.season_number = ?
			 ORDER BY e.episode_number ASC`, seriesID, *seasonNum)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+episodeColumns+` FROM episodes WHERE series_id = ? ORDER BY episode_number ASC`, seriesID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Episode
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetEpisode(ctx context.Context, id string) (*Episode, error) {
	return scanEpisode(r.db.QueryRowContext(ctx,
		`SELECT `+episodeColumns+` FROM episodes WHERE id = ?`, id))
}

func (r *sqlRepo) CreateEpisode(ctx context.Context, e *Episode) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO episodes (id, series_id, season_id, episode_number, title, overview, air_date, runtime, still_path, monitored, has_file, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.SeriesID, e.SeasonID, e.EpisodeNumber, e.Title, e.Overview,
		e.AirDate, e.Runtime, e.StillPath, e.Monitored, e.HasFile, e.CreatedAt.Format(time.RFC3339), e.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func (r *sqlRepo) UpdateEpisode(ctx context.Context, e *Episode) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE episodes SET title = ?, overview = ?, air_date = ?, runtime = ?, still_path = ?, monitored = ?, has_file = ?, updated_at = ?
		 WHERE id = ?`,
		e.Title, e.Overview, e.AirDate, e.Runtime, e.StillPath, e.Monitored, e.HasFile, e.UpdatedAt.Format(time.RFC3339), e.ID,
	)
	return err
}

// EpisodeFile operations

func (r *sqlRepo) CreateEpisodeFile(ctx context.Context, f *EpisodeFile) error {
	mediaBytes, _ := json.Marshal(f.MediaInfo)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO episode_files (id, episode_id, series_id, file_path, file_size, quality, source, resolution, codec, media_info, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.EpisodeID, f.SeriesID, f.FilePath, f.FileSize,
		f.Quality, f.Source, f.Resolution, f.Codec, string(mediaBytes),
		f.CreatedAt.Format(time.RFC3339), f.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}

	// Update episode's has_file flag to true
	_, err = r.db.ExecContext(ctx,
		`UPDATE episodes SET has_file = true, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), f.EpisodeID,
	)
	if err != nil {
		return err
	}

	// Link this file to the episode in library_files (for UI unmapped tracking).
	// This allows the library view to correctly track which files are mapped to media.
	_, _ = r.db.ExecContext(ctx,
		`UPDATE library_files SET media_id = ? WHERE path = ?`,
		f.EpisodeID, f.FilePath,
	)
	return nil
}

// Bulk delete operations for refresh

func (r *sqlRepo) DeleteSeasonsBySeriesID(ctx context.Context, seriesID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM seasons WHERE series_id = ?`, seriesID)
	return err
}

func (r *sqlRepo) DeleteEpisodesBySeriesID(ctx context.Context, seriesID string) error {
	r.db.ExecContext(ctx, `DELETE FROM episode_files WHERE episode_id IN (SELECT id FROM episodes WHERE series_id = ?)`, seriesID)
	_, err := r.db.ExecContext(ctx, `DELETE FROM episodes WHERE series_id = ?`, seriesID)
	return err
}

func (r *sqlRepo) DeleteCreditsBySeriesID(ctx context.Context, seriesID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM series_credits WHERE series_id = ?`, seriesID)
	return err
}

// Credits operations

func (r *sqlRepo) GetCredits(ctx context.Context, seriesID string) ([]*SeriesCredit, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, series_id, person_name, character_name, role, profile_path, tmdb_person_id, display_order
		 FROM series_credits WHERE series_id = ? ORDER BY display_order ASC`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*SeriesCredit
	for rows.Next() {
		c := &SeriesCredit{}
		if err := rows.Scan(&c.ID, &c.SeriesID, &c.PersonName, &c.CharacterName, &c.Role, &c.ProfilePath, &c.TMDBPersonID, &c.DisplayOrder); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (r *sqlRepo) SaveCredits(ctx context.Context, seriesID string, credits []*SeriesCredit) error {
	// Delete existing credits first
	if _, err := r.db.ExecContext(ctx, `DELETE FROM series_credits WHERE series_id = ?`, seriesID); err != nil {
		return err
	}

	for _, c := range credits {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO series_credits (series_id, person_name, character_name, role, profile_path, tmdb_person_id, display_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			seriesID, c.PersonName, c.CharacterName, c.Role, c.ProfilePath, c.TMDBPersonID, c.DisplayOrder,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *sqlRepo) GetEpisodeStats(ctx context.Context, seriesID string) (*EpisodeStats, error) {
	stats := &EpisodeStats{}
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) as total,
			SUM(CASE WHEN has_file = 1 THEN 1 ELSE 0 END) as downloaded,
			SUM(CASE WHEN e.monitored = 1 THEN 1 ELSE 0 END) as monitored,
			SUM(CASE WHEN e.monitored = 1 AND has_file = 0 AND (air_date != '' AND air_date <= date('now'))
				AND s.season_number != 0 THEN 1 ELSE 0 END) as missing,
			SUM(CASE WHEN air_date != '' AND air_date <= date('now') THEN 1 ELSE 0 END) as aired
		 FROM episodes e
		 JOIN seasons s ON e.season_id = s.id
		 WHERE e.series_id = ?`, seriesID,
	).Scan(&stats.TotalEpisodes, &stats.DownloadedEpisodes, &stats.MonitoredEpisodes, &stats.MissingEpisodes, &stats.AiredEpisodes)
	return stats, err
}

func (r *sqlRepo) GetAllEpisodeStats(ctx context.Context) (map[string]*EpisodeStats, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT
			e.series_id,
			COUNT(*) as total,
			SUM(CASE WHEN has_file = 1 THEN 1 ELSE 0 END) as downloaded,
			SUM(CASE WHEN e.monitored = 1 THEN 1 ELSE 0 END) as monitored,
			SUM(CASE WHEN e.monitored = 1 AND has_file = 0 AND (air_date != '' AND air_date <= date('now'))
				AND s.season_number != 0 THEN 1 ELSE 0 END) as missing,
			SUM(CASE WHEN air_date != '' AND air_date <= date('now') THEN 1 ELSE 0 END) as aired
		 FROM episodes e
		 JOIN seasons s ON e.season_id = s.id
		 GROUP BY e.series_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*EpisodeStats)
	for rows.Next() {
		var seriesID string
		s := &EpisodeStats{}
		if err := rows.Scan(&seriesID, &s.TotalEpisodes, &s.DownloadedEpisodes, &s.MonitoredEpisodes, &s.MissingEpisodes, &s.AiredEpisodes); err != nil {
			return nil, err
		}
		result[seriesID] = s
	}
	return result, rows.Err()
}

func (r *sqlRepo) GetSeasonEpisodeStats(ctx context.Context, seriesID string) (map[string]*EpisodeStats, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT
			e.season_id,
			COUNT(*) as total,
			SUM(CASE WHEN has_file = 1 THEN 1 ELSE 0 END) as downloaded,
			SUM(CASE WHEN e.monitored = 1 THEN 1 ELSE 0 END) as monitored,
			SUM(CASE WHEN e.monitored = 1 AND has_file = 0 AND (air_date != '' AND air_date <= date('now'))
				AND s.season_number != 0 THEN 1 ELSE 0 END) as missing,
			SUM(CASE WHEN air_date != '' AND air_date <= date('now') THEN 1 ELSE 0 END) as aired
		 FROM episodes e
		 JOIN seasons s ON e.season_id = s.id
		 WHERE e.series_id = ? GROUP BY e.season_id`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*EpisodeStats)
	for rows.Next() {
		var seasonID string
		s := &EpisodeStats{}
		if err := rows.Scan(&seasonID, &s.TotalEpisodes, &s.DownloadedEpisodes, &s.MonitoredEpisodes, &s.MissingEpisodes, &s.AiredEpisodes); err != nil {
			return nil, err
		}
		result[seasonID] = s
	}
	return result, rows.Err()
}

func now() time.Time {
	return time.Now().UTC()
}

// parseTime parses a time string from SQLite in various formats.
func parseTime(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05+00:00",
		"2006-01-02 15:04:05.999999999+00:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
