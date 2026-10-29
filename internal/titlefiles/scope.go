package titlefiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) resolve(ctx context.Context, mediaType, id string, season *int) (scope, error) {
	var sc scope
	table := ""
	switch mediaType {
	case "movie":
		table = "movies"
		if season != nil {
			return sc, fmt.Errorf("%w: season is only supported for series", errInvalid)
		}
	case "series":
		table = "series"
	default:
		return sc, fmt.Errorf("%w: media type must be movie or series", errInvalid)
	}
	var year int
	active := ""
	if mediaType == "movie" {
		active = " AND m.deleted_at IS NULL"
	}
	err := s.db.QueryRowContext(ctx, `SELECT m.title, COALESCE(m.year,0), l.path FROM `+table+` m
		JOIN libraries l ON l.id=m.library_id WHERE m.id=?`+active, id).
		Scan(&sc.title, &year, &sc.library)
	if errors.Is(err, sql.ErrNoRows) {
		return sc, errNotFound
	}
	if err != nil {
		return sc, err
	}
	if season != nil {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM seasons WHERE series_id=? AND season_number=?`, id, *season).Scan(&count); err != nil {
			return sc, err
		}
		if count == 0 {
			return sc, errNotFound
		}
	}
	sc.library, err = filepath.Abs(sc.library)
	if err != nil {
		return sc, err
	}
	query := `SELECT file_path, COALESCE(quality,''), COALESCE(format,''), 0, deleted_at IS NULL FROM movie_files WHERE movie_id=?`
	if mediaType == "series" {
		query = `SELECT f.file_path, COALESCE(f.quality,''), COALESCE(f.codec,''), sn.season_number, true
			FROM episode_files f JOIN episodes e ON e.id=f.episode_id JOIN seasons sn ON sn.id=e.season_id WHERE f.series_id=?`
	}
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return sc, err
	}
	for rows.Next() {
		var f trackedFile
		if err := rows.Scan(&f.path, &f.quality, &f.format, &f.season, &f.active); err != nil {
			_ = rows.Close()
			return sc, err
		}
		sc.tracked = append(sc.tracked, f)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return sc, err
	}
	candidates := map[string]bool{}
	for _, file := range sc.tracked {
		rel, err := filepath.Rel(sc.library, file.path)
		if err != nil || !filepath.IsLocal(rel) {
			return sc, fmt.Errorf("%w: tracked file is outside its library", errConflict)
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 2 {
			return sc, fmt.Errorf("%w: loose files in a shared library root are not supported", errConflict)
		}
		candidate := filepath.Join(sc.library, parts[0])
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return sc, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return sc, fmt.Errorf("%w: title folder must be a real directory, not a symbolic link", errConflict)
		}
		candidates[candidate] = true
	}
	if len(candidates) == 0 {
		var saved string
		err := s.db.QueryRowContext(ctx, `SELECT f.folder FROM title_file_folders f
			JOIN libraries l ON l.id=f.library_id JOIN `+table+` m ON m.library_id=l.id
			WHERE f.media_type=? AND f.title_id=? AND m.id=?`, mediaType, id, id).Scan(&saved)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return sc, err
		}
		if saved != "" {
			if filepath.Base(saved) != saved || !filepath.IsLocal(saved) {
				return sc, fmt.Errorf("%w: invalid saved folder", errConflict)
			}
			candidate := filepath.Join(sc.library, saved)
			info, err := os.Lstat(candidate)
			if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				candidates[candidate] = true
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return sc, err
			}
		}
	}
	if len(candidates) == 0 {
		entries, err := os.ReadDir(sc.library)
		if err != nil {
			return sc, err
		}
		conventional := fmt.Sprintf("%s (%d)", sc.title, year)
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			if strings.EqualFold(name, conventional) || strings.EqualFold(name, sc.title) {
				candidates[filepath.Join(sc.library, name)] = true
			}
		}
	}
	if len(candidates) == 0 {
		return sc, errNotFound
	}
	if len(candidates) != 1 {
		return sc, fmt.Errorf("%w: title maps to multiple folders", errConflict)
	}
	for path := range candidates {
		sc.root = path
	}
	resolvedLibrary, err := filepath.EvalSymlinks(sc.library)
	if err != nil {
		return sc, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(sc.root)
	if err != nil {
		return sc, err
	}
	rel, err := filepath.Rel(resolvedLibrary, resolvedRoot)
	if err != nil || !filepath.IsLocal(rel) || rel == "." {
		return sc, fmt.Errorf("%w: title folder escapes library", errConflict)
	}
	// Refuse a folder shared by tracked files belonging to another title.
	otherMovie, otherSeries := "", ""
	if mediaType == "movie" {
		otherMovie = id
	} else {
		otherSeries = id
	}
	var shared int
	prefix := sc.root + string(filepath.Separator)
	err = s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM movie_files WHERE movie_id<>? AND deleted_at IS NULL AND substr(file_path,1,?)=?) +
		(SELECT COUNT(*) FROM episode_files WHERE series_id<>? AND substr(file_path,1,?)=?)`,
		otherMovie, len(prefix), prefix, otherSeries, len(prefix), prefix).Scan(&shared)
	if err != nil {
		return sc, err
	}
	if shared != 0 {
		return sc, fmt.Errorf("%w: folder contains tracked files for another title", errConflict)
	}
	return sc, nil
}
