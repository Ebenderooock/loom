package proxies_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/indexers/proxies"
)

func TestProxyConfigRequiresAdminRedactsAndPreservesCredentials(t *testing.T) {
	t.Parallel()
	r, svc := newProxiesRouter(t)
	if _, err := svc.Create(context.Background(), proxies.Proxy{
		ID: "one", Kind: proxies.KindHTTP, Name: "One", Enabled: true,
		Config: json.RawMessage(`{"url":"http://url-user:url-password@example.test:8080?token=url-token","username":"stored-user","password":"stored-password"}`),
	}); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, role string) *httptest.ResponseRecorder {
		t.Helper()
		ctx := auth.WithIdentity(context.Background(), &auth.Identity{Roles: []string{role}})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx))
		return rr
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/proxies/"},
		{http.MethodPut, "/api/v1/proxies/one"},
		{http.MethodPatch, "/api/v1/proxies/one"},
		{http.MethodDelete, "/api/v1/proxies/one"},
		{http.MethodPost, "/api/v1/proxies/one/test"},
	} {
		if rr := request(tc.method, tc.path, `{}`, "user"); rr.Code != http.StatusForbidden {
			t.Errorf("%s %s status=%d; want 403", tc.method, tc.path, rr.Code)
		}
	}
	assertRedacted := func(rr *httptest.ResponseRecorder) {
		t.Helper()
		for _, secret := range []string{"url-user", "url-password", "url-token", "stored-user", "stored-password"} {
			if strings.Contains(rr.Body.String(), secret) {
				t.Fatal("proxy response leaked a credential")
			}
		}
	}
	for _, path := range []string{"/api/v1/proxies/", "/api/v1/proxies/one"} {
		rr := request(http.MethodGet, path, "", "user")
		if rr.Code != http.StatusOK {
			t.Fatalf("GET status=%d", rr.Code)
		}
		assertRedacted(rr)
		if !strings.Contains(rr.Body.String(), "example.test") {
			t.Fatal("redaction removed the ordinary proxy address")
		}
	}
	for _, method := range []string{http.MethodPatch, http.MethodPut} {
		rr := request(method, "/api/v1/proxies/one",
			`{"kind":"http","name":"Edited","config":{"url":"http://example.test:8080/changed","username":"","password":"********"}}`, "admin")
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d", method, rr.Code)
		}
		assertRedacted(rr)
		stored, err := svc.Get(context.Background(), "one")
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"url-password", "url-token", "stored-user", "stored-password"} {
			if !strings.Contains(string(stored.Config), secret) {
				t.Fatal("proxy edit lost a saved credential")
			}
		}
	}
	rr := request(http.MethodPatch, "/api/v1/proxies/one", `{"config":{"url":"http://example.test:8080/changed","password":"replacement"}}`, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("replacement status=%d", rr.Code)
	}
	stored, err := svc.Get(context.Background(), "one")
	if err != nil || !strings.Contains(string(stored.Config), "replacement") || strings.Contains(string(stored.Config), "stored-password") {
		t.Fatal("explicit replacement did not replace the stored credential")
	}
}
