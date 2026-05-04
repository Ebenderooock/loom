-- name: CreateMovie :one
INSERT INTO movies (
    id, title, year, imdb_id, tmdb_id, tvdb_id,
    overview, genres, runtime, rating,
    backdrop_path, poster_path,
    metadata_provider, last_search_at, monitoring_status,
    created_at, updated_at
)
VALUES (
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?,
    ?, ?, ?,
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
RETURNING *;

-- name: GetMovie :one
SELECT * FROM movies WHERE id = ? AND deleted_at IS NULL LIMIT 1;

-- name: GetMovieByTMDBID :one
SELECT * FROM movies WHERE tmdb_id = ? AND deleted_at IS NULL LIMIT 1;

-- name: GetMovieByIMDBID :one
SELECT * FROM movies WHERE imdb_id = ? AND deleted_at IS NULL LIMIT 1;

-- name: GetMovieBYTVDBID :one
SELECT * FROM movies WHERE tvdb_id = ? AND deleted_at IS NULL LIMIT 1;

-- name: ListMovies :many
SELECT * FROM movies WHERE deleted_at IS NULL ORDER BY title ASC, year DESC;

-- name: CountMovies :one
SELECT COUNT(*) FROM movies WHERE deleted_at IS NULL;

-- name: ListMoviesByMonitoringStatus :many
SELECT * FROM movies WHERE monitoring_status = ? AND deleted_at IS NULL ORDER BY title ASC;

-- name: ListMoviesByYear :many
SELECT * FROM movies WHERE year = ? AND deleted_at IS NULL ORDER BY title ASC;

-- name: SearchMovies :many
SELECT * FROM movies
WHERE (title LIKE ? OR overview LIKE ?)
AND deleted_at IS NULL
ORDER BY title ASC
LIMIT ?
OFFSET ?;

-- name: UpdateMovie :one
UPDATE movies
SET title = COALESCE(sqlc.narg('title'), title),
    year = COALESCE(sqlc.narg('year'), year),
    overview = COALESCE(sqlc.narg('overview'), overview),
    genres = COALESCE(sqlc.narg('genres'), genres),
    runtime = COALESCE(sqlc.narg('runtime'), runtime),
    rating = COALESCE(sqlc.narg('rating'), rating),
    backdrop_path = COALESCE(sqlc.narg('backdrop_path'), backdrop_path),
    poster_path = COALESCE(sqlc.narg('poster_path'), poster_path),
    monitoring_status = COALESCE(sqlc.narg('monitoring_status'), monitoring_status),
    last_search_at = COALESCE(sqlc.narg('last_search_at'), last_search_at),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SetMonitoringStatus :one
UPDATE movies
SET monitoring_status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteMovie :exec
UPDATE movies SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: CreateRootFolder :one
INSERT INTO root_folders (
    id, path, free_space, unmapped_count,
    created_at, updated_at
)
VALUES (
    ?, ?, ?, ?,
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
RETURNING *;

-- name: GetRootFolder :one
SELECT * FROM root_folders WHERE id = ? AND deleted_at IS NULL LIMIT 1;

-- name: GetRootFolderByPath :one
SELECT * FROM root_folders WHERE path = ? AND deleted_at IS NULL LIMIT 1;

-- name: ListRootFolders :many
SELECT * FROM root_folders WHERE deleted_at IS NULL ORDER BY path ASC;

-- name: UpdateRootFolderFreeSpace :exec
UPDATE root_folders
SET free_space = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateRootFolderUnmappedCount :exec
UPDATE root_folders
SET unmapped_count = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: SoftDeleteRootFolder :exec
UPDATE root_folders SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: CreateMovieFile :one
INSERT INTO movie_files (
    id, movie_id, file_path, size, quality, format,
    media_info, file_date, date_added,
    created_at, updated_at
)
VALUES (
    ?, ?, ?, ?, ?, ?,
    ?, ?, ?,
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
)
RETURNING *;

-- name: GetMovieFile :one
SELECT * FROM movie_files WHERE id = ? AND deleted_at IS NULL LIMIT 1;

-- name: GetMovieFileByPath :one
SELECT * FROM movie_files WHERE file_path = ? AND deleted_at IS NULL LIMIT 1;

-- name: ListMovieFilesByMovieID :many
SELECT * FROM movie_files WHERE movie_id = ? AND deleted_at IS NULL ORDER BY file_date DESC;

-- name: UpdateMovieFile :one
UPDATE movie_files
SET quality = COALESCE(sqlc.narg('quality'), quality),
    format = COALESCE(sqlc.narg('format'), format),
    media_info = COALESCE(sqlc.narg('media_info'), media_info),
    file_date = COALESCE(sqlc.narg('file_date'), file_date),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteMovieFile :exec
UPDATE movie_files SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: DeleteMovieFilesByMovieID :exec
UPDATE movie_files SET deleted_at = CURRENT_TIMESTAMP WHERE movie_id = ?;
