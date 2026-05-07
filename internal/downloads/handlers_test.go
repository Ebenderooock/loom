package downloads_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/downloads"
)

func newRouterForTest(t *testing.T) (*chi.Mux, *downloads.Service) {
	t.Helper()
	svc := newServiceForTest(t)
	r := chi.NewMux()
	svc.Mount(r)
	return r, svc
}

func TestHandlersCreateAndGet(t *testing.T) {
	t.Parallel()
	r, _ := newRouterForTest(t)

	body := strings.NewReader(`{"id":"n1","name":"Null","kind":"builtin/null","protocol":"torrent","enabled":true}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("Create status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/download-clients/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("List status=%d", rr.Code)
	}
	var listResp struct {
		Clients []downloads.DefinitionWithHealth `json:"download_clients"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResp.Clients) != 1 || listResp.Clients[0].ID != "n1" {
		t.Fatalf("List unexpected: %#v", listResp.Clients)
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/download-clients/n1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("Get status=%d", rr.Code)
	}
}

func TestHandlersCreateRejectsBadRequest(t *testing.T) {
	t.Parallel()
	r, _ := newRouterForTest(t)

	cases := []string{
		`{}`,
		`{"name":"x","protocol":"torrent"}`,                       // missing kind
		`{"kind":"builtin/null","protocol":"torrent"}`,            // missing name
		`{"kind":"builtin/null","name":"x"}`,                      // missing protocol
		`{"kind":"unknown-kind","name":"x","protocol":"torrent"}`, // unknown kind
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status=%d body=%s", body, rr.Code, rr.Body.String())
		}
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode err: %v", err)
		}
		if env.Error.Code == "" {
			t.Fatalf("body %q: missing error code", body)
		}
	}
}

func TestHandlersTestEndpoint(t *testing.T) {
	t.Parallel()
	r, svc := newRouterForTest(t)
	if _, err := svc.Create(context.Background(), downloads.Definition{
		ID: "n1", Name: "Null", Kind: downloads.KindNull, Protocol: downloads.ProtocolTorrent, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/n1/test", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("Test status=%d body=%s", rr.Code, rr.Body.String())
	}
	var tr struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !tr.OK || tr.Error != "" {
		t.Fatalf("Test response: %#v", tr)
	}
}

func TestHandlersGetNotFound(t *testing.T) {
	t.Parallel()
	r, _ := newRouterForTest(t)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/download-clients/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("Get missing status=%d", rr.Code)
	}
}

func TestHandlersCategoriesAndFreeSpaceAndItems(t *testing.T) {
	t.Parallel()
	r, svc := newRouterForTest(t)
	if _, err := svc.Create(context.Background(), downloads.Definition{
		ID: "n1", Name: "Null", Kind: downloads.KindNull, Protocol: downloads.ProtocolTorrent, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, path := range []string{"/categories", "/free-space", "/items"} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/download-clients/n1"+path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}

	// Pause/resume with empty body.
	for _, op := range []string{"pause", "resume"} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/n1/"+op, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", op, rr.Code, rr.Body.String())
		}
	}

	// Add (POST /items)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/n1/items", bytes.NewReader([]byte(`{"magnet":"magnet:?xt=test"}`))))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("add status=%d body=%s", rr.Code, rr.Body.String())
	}
}
