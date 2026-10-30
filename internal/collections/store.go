package collections

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Store persists bounded collection aggregates. Members/provenance/local intent
// are one atomic document; source identities are relational unique keys. Revision
// checks protect concurrent refreshes, edits and merges across server processes.
type Store struct{ db *sql.DB }

// NewStore uses the server's migrated SQLite or PostgreSQL connection.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func decode(body string, revision int) (*Collection, error) {
	var c Collection
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		return nil, err
	}
	c.Revision = revision
	return &c, nil
}

// Get resolves both canonical collection IDs and stable merged-ID redirects.
func (s *Store) Get(ctx context.Context, id string) (*Collection, error) {
	var body string
	var revision int
	err := s.db.QueryRowContext(ctx, `SELECT body, revision FROM collections WHERE id = $1`, id).Scan(&body, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, `SELECT c.body, c.revision FROM collection_redirects r JOIN collections c ON c.id = r.collection_id WHERE r.id = $1`, id).Scan(&body, &revision)
	}
	if err != nil {
		return nil, err
	}
	return decode(body, revision)
}

// List returns persisted collections without contacting external providers.
func (s *Store) List(ctx context.Context) ([]*Collection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT body, revision FROM collections ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*Collection{}
	for rows.Next() {
		var body string
		var rev int
		if err := rows.Scan(&body, &rev); err != nil {
			return nil, err
		}
		c, err := decode(body, rev)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FindSource resolves the owner of an exact provider/kind/ID reference.
func (s *Store) FindSource(ctx context.Context, ref Source) (*Collection, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT collection_id FROM collection_sources WHERE provider=$1 AND kind=$2 AND external_id=$3`, ref.Provider, ref.Kind, ref.ExternalID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func encode(c *Collection) ([]byte, error) {
	copy := *c
	copy.Total, copy.InLibrary = 0, 0
	copy.Members = append([]Member{}, c.Members...)
	for i := range copy.Members {
		m := &copy.Members[i]
		m.MediaID, m.LibraryID, m.InLibrary, m.Monitored, m.HasFile = "", "", false, false, nil
	}
	body, err := json.Marshal(copy)
	if len(body) > 8<<20 {
		return nil, fmt.Errorf("%w: collection exceeds 8 MiB", ErrInvalid)
	}
	return body, err
}

func write(ctx context.Context, tx *sql.Tx, c *Collection, create bool) error {
	body, err := encode(c)
	if err != nil {
		return err
	}
	if create {
		_, err = tx.ExecContext(ctx, `INSERT INTO collections(id, body, revision) VALUES($1,$2,0)`, c.ID, string(body))
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE collections SET body=$1, revision=revision+1 WHERE id=$2 AND revision=$3`, string(body), c.ID, c.Revision)
		if err == nil {
			n, e := result.RowsAffected()
			if e != nil {
				return e
			}
			if n != 1 {
				return ErrConflict
			}
		}
	}
	if err != nil {
		return err
	}
	for _, ref := range c.Sources {
		var owner string
		err := tx.QueryRowContext(ctx, `SELECT collection_id FROM collection_sources WHERE provider=$1 AND kind=$2 AND external_id=$3`, ref.Provider, ref.Kind, ref.ExternalID).Scan(&owner)
		if err == nil {
			if owner != c.ID {
				return ErrConflict
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO collection_sources(provider,kind,external_id,collection_id) VALUES($1,$2,$3,$4)`, ref.Provider, ref.Kind, ref.ExternalID, c.ID); err != nil {
			return err
		}
	}
	return nil
}

// Save writes the aggregate and unique source ownership in one transaction.
func (s *Store) Save(ctx context.Context, c *Collection, create bool) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if create {
		c.ID, c.Revision = uuid.NewString(), 0
		if c.Sources == nil {
			c.Sources = []Source{}
		}
		if c.Members == nil {
			c.Members = []Member{}
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := write(ctx, tx, c, create); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !create {
		c.Revision++
	}
	return nil
}

// Merge atomically combines local intent, sources and members, retaining links.
func (s *Store) Merge(ctx context.Context, into, from *Collection) error {
	if into.ID == from.ID {
		return fmt.Errorf("%w: cannot merge a collection into itself", ErrInvalid)
	}
	members := into.Members
	for _, member := range from.Members {
		var err error
		members, err = coalesce(members, member)
		if err != nil {
			return err
		}
	}
	into.Members = members
	into.Sources = append(into.Sources, from.Sources...)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	// Lock/update both rows in stable order, with optimistic revision checks.
	pair := []*Collection{into, from}
	if strings.Compare(into.ID, from.ID) > 0 {
		pair[0], pair[1] = from, into
	}
	for _, c := range pair {
		result, err := tx.ExecContext(ctx, `UPDATE collections SET revision=revision WHERE id=$1 AND revision=$2`, c.ID, c.Revision)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrConflict
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collection_sources SET collection_id=$1 WHERE collection_id=$2`, into.ID, from.ID); err != nil {
		return err
	}
	if err := write(ctx, tx, into, false); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collection_redirects SET collection_id=$1 WHERE collection_id=$2`, into.ID, from.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO collection_redirects(id,collection_id) VALUES($1,$2)`, from.ID, into.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM collections WHERE id=$1`, from.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	into.Revision++
	return nil
}
