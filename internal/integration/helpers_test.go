//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func requireStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode == want {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Fatalf("unexpected status: got=%d want=%d body=%s", resp.StatusCode, want, string(body))
}

func decodeJSON[T any](t testing.TB, resp *http.Response, out *T) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func mustJSONBody(t testing.TB, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return bytes.NewReader(b)
}

func mustCreateLibrary(t testing.TB, s *TestServer, libPath string) string {
	t.Helper()
	resp := s.MustPost(t, "/api/v1/libraries", map[string]any{
		"name":               "Movies",
		"path":               libPath,
		"media_type":         "movie",
		"quality_profile_id": firstQualityProfileID(t, s),
		"monitor_on_add":     true,
	})
	requireStatus(t, resp, http.StatusCreated)
	var out struct {
		ID string `json:"id"`
	}
	decodeJSON(t, resp, &out)
	if out.ID == "" {
		t.Fatalf("library id missing in response")
	}
	return out.ID
}

func firstQualityProfileID(t testing.TB, s *TestServer) string {
	t.Helper()
	resp := s.MustGet(t, "/api/v1/quality-profiles")
	requireStatus(t, resp, http.StatusOK)
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, resp, &payload)
	if len(payload.Data) == 0 || payload.Data[0].ID == "" {
		t.Fatalf("no quality profiles found")
	}
	return payload.Data[0].ID
}
