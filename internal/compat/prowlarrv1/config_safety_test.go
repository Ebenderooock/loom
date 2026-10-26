package prowlarrv1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/indexers/throttle"
)

type configSafetyRepo struct {
	indexers.Repository
	def indexers.Definition
}

func (r configSafetyRepo) List(context.Context) ([]indexers.Definition, error) {
	return []indexers.Definition{r.def}, nil
}
func (configSafetyRepo) ListHealth(context.Context) (map[string]indexers.Health, error) {
	return nil, nil
}
func (configSafetyRepo) GetRateLimit(context.Context, string) (throttle.Config, error) {
	return throttle.Config{}, indexers.ErrNotFound
}

func TestCompatIndexerListAndGetDoNotExposeConfigCredentials(t *testing.T) {
	t.Parallel()
	def := indexers.Definition{
		ID: "one", Kind: indexers.KindNull, Name: "One",
		Config: json.RawMessage(`{"url":"https://url-user:url-password@example.test/api?API_Key=secret-key&authorization=secret-auth&category=movies","api_key":"top-secret","credentials":{"custom":"nested-secret"}}`),
	}
	svc, err := indexers.NewService(indexers.ServiceOptions{Repository: configSafetyRepo{def: def}})
	if err != nil {
		t.Fatal(err)
	}
	r := Router(NewHandler(svc, nil, nil))
	for _, path := range []string{"/api/v1/indexer", "/api/v1/indexer/" + strconv.Itoa(intID(def.ID))} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d", path, rr.Code)
		}
		for _, secret := range []string{"url-user", "url-password", "secret-key", "secret-auth", "top-secret", "nested-secret"} {
			if strings.Contains(rr.Body.String(), secret) {
				t.Fatal("compatibility response leaked credentials")
			}
		}
		if !strings.Contains(rr.Body.String(), "example.test") || !strings.Contains(rr.Body.String(), "category=movies") {
			t.Fatal("compatibility redaction removed ordinary configuration")
		}
	}
}

func TestCompatMalformedCredentialURLsFailClosed(t *testing.T) {
	for _, raw := range []string{
		"https://user:password%zz@example.test",
		"https://example.test?api_key=secret%zz",
	} {
		if strings.Contains(redactConfigURL(raw), "secret") || strings.Contains(redactConfigURL(raw), "password") {
			t.Fatal("malformed URL bypassed credential redaction")
		}
	}
}
