package updates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/storage"
)

type ackTestDB struct {
	testDB
	identity storage.PostgresIdentity
	ok       bool
}

func (d *ackTestDB) PostgresIdentity() (storage.PostgresIdentity, bool) {
	return d.identity, d.ok
}

func postgresTestService(t *testing.T) (*Service, *ackTestDB) {
	t.Helper()
	s := testService(t)
	db := &ackTestDB{
		testDB:   testDB{raw: s.db.DB(), engine: storage.EnginePostgres},
		identity: storage.PostgresIdentity{Host: "db.example", Port: 5432, Database: "loom", User: "service"},
		ok:       true,
	}
	s.db = db
	return s, db
}

func TestPostgresAcknowledgementAPI(t *testing.T) {
	s, _ := postgresTestService(t)
	router := s.Router(func(next http.Handler) http.Handler { return next })
	request := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, "/", strings.NewReader(body)))
		return w
	}
	w := request("GET", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"isPostgres":true`) ||
		!strings.Contains(w.Body.String(), `"postgresDedicatedAck":false`) {
		t.Fatalf("defaults: %d %s", w.Code, w.Body.String())
	}
	w = request("PUT", `{"channel":"stable","autoInstall":false,"postgresDedicatedAck":true}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"postgresDedicatedAck":true`) {
		t.Fatalf("acknowledge: %d %s", w.Code, w.Body.String())
	}
	w = request("GET", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"postgresDedicatedAck":true`) ||
		!strings.Contains(w.Body.String(), `"canAutoInstall":false`) ||
		strings.Contains(w.Body.String(), "db.example") || strings.Contains(w.Body.String(), "service") {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	var raw string
	if err := s.db.DB().QueryRow(`SELECT value FROM schema_meta WHERE key = 'updates.postgresDedicatedAck'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var record postgresAckRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil || !record.Acknowledged || len(record.Identity) != 64 || strings.Contains(raw, "db.example") {
		t.Fatalf("invalid stored binding: %s %v", raw, err)
	}
	if err := s.db.DB().QueryRow(`SELECT value FROM schema_meta WHERE key = 'updates.settings'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "postgresDedicatedAck") {
		t.Fatal("legacy settings record changed")
	}
	w = request("PUT", `{"channel":"stable","autoInstall":true,"postgresDedicatedAck":true}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("automatic installation enabled: %d %s", w.Code, w.Body.String())
	}
	w = request("PUT", `{"channel":"stable","postgresDedicatedAck":true,"host":"another-host"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("client-supplied identity accepted: %d %s", w.Code, w.Body.String())
	}
	w = request("PUT", `{"channel":"stable"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"postgresDedicatedAck":false`) {
		t.Fatalf("omitted acknowledgement did not clear: %d %s", w.Code, w.Body.String())
	}
	if err := s.db.DB().QueryRow(`SELECT value FROM schema_meta WHERE key = 'updates.postgresDedicatedAck'`).Scan(&raw); err != sql.ErrNoRows {
		t.Fatalf("acknowledgement retained: %v", err)
	}
}

func TestPostgresAcknowledgementInvalidatedOnIdentityChanges(t *testing.T) {
	for _, field := range []string{"host", "port", "database", "user", "unavailable"} {
		t.Run(field, func(t *testing.T) {
			s, db := postgresTestService(t)
			ctx := context.Background()
			if err := s.set(ctx, Settings{Channel: "stable", PostgresDedicatedAck: true}); err != nil {
				t.Fatal(err)
			}
			original := db.identity
			switch field {
			case "host":
				db.identity.Host = "another.example"
			case "port":
				db.identity.Port++
			case "database":
				db.identity.Database = "other"
			case "user":
				db.identity.User = "other"
			case "unavailable":
				db.ok = false
			}
			// A new server invalidates stale consent before serving any request.
			if err := NewService(db, "1.0.0").InitializePostgresAcknowledgement(ctx); err != nil {
				t.Fatal(err)
			}
			db.identity, db.ok = original, true
			got, err := s.get(ctx)
			if err != nil || got.PostgresDedicatedAck {
				t.Fatalf("acknowledgement revived: %+v %v", got, err)
			}
		})
	}
}

func TestPostgresAcknowledgementUnavailableIdentityFailsClosed(t *testing.T) {
	s, db := postgresTestService(t)
	db.ok = false
	router := s.Router(func(next http.Handler) http.Handler { return next })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("PUT", "/", strings.NewReader(`{"channel":"stable","postgresDedicatedAck":true}`)))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "identity") {
		t.Fatalf("unknown identity accepted: %d %s", w.Code, w.Body.String())
	}
	s = testService(t)
	if err := s.set(context.Background(), Settings{Channel: "stable", PostgresDedicatedAck: true}); !errors.Is(err, errPostgresIdentityUnavailable) {
		t.Fatalf("SQLite acknowledgement accepted: %v", err)
	}
}

func TestPostgresAcknowledgementPersistsAndRejectsInvalidRecord(t *testing.T) {
	s, db := postgresTestService(t)
	ctx := context.Background()
	if err := s.set(ctx, Settings{Channel: "stable", PostgresDedicatedAck: true}); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(db, "1.0.0")
	if err := restarted.InitializePostgresAcknowledgement(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := restarted.get(ctx)
	if err != nil || !got.PostgresDedicatedAck {
		t.Fatalf("acknowledgement did not persist: %+v %v", got, err)
	}
	if _, err := db.DB().Exec(`UPDATE schema_meta SET value = '{"acknowledged":true,"identity":"bad"}' WHERE key = 'updates.postgresDedicatedAck'`); err != nil {
		t.Fatal(err)
	}
	got, err = restarted.get(ctx)
	if err != nil || got.PostgresDedicatedAck {
		t.Fatalf("invalid binding accepted: %+v %v", got, err)
	}
}

func TestPostgresAcknowledgementRequiresAdmin(t *testing.T) {
	s, _ := postgresTestService(t)
	w := httptest.NewRecorder()
	s.Router(nil).ServeHTTP(w, httptest.NewRequest("PUT", "/", strings.NewReader(`{"channel":"stable","postgresDedicatedAck":true}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("unauthorized acknowledgement: %d", w.Code)
	}
	got, err := s.get(context.Background())
	if err != nil || got.PostgresDedicatedAck {
		t.Fatal("unauthorized request changed acknowledgement")
	}
}
