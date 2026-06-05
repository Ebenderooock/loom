package proxies_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/proxies"
)

func newProxiesRouter(t *testing.T) (*chi.Mux, *proxies.Service) {
	t.Helper()
	_, raw := openTestDB(t)
	repo := proxies.NewSQLiteRepository(raw)
	svc, err := proxies.NewService(proxies.ServiceOptions{Repository: repo, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	r := chi.NewRouter()
	svc.Mount(r)
	return r, svc
}

func TestHandlersCRUD(t *testing.T) {
	t.Parallel()
	r, _ := newProxiesRouter(t)

	// Create.
	body := bytes.NewBufferString(`{"kind":"http","name":"P1","config":{"url":"http://x:8080"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/proxies/", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Create status %d body=%s", rec.Code, rec.Body.String())
	}
	var created proxies.Proxy
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated ID")
	}

	// List.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proxies/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("List status %d", rec.Code)
	}

	// Get.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proxies/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("Get status %d body=%s", rec.Code, rec.Body.String())
	}

	// Patch.
	patchBody := bytes.NewBufferString(`{"name":"P1-renamed"}`)
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/proxies/"+created.ID, patchBody)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Patch status %d body=%s", rec.Code, rec.Body.String())
	}

	// Delete.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/proxies/"+created.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("Delete status %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlersBadConfig(t *testing.T) {
	t.Parallel()
	r, _ := newProxiesRouter(t)
	body := bytes.NewBufferString(`{"kind":"http","name":"x","config":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/proxies/", body)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlersDeleteInUseReturns409(t *testing.T) {
	t.Parallel()
	_, raw := openTestDB(t)
	proxyRepo := proxies.NewSQLiteRepository(raw)
	idxRepo := indexers.NewSQLiteRepository(raw)
	svc, _ := proxies.NewService(proxies.ServiceOptions{Repository: proxyRepo, Logger: quietLogger()})
	r := chi.NewRouter()
	svc.Mount(r)

	ctx := context.Background()
	if _, err := proxyRepo.Create(ctx, proxies.Proxy{
		ID: "p1", Kind: proxies.KindHTTP, Name: "p1", Enabled: true,
		Config: json.RawMessage(`{"url":"http://x:8080"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := idxRepo.Create(ctx, indexers.Definition{
		ID: "i1", Kind: indexers.KindNull, Name: "i1", Enabled: true,
		Config: json.RawMessage(`{}`), ProxyID: "p1",
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/proxies/p1", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Details struct {
				IndexerIDs []string `json:"indexer_ids"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error.Code != "proxy_in_use" {
		t.Errorf("code: %s", env.Error.Code)
	}
	if len(env.Error.Details.IndexerIDs) != 1 || env.Error.Details.IndexerIDs[0] != "i1" {
		t.Errorf("indexer_ids: %v", env.Error.Details.IndexerIDs)
	}
}

func TestHandlersGetNotFound(t *testing.T) {
	t.Parallel()
	r, _ := newProxiesRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proxies/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
