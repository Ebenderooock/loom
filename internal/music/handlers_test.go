package music

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAlbumLookupRejectsInvalidLimits(t *testing.T) {
	for _, limit := range []string{"invalid", "0", "-1", "101"} {
		t.Run(limit, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/lookup?q=album&limit="+limit, nil)
			response := httptest.NewRecorder()
			AlbumRouter(nil).ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", response.Code)
			}
		})
	}
}
