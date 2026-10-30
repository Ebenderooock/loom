package httpparam

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestParamDecodesBrowserEncodedSegments(t *testing.T) {
	for path, want := range map[string]string{
		"/items/mission%3A-impossible-%26-friends-1996": "mission:-impossible-&-friends-1996",
		"/items/face%2Foff-1997":                        "face/off-1997",
		"/items/50%25-off":                              "50%-off",
		"/items/who%3F-part-%231":                       "who?-part-#1",
		"/items/literal-%2525-escape":                   "literal-%25-escape",
		"/items/50%25-off%3A":                           "50%-off:",
		"/items/plain-2026":                             "plain-2026",
	} {
		var got string
		r := chi.NewRouter()
		r.Get("/items/{id}", func(_ http.ResponseWriter, r *http.Request) { got = Param(r, "id") })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if got != want {
			t.Errorf("%s: got %q want %q", path, got, want)
		}
	}
}
