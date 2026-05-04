-- name: CreateMovie :exec
INSERT INTO movies (id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, monitoring_status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetMovie :one
SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
FROM movies
WHERE id = ? AND deleted_at IS NULL;

-- name: UpdateMovie :exec
UPDATE movies
SET title = ?, year = ?, overview = ?, genres = ?, runtime = ?, rating = ?, backdrop_path = ?, poster_path = ?, monitoring_status = ?, updated_at = ?
WHERE id = ? AND deleted_at IS NULL;

-- name: UpdateMovieLastSearch :exec
UPDATE movies
SET last_search_at = ?, updated_at = ?
WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteMovie :exec
UPDATE movies
SET deleted_at = ?, updated_at = ?
WHERE id = ?;

-- name: ListMovies :many
SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
FROM movies
WHERE deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT ? OFFSET ?;

-- name: ListMoviesByMonitoringStatus :many
SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
FROM movies
WHERE deleted_at IS NULL AND monitoring_status = ?
ORDER BY updated_at DESC
LIMIT ? OFFSET ?;

-- name: SearchMoviesByTitle :many
SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
FROM movies
WHERE deleted_at IS NULL AND (title LIKE ? OR LOWER(title) LIKE LOWER(?))
ORDER BY year DESC, title ASC
LIMIT ? OFFSET ?;

-- name: GetMovieByTMDBID :one
SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
FROM movies
WHERE tmdb_id = ? AND deleted_at IS NULL;

-- name: GetMovieByIMDBID :one
SELECT id, title, year, imdb_id, tmdb_id, tvdb_id, overview, genres, runtime, rating, backdrop_path, poster_path, metadata_provider, last_search_at, monitoring_status, created_at, updated_at, deleted_at
FROM movies
WHERE imdb_id = ? AND deleted_at IS NULL;

-- name: CountMovies :one
SELECT COUNT(*) as count
FROM movies
WHERE deleted_at IS NULL;

-- name: CreateRootFolder :exec
INSERT INTO root_folders (id, path, created_at, updated_at)
VALUES (?, ?, ?, ?);

-- name: GetRootFolder :one
SELECT id, path, free_space, unmapped_count, created_at, updated_at, deleted_at
FROM root_folders
WHERE id = ? AND deleted_at IS NULL;

-- name: UpdateRootFolder :exec
UPDATE root_folders
SET path = ?, free_space = ?, unmapped_count = ?, updated_at = ?
WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteRootFolder :exec
UPDATE root_folders
SET deleted_at = ?, updated_at = ?
WHERE id = ?;

-- name: ListRootFolders :many
SELECT id, path, free_space, unmapped_count, created_at, updated_at, deleted_at
FROM root_folders
WHERE deleted_at IS NULL
ORDER BY path ASC;

-- name: GetRootFolderByPath :one
SELECT id, path, free_space, unmapped_count, created_at, updated_at, deleted_at
FROM root_folders
WHERE path = ? AND deleted_at IS NULL;

-- name: CreateMovieFile :exec
INSERT INTO movie_files (id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetMovieFile :one
SELECT id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at, deleted_at
FROM movie_files
WHERE id = ? AND deleted_at IS NULL;

-- name: UpdateMovieFile :exec
UPDATE movie_files
SET file_path = ?, size = ?, quality = ?, format = ?, media_info = ?, file_date = ?, updated_at = ?
WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteMovieFile :exec
UPDATE movie_files
SET deleted_at = ?, updated_at = ?
WHERE id = ?;

-- name: ListMovieFilesByMovie :many
SELECT id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at, deleted_at
FROM movie_files
WHERE movie_id = ? AND deleted_at IS NULL
ORDER BY file_path ASC;

-- name: GetMovieFileByPath :one
SELECT id, movie_id, file_path, size, quality, format, media_info, file_date, date_added, created_at, updated_at, deleted_at
FROM movie_files
WHERE file_path = ? AND deleted_at IS NULL;

-- name: CountMovieFiles :one
SELECT COUNT(*) as count
FROM movie_files
WHERE movie_id = ? AND deleted_at IS NULL;
