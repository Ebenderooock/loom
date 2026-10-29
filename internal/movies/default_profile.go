package movies

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// MissingQualityProfileSQL checks both generations without treating valid legacy
// profiles as missing. quality_profile_id belongs to the enclosing title query.
const MissingQualityProfileSQL = `NOT EXISTS (SELECT 1 FROM quality_profiles_v2 WHERE id = quality_profile_id)
	AND NOT EXISTS (SELECT 1 FROM quality_profiles WHERE id = quality_profile_id AND deleted_at IS NULL)`

// IsQualityProfileMissing checks whether a selection refers to an active profile.
func IsQualityProfileMissing(ctx context.Context, db *sql.DB, id string) (bool, error) {
	var missing bool
	err := db.QueryRowContext(ctx, `SELECT NOT EXISTS (SELECT 1 FROM quality_profiles_v2 WHERE id = ?)
		AND NOT EXISTS (SELECT 1 FROM quality_profiles WHERE id = ? AND deleted_at IS NULL)`, id, id).Scan(&missing)
	if err != nil {
		return false, fmt.Errorf("check quality profile selection: %w", err)
	}
	return missing, nil
}

// ResolveQualityProfileID preserves an explicit selection or resolves the Any
// profile for newly added movies and series. Prefer the current profile table;
// legacy installations can still use their v1 Any profile.
func ResolveQualityProfileID(ctx context.Context, db *sql.DB, selected string) (string, error) {
	// Older import lists used this placeholder instead of a real profile ID.
	if selected == "default" {
		missing, err := IsQualityProfileMissing(ctx, db, selected)
		if err != nil {
			return "", fmt.Errorf("resolve import-list quality profile: %w", err)
		}
		if missing {
			selected = ""
		}
	}
	if strings.TrimSpace(selected) != "" {
		return selected, nil
	}
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM quality_profiles_v2 WHERE name = 'Any' ORDER BY id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRowContext(ctx, `SELECT id FROM quality_profiles WHERE name = 'Any' AND deleted_at IS NULL ORDER BY id LIMIT 1`).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("default Any quality profile is unavailable; select a quality profile before adding this title: %w", err)
	}
	if err != nil {
		return "", fmt.Errorf("resolve default Any quality profile: %w", err)
	}
	if strings.TrimSpace(id) == "" {
		return "", errors.New("default Any quality profile has no ID; select a quality profile before adding this title")
	}
	return id, nil
}
