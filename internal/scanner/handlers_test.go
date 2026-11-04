package scanner

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestStartScanRoutes(t *testing.T) {
	for _, mediaType := range []string{"movies", "series", "artists"} {
		t.Run(mediaType, func(t *testing.T) {
			r := chi.NewRouter()
			child := chi.NewRouter()
			switch mediaType {
			case "movies":
				RegisterRoutes(child, nil, nil)
			case "series":
				RegisterSeriesRoutes(child, nil, nil)
			case "artists":
				RegisterMusicRoutes(child, nil, nil)
			}
			r.Mount("/api/v1/"+mediaType, child)

			for _, suffix := range []string{"/scan", "/scan/"} {
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/"+mediaType+suffix, strings.NewReader("{}"))
				r.ServeHTTP(rec, req)
				if rec.Code != http.StatusBadRequest {
					t.Errorf("%s: status = %d, want 400; body = %s", suffix, rec.Code, rec.Body.String())
				}
			}
		})
	}
}
