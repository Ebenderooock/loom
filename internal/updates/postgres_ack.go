package updates

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/ebenderooock/loom/internal/storage"
)

var errPostgresIdentityUnavailable = errors.New("PostgreSQL acknowledgement requires a single known effective database identity")

type postgresIdentitySource interface {
	PostgresIdentity() (storage.PostgresIdentity, bool)
}

type postgresAckRecord struct {
	Acknowledged bool   `json:"acknowledged"`
	Identity     string `json:"identity"`
}

func (s *Service) postgresIdentityFingerprint() (string, bool) {
	if s.db.Engine() != storage.EnginePostgres {
		return "", false
	}
	source, ok := s.db.(postgresIdentitySource)
	if !ok {
		return "", false
	}
	id, ok := source.PostgresIdentity()
	if !ok || id.Host == "" || id.Port == 0 || id.Database == "" || id.User == "" {
		return "", false
	}
	raw, err := json.Marshal([]string{"loom-postgres-ack-v1", id.Host, strconv.Itoa(int(id.Port)), id.Database, id.User})
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), true
}

// InitializePostgresAcknowledgement invalidates stale consent in the current
// database before serving requests. Consent in a different, offline database
// cannot be invalidated here and must not authorize an update.
func (s *Service) InitializePostgresAcknowledgement(ctx context.Context) error {
	if s.db.Engine() != storage.EnginePostgres {
		return nil
	}
	_, err := s.postgresAcknowledged(ctx)
	return err
}

func (s *Service) postgresAcknowledged(ctx context.Context) (bool, error) {
	var raw string
	err := s.db.DB().QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key = 'updates.postgresDedicatedAck'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var record postgresAckRecord
	fingerprint, ok := s.postgresIdentityFingerprint()
	if decodeStrict([]byte(raw), &record) == nil && record.Acknowledged && ok && record.Identity == fingerprint {
		return true, nil
	}
	// Do not erase an acknowledgement concurrently replaced by another admin.
	_, err = s.db.DB().ExecContext(ctx,
		`DELETE FROM schema_meta WHERE key = 'updates.postgresDedicatedAck' AND value = $1`, raw)
	return false, err
}
