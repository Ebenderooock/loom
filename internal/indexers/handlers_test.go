package indexers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/indexers"
)

func newServiceForHTTP(t *testing.T) *indexers.Service {
	t.Helper()
	_, raw := openTestDB(t)
	repo := indexers.NewSQLiteRepository(raw)
	svc, err := indexers.NewService(indexers.ServiceOptions{Repository: repo, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func mountTestHTTP(t *testing.T, svc *indexers.Service) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	svc.Mount(r)
	return httptest.NewServer(r)
}

func TestHTTPCreateGetListDelete(t *testing.T) {
	t.Parallel()
	svc := newServiceForHTTP(t)
	ts := mountTestHTTP(t, svc)
	defer ts.Close()

	body := bytes.NewBufferString(`{"id":"n1","kind":"builtin/null","name":"N1"}`)
	resp, err := http.Post(ts.URL+"/api/v1/indexers/", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/indexers/")
	if err != nil {
		t.Fatalf("GET list: %v", err)
	}
	var list struct {
		Indexers []indexers.DefinitionWithHealth `json:"indexers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(list.Indexers) != 1 {
		t.Fatalf("list len=%d", len(list.Indexers))
	}

	resp, err = http.Get(ts.URL + "/api/v1/indexers/n1/caps")
	if err != nil {
		t.Fatalf("GET caps: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("caps status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/indexers/n1/test", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST test: %v", err)
	}
	var tr struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatalf("decode test: %v", err)
	}
	resp.Body.Close()
	if !tr.OK {
		t.Fatal("test reported not ok for null indexer")
	}

	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/indexers/n1", nil)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestHTTPGetMissingReturns404(t *testing.T) {
	t.Parallel()
	svc := newServiceForHTTP(t)
	ts := mountTestHTTP(t, svc)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/indexers/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var env map[string]map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&env)
	resp.Body.Close()
	if env["error"]["code"] != "not_found" {
		t.Fatalf("error envelope: %#v", env)
	}
}

func TestHTTPSearchEndpoint(t *testing.T) {
	t.Parallel()
	svc := newServiceForHTTP(t)
	ts := mountTestHTTP(t, svc)
	defer ts.Close()

	// Seed a null indexer so the registry is non-empty.
	_, _ = svc.Create(context.Background(), indexers.Definition{
		ID: "ns", Kind: indexers.KindNull, Name: "S", Enabled: true,
	})

	body := bytes.NewBufferString(`{"query":"anything"}`)
	resp, err := http.Post(ts.URL+"/api/v1/indexers/search", "application/json", body)
	if err != nil {
		t.Fatalf("POST search: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var out indexers.AggregatedResults
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Errors == nil {
		t.Fatal("Errors map should be present even when empty")
	}
}
