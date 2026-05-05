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

	AddMovieFile(ctx context.Context, mf *MovieFile) error
	GetMovieFile(ctx context.Context, id string) (*MovieFile, error)
	UpdateMovieFile(ctx context.Context, mf *MovieFile) error
	DeleteMovieFile(ctx context.Context, id string) error
	ListMovieFilesByMovie(ctx context.Context, movieID string) ([]*MovieFile, error)
	GetMovieFileByPath(ctx context.Context, path string) (*MovieFile, error)

	// Quality definitions
	AddQualityDefinition(ctx context.Context, qd *QualityDefinition) error
	GetQualityDefinition(ctx context.Context, id string) (*QualityDefinition, error)
	UpdateQualityDefinition(ctx context.Context, qd *QualityDefinition) error
	DeleteQualityDefinition(ctx context.Context, id string) error
	ListQualityDefinitions(ctx context.Context) ([]*QualityDefinition, error)
	GetQualityDefinitionByName(ctx context.Context, name string) (*QualityDefinition, error)

	// Quality profiles
	AddQualityProfile(ctx context.Context, qp *QualityProfile) error
	GetQualityProfile(ctx context.Context, id string) (*QualityProfile, error)
	UpdateQualityProfile(ctx context.Context, qp *QualityProfile) error
	DeleteQualityProfile(ctx context.Context, id string) error
	ListQualityProfiles(ctx context.Context) ([]*QualityProfile, error)
	GetQualityProfileByName(ctx context.Context, name string) (*QualityProfile, error)

	// Custom formats
	AddCustomFormat(ctx context.Context, cf *CustomFormat) error
	GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error)
	UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error
	DeleteCustomFormat(ctx context.Context, id string) error
	ListCustomFormats(ctx context.Context) ([]*CustomFormat, error)
	GetCustomFormatByName(ctx context.Context, name string) (*CustomFormat, error)
}

// NewRepository creates a new repository for the given database.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

// sqlRepo implements Repository using database/sql
type sqlRepo struct {
	db *sql.DB
}

// movie column list used by all SELECT queries
const movieColumns = `id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, quality_profile_id, library_id, status, release_date, last_search_at, monitoring_status, created_at, updated_at, deleted_at`

// scanMovie scans a movie row from the database.
func scanMovie(scanner interface{ Scan(dest ...interface{}) error }) (*Movie, error) {
	movie := &Movie{}
	var genreBytes []byte
	err := scanner.Scan(&movie.ID, &movie.Title, &movie.Year, &movie.IMDBID, &movie.TMDBID, &movie.TVDBID, &movie.Overview, &genreBytes,
		&movie.Runtime, &movie.Rating, &movie.BackdropPath, &movie.PosterPath, &movie.MetadataProvider,
		&movie.QualityProfileID, &movie.LibraryID, &movie.Status, &movie.ReleaseDate,
		&movie.LastSearchAt, &movie.MonitoringStatus, &movie.CreatedAt, &movie.UpdatedAt, &movie.DeletedAt)
	if err != nil {
		return nil, err
	}
	if len(genreBytes) > 0 {
		_ = json.Unmarshal(genreBytes, &movie.Genres)
	}
	return movie, nil
}

// Movie operations

func (r *sqlRepo) AddMovie(ctx context.Context, movie *Movie) error {
	genreBytes, _ := json.Marshal(movie.Genres)
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO movies (id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, quality_profile_id, library_id, status, release_date, monitoring_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		movie.ID, movie.Title, movie.Year, movie.IMDBID, movie.TMDBID, movie.TVDBID, movie.Overview, string(genreBytes),
		movie.Runtime, movie.Rating, movie.BackdropPath, movie.PosterPath, movie.MetadataProvider,
		movie.QualityProfileID, movie.LibraryID, movie.Status, movie.ReleaseDate,
		movie.MonitoringStatus, movie.CreatedAt, movie.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) GetMovie(ctx context.Context, id string) (*Movie, error) {
	return scanMovie(r.db.QueryRowContext(ctx,
		`SELECT `+movieColumns+` FROM movies WHERE id = ? AND deleted_at IS NULL`, id))
}

func (r *sqlRepo) UpdateMovie(ctx context.Context, movie *Movie) error {
	genreBytes, _ := json.Marshal(movie.Genres)
	_, err := r.db.ExecContext(ctx,
		`UPDATE movies SET title = ?, year = ?, overview = ?, genres = ?, runtime = ?, rating = ?, backdrop_path = ?, poster_path = ?,
		 quality_profile_id = ?, library_id = ?, status = ?, release_date = ?, monitoring_status = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		movie.Title, movie.Year, movie.Overview, string(genreBytes), movie.Runtime, movie.Rating, movie.BackdropPath, movie.PosterPath,
		movie.QualityProfileID, movie.LibraryID, movie.Status, movie.ReleaseDate, movie.MonitoringStatus, movie.UpdatedAt, movie.ID,
	)
	return err
}

func (r *sqlRepo) DeleteMovie(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM movies WHERE id = ?`,
		id,
	)
	return err
}

func (r *sqlRepo) ListMovies(ctx context.Context, limit int, offset int) ([]*Movie, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+movieColumns+` FROM movies WHERE deleted_at IS NULL ORDER BY updated_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		movie, err := scanMovie(rows)
		if err != nil {
			return nil, err
		}
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

func (r *sqlRepo) SearchMovies(ctx context.Context, query string) ([]*Movie, error) {
	searchQuery := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+movieColumns+` FROM movies WHERE deleted_at IS NULL AND (title LIKE ? OR LOWER(title) LIKE LOWER(?)) ORDER BY year DESC, title ASC LIMIT 50`,
		searchQuery, searchQuery,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		movie, err := scanMovie(rows)
		if err != nil {
			return nil, err
		}
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

func (r *sqlRepo) GetMovieByTMDBID(ctx context.Context, tmdbID string) (*Movie, error) {
	return scanMovie(r.db.QueryRowContext(ctx,
		`SELECT `+movieColumns+` FROM movies WHERE tmdb_id = ? AND deleted_at IS NULL`, tmdbID))
}

func (r *sqlRepo) GetMovieByIMDBID(ctx context.Context, imdbID string) (*Movie, error) {
	return scanMovie(r.db.QueryRowContext(ctx,
		`SELECT `+movieColumns+` FROM movies WHERE imdb_id = ? AND deleted_at IS NULL`, imdbID))
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

// Quality Definition methods

// AddQualityDefinition adds a new quality definition to the database.
func (r *sqlRepo) AddQualityDefinition(ctx context.Context, qd *QualityDefinition) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO quality_definitions (id, name, title, source, resolution, modifier, min_file_size, max_file_size, preferred_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		qd.ID, qd.Name, qd.Title, qd.Source, qd.Resolution, qd.Modifier, qd.MinFileSize, qd.MaxFileSize, qd.PreferredAt, qd.CreatedAt, qd.UpdatedAt,
	)
	return err
}

// GetQualityDefinition retrieves a quality definition by ID.
func (r *sqlRepo) GetQualityDefinition(ctx context.Context, id string) (*QualityDefinition, error) {
	qd := &QualityDefinition{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, title, source, resolution, modifier, min_file_size, max_file_size, preferred_at, created_at, updated_at, deleted_at
		 FROM quality_definitions WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&qd.ID, &qd.Name, &qd.Title, &qd.Source, &qd.Resolution, &qd.Modifier, &qd.MinFileSize, &qd.MaxFileSize, &qd.PreferredAt, &qd.CreatedAt, &qd.UpdatedAt, &qd.DeletedAt)
	if err != nil {
		return nil, err
	}
	return qd, nil
}

// UpdateQualityDefinition updates an existing quality definition.
func (r *sqlRepo) UpdateQualityDefinition(ctx context.Context, qd *QualityDefinition) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE quality_definitions
		 SET name = ?, title = ?, source = ?, resolution = ?, modifier = ?, min_file_size = ?, max_file_size = ?, preferred_at = ?, updated_at = ?
		 WHERE id = ?`,
		qd.Name, qd.Title, qd.Source, qd.Resolution, qd.Modifier, qd.MinFileSize, qd.MaxFileSize, qd.PreferredAt, qd.UpdatedAt, qd.ID,
	)
	return err
}

// DeleteQualityDefinition soft-deletes a quality definition.
func (r *sqlRepo) DeleteQualityDefinition(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE quality_definitions SET deleted_at = ? WHERE id = ?`,
		time.Now(), id,
	)
	return err
}

// ListQualityDefinitions retrieves all non-deleted quality definitions.
func (r *sqlRepo) ListQualityDefinitions(ctx context.Context) ([]*QualityDefinition, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, title, source, resolution, modifier, min_file_size, max_file_size, preferred_at, created_at, updated_at, deleted_at
		 FROM quality_definitions WHERE deleted_at IS NULL ORDER BY title ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var defs []*QualityDefinition
	for rows.Next() {
		qd := &QualityDefinition{}
		err := rows.Scan(&qd.ID, &qd.Name, &qd.Title, &qd.Source, &qd.Resolution, &qd.Modifier, &qd.MinFileSize, &qd.MaxFileSize, &qd.PreferredAt, &qd.CreatedAt, &qd.UpdatedAt, &qd.DeletedAt)
		if err != nil {
			return nil, err
		}
		defs = append(defs, qd)
	}
	return defs, rows.Err()
}

// GetQualityDefinitionByName retrieves a quality definition by name.
func (r *sqlRepo) GetQualityDefinitionByName(ctx context.Context, name string) (*QualityDefinition, error) {
	qd := &QualityDefinition{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, title, source, resolution, modifier, min_file_size, max_file_size, preferred_at, created_at, updated_at, deleted_at
		 FROM quality_definitions WHERE name = ? AND deleted_at IS NULL`,
		name,
	).Scan(&qd.ID, &qd.Name, &qd.Title, &qd.Source, &qd.Resolution, &qd.Modifier, &qd.MinFileSize, &qd.MaxFileSize, &qd.PreferredAt, &qd.CreatedAt, &qd.UpdatedAt, &qd.DeletedAt)
	if err != nil {
		return nil, err
	}
	return qd, nil
}

// Quality Profile methods

// AddQualityProfile adds a new quality profile to the database.
func (r *sqlRepo) AddQualityProfile(ctx context.Context, qp *QualityProfile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	formatItems, _ := json.Marshal(qp.FormatItems)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO quality_profiles (id, name, upgrade_allowed, cutoff, language, format_items, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		qp.ID, qp.Name, qp.UpgradeAllowed, qp.Cutoff, qp.Language, string(formatItems), qp.CreatedAt, qp.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Insert quality profile items
	for _, item := range qp.Items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO quality_profile_items (profile_id, quality_definition_id, preferred, allowed)
			 VALUES (?, ?, ?, ?)`,
			qp.ID, item.ID, item.Preferred, item.Allowed,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetQualityProfile retrieves a quality profile by ID.
func (r *sqlRepo) GetQualityProfile(ctx context.Context, id string) (*QualityProfile, error) {
	qp := &QualityProfile{}
	var formatItemsStr string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, upgrade_allowed, cutoff, language, format_items, created_at, updated_at, deleted_at
		 FROM quality_profiles WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(&qp.ID, &qp.Name, &qp.UpgradeAllowed, &qp.Cutoff, &qp.Language, &formatItemsStr, &qp.CreatedAt, &qp.UpdatedAt, &qp.DeletedAt)
	if err != nil {
		return nil, err
	}

	if formatItemsStr != "" {
		_ = json.Unmarshal([]byte(formatItemsStr), &qp.FormatItems)
	}

	// Fetch quality profile items
	rows, err := r.db.QueryContext(ctx,
		`SELECT quality_definition_id, preferred, allowed FROM quality_profile_items WHERE profile_id = ?`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		item := QualityProfileItem{}
		err := rows.Scan(&item.ID, &item.Preferred, &item.Allowed)
		if err != nil {
			return nil, err
		}
		qp.Items = append(qp.Items, item)
	}

	return qp, rows.Err()
}

// UpdateQualityProfile updates an existing quality profile.
func (r *sqlRepo) UpdateQualityProfile(ctx context.Context, qp *QualityProfile) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	formatItems, _ := json.Marshal(qp.FormatItems)

	_, err = tx.ExecContext(ctx,
		`UPDATE quality_profiles
		 SET name = ?, upgrade_allowed = ?, cutoff = ?, language = ?, format_items = ?, updated_at = ?
		 WHERE id = ?`,
		qp.Name, qp.UpgradeAllowed, qp.Cutoff, qp.Language, string(formatItems), qp.UpdatedAt, qp.ID,
	)
	if err != nil {
		return err
	}

	// Delete old items and reinsert
	_, err = tx.ExecContext(ctx, `DELETE FROM quality_profile_items WHERE profile_id = ?`, qp.ID)
	if err != nil {
		return err
	}

	for _, item := range qp.Items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO quality_profile_items (profile_id, quality_definition_id, preferred, allowed)
			 VALUES (?, ?, ?, ?)`,
			qp.ID, item.ID, item.Preferred, item.Allowed,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteQualityProfile soft-deletes a quality profile.
func (r *sqlRepo) DeleteQualityProfile(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE quality_profiles SET deleted_at = ? WHERE id = ?`,
		time.Now(), id,
	)
	return err
}

// ListQualityProfiles retrieves all non-deleted quality profiles.
func (r *sqlRepo) ListQualityProfiles(ctx context.Context) ([]*QualityProfile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, upgrade_allowed, cutoff, language, format_items, created_at, updated_at, deleted_at
		 FROM quality_profiles WHERE deleted_at IS NULL ORDER BY name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []*QualityProfile
	for rows.Next() {
		qp := &QualityProfile{}
		var formatItemsStr string
		err := rows.Scan(&qp.ID, &qp.Name, &qp.UpgradeAllowed, &qp.Cutoff, &qp.Language, &formatItemsStr, &qp.CreatedAt, &qp.UpdatedAt, &qp.DeletedAt)
		if err != nil {
			return nil, err
		}

		if formatItemsStr != "" {
			_ = json.Unmarshal([]byte(formatItemsStr), &qp.FormatItems)
		}

		profiles = append(profiles, qp)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	// Fetch items for each profile (separate queries to avoid nested cursors
	// which deadlock with SetMaxOpenConns(1)).
	for _, qp := range profiles {
		itemRows, err := r.db.QueryContext(ctx,
			`SELECT quality_definition_id, preferred, allowed FROM quality_profile_items WHERE profile_id = ?`,
			qp.ID,
		)
		if err != nil {
			return nil, err
		}

		for itemRows.Next() {
			item := QualityProfileItem{}
			if err := itemRows.Scan(&item.ID, &item.Preferred, &item.Allowed); err != nil {
				itemRows.Close()
				return nil, err
			}
			qp.Items = append(qp.Items, item)
		}
		itemRows.Close()
	}

	return profiles, nil
}

// GetQualityProfileByName retrieves a quality profile by name.
func (r *sqlRepo) GetQualityProfileByName(ctx context.Context, name string) (*QualityProfile, error) {
	qp := &QualityProfile{}
	var formatItemsStr string

	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, upgrade_allowed, cutoff, language, format_items, created_at, updated_at, deleted_at
		 FROM quality_profiles WHERE name = ? AND deleted_at IS NULL`,
		name,
	).Scan(&qp.ID, &qp.Name, &qp.UpgradeAllowed, &qp.Cutoff, &qp.Language, &formatItemsStr, &qp.CreatedAt, &qp.UpdatedAt, &qp.DeletedAt)
	if err != nil {
		return nil, err
	}

	if formatItemsStr != "" {
		_ = json.Unmarshal([]byte(formatItemsStr), &qp.FormatItems)
	}

	// Fetch quality profile items
	rows, err := r.db.QueryContext(ctx,
		`SELECT quality_definition_id, preferred, allowed FROM quality_profile_items WHERE profile_id = ?`,
		qp.ID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		item := QualityProfileItem{}
		err := rows.Scan(&item.ID, &item.Preferred, &item.Allowed)
		if err != nil {
			return nil, err
		}
		qp.Items = append(qp.Items, item)
	}

	return qp, rows.Err()
}

// Custom Format Repository Methods

// AddCustomFormat adds a new custom format with its filters.
func (r *sqlRepo) AddCustomFormat(ctx context.Context, cf *CustomFormat) error {
if cf == nil || cf.ID == "" {
return sql.ErrNoRows
}

// Serialize tags to JSON
tagsJSON, err := json.Marshal(cf.Tags)
if err != nil {
return err
}

// Insert custom format
_, err = r.db.ExecContext(ctx,
`INSERT INTO custom_formats (id, name, description, tags, created_at, updated_at) 
 VALUES (?, ?, ?, ?, ?, ?)`,
cf.ID, cf.Name, cf.Description, string(tagsJSON), cf.CreatedAt, cf.UpdatedAt,
)
if err != nil {
return err
}

// Insert filters
for _, filter := range cf.Filters {
	if filter.ID == "" {
		filter.ID = cf.ID + "_" + filter.Field + "_" + string(filter.Condition)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO custom_format_filters (id, custom_format_id, field, condition, value, "order", created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		filter.ID, cf.ID, filter.Field, filter.Condition, filter.Value, filter.Order, filter.CreatedAt, filter.UpdatedAt,
	)
	if err != nil {
		return err
}
}

return nil
}

// GetCustomFormat retrieves a custom format by ID with its filters.
func (r *sqlRepo) GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error) {
cf := &CustomFormat{}
var tagsJSON string

err := r.db.QueryRowContext(ctx,
`SELECT id, name, description, tags, created_at, updated_at, deleted_at 
 FROM custom_formats WHERE id = ?`,
id,
).Scan(&cf.ID, &cf.Name, &cf.Description, &tagsJSON, &cf.CreatedAt, &cf.UpdatedAt, &cf.DeletedAt)

if err != nil {
return nil, err
}

// Deserialize tags
if tagsJSON != "" {
err = json.Unmarshal([]byte(tagsJSON), &cf.Tags)
if err != nil {
return nil, err
}
}

// Fetch filters
rows, err := r.db.QueryContext(ctx,
`SELECT id, custom_format_id, field, condition, value, "order", created_at, updated_at 
 FROM custom_format_filters WHERE custom_format_id = ? ORDER BY "order"`,
id,
)
if err != nil {
return nil, err
}
defer rows.Close()

for rows.Next() {
filter := CustomFormatFilter{}
err := rows.Scan(&filter.ID, &filter.CustomFormatID, &filter.Field, &filter.Condition, &filter.Value, &filter.Order, &filter.CreatedAt, &filter.UpdatedAt)
if err != nil {
return nil, err
}
cf.Filters = append(cf.Filters, filter)
}

return cf, rows.Err()
}

// UpdateCustomFormat updates an existing custom format and its filters.
func (r *sqlRepo) UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error {
if cf == nil || cf.ID == "" {
return sql.ErrNoRows
}

// Serialize tags to JSON
tagsJSON, err := json.Marshal(cf.Tags)
if err != nil {
return err
}

// Update custom format
_, err = r.db.ExecContext(ctx,
`UPDATE custom_formats SET name = ?, description = ?, tags = ?, updated_at = ? 
 WHERE id = ?`,
cf.Name, cf.Description, string(tagsJSON), cf.UpdatedAt, cf.ID,
)
if err != nil {
return err
}

// Delete existing filters
_, err = r.db.ExecContext(ctx, `DELETE FROM custom_format_filters WHERE custom_format_id = ?`, cf.ID)
if err != nil {
return err
}

// Insert new filters
for _, filter := range cf.Filters {
	if filter.ID == "" {
		filter.ID = cf.ID + "_" + filter.Field + "_" + string(filter.Condition)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO custom_format_filters (id, custom_format_id, field, condition, value, "order", created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		filter.ID, cf.ID, filter.Field, filter.Condition, filter.Value, filter.Order, time.Now(), time.Now(),
	)
	if err != nil {
		return err
}
}

return nil
}

// DeleteCustomFormat marks a custom format as deleted (soft delete).
func (r *sqlRepo) DeleteCustomFormat(ctx context.Context, id string) error {
_, err := r.db.ExecContext(ctx,
`UPDATE custom_formats SET deleted_at = ? WHERE id = ?`,
time.Now(), id,
)
return err
}

// ListCustomFormats retrieves all non-deleted custom formats.
func (r *sqlRepo) ListCustomFormats(ctx context.Context) ([]*CustomFormat, error) {
rows, err := r.db.QueryContext(ctx,
`SELECT id FROM custom_formats WHERE deleted_at IS NULL ORDER BY name`,
)
if err != nil {
return nil, err
}
defer rows.Close()

var ids []string
for rows.Next() {
var id string
if err := rows.Scan(&id); err != nil {
return nil, err
}
ids = append(ids, id)
}
if err := rows.Err(); err != nil {
return nil, err
}
rows.Close()

var formats []*CustomFormat
for _, id := range ids {
cf, err := r.GetCustomFormat(ctx, id)
if err != nil {
return nil, err
}
formats = append(formats, cf)
}

return formats, nil
}

// GetCustomFormatByName retrieves a custom format by name.
func (r *sqlRepo) GetCustomFormatByName(ctx context.Context, name string) (*CustomFormat, error) {
var id string
err := r.db.QueryRowContext(ctx,
`SELECT id FROM custom_formats WHERE name = ? AND deleted_at IS NULL`,
name,
).Scan(&id)

if err != nil {
if err == sql.ErrNoRows {
return nil, nil
}
return nil, err
}

return r.GetCustomFormat(ctx, id)
}
