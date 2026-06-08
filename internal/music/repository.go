package music

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Repository defines data access for artists, albums, releases, tracks and the
// music quality/metadata profiles.
type Repository interface {
	// Artists
	ListArtists(ctx context.Context) ([]*Artist, error)
	GetArtist(ctx context.Context, id string) (*Artist, error)
	GetArtistByMBID(ctx context.Context, mbid string) (*Artist, error) // includes soft-deleted
	CreateArtist(ctx context.Context, a *Artist) error
	UpdateArtist(ctx context.Context, a *Artist) error
	SoftDeleteArtist(ctx context.Context, id string) error

	// Albums
	ListAlbumsByArtist(ctx context.Context, artistID string) ([]*Album, error)
	GetAlbum(ctx context.Context, id string) (*Album, error)
	GetAlbumByMBID(ctx context.Context, mbid string) (*Album, error)
	CreateAlbum(ctx context.Context, al *Album) error
	UpdateAlbum(ctx context.Context, al *Album) error

	// Album releases (editions)
	ListReleasesByAlbum(ctx context.Context, albumID string) ([]*AlbumRelease, error)
	ReplaceReleases(ctx context.Context, albumID string, releases []*AlbumRelease) error

	// Tracks
	ListTracksByAlbum(ctx context.Context, albumID string) ([]*Track, error)
	ReplaceTracks(ctx context.Context, albumID string, tracks []*Track) error
	MarkTrackHasFile(ctx context.Context, trackID string, hasFile bool) error

	// Track files (physical audio files linked to a track)
	CreateTrackFile(ctx context.Context, tf *TrackFile) error
	GetTrackFileByPath(ctx context.Context, path string) (*TrackFile, error)
	ListTrackFilesByArtist(ctx context.Context, artistID string) ([]*TrackFile, error)
	DeleteTrackFile(ctx context.Context, id string) error

	// Profiles / quality definitions (read-only in M1)
	ListAudioQualityDefinitions(ctx context.Context) ([]*AudioQualityDefinition, error)
	ListAudioQualityProfiles(ctx context.Context) ([]*AudioQualityProfile, error)
	GetAudioQualityProfile(ctx context.Context, id string) (*AudioQualityProfile, error)
	ListMetadataProfiles(ctx context.Context) ([]*MetadataProfile, error)
	GetMetadataProfile(ctx context.Context, id string) (*MetadataProfile, error)

	// Stats
	GetArtistStats(ctx context.Context, artistID string) (*ArtistStats, error)
	GetAllArtistStats(ctx context.Context) (map[string]*ArtistStats, error)
}

// NewRepository creates a Repository backed by the given database.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

type sqlRepo struct {
	db *sql.DB
}

// --- column lists ---

const artistColumns = `id, mbid, name, sort_name, disambiguation, artist_type, country, overview, genres, image_url, path, library_id, quality_profile_id, metadata_profile_id, monitoring_status, metadata_provider, last_search_at, created_at, updated_at`

const albumColumns = `id, mbid, artist_id, title, album_type, secondary_types, release_date, genres, cover_art_url, overview, monitored, selected_release_id, last_search_at, releases_fetched_at, tracks_fetched_at, created_at, updated_at`

const releaseColumns = `id, mbid, album_id, title, disambiguation, status, release_date, country, label, format, media_count, track_count, created_at, updated_at`

const trackColumns = `id, recording_mbid, track_mbid, album_id, release_id, title, track_number, disc_number, duration_ms, artist_name, monitored, has_file, created_at, updated_at`

const trackFileColumns = `id, track_id, album_id, artist_id, file_path, size, quality, format, bitrate, media_info, file_date, date_added, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// --- artists ---

func scanArtist(s rowScanner) (*Artist, error) {
	a := &Artist{}
	var mbid, sortName, disamb, artistType, country, overview, imageURL, path sql.NullString
	var libraryID, qpID, mpID, provider sql.NullString
	var status string
	var genreBytes []byte
	var lastSearch sql.NullString
	var createdStr, updatedStr string
	err := s.Scan(
		&a.ID, &mbid, &a.Name, &sortName, &disamb, &artistType, &country,
		&overview, &genreBytes, &imageURL, &path, &libraryID, &qpID, &mpID,
		&status, &provider, &lastSearch, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	a.MBID = mbid.String
	a.SortName = sortName.String
	a.Disambiguation = disamb.String
	a.ArtistType = artistType.String
	a.Country = country.String
	a.Overview = overview.String
	a.ImageURL = imageURL.String
	a.Path = path.String
	a.LibraryID = libraryID.String
	a.QualityProfileID = qpID.String
	a.MetadataProfileID = mpID.String
	a.MetadataProvider = provider.String
	a.MonitoringStatus = MonitoringStatus(status)
	_ = a.Genres.Scan(genreBytes)
	a.LastSearchAt = parseNullTime(lastSearch)
	a.CreatedAt = parseTime(createdStr)
	a.UpdatedAt = parseTime(updatedStr)
	return a, nil
}

func (r *sqlRepo) ListArtists(ctx context.Context) ([]*Artist, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+artistColumns+` FROM artists WHERE deleted_at IS NULL ORDER BY sort_name, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Artist
	for rows.Next() {
		a, err := scanArtist(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetArtist(ctx context.Context, id string) (*Artist, error) {
	a, err := scanArtist(r.db.QueryRowContext(ctx,
		`SELECT `+artistColumns+` FROM artists WHERE id = ? AND deleted_at IS NULL`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

// GetArtistByMBID returns an artist by MBID even if soft-deleted (used to revive
// re-added artists). Returns (nil, nil) when not found.
func (r *sqlRepo) GetArtistByMBID(ctx context.Context, mbid string) (*Artist, error) {
	a, err := scanArtist(r.db.QueryRowContext(ctx,
		`SELECT `+artistColumns+` FROM artists WHERE mbid = ?`, mbid))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (r *sqlRepo) CreateArtist(ctx context.Context, a *Artist) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO artists (id, mbid, name, sort_name, disambiguation, artist_type, country, overview, genres, image_url, path, library_id, quality_profile_id, metadata_profile_id, monitoring_status, metadata_provider, last_search_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, nullString(a.MBID), a.Name, a.SortName, a.Disambiguation, a.ArtistType,
		a.Country, a.Overview, a.Genres, a.ImageURL, a.Path,
		nullString(a.LibraryID), nullString(a.QualityProfileID), nullString(a.MetadataProfileID),
		string(a.MonitoringStatus), a.MetadataProvider, nullTime(a.LastSearchAt),
		a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) UpdateArtist(ctx context.Context, a *Artist) error {
	a.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE artists SET mbid=?, name=?, sort_name=?, disambiguation=?, artist_type=?, country=?, overview=?, genres=?, image_url=?, path=?, library_id=?, quality_profile_id=?, metadata_profile_id=?, monitoring_status=?, metadata_provider=?, last_search_at=?, updated_at=?, deleted_at=NULL WHERE id=?`,
		nullString(a.MBID), a.Name, a.SortName, a.Disambiguation, a.ArtistType, a.Country,
		a.Overview, a.Genres, a.ImageURL, a.Path,
		nullString(a.LibraryID), nullString(a.QualityProfileID), nullString(a.MetadataProfileID),
		string(a.MonitoringStatus), a.MetadataProvider, nullTime(a.LastSearchAt),
		a.UpdatedAt, a.ID,
	)
	return err
}

func (r *sqlRepo) SoftDeleteArtist(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE artists SET deleted_at=?, updated_at=? WHERE id=?`,
		time.Now().UTC(), time.Now().UTC(), id)
	return err
}

// --- albums ---

func scanAlbum(s rowScanner) (*Album, error) {
	al := &Album{}
	var mbid, albumType, releaseDate, coverArt, overview, selectedRelease sql.NullString
	var secondaryBytes, genreBytes []byte
	var lastSearch, releasesFetched, tracksFetched sql.NullString
	var createdStr, updatedStr string
	err := s.Scan(
		&al.ID, &mbid, &al.ArtistID, &al.Title, &albumType, &secondaryBytes,
		&releaseDate, &genreBytes, &coverArt, &overview, &al.Monitored,
		&selectedRelease, &lastSearch, &releasesFetched, &tracksFetched,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	al.MBID = mbid.String
	al.AlbumType = albumType.String
	al.ReleaseDate = releaseDate.String
	al.CoverArtURL = coverArt.String
	al.Overview = overview.String
	al.SelectedReleaseID = selectedRelease.String
	_ = al.SecondaryTypes.Scan(secondaryBytes)
	_ = al.Genres.Scan(genreBytes)
	al.LastSearchAt = parseNullTime(lastSearch)
	al.ReleasesFetchedAt = parseNullTime(releasesFetched)
	al.TracksFetchedAt = parseNullTime(tracksFetched)
	al.CreatedAt = parseTime(createdStr)
	al.UpdatedAt = parseTime(updatedStr)
	return al, nil
}

func (r *sqlRepo) ListAlbumsByArtist(ctx context.Context, artistID string) ([]*Album, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+albumColumns+` FROM albums WHERE artist_id = ? AND deleted_at IS NULL ORDER BY release_date, title`, artistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Album
	for rows.Next() {
		al, err := scanAlbum(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, al)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetAlbum(ctx context.Context, id string) (*Album, error) {
	al, err := scanAlbum(r.db.QueryRowContext(ctx,
		`SELECT `+albumColumns+` FROM albums WHERE id = ? AND deleted_at IS NULL`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return al, err
}

func (r *sqlRepo) GetAlbumByMBID(ctx context.Context, mbid string) (*Album, error) {
	al, err := scanAlbum(r.db.QueryRowContext(ctx,
		`SELECT `+albumColumns+` FROM albums WHERE mbid = ?`, mbid))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return al, err
}

func (r *sqlRepo) CreateAlbum(ctx context.Context, al *Album) error {
	now := time.Now().UTC()
	if al.CreatedAt.IsZero() {
		al.CreatedAt = now
	}
	al.UpdatedAt = now
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO albums (id, mbid, artist_id, title, album_type, secondary_types, release_date, genres, cover_art_url, overview, monitored, selected_release_id, last_search_at, releases_fetched_at, tracks_fetched_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		al.ID, nullString(al.MBID), al.ArtistID, al.Title, al.AlbumType, al.SecondaryTypes,
		al.ReleaseDate, al.Genres, al.CoverArtURL, al.Overview, al.Monitored,
		nullString(al.SelectedReleaseID), nullTime(al.LastSearchAt),
		nullTime(al.ReleasesFetchedAt), nullTime(al.TracksFetchedAt),
		al.CreatedAt, al.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) UpdateAlbum(ctx context.Context, al *Album) error {
	al.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`UPDATE albums SET mbid=?, title=?, album_type=?, secondary_types=?, release_date=?, genres=?, cover_art_url=?, overview=?, monitored=?, selected_release_id=?, last_search_at=?, releases_fetched_at=?, tracks_fetched_at=?, updated_at=? WHERE id=?`,
		nullString(al.MBID), al.Title, al.AlbumType, al.SecondaryTypes, al.ReleaseDate,
		al.Genres, al.CoverArtURL, al.Overview, al.Monitored,
		nullString(al.SelectedReleaseID), nullTime(al.LastSearchAt),
		nullTime(al.ReleasesFetchedAt), nullTime(al.TracksFetchedAt),
		al.UpdatedAt, al.ID,
	)
	return err
}

// --- album releases ---

func scanRelease(s rowScanner) (*AlbumRelease, error) {
	rel := &AlbumRelease{}
	var mbid, title, disamb, status, releaseDate, country, label, format sql.NullString
	var createdStr, updatedStr string
	err := s.Scan(
		&rel.ID, &mbid, &rel.AlbumID, &title, &disamb, &status, &releaseDate,
		&country, &label, &format, &rel.MediaCount, &rel.TrackCount,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	rel.MBID = mbid.String
	rel.Title = title.String
	rel.Disambiguation = disamb.String
	rel.Status = status.String
	rel.ReleaseDate = releaseDate.String
	rel.Country = country.String
	rel.Label = label.String
	rel.Format = format.String
	rel.CreatedAt = parseTime(createdStr)
	rel.UpdatedAt = parseTime(updatedStr)
	return rel, nil
}

func (r *sqlRepo) ListReleasesByAlbum(ctx context.Context, albumID string) ([]*AlbumRelease, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+releaseColumns+` FROM album_releases WHERE album_id = ? ORDER BY release_date, title`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AlbumRelease
	for rows.Next() {
		rel, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rel)
	}
	return list, rows.Err()
}

// ReplaceReleases deletes existing releases for an album and inserts the given
// set in a single transaction.
func (r *sqlRepo) ReplaceReleases(ctx context.Context, albumID string, releases []*AlbumRelease) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM album_releases WHERE album_id = ?`, albumID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, rel := range releases {
		if rel.CreatedAt.IsZero() {
			rel.CreatedAt = now
		}
		rel.UpdatedAt = now
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO album_releases (id, mbid, album_id, title, disambiguation, status, release_date, country, label, format, media_count, track_count, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rel.ID, nullString(rel.MBID), rel.AlbumID, rel.Title, rel.Disambiguation, rel.Status,
			rel.ReleaseDate, rel.Country, rel.Label, rel.Format, rel.MediaCount, rel.TrackCount,
			rel.CreatedAt, rel.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- tracks ---

func scanTrack(s rowScanner) (*Track, error) {
	t := &Track{}
	var recMBID, trackMBID, releaseID, title, artistName sql.NullString
	var createdStr, updatedStr string
	err := s.Scan(
		&t.ID, &recMBID, &trackMBID, &t.AlbumID, &releaseID, &title,
		&t.TrackNumber, &t.DiscNumber, &t.DurationMs, &artistName,
		&t.Monitored, &t.HasFile, &createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	t.RecordingMBID = recMBID.String
	t.TrackMBID = trackMBID.String
	t.ReleaseID = releaseID.String
	t.Title = title.String
	t.ArtistName = artistName.String
	t.CreatedAt = parseTime(createdStr)
	t.UpdatedAt = parseTime(updatedStr)
	return t, nil
}

func (r *sqlRepo) ListTracksByAlbum(ctx context.Context, albumID string) ([]*Track, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+trackColumns+` FROM tracks WHERE album_id = ? ORDER BY disc_number, track_number`, albumID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// ReplaceTracks deletes existing tracks for an album and inserts the given set
// in a single transaction.
func (r *sqlRepo) ReplaceTracks(ctx context.Context, albumID string, tracks []*Track) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM tracks WHERE album_id = ?`, albumID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, t := range tracks {
		if t.CreatedAt.IsZero() {
			t.CreatedAt = now
		}
		t.UpdatedAt = now
		if t.DiscNumber == 0 {
			t.DiscNumber = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tracks (id, recording_mbid, track_mbid, album_id, release_id, title, track_number, disc_number, duration_ms, artist_name, monitored, has_file, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, nullString(t.RecordingMBID), nullString(t.TrackMBID), t.AlbumID,
			nullString(t.ReleaseID), t.Title, t.TrackNumber, t.DiscNumber, t.DurationMs,
			t.ArtistName, t.Monitored, t.HasFile, t.CreatedAt, t.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MarkTrackHasFile updates a track's has_file flag.
func (r *sqlRepo) MarkTrackHasFile(ctx context.Context, trackID string, hasFile bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tracks SET has_file = ?, updated_at = ? WHERE id = ?`,
		hasFile, time.Now().UTC(), trackID,
	)
	return err
}

// --- track files ---

func scanTrackFile(s rowScanner) (*TrackFile, error) {
	var tf TrackFile
	var trackID, albumID, artistID sql.NullString
	var fileDate sql.NullString
	var dateAdded, createdAt, updatedAt string
	if err := s.Scan(
		&tf.ID, &trackID, &albumID, &artistID, &tf.FilePath, &tf.Size,
		&tf.Quality, &tf.Format, &tf.Bitrate, &tf.MediaInfo, &fileDate,
		&dateAdded, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	tf.TrackID = trackID.String
	tf.AlbumID = albumID.String
	tf.ArtistID = artistID.String
	tf.FileDate = parseNullTime(fileDate)
	tf.DateAdded = parseTime(dateAdded)
	tf.CreatedAt = parseTime(createdAt)
	tf.UpdatedAt = parseTime(updatedAt)
	return &tf, nil
}

func (r *sqlRepo) CreateTrackFile(ctx context.Context, tf *TrackFile) error {
	now := time.Now().UTC()
	if tf.CreatedAt.IsZero() {
		tf.CreatedAt = now
	}
	if tf.DateAdded.IsZero() {
		tf.DateAdded = now
	}
	tf.UpdatedAt = now
	if tf.MediaInfo == nil {
		tf.MediaInfo = MediaInfoMap{}
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO track_files (id, track_id, album_id, artist_id, file_path, size, quality, format, bitrate, media_info, file_date, date_added, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tf.ID, nullString(tf.TrackID), nullString(tf.AlbumID), nullString(tf.ArtistID),
		tf.FilePath, tf.Size, tf.Quality, tf.Format, tf.Bitrate, tf.MediaInfo,
		nullTime(tf.FileDate), tf.DateAdded, tf.CreatedAt, tf.UpdatedAt,
	)
	return err
}

func (r *sqlRepo) GetTrackFileByPath(ctx context.Context, path string) (*TrackFile, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+trackFileColumns+` FROM track_files WHERE file_path = ? AND deleted_at IS NULL`, path)
	tf, err := scanTrackFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return tf, nil
}

func (r *sqlRepo) ListTrackFilesByArtist(ctx context.Context, artistID string) ([]*TrackFile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+trackFileColumns+` FROM track_files WHERE artist_id = ? AND deleted_at IS NULL ORDER BY file_path`, artistID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var files []*TrackFile
	for rows.Next() {
		tf, err := scanTrackFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, tf)
	}
	return files, rows.Err()
}

func (r *sqlRepo) DeleteTrackFile(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE track_files SET deleted_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

// --- quality definitions / profiles ---

func (r *sqlRepo) ListAudioQualityDefinitions(ctx context.Context) ([]*AudioQualityDefinition, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, format, bitrate, vbr, lossless, tier_order FROM audio_quality_definitions ORDER BY tier_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AudioQualityDefinition
	for rows.Next() {
		d := &AudioQualityDefinition{}
		if err := rows.Scan(&d.ID, &d.Name, &d.Format, &d.Bitrate, &d.VBR, &d.Lossless, &d.TierOrder); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func scanAudioProfile(s rowScanner) (*AudioQualityProfile, error) {
	p := &AudioQualityProfile{}
	var items []byte
	var cutoff sql.NullString
	var createdStr, updatedStr string
	if err := s.Scan(&p.ID, &p.Name, &items, &cutoff, &p.UpgradeAllowed, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	p.Items = items
	p.Cutoff = cutoff.String
	p.CreatedAt = parseTime(createdStr)
	p.UpdatedAt = parseTime(updatedStr)
	return p, nil
}

func (r *sqlRepo) ListAudioQualityProfiles(ctx context.Context) ([]*AudioQualityProfile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, items, cutoff, upgrade_allowed, created_at, updated_at FROM audio_quality_profiles WHERE deleted_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AudioQualityProfile
	for rows.Next() {
		p, err := scanAudioProfile(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetAudioQualityProfile(ctx context.Context, id string) (*AudioQualityProfile, error) {
	p, err := scanAudioProfile(r.db.QueryRowContext(ctx,
		`SELECT id, name, items, cutoff, upgrade_allowed, created_at, updated_at FROM audio_quality_profiles WHERE id = ? AND deleted_at IS NULL`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func scanMetadataProfile(s rowScanner) (*MetadataProfile, error) {
	p := &MetadataProfile{}
	var primary, secondary, statuses []byte
	var createdStr, updatedStr string
	if err := s.Scan(&p.ID, &p.Name, &primary, &secondary, &statuses, &createdStr, &updatedStr); err != nil {
		return nil, err
	}
	_ = p.PrimaryTypes.Scan(primary)
	_ = p.SecondaryTypes.Scan(secondary)
	_ = p.ReleaseStatuses.Scan(statuses)
	p.CreatedAt = parseTime(createdStr)
	p.UpdatedAt = parseTime(updatedStr)
	return p, nil
}

func (r *sqlRepo) ListMetadataProfiles(ctx context.Context) ([]*MetadataProfile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, primary_types, secondary_types, release_statuses, created_at, updated_at FROM metadata_profiles WHERE deleted_at IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*MetadataProfile
	for rows.Next() {
		p, err := scanMetadataProfile(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *sqlRepo) GetMetadataProfile(ctx context.Context, id string) (*MetadataProfile, error) {
	p, err := scanMetadataProfile(r.db.QueryRowContext(ctx,
		`SELECT id, name, primary_types, secondary_types, release_statuses, created_at, updated_at FROM metadata_profiles WHERE id = ? AND deleted_at IS NULL`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// --- stats ---

func (r *sqlRepo) GetArtistStats(ctx context.Context, artistID string) (*ArtistStats, error) {
	all, err := r.GetAllArtistStats(ctx)
	if err != nil {
		return nil, err
	}
	if s, ok := all[artistID]; ok {
		return s, nil
	}
	return &ArtistStats{}, nil
}

// GetAllArtistStats computes album/track rollups for every artist in one pass.
func (r *sqlRepo) GetAllArtistStats(ctx context.Context) (map[string]*ArtistStats, error) {
	stats := make(map[string]*ArtistStats)

	// Album counts per artist.
	albRows, err := r.db.QueryContext(ctx,
		`SELECT artist_id, COUNT(*), SUM(CASE WHEN monitored THEN 1 ELSE 0 END)
		 FROM albums WHERE deleted_at IS NULL GROUP BY artist_id`)
	if err != nil {
		return nil, err
	}
	defer albRows.Close()
	for albRows.Next() {
		var artistID string
		var total, monitored int
		if err := albRows.Scan(&artistID, &total, &monitored); err != nil {
			return nil, err
		}
		s := getOrInit(stats, artistID)
		s.AlbumCount = total
		s.MonitoredAlbumCount = monitored
	}
	if err := albRows.Err(); err != nil {
		return nil, err
	}

	// Track counts per artist (joined via albums).
	trkRows, err := r.db.QueryContext(ctx,
		`SELECT al.artist_id, COUNT(*),
		        SUM(CASE WHEN t.has_file THEN 1 ELSE 0 END),
		        SUM(CASE WHEN t.monitored AND NOT t.has_file THEN 1 ELSE 0 END)
		 FROM tracks t JOIN albums al ON al.id = t.album_id
		 WHERE al.deleted_at IS NULL GROUP BY al.artist_id`)
	if err != nil {
		return nil, err
	}
	defer trkRows.Close()
	for trkRows.Next() {
		var artistID string
		var total, withFile, missing int
		if err := trkRows.Scan(&artistID, &total, &withFile, &missing); err != nil {
			return nil, err
		}
		s := getOrInit(stats, artistID)
		s.TrackCount = total
		s.TrackFileCount = withFile
		s.MissingTrackCount = missing
	}
	return stats, trkRows.Err()
}

func getOrInit(m map[string]*ArtistStats, id string) *ArtistStats {
	if s, ok := m[id]; ok {
		return s
	}
	s := &ArtistStats{}
	m[id] = s
	return s
}

// --- helpers ---

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t *time.Time) interface{} {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC()
}

func parseNullTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t := parseTime(ns.String)
	if t.IsZero() {
		return nil
	}
	return &t
}

func parseTime(s string) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05+00:00",
		"2006-01-02 15:04:05.999999999+00:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
