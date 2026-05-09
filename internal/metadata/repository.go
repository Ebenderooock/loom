package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Repository persists metadata records for movies, series, and episodes.
// It provides a simple CRUD interface; the Service layer handles caching
// and provider orchestration. Engine-specific adapters (SQLite, Postgres)
// implement this interface; the rest of the package never touches
// sqlc-generated types directly.
type Repository interface {
	// Movies
	GetMovie(ctx context.Context, id string) (*MovieMetadata, error)
	PutMovie(ctx context.Context, id string, movie *MovieMetadata) error
	GetMovieByExternalID(ctx context.Context, idType, idValue string) (*MovieMetadata, error)

	// Series
	GetSeries(ctx context.Context, id string) (*SeriesMetadata, error)
	PutSeries(ctx context.Context, id string, series *SeriesMetadata) error
	GetSeriesByExternalID(ctx context.Context, idType, idValue string) (*SeriesMetadata, error)

	// Episodes
	GetEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error)
	PutEpisode(ctx context.Context, id string, seriesID string, season, episode int, ep *EpisodeMetadata) error

	// Maintenance
	DeleteExpiredMovies(ctx context.Context) (int64, error)
	DeleteExpiredSeries(ctx context.Context) (int64, error)
	DeleteExpiredEpisodes(ctx context.Context) (int64, error)
}

// --- SQLite adapter -----------------------------------------------

type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository builds a Repository over a SQLite connection.
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

func (s *sqliteRepo) GetMovie(ctx context.Context, id string) (*MovieMetadata, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT cached_json FROM metadata_movies WHERE id = ?
	`, id)
	var jsonData string
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get movie %q: %w", id, err)
	}
	var m MovieMetadata
	if err := json.Unmarshal([]byte(jsonData), &m); err != nil {
		return nil, fmt.Errorf("metadata: decode movie %q: %w", id, err)
	}
	return &m, nil
}

func (s *sqliteRepo) PutMovie(ctx context.Context, id string, movie *MovieMetadata) error {
	jsonData, err := json.Marshal(movie)
	if err != nil {
		return fmt.Errorf("metadata: encode movie %q: %w", id, err)
	}

	// Use REPLACE to upsert
	_, err = s.db.ExecContext(ctx, `
		REPLACE INTO metadata_movies (
			id, tmdb_id, imdb_id, tvdb_id, title, year, overview,
			poster_path, release_date, theatrical_date, digital_date,
			runtime, rating, cached_json,
			cached_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now', '+7 days'))
	`, id, movie.TMDBID, movie.IMDBID, movie.TVDBID, movie.Title, movie.Year,
		movie.Overview, movie.PosterPath, movie.ReleaseDate,
		movie.TheatricalDate, movie.DigitalDate,
		movie.Runtime, movie.Rating, string(jsonData))
	if err != nil {
		return fmt.Errorf("metadata: put movie %q: %w", id, err)
	}
	return nil
}

func (s *sqliteRepo) GetMovieByExternalID(ctx context.Context, idType, idValue string) (*MovieMetadata, error) {
	col := fmt.Sprintf("%s_id", idType)
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT cached_json FROM metadata_movies WHERE %s = ?
	`, col), idValue)
	var jsonData string
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get movie by %s=%s: %w", idType, idValue, err)
	}
	var m MovieMetadata
	if err := json.Unmarshal([]byte(jsonData), &m); err != nil {
		return nil, fmt.Errorf("metadata: decode movie %s=%s: %w", idType, idValue, err)
	}
	return &m, nil
}

func (s *sqliteRepo) GetSeries(ctx context.Context, id string) (*SeriesMetadata, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT cached_json FROM metadata_series WHERE id = ?
	`, id)
	var jsonData string
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get series %q: %w", id, err)
	}
	var ser SeriesMetadata
	if err := json.Unmarshal([]byte(jsonData), &ser); err != nil {
		return nil, fmt.Errorf("metadata: decode series %q: %w", id, err)
	}
	return &ser, nil
}

func (s *sqliteRepo) PutSeries(ctx context.Context, id string, series *SeriesMetadata) error {
	jsonData, err := json.Marshal(series)
	if err != nil {
		return fmt.Errorf("metadata: encode series %q: %w", id, err)
	}

	_, err = s.db.ExecContext(ctx, `
		REPLACE INTO metadata_series (
			id, tmdb_id, imdb_id, tvdb_id, title, overview,
			poster_path, first_air_date, rating, seasons, cached_json,
			cached_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now', '+7 days'))
	`, id, series.TMDBID, series.IMDBID, series.TVDBID, series.Title,
		series.Overview, series.PosterPath, series.FirstAirDate,
		series.Rating, series.Seasons, string(jsonData))
	if err != nil {
		return fmt.Errorf("metadata: put series %q: %w", id, err)
	}
	return nil
}

func (s *sqliteRepo) GetSeriesByExternalID(ctx context.Context, idType, idValue string) (*SeriesMetadata, error) {
	col := fmt.Sprintf("%s_id", idType)
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT cached_json FROM metadata_series WHERE %s = ?
	`, col), idValue)
	var jsonData string
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get series by %s=%s: %w", idType, idValue, err)
	}
	var ser SeriesMetadata
	if err := json.Unmarshal([]byte(jsonData), &ser); err != nil {
		return nil, fmt.Errorf("metadata: decode series %s=%s: %w", idType, idValue, err)
	}
	return &ser, nil
}

func (s *sqliteRepo) GetEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT cached_json FROM metadata_episodes WHERE series_id = ? AND season = ? AND episode = ?
	`, seriesID, season, episode)
	var jsonData string
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get episode %s S%dE%d: %w", seriesID, season, episode, err)
	}
	var ep EpisodeMetadata
	if err := json.Unmarshal([]byte(jsonData), &ep); err != nil {
		return nil, fmt.Errorf("metadata: decode episode %s S%dE%d: %w", seriesID, season, episode, err)
	}
	return &ep, nil
}

func (s *sqliteRepo) PutEpisode(ctx context.Context, id string, seriesID string, season, episode int, ep *EpisodeMetadata) error {
	jsonData, err := json.Marshal(ep)
	if err != nil {
		return fmt.Errorf("metadata: encode episode %s S%dE%d: %w", seriesID, season, episode, err)
	}

	_, err = s.db.ExecContext(ctx, `
		REPLACE INTO metadata_episodes (
			id, series_id, season, episode, tvdb_id, tmdb_id, title,
			overview, air_date, runtime, rating, cached_json,
			cached_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now', '+7 days'))
	`, id, seriesID, season, episode, ep.TVDBID, ep.TMDBID, ep.Title,
		ep.Overview, ep.AirDate, ep.Runtime, ep.Rating, string(jsonData))
	if err != nil {
		return fmt.Errorf("metadata: put episode %s S%dE%d: %w", seriesID, season, episode, err)
	}
	return nil
}

func (s *sqliteRepo) DeleteExpiredMovies(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM metadata_movies WHERE expires_at < datetime('now')
	`)
	if err != nil {
		return 0, fmt.Errorf("metadata: delete expired movies: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metadata: rows affected: %w", err)
	}
	return n, nil
}

func (s *sqliteRepo) DeleteExpiredSeries(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM metadata_series WHERE expires_at < datetime('now')
	`)
	if err != nil {
		return 0, fmt.Errorf("metadata: delete expired series: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metadata: rows affected: %w", err)
	}
	return n, nil
}

func (s *sqliteRepo) DeleteExpiredEpisodes(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM metadata_episodes WHERE expires_at < datetime('now')
	`)
	if err != nil {
		return 0, fmt.Errorf("metadata: delete expired episodes: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metadata: rows affected: %w", err)
	}
	return n, nil
}

// --- Postgres adapter -----------------------------------------------

type pgRepo struct {
	db *sql.DB
}

// NewPostgresRepository builds a Repository over a Postgres connection.
func NewPostgresRepository(db *sql.DB) Repository {
	return &pgRepo{db: db}
}

func (p *pgRepo) GetMovie(ctx context.Context, id string) (*MovieMetadata, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT cached_json FROM metadata_movies WHERE id = $1
	`, id)
	var jsonData []byte
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get movie %q: %w", id, err)
	}
	var m MovieMetadata
	if err := json.Unmarshal(jsonData, &m); err != nil {
		return nil, fmt.Errorf("metadata: decode movie %q: %w", id, err)
	}
	return &m, nil
}

func (p *pgRepo) PutMovie(ctx context.Context, id string, movie *MovieMetadata) error {
	jsonData, err := json.Marshal(movie)
	if err != nil {
		return fmt.Errorf("metadata: encode movie %q: %w", id, err)
	}

	_, err = p.db.ExecContext(ctx, `
		INSERT INTO metadata_movies (
			id, tmdb_id, imdb_id, tvdb_id, title, year, overview,
			poster_path, release_date, theatrical_date, digital_date,
			runtime, rating, cached_json,
			cached_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW() + INTERVAL '7 days')
		ON CONFLICT (id) DO UPDATE SET
			tmdb_id = $2, imdb_id = $3, tvdb_id = $4, title = $5, year = $6,
			overview = $7, poster_path = $8, release_date = $9,
			theatrical_date = $10, digital_date = $11,
			runtime = $12, rating = $13, cached_json = $14,
			cached_at = NOW(), expires_at = NOW() + INTERVAL '7 days'
	`, id, movie.TMDBID, movie.IMDBID, movie.TVDBID, movie.Title, movie.Year,
		movie.Overview, movie.PosterPath, movie.ReleaseDate,
		movie.TheatricalDate, movie.DigitalDate,
		movie.Runtime, movie.Rating, jsonData)
	if err != nil {
		return fmt.Errorf("metadata: put movie %q: %w", id, err)
	}
	return nil
}

func (p *pgRepo) GetMovieByExternalID(ctx context.Context, idType, idValue string) (*MovieMetadata, error) {
	col := fmt.Sprintf("%s_id", idType)
	row := p.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT cached_json FROM metadata_movies WHERE %s = $1
	`, col), idValue)
	var jsonData []byte
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get movie by %s=%s: %w", idType, idValue, err)
	}
	var m MovieMetadata
	if err := json.Unmarshal(jsonData, &m); err != nil {
		return nil, fmt.Errorf("metadata: decode movie %s=%s: %w", idType, idValue, err)
	}
	return &m, nil
}

func (p *pgRepo) GetSeries(ctx context.Context, id string) (*SeriesMetadata, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT cached_json FROM metadata_series WHERE id = $1
	`, id)
	var jsonData []byte
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get series %q: %w", id, err)
	}
	var ser SeriesMetadata
	if err := json.Unmarshal(jsonData, &ser); err != nil {
		return nil, fmt.Errorf("metadata: decode series %q: %w", id, err)
	}
	return &ser, nil
}

func (p *pgRepo) PutSeries(ctx context.Context, id string, series *SeriesMetadata) error {
	jsonData, err := json.Marshal(series)
	if err != nil {
		return fmt.Errorf("metadata: encode series %q: %w", id, err)
	}

	_, err = p.db.ExecContext(ctx, `
		INSERT INTO metadata_series (
			id, tmdb_id, imdb_id, tvdb_id, title, overview,
			poster_path, first_air_date, rating, seasons, cached_json,
			cached_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW() + INTERVAL '7 days')
		ON CONFLICT (id) DO UPDATE SET
			tmdb_id = $2, imdb_id = $3, tvdb_id = $4, title = $5,
			overview = $6, poster_path = $7, first_air_date = $8,
			rating = $9, seasons = $10, cached_json = $11, cached_at = NOW(),
			expires_at = NOW() + INTERVAL '7 days'
	`, id, series.TMDBID, series.IMDBID, series.TVDBID, series.Title,
		series.Overview, series.PosterPath, series.FirstAirDate,
		series.Rating, series.Seasons, jsonData)
	if err != nil {
		return fmt.Errorf("metadata: put series %q: %w", id, err)
	}
	return nil
}

func (p *pgRepo) GetSeriesByExternalID(ctx context.Context, idType, idValue string) (*SeriesMetadata, error) {
	col := fmt.Sprintf("%s_id", idType)
	row := p.db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT cached_json FROM metadata_series WHERE %s = $1
	`, col), idValue)
	var jsonData []byte
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get series by %s=%s: %w", idType, idValue, err)
	}
	var ser SeriesMetadata
	if err := json.Unmarshal(jsonData, &ser); err != nil {
		return nil, fmt.Errorf("metadata: decode series %s=%s: %w", idType, idValue, err)
	}
	return &ser, nil
}

func (p *pgRepo) GetEpisode(ctx context.Context, seriesID string, season, episode int) (*EpisodeMetadata, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT cached_json FROM metadata_episodes WHERE series_id = $1 AND season = $2 AND episode = $3
	`, seriesID, season, episode)
	var jsonData []byte
	if err := row.Scan(&jsonData); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("metadata: get episode %s S%dE%d: %w", seriesID, season, episode, err)
	}
	var ep EpisodeMetadata
	if err := json.Unmarshal(jsonData, &ep); err != nil {
		return nil, fmt.Errorf("metadata: decode episode %s S%dE%d: %w", seriesID, season, episode, err)
	}
	return &ep, nil
}

func (p *pgRepo) PutEpisode(ctx context.Context, id string, seriesID string, season, episode int, ep *EpisodeMetadata) error {
	jsonData, err := json.Marshal(ep)
	if err != nil {
		return fmt.Errorf("metadata: encode episode %s S%dE%d: %w", seriesID, season, episode, err)
	}

	_, err = p.db.ExecContext(ctx, `
		INSERT INTO metadata_episodes (
			id, series_id, season, episode, tvdb_id, tmdb_id, title,
			overview, air_date, runtime, rating, cached_json,
			cached_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW() + INTERVAL '7 days')
		ON CONFLICT (id) DO UPDATE SET
			series_id = $2, season = $3, episode = $4, tvdb_id = $5, tmdb_id = $6,
			title = $7, overview = $8, air_date = $9, runtime = $10, rating = $11,
			cached_json = $12, cached_at = NOW(), expires_at = NOW() + INTERVAL '7 days'
	`, id, seriesID, season, episode, ep.TVDBID, ep.TMDBID, ep.Title,
		ep.Overview, ep.AirDate, ep.Runtime, ep.Rating, jsonData)
	if err != nil {
		return fmt.Errorf("metadata: put episode %s S%dE%d: %w", seriesID, season, episode, err)
	}
	return nil
}

func (p *pgRepo) DeleteExpiredMovies(ctx context.Context) (int64, error) {
	result, err := p.db.ExecContext(ctx, `
		DELETE FROM metadata_movies WHERE expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("metadata: delete expired movies: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metadata: rows affected: %w", err)
	}
	return n, nil
}

func (p *pgRepo) DeleteExpiredSeries(ctx context.Context) (int64, error) {
	result, err := p.db.ExecContext(ctx, `
		DELETE FROM metadata_series WHERE expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("metadata: delete expired series: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metadata: rows affected: %w", err)
	}
	return n, nil
}

func (p *pgRepo) DeleteExpiredEpisodes(ctx context.Context) (int64, error) {
	result, err := p.db.ExecContext(ctx, `
		DELETE FROM metadata_episodes WHERE expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("metadata: delete expired episodes: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("metadata: rows affected: %w", err)
	}
	return n, nil
}
