package customformats

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrTierReferenced prevents deletion of a tier still used by a profile or file.
var ErrTierReferenced = errors.New("custom tier is referenced by a profile or imported file")

// ListTiers returns current matcher-backed tiers.
func (s *Store) ListTiers(ctx context.Context) ([]Tier, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, specifications FROM custom_quality_tiers ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var tiers []Tier
	for rows.Next() {
		var tier Tier
		var specs string
		if err := rows.Scan(&tier.ID, &tier.Name, &specs); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(specs), &tier.Specifications); err != nil {
			return nil, fmt.Errorf("invalid tier %q: %w", tier.ID, err)
		}
		tiers = append(tiers, tier)
	}
	return tiers, rows.Err()
}

// SaveTier creates or updates a tier; identifiers are allocated on creation.
func (s *Store) SaveTier(ctx context.Context, tier *Tier, create bool) error {
	if create {
		tier.ID = "ct_" + uuid.NewString()
	}
	if err := tier.Validate(); err != nil {
		return err
	}
	specs, err := json.Marshal(tier.Specifications)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var result sql.Result
	if create {
		result, err = s.db.ExecContext(ctx, `INSERT INTO custom_quality_tiers (id,name,specifications,created_at,updated_at) VALUES (?,?,?,?,?)`, tier.ID, tier.Name, string(specs), now, now)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE custom_quality_tiers SET name=?, specifications=?, updated_at=? WHERE id=?`, tier.Name, string(specs), now, tier.ID)
	}
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteTier atomically rejects deletions of profiles' or files' tier identities.
func (s *Store) DeleteTier(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM custom_quality_tiers
		WHERE id = ?
		AND NOT EXISTS (SELECT 1 FROM quality_profiles_v2 p, json_each(CASE WHEN json_valid(p.items) THEN p.items ELSE '[]' END) i
			WHERE json_extract(i.value, '$.kind') = 'custom' AND json_extract(i.value, '$.id') = ?)
		AND NOT EXISTS (SELECT 1 FROM movie_files WHERE quality = ?)
		AND NOT EXISTS (SELECT 1 FROM episode_files WHERE quality = ?)`, id, id, StoredTierID(id), StoredTierID(id))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 0 {
		return err
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_quality_tiers WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return ErrTierReferenced
	}
	return sql.ErrNoRows
}
