package downloads_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/downloads"
)

const blockedHash = "0123456789abcdef0123456789abcdef01234567"

func newBlocklistRouter(t *testing.T) *chi.Mux {
	t.Helper()
	_, raw := openTestDB(t)
	svc, err := downloads.NewService(downloads.ServiceOptions{
		Repository: downloads.NewSQLiteRepository(raw),
		Logger:     quietLogger(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	bl := downloads.NewBlocklistStore(raw)
	if err := bl.Add(context.Background(), downloads.BlocklistEntry{
		ID:          "b1",
		Title:       "Bad.Release.1080p",
		ReleaseHash: blockedHash,
		Reason:      "stalled",
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("blocklist add: %v", err)
	}
	svc.SetBlocklist(bl)
	svc.SetHistoryStore(downloads.NewHistoryStore(raw))

	r := chi.NewMux()
	svc.Mount(r)
	downloads.MountBlocklist(r, bl)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/",
		strings.NewReader(`{"id":"n1","name":"Null","kind":"builtin/null","protocol":"torrent","enabled":true}`)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create client: %d %s", rr.Code, rr.Body.String())
	}
	return r
}

func TestGrabRejectsBlocklistedRelease(t *testing.T) {
	t.Parallel()
	r := newBlocklistRouter(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"blocked by infohash", `{"title":"Other.Name","infohash":"` + strings.ToUpper(blockedHash) + `"}`, http.StatusConflict},
		{"blocked by magnet", `{"title":"Other.Name","magnet":"magnet:?xt=urn:btih:` + blockedHash + `&dn=x"}`, http.StatusConflict},
		{"blocked by title", `{"title":"bad.release.1080p","magnet":"magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff"}`, http.StatusConflict},
		{"ignore blocklist", `{"title":"Bad.Release.1080p","infohash":"` + blockedHash + `","ignore_blocklist":true}`, http.StatusAccepted},
		{"not blocked", `{"title":"Good.Release","magnet":"magnet:?xt=urn:btih:ffffffffffffffffffffffffffffffffffffffff"}`, http.StatusAccepted},
	}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/n1/items", strings.NewReader(tc.body)))
		if rr.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rr.Code, tc.want, rr.Body.String())
		}
	}
}

func TestBlocklistCheckEndpoint(t *testing.T) {
	t.Parallel()
	r := newBlocklistRouter(t)

	body := `{"releases":[
		{"title":"Bad.Release.1080p"},
		{"title":"x","magnet":"magnet:?xt=urn:btih:` + blockedHash + `"},
		{"title":"Good.Release","infohash":"ffffffffffffffffffffffffffffffffffffffff"}
	]}`
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/blocklist/check", strings.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Blocked []bool `json:"blocked"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	want := []bool{true, true, false}
	if len(resp.Blocked) != len(want) {
		t.Fatalf("blocked = %v, want %v", resp.Blocked, want)
	}
	for i := range want {
		if resp.Blocked[i] != want[i] {
			t.Errorf("blocked[%d] = %v, want %v", i, resp.Blocked[i], want[i])
		}
	}
}
