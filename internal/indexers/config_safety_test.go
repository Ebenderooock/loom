package indexers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/indexers"
)

type failingConfigIndexer struct {
	indexers.Indexer
	err error
}

func (ix failingConfigIndexer) Test(context.Context) error { return ix.err }

func TestIndexerEditTestsMergeSecretsWithoutSavingAndRedactErrors(t *testing.T) {
	t.Parallel()
	const kind indexers.Kind = "test/config-safety"
	nullFactory, err := indexers.LookupKind(indexers.KindNull)
	if err != nil {
		t.Fatal(err)
	}
	tested := make(chan indexers.Definition, 4)
	indexers.RegisterKind(kind, func(ctx context.Context, def indexers.Definition) (indexers.Indexer, error) {
		if def.ID == "_test_ephemeral" {
			tested <- def
		}
		ix, err := nullFactory(ctx, def)
		return failingConfigIndexer{Indexer: ix, err: errors.New("stored-cookie stored-api-key url-password")}, err
	})
	svc := newServiceForHTTP(t)
	r := chi.NewRouter()
	svc.Mount(r)
	config := json.RawMessage(`{"url":"https://url-user:url-password@example.test/api?api_key=url-key&category=movies","api_key":"stored-api-key","auth":{"cookie":"stored-cookie"},"credentials":{"custom":"custom-secret"}}`)
	if _, err := svc.Create(context.Background(), indexers.Definition{
		ID: "one", Name: "One", Kind: kind, Enabled: true, Config: config,
	}); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithIdentity(context.Background(), &auth.Identity{Roles: []string{"admin"}})
	for _, incoming := range []string{
		`{"url":"https://example.test/changed?category=tv","api_key":"","auth":{},"credentials":{}}`,
		`{"url":"https://example.test/changed?category=tv","api_key":"********","auth":{"cookie":"[redacted]"}}`,
	} {
		body := `{"id":"one","kind":"test/config-safety","name":"Unsaved","config":` + incoming + `}`
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/indexers/test", strings.NewReader(body)).WithContext(ctx))
		if rr.Code != http.StatusOK {
			t.Fatalf("test status=%d", rr.Code)
		}
		for _, secret := range []string{"stored-cookie", "stored-api-key", "url-password"} {
			if strings.Contains(rr.Body.String(), secret) {
				t.Fatal("test response leaked a stored secret")
			}
		}
		def := <-tested
		for _, secret := range []string{"stored-api-key", "stored-cookie", "custom-secret", "url-password", "url-key"} {
			if !strings.Contains(string(def.Config), secret) {
				t.Fatal("test configuration lost a saved credential")
			}
		}
		if !strings.Contains(string(def.Config), "/changed") {
			t.Fatal("test ignored the unsaved URL")
		}
		stored, err := svc.Get(context.Background(), "one")
		if err != nil || string(stored.Config) != string(config) || stored.Name != "One" {
			t.Fatal("test persisted unsaved configuration")
		}
	}
	for _, path := range []string{"/api/v1/indexers/", "/api/v1/indexers/one", "/api/v1/indexers/one/test"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/test") {
			method = http.MethodPost
		}
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, nil).WithContext(ctx))
		for _, secret := range []string{"url-user", "url-password", "url-key", "stored-api-key", "stored-cookie", "custom-secret"} {
			if strings.Contains(rr.Body.String(), secret) {
				t.Fatal("configuration or health response leaked a credential")
			}
		}
	}
}

func TestAlternateIndexerConfigAndDiagnosticRoutesRequireAdmin(t *testing.T) {
	t.Parallel()
	_, db := openTestDB(t)
	svc, err := indexers.NewService(indexers.ServiceOptions{
		Repository: indexers.NewSQLiteRepository(db), Logger: quietLogger(),
		QueryLog: indexers.NewQueryLog(db),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	svc.Mount(r)
	// Nil stores are intentional: authorization must run before any side effect.
	indexers.MountRuleRoutes(r, nil, svc)
	ctx := auth.WithIdentity(context.Background(), &auth.Identity{Roles: []string{"user"}})
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/indexers/import-jackett"},
		{http.MethodPost, "/api/v1/indexers/rules/"},
		{http.MethodDelete, "/api/v1/indexers/rules/one"},
		{http.MethodGet, "/api/v1/indexers/health/"},
		{http.MethodGet, "/api/v1/indexers/health/one"},
		{http.MethodPost, "/api/v1/indexers/health/reset"},
		{http.MethodPost, "/api/v1/indexers/health/one/reset"},
		{http.MethodGet, "/api/v1/search/log/"},
		{http.MethodGet, "/api/v1/search/log/one"},
		{http.MethodDelete, "/api/v1/search/log/"},
	} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(tc.method, tc.path, nil).WithContext(ctx))
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s %s status=%d; want 403", tc.method, tc.path, rr.Code)
		}
	}
}
