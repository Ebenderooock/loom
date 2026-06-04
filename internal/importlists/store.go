package importlists

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store handles persistence for import lists, items, and exclusions.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given *sql.DB.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// --- Import Lists CRUD ---

// ListAll returns all configured import lists.
func (s *Store) ListAll(ctx context.Context) ([]*ImportList, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, list_type, enabled, url, api_key, access_token,
		       sync_interval_minutes, root_folder_path, quality_profile_id,
		       media_type, monitor_type, search_on_add, last_sync, settings,
		       created_at, updated_at, mode
		FROM import_lists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lists []*ImportList
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			return nil, err
		}
		lists = append(lists, l)
	}
	return lists, rows.Err()
}

// Get returns a single import list by ID.
func (s *Store) Get(ctx context.Context, id string) (*ImportList, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, list_type, enabled, url, api_key, access_token,
		       sync_interval_minutes, root_folder_path, quality_profile_id,
		       media_type, monitor_type, search_on_add, last_sync, settings,
		       created_at, updated_at, mode
		FROM import_lists WHERE id = ?`, id)

	l := &ImportList{}
	var (
		enabled, searchOnAdd int
		url, apiKey, token, rootFolder sql.NullString
		lastSync sql.NullTime
		mode sql.NullString
	)
	err := row.Scan(
		&l.ID, &l.Name, &l.ListType, &enabled, &url, &apiKey, &token,
		&l.SyncIntervalMinutes, &rootFolder, &l.QualityProfileID,
		&l.MediaType, &l.MonitorType, &searchOnAdd, &lastSync, &l.Settings,
		&l.CreatedAt, &l.UpdatedAt, &mode,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.Enabled = enabled != 0
	l.SearchOnAdd = searchOnAdd != 0
	l.URL = url.String
	l.APIKey = apiKey.String
	l.AccessToken = token.String
	l.LibraryPath = rootFolder.String
	l.Mode = normalizeMode(mode.String)
	if lastSync.Valid {
		l.LastSync = &lastSync.Time
	}
	return l, nil
}

// Create inserts a new import list.
func (s *Store) Create(ctx context.Context, l *ImportList) error {
	if l.ID == "" {
		l.ID = makeSlug(l.Name)
	}
	now := time.Now().UTC()
	l.CreatedAt = now
	l.UpdatedAt = now
	if l.Settings == "" {
		l.Settings = "{}"
	}
	if l.Mode == "" {
		l.Mode = ListModeAuto
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO import_lists
		(id, name, list_type, enabled, url, api_key, access_token,
		 sync_interval_minutes, root_folder_path, quality_profile_id,
		 media_type, monitor_type, search_on_add, settings, created_at, updated_at, mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.Name, l.ListType, boolToInt(l.Enabled),
		nullStr(l.URL), nullStr(l.APIKey), nullStr(l.AccessToken),
		l.SyncIntervalMinutes, nullStr(l.LibraryPath), l.QualityProfileID,
		l.MediaType, l.MonitorType, boolToInt(l.SearchOnAdd),
		l.Settings, l.CreatedAt, l.UpdatedAt, string(l.Mode),
	)
	return err
}

// Update modifies an existing import list.
func (s *Store) Update(ctx context.Context, l *ImportList) error {
	l.UpdatedAt = time.Now().UTC()
	if l.Mode == "" {
		l.Mode = ListModeAuto
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE import_lists SET
			name = ?, list_type = ?, enabled = ?, url = ?, api_key = ?,
			access_token = ?, sync_interval_minutes = ?, root_folder_path = ?,
			quality_profile_id = ?, media_type = ?, monitor_type = ?,
			search_on_add = ?, settings = ?, updated_at = ?, mode = ?
		WHERE id = ?`,
		l.Name, l.ListType, boolToInt(l.Enabled),
		nullStr(l.URL), nullStr(l.APIKey), nullStr(l.AccessToken),
		l.SyncIntervalMinutes, nullStr(l.LibraryPath), l.QualityProfileID,
		l.MediaType, l.MonitorType, boolToInt(l.SearchOnAdd),
		l.Settings, l.UpdatedAt, string(l.Mode), l.ID,
	)
	return err
}

// Delete removes an import list and its items (via CASCADE).
func (s *Store) Delete(ctx context.Context, id string) error {
	// SQLite may not enforce FK cascades; delete items explicitly.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM import_list_items WHERE list_id = ?`, id)
	_, err := s.db.ExecContext(ctx, `DELETE FROM import_lists WHERE id = ?`, id)
	return err
}

// UpdateLastSync sets the last_sync timestamp for a list.
func (s *Store) UpdateLastSync(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE import_lists SET last_sync = ?, updated_at = ? WHERE id = ?`,
		t, t, id)
	return err
}

// --- Import List Items ---

// ListItems returns all items for a given list.
func (s *Store) ListItems(ctx context.Context, listID string) ([]*ImportListItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, list_id, external_id, title, year, imdb_id, tmdb_id,
		       tvdb_id, media_type, status, last_seen, created_at, poster_path, overview, genres
		FROM import_list_items WHERE list_id = ? ORDER BY title`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*ImportListItem
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpsertItem creates or updates an import list item.
func (s *Store) UpsertItem(ctx context.Context, item *ImportListItem) error {
	if item.ID == "" {
		item.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	item.LastSeen = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO import_list_items
		(id, list_id, external_id, title, year, imdb_id, tmdb_id, tvdb_id, media_type, status, last_seen, created_at, poster_path, overview, genres)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title, year = excluded.year,
			imdb_id = excluded.imdb_id, tmdb_id = excluded.tmdb_id,
			tvdb_id = excluded.tvdb_id, media_type = excluded.media_type,
			status = excluded.status, last_seen = excluded.last_seen,
			poster_path = excluded.poster_path, overview = excluded.overview,
			genres = excluded.genres`,
		item.ID, item.ListID, item.ExternalID, item.Title, item.Year,
		nullStr(item.IMDbID), nullStr(item.TMDbID), nullStr(item.TVDbID),
		nullStr(item.MediaType), item.Status, item.LastSeen, now,
		item.PosterPath, item.Overview, encodeGenres(item.Genres),
	)
	return err
}

// FindItemByExternalID looks up an existing item by list + external ID.
func (s *Store) FindItemByExternalID(ctx context.Context, listID, externalID string) (*ImportListItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, list_id, external_id, title, year, imdb_id, tmdb_id,
		       tvdb_id, media_type, status, last_seen, created_at, poster_path, overview, genres
		FROM import_list_items WHERE list_id = ? AND external_id = ?`, listID, externalID)

	item := &ImportListItem{}
	var year sql.NullInt64
	var imdb, tmdb, tvdb, mediaType, poster, overview, genres sql.NullString
	err := row.Scan(
		&item.ID, &item.ListID, &item.ExternalID, &item.Title, &year,
		&imdb, &tmdb, &tvdb, &mediaType, &item.Status, &item.LastSeen, &item.CreatedAt,
		&poster, &overview, &genres,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if year.Valid {
		y := int(year.Int64)
		item.Year = &y
	}
	item.IMDbID = imdb.String
	item.TMDbID = tmdb.String
	item.TVDbID = tvdb.String
	item.MediaType = mediaType.String
	item.PosterPath = poster.String
	item.Overview = overview.String
	item.Genres = decodeGenres(genres.String)
	return item, nil
}

// GetItem returns a single import list item by ID.
func (s *Store) GetItem(ctx context.Context, id string) (*ImportListItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, list_id, external_id, title, year, imdb_id, tmdb_id,
		       tvdb_id, media_type, status, last_seen, created_at, poster_path, overview, genres
		FROM import_list_items WHERE id = ?`, id)
	item, err := scanItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

// discoverRow pairs a discover item with its parent list context.
type discoverRow struct {
	Item     *ImportListItem
	ListName string
	MediaType string
}

// ListDiscoverItems returns items belonging to enabled discover-mode lists,
// filtered by effective media type ("movie" or "series"), excluding excluded items.
func (s *Store) ListDiscoverItems(ctx context.Context, mediaType string) ([]*discoverRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.list_id, i.external_id, i.title, i.year, i.imdb_id, i.tmdb_id,
		       i.tvdb_id, i.media_type, i.status, i.last_seen, i.created_at,
		       i.poster_path, i.overview, i.genres, l.name, l.media_type
		FROM import_list_items i
		JOIN import_lists l ON i.list_id = l.id
		WHERE l.mode = 'discover' AND l.enabled = 1 AND i.status != 'excluded'
		  AND COALESCE(NULLIF(i.media_type, ''), l.media_type) = ?
		ORDER BY i.title`, mediaType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*discoverRow
	for rows.Next() {
		item := &ImportListItem{}
		var year sql.NullInt64
		var imdb, tmdb, tvdb, itemMedia, poster, overview, genres sql.NullString
		var listName, listMedia string
		if err := rows.Scan(
			&item.ID, &item.ListID, &item.ExternalID, &item.Title, &year,
			&imdb, &tmdb, &tvdb, &itemMedia, &item.Status, &item.LastSeen, &item.CreatedAt,
			&poster, &overview, &genres, &listName, &listMedia,
		); err != nil {
			return nil, err
		}
		if year.Valid {
			y := int(year.Int64)
			item.Year = &y
		}
		item.IMDbID = imdb.String
		item.TMDbID = tmdb.String
		item.TVDbID = tvdb.String
		item.MediaType = itemMedia.String
		item.PosterPath = poster.String
		item.Overview = overview.String
		item.Genres = decodeGenres(genres.String)
		eff := item.MediaType
		if eff == "" {
			eff = listMedia
		}
		out = append(out, &discoverRow{Item: item, ListName: listName, MediaType: eff})
	}
	return out, rows.Err()
}

// --- Exclusions ---

// ListExclusions returns all exclusions.
func (s *Store) ListExclusions(ctx context.Context) ([]*ImportListExclusion, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tmdb_id, tvdb_id, imdb_id, title, year, created_at
		FROM import_list_exclusions ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exclusions []*ImportListExclusion
	for rows.Next() {
		e := &ImportListExclusion{}
		var tmdb, tvdb, imdb sql.NullString
		var year sql.NullInt64
		if err := rows.Scan(&e.ID, &tmdb, &tvdb, &imdb, &e.Title, &year, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.TMDbID = tmdb.String
		e.TVDbID = tvdb.String
		e.IMDbID = imdb.String
		if year.Valid {
			y := int(year.Int64)
			e.Year = &y
		}
		exclusions = append(exclusions, e)
	}
	return exclusions, rows.Err()
}

// CreateExclusion adds an exclusion.
func (s *Store) CreateExclusion(ctx context.Context, e *ImportListExclusion) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	e.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO import_list_exclusions (id, tmdb_id, tvdb_id, imdb_id, title, year, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, nullStr(e.TMDbID), nullStr(e.TVDbID), nullStr(e.IMDbID),
		e.Title, e.Year, e.CreatedAt,
	)
	return err
}

// DeleteExclusion removes an exclusion.
func (s *Store) DeleteExclusion(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM import_list_exclusions WHERE id = ?`, id)
	return err
}

// IsExcluded checks if an item is excluded by any known external ID.
func (s *Store) IsExcluded(ctx context.Context, imdbID, tmdbID, tvdbID string) (bool, error) {
	var conditions []string
	var args []any
	if imdbID != "" {
		conditions = append(conditions, "imdb_id = ?")
		args = append(args, imdbID)
	}
	if tmdbID != "" {
		conditions = append(conditions, "tmdb_id = ?")
		args = append(args, tmdbID)
	}
	if tvdbID != "" {
		conditions = append(conditions, "tvdb_id = ?")
		args = append(args, tvdbID)
	}
	if len(conditions) == 0 {
		return false, nil
	}
	query := fmt.Sprintf(
		`SELECT COUNT(*) FROM import_list_exclusions WHERE %s`,
		strings.Join(conditions, " OR "),
	)
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// ItemCount returns the number of items for a given list.
func (s *Store) ItemCount(ctx context.Context, listID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM import_list_items WHERE list_id = ?`, listID).Scan(&count)
	return count, err
}

// --- helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanList(row rowScanner) (*ImportList, error) {
	l := &ImportList{}
	var (
		enabled, searchOnAdd int
		url, apiKey, token, rootFolder sql.NullString
		lastSync sql.NullTime
		mode sql.NullString
	)
	err := row.Scan(
		&l.ID, &l.Name, &l.ListType, &enabled, &url, &apiKey, &token,
		&l.SyncIntervalMinutes, &rootFolder, &l.QualityProfileID,
		&l.MediaType, &l.MonitorType, &searchOnAdd, &lastSync, &l.Settings,
		&l.CreatedAt, &l.UpdatedAt, &mode,
	)
	if err != nil {
		return nil, err
	}
	l.Enabled = enabled != 0
	l.SearchOnAdd = searchOnAdd != 0
	l.URL = url.String
	l.APIKey = apiKey.String
	l.AccessToken = token.String
	l.LibraryPath = rootFolder.String
	l.Mode = normalizeMode(mode.String)
	if lastSync.Valid {
		l.LastSync = &lastSync.Time
	}
	return l, nil
}

func scanItem(row rowScanner) (*ImportListItem, error) {
	item := &ImportListItem{}
	var year sql.NullInt64
	var imdb, tmdb, tvdb, mediaType, poster, overview, genres sql.NullString
	err := row.Scan(
		&item.ID, &item.ListID, &item.ExternalID, &item.Title, &year,
		&imdb, &tmdb, &tvdb, &mediaType, &item.Status, &item.LastSeen, &item.CreatedAt,
		&poster, &overview, &genres,
	)
	if err != nil {
		return nil, err
	}
	if year.Valid {
		y := int(year.Int64)
		item.Year = &y
	}
	item.IMDbID = imdb.String
	item.TMDbID = tmdb.String
	item.TVDbID = tvdb.String
	item.MediaType = mediaType.String
	item.PosterPath = poster.String
	item.Overview = overview.String
	item.Genres = decodeGenres(genres.String)
	return item, nil
}

// encodeGenres serialises genre names as a pipe-separated string. Pipe is used
// (rather than comma) because genre names may contain commas/ampersands.
func encodeGenres(genres []string) string {
	cleaned := make([]string, 0, len(genres))
	for _, g := range genres {
		if g = strings.TrimSpace(g); g != "" {
			cleaned = append(cleaned, g)
		}
	}
	return strings.Join(cleaned, "|")
}

// decodeGenres parses a pipe-separated genre string back into a slice.
func decodeGenres(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// normalizeMode coerces a stored mode string to a valid ListMode, defaulting to auto.
func normalizeMode(s string) ListMode {
	if ListMode(s) == ListModeDiscover {
		return ListModeDiscover
	}
	return ListModeAuto
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func makeSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = uuid.New().String()[:8]
	}
	return s
}
