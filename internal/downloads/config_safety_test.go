package downloads_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/downloads"
)

type failingConfigClient struct {
	downloads.DownloadClient
}

func (failingConfigClient) Test(context.Context) error {
	return errors.New("stored-password stored-api-key stored-cookie url-password")
}

func TestDownloadEditTestsMergeMaskedCredentialsWithoutSaving(t *testing.T) {
	t.Parallel()
	const kind downloads.Kind = "test/config-safety"
	nullFactory, err := downloads.LookupKind(downloads.KindNull)
	if err != nil {
		t.Fatal(err)
	}
	tested := make(chan downloads.Definition, 4)
	downloads.RegisterKind(kind, func(ctx context.Context, def downloads.Definition) (downloads.DownloadClient, error) {
		if def.ID == "_test_ephemeral" {
			tested <- def
		}
		c, err := nullFactory(ctx, def)
		return failingConfigClient{DownloadClient: c}, err
	})
	r, svc := newRouterForTest(t)
	config := json.RawMessage(`{"api_key":"stored-api-key","auth":{"cookie":"stored-cookie"}}`)
	if _, err := svc.Create(context.Background(), downloads.Definition{
		ID: "one", Name: "One", Kind: kind, Protocol: downloads.ProtocolTorrent, Enabled: true,
		Host:     "http://url-user:url-password@example.test?token=url-token",
		Username: "stored-user", Password: "stored-password", Config: config,
	}); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithIdentity(context.Background(), &auth.Identity{Roles: []string{"admin"}})
	for _, mask := range []string{"", "********", "[redacted]"} {
		body, _ := json.Marshal(map[string]any{
			"id": "one", "name": "Unsaved", "kind": kind, "protocol": "torrent",
			"host": "http://example.test/changed", "username": mask, "password": mask,
			"config": map[string]any{"api_key": mask, "auth": map[string]any{}},
		})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/v1/download-clients/test", strings.NewReader(string(body))).WithContext(ctx))
		if rr.Code != http.StatusOK {
			t.Fatalf("test status=%d", rr.Code)
		}
		for _, secret := range []string{"stored-password", "stored-api-key", "stored-cookie", "url-password"} {
			if strings.Contains(rr.Body.String(), secret) {
				t.Fatal("test response leaked a credential")
			}
		}
		def := <-tested
		if def.Username != "stored-user" || def.Password != "stored-password" ||
			!strings.Contains(def.Host, "url-password") || !strings.Contains(def.Host, "/changed") ||
			!strings.Contains(string(def.Config), "stored-api-key") || !strings.Contains(string(def.Config), "stored-cookie") {
			t.Fatal("test did not merge saved credentials and unsaved settings")
		}
		stored, err := svc.Get(context.Background(), "one")
		if err != nil || stored.Name != "One" || string(stored.Config) != string(config) {
			t.Fatal("test persisted unsaved settings")
		}
	}
	for _, method := range []string{http.MethodPatch, http.MethodPut} {
		body := `{"kind":"test/config-safety","name":"One","protocol":"torrent","enabled":true,"host":"http://example.test","username":"********","password":"[redacted]"}`
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(method, "/api/v1/download-clients/one", strings.NewReader(body)).WithContext(ctx))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d", method, rr.Code)
		}
		stored, err := svc.Get(context.Background(), "one")
		if err != nil || stored.Username != "stored-user" || stored.Password != "stored-password" ||
			!strings.Contains(stored.Host, "url-password") {
			t.Fatal("masked edit lost credentials")
		}
	}
}
