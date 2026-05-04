package movies

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Repository defines CRUD operations for movies, root folders, and movie files.
type Repository interface {
	AddMovie(ctx context.Context, movie *Movie) error
	GetMovie(ctx context.Context, id string) (*Movie, error)
	UpdateMovie(ctx context.Context, movie *Movie) error
	DeleteMovie(ctx context.Context, id string) error
	ListMovies(ctx context.Context, limit int, offset int) ([]*Movie, error)
	SearchMovies(ctx context.Context, query string) ([]*Movie, error)
	GetMovieByTMDBID(ctx context.Context, tmdbID string) (*Movie, error)
	GetMovieByIMDBID(ctx context.Context, imdbID string) (*Movie, error)

	AddRootFolder(ctx context.Context, rf *RootFolder) error
	GetRootFolder(ctx context.Context, id string) (*RootFolder, error)
	UpdateRootFolder(ctx context.Context, rf *RootFolder) error
	DeleteRootFolder(ctx context.Context, id string) error
	ListRootFolders(ctx context.Context) ([]*RootFolder, error)
	GetRootFolderByPath(ctx context.Context, path string) (*RootFolder, error)

	AddMovieFile(ctx context.Context, mf *MovieFile) error
	GetMovieFile(ctx context.Context, id string) (*MovieFile, error)
	UpdateMovieFile(ctx context.Context, mf *MovieFile) error
	DeleteMovieFile(ctx context.Context, id string) error
	ListMovieFilesByMovie(ctx context.Context, movieID string) ([]*MovieFile, error)
	GetMovieFileByPath(ctx context.Context, path string) (*MovieFile, error)
}

// NewRepository creates a new repository for the given database.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

// sqlRepo implements Repository using database/sql
type sqlRepo struct {
	db *sql.DB
}

// Movie operations

func (r *sqlRepo) AddMovie(ctx context.Context, movie *Movie) error {
	genreBytes, _ := json.Marshal(movie.Genres)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO movies (id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, monitoring_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		movie.ID, movie.Title, movie.Year, movie.IMDBID, movie.TMDBID, movie.TVDBID, movie.Overview, string(genreBytes),
		movie.Runtime, movie.Rating, movie.BackdropPath, movie.PosterPath, movie.MetadataProvider, movie.MonitoringStatus, movie.CreatedAt, movie.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) GetMovie(ctx context.Context, id string) (*Movie, error) {
	movie := &Movie{}
	var genreBytes []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
		 FROM movies WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&movie.ID, &movie.Title, &movie.Year, &movie.IMDBID, &movie.TMDBID, &movie.TVDBID, &movie.Overview, &genreBytes,
		&movie.Runtime, &movie.Rating, &movie.BackdropPath, &movie.PosterPath, &movie.MetadataProvider, &movie.LastSearchAt, &movie.MonitoringStatus, &movie.CreatedAt, &movie.UpdatedAt, &movie.DeletedAt)
	if err != nil {
		return nil, err
	}
	if len(genreBytes) > 0 {
		_ = json.Unmarshal(genreBytes, &movie.Genres)
	}
	return movie, nil
}

func (r *sqlRepo) UpdateMovie(ctx context.Context, movie *Movie) error {
	genreBytes, _ := json.Marshal(movie.Genres)
	_, err := r.db.ExecContext(ctx,
		`UPDATE movies SET title = ?, year = ?, overview = ?, genres = ?, runtime = ?, rating = ?, backdrop_path = ?, poster_path = ?, monitoring_status = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		movie.Title, movie.Year, movie.Overview, string(genreBytes), movie.Runtime, movie.Rating, movie.BackdropPath, movie.PosterPath, movie.MonitoringStatus, movie.UpdatedAt, movie.ID,
	)
	return err
}

func (r *sqlRepo) DeleteMovie(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE movies SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		time.Now(), time.Now(), id,
	)
	return err
}

func (r *sqlRepo) ListMovies(ctx context.Context, limit int, offset int) ([]*Movie, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
		 FROM movies WHERE deleted_at IS NULL ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		movie := &Movie{}
		var genreBytes []byte
		if err := rows.Scan(&movie.ID, &movie.Title, &movie.Year, &movie.IMDBID, &movie.TMDBID, &movie.TVDBID, &movie.Overview, &genreBytes,
			&movie.Runtime, &movie.Rating, &movie.BackdropPath, &movie.PosterPath, &movie.MetadataProvider, &movie.LastSearchAt, &movie.MonitoringStatus, &movie.CreatedAt, &movie.UpdatedAt, &movie.DeletedAt); err != nil {
			return nil, err
		}
		if len(genreBytes) > 0 {
			_ = json.Unmarshal(genreBytes, &movie.Genres)
		}
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

func (r *sqlRepo) SearchMovies(ctx context.Context, query string) ([]*Movie, error) {
	searchQuery := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
		 FROM movies WHERE deleted_at IS NULL AND (title LIKE ? OR LOWER(title) LIKE LOWER(?)) ORDER BY year DESC, title ASC LIMIT 50`,
		searchQuery, searchQuery,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		movie := &Movie{}
		var genreBytes []byte
		if err := rows.Scan(&movie.ID, &movie.Title, &movie.Year, &movie.IMDBID, &movie.TMDBID, &movie.TVDBID, &movie.Overview, &genreBytes,
			&movie.Runtime, &movie.Rating, &movie.BackdropPath, &movie.PosterPath, &movie.MetadataProvider, &movie.LastSearchAt, &movie.MonitoringStatus, &movie.CreatedAt, &movie.UpdatedAt, &movie.DeletedAt); err != nil {
			return nil, err
		}
		if len(genreBytes) > 0 {
			_ = json.Unmarshal(genreBytes, &movie.Genres)
		}
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

func (r *sqlRepo) GetMovieByTMDBID(ctx context.Context, tmdbID string) (*Movie, error) {
	movie := &Movie{}
	var genreBytes []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
		 FROM movies WHERE tmdb_id = ? AND deleted_at IS NULL`,
		tmdbID,
	).Scan(&movie.ID, &movie.Title, &movie.Year, &movie.IMDBID, &movie.TMDBID, &movie.TVDBID, &movie.Overview, &genreBytes,
		&movie.Runtime, &movie.Rating, &movie.BackdropPath, &movie.PosterPath, &movie.MetadataProvider, &movie.LastSearchAt, &movie.MonitoringStatus, &movie.CreatedAt, &movie.UpdatedAt, &movie.DeletedAt)
	if err != nil {
		return nil, err
	}
	if len(genreBytes) > 0 {
		_ = json.Unmarshal(genreBytes, &movie.Genres)
	}
	return movie, nil
}

func (r *sqlRepo) GetMovieByIMDBID(ctx context.Context, imdbID string) (*Movie, error) {
	movie := &Movie{}
	var genreBytes []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
		 FROM movies WHERE imdb_id = ? AND deleted_at IS NULL`,
		imdbID,
	).Scan(&movie.ID, &movie.Title, &movie.Year, &movie.IMDBID, &movie.TMDBID, &movie.TVDBID, &movie.Overview, &genreBytes,
		&movie.Runtime, &movie.Rating, &movie.BackdropPath, &movie.PosterPath, &movie.MetadataProvider, &movie.LastSearchAt, &movie.MonitoringStatus, &movie.CreatedAt, &movie.UpdatedAt, &movie.DeletedAt)
	if err != nil {
		return nil, err
	}
	if len(genreBytes) > 0 {
		_ = json.Unmarshal(genreBytes, &movie.Genres)
	}
	return movie, nil
}

// RootFolder operations

func (r *sqlRepo) AddRootFolder(ctx context.Context, rf *RootFolder) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO root_folders (id, path, free_space, unmapped_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		rf.ID, rf.Path, rf.FreeSpace, rf.UnmappedCount, rf.CreatedAt, rf.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) GetRootFolder(ctx context.Context, id string) (*RootFolder, error) {
	rf := &RootFolder{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, path, free_space, unmapped_count, created_at, updated_at, deleted_at FROM root_folders WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&rf.ID, &rf.Path, &rf.FreeSpace, &rf.UnmappedCount, &rf.CreatedAt, &rf.UpdatedAt, &rf.DeletedAt)
	return rf, err
}

func (r *sqlRepo) UpdateRootFolder(ctx context.Context, rf *RootFolder) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE root_folders SET path = ?, free_space = ?, unmapped_count = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		rf.Path, rf.FreeSpace, rf.UnmappedCount, rf.UpdatedAt, rf.ID,
	)
	return err
}

func (r *sqlRepo) DeleteRootFolder(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE root_folders SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		time.Now(), time.Now(), id,
	)
	return err
}

func (r *sqlRepo) ListRootFolders(ctx context.Context) ([]*RootFolder, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, path, free_space, unmapped_count, created_at, updated_at, deleted_at FROM root_folders WHERE deleted_at IS NULL ORDER BY path ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []*RootFolder
	for rows.Next() {
		rf := &RootFolder{}
		if err := rows.Scan(&rf.ID, &rf.Path, &rf.FreeSpace, &rf.UnmappedCount, &rf.CreatedAt, &rf.UpdatedAt, &rf.DeletedAt); err != nil {
			return nil, err
		}
		folders = append(folders, rf)
	}
	return folders, rows.Err()
}

func (r *sqlRepo) GetRootFolderByPath(ctx context.Context, path string) (*RootFolder, error) {
	rf := &RootFolder{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, path, free_space, unmapped_count, created_at, updated_at, deleted_at FROM root_folders WHERE path = ? AND deleted_at IS NULL`,
		path,
	).Scan(&rf.ID, &rf.Path, &rf.FreeSpace, &rf.UnmappedCount, &rf.CreatedAt, &rf.UpdatedAt, &rf.DeletedAt)
	return rf, err
}

// MovieFile operations

func (r *sqlRepo) AddMovieFile(ctx context.Context, mf *MovieFile) error {
	mediaInfoBytes, _ := json.Marshal(mf.MediaInfo)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO movie_files (id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mf.ID, mf.MovieID, mf.FilePath, mf.Size, mf.Quality, mf.Format, string(mediaInfoBytes), mf.FileDate, mf.DateAdded, mf.CreatedAt, mf.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) GetMovieFile(ctx context.Context, id string) (*MovieFile, error) {
	mf := &MovieFile{}
	var mediaInfoBytes []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at, deleted_at
		 FROM movie_files WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&mf.ID, &mf.MovieID, &mf.FilePath, &mf.Size, &mf.Quality, &mf.Format, &mediaInfoBytes, &mf.FileDate, &mf.DateAdded, &mf.CreatedAt, &mf.UpdatedAt, &mf.DeletedAt)
	if err != nil {
		return nil, err
	}
	if len(mediaInfoBytes) > 0 {
		_ = json.Unmarshal(mediaInfoBytes, &mf.MediaInfo)
	}
	return mf, nil
}

func (r *sqlRepo) UpdateMovieFile(ctx context.Context, mf *MovieFile) error {
	mediaInfoBytes, _ := json.Marshal(mf.MediaInfo)
	_, err := r.db.ExecContext(ctx,
		`UPDATE movie_files SET file_path = ?, size = ?, quality = ?, format = ?, media_info = ?, file_date = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		mf.FilePath, mf.Size, mf.Quality, mf.Format, string(mediaInfoBytes), mf.FileDate, mf.UpdatedAt, mf.ID,
	)
	return err
}

func (r *sqlRepo) DeleteMovieFile(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE movie_files SET deleted_at = ?, updated_at = ? WHERE id = ?`,
		time.Now(), time.Now(), id,
	)
	return err
}

func (r *sqlRepo) ListMovieFilesByMovie(ctx context.Context, movieID string) ([]*MovieFile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at, deleted_at
		 FROM movie_files WHERE movie_id = ? AND deleted_at IS NULL ORDER BY file_path ASC`,
		movieID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*MovieFile
	for rows.Next() {
		mf := &MovieFile{}
		var mediaInfoBytes []byte
		if err := rows.Scan(&mf.ID, &mf.MovieID, &mf.FilePath, &mf.Size, &mf.Quality, &mf.Format, &mediaInfoBytes, &mf.FileDate, &mf.DateAdded, &mf.CreatedAt, &mf.UpdatedAt, &mf.DeletedAt); err != nil {
			return nil, err
		}
		if len(mediaInfoBytes) > 0 {
			_ = json.Unmarshal(mediaInfoBytes, &mf.MediaInfo)
		}
		files = append(files, mf)
	}
	return files, rows.Err()
}

func (r *sqlRepo) GetMovieFileByPath(ctx context.Context, path string) (*MovieFile, error) {
	mf := &MovieFile{}
	var mediaInfoBytes []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at, deleted_at
		 FROM movie_files WHERE file_path = ? AND deleted_at IS NULL`,
		path,
	).Scan(&mf.ID, &mf.MovieID, &mf.FilePath, &mf.Size, &mf.Quality, &mf.Format, &mediaInfoBytes, &mf.FileDate, &mf.DateAdded, &mf.CreatedAt, &mf.UpdatedAt, &mf.DeletedAt)
	if err != nil {
		return nil, err
	}
	if len(mediaInfoBytes) > 0 {
		_ = json.Unmarshal(mediaInfoBytes, &mf.MediaInfo)
	}
	return mf, nil
}
