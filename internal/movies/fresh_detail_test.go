package movies_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/movies"
)

func TestNewlyAddedMovieDetailsLoadFromPersistedRecord(t *testing.T) {
	t.Parallel()
	db := profileDB(t)
	seedAnyProfiles(t, db)
	router := movies.Router(movies.NewService(movies.NewRepository(db)))
	for i, title := range []string{
		"Mission: Impossible & Friends",
		"AC/DC: Let There Be Rock",
		"Who? Part #1",
		"50% Off",
		"Literal %25 Escape",
	} {
		t.Run(title, func(t *testing.T) {
			// Mirrors the title drawer payload, including provider fields the API ignores.
			body := fmt.Sprintf(`{"title":%q,"name":"","year":2026,"tmdb_id":"%d","imdb_id":"","overview":"","poster_path":"","release_date":"2026-01-02","genres":["Drama"],"status":"Released","metadata_provider":"tmdb","quality_profile_id":"","library_id":"movies","monitoring_status":"monitored","search":false}`, title, 9000+i)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
			if rec.Code != http.StatusCreated {
				t.Fatalf("add status=%d body=%s", rec.Code, rec.Body)
			}
			var created struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
				t.Fatalf("created id=%q err=%v", created.ID, err)
			}
			encoded := encodeURIComponent(created.ID)

			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+encoded, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("detail %s status=%d body=%s", encoded, rec.Code, rec.Body)
			}
			var detail struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil || detail.ID != created.ID {
				t.Fatalf("detail id=%q want %q err=%v", detail.ID, created.ID, err)
			}

			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/"+encoded+"/monitoring", strings.NewReader(`{"status":"unmonitored"}`)))
			if rec.Code != http.StatusOK {
				t.Fatalf("monitoring %s status=%d body=%s", encoded, rec.Code, rec.Body)
			}

			rec = httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files/"+encoded, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("files %s status=%d body=%s", encoded, rec.Code, rec.Body)
			}
		})
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing%3A-title", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing movie status=%d body=%s", rec.Code, rec.Body)
	}
}

// encodeURIComponent mirrors the browser function used by the web client.
func encodeURIComponent(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-_.!~*'()", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
