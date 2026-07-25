package devmode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/devmode"
)

func TestStartReturnsValidURLs(t *testing.T) {
	stubs, stop := devmode.Start()
	defer stop()

	if stubs == nil {
		t.Fatal("Start() returned nil stubs")
	}
	for _, u := range []string{stubs.TMDbURL, stubs.TVDbURL, stubs.MusicBrainzURL} {
		if !strings.HasPrefix(u, "http://127.0.0.1:") {
			t.Errorf("expected localhost URL, got %q", u)
		}
	}
}

func TestTMDbStub(t *testing.T) {
	stubs, stop := devmode.Start()
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	tests := []struct {
		name    string
		path    string
		wantKey string
	}{
		{"movie by id", "/movie/550", "id"},
		{"tv by id", "/tv/1396", "id"},
		{"search movie", "/search/movie?query=fight", "results"},
		{"search tv", "/search/tv?query=breaking", "results"},
		{"movie credits", "/movie/550/credits", "cast"},
		{"configuration", "/configuration", "images"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(ctx, "GET", stubs.TMDbURL+tc.path, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if _, ok := body[tc.wantKey]; !ok {
				t.Errorf("expected key %q in response, keys: %v", tc.wantKey, keys(body))
			}
		})
	}
}

func TestTVDbStub(t *testing.T) {
	stubs, stop := devmode.Start()
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	tests := []struct {
		name    string
		method  string
		path    string
		wantKey string
	}{
		{"login", "POST", "/login", "data"},
		{"series by id", "GET", "/series/81189", "data"},
		{"series episodes", "GET", "/series/81189/episodes/official", "data"},
		{"search", "GET", "/search?query=breaking+bad&type=series", "data"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(ctx, tc.method, stubs.TVDbURL+tc.path, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d for %s %s", resp.StatusCode, tc.method, tc.path)
			}
			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if _, ok := body[tc.wantKey]; !ok {
				t.Errorf("expected key %q in response, keys: %v", tc.wantKey, keys(body))
			}
		})
	}
}

func TestMusicBrainzStub(t *testing.T) {
	stubs, stop := devmode.Start()
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	ctx := context.Background()

	tests := []struct {
		name    string
		path    string
		wantKey string
	}{
		{"artist by mbid", "/artist/f27ec8db-af05-4f36-916e-3d57f91ecf5e?fmt=json", "id"},
		{"artist search", "/artist?fmt=json&query=jackson", "count"},
		{"release by mbid", "/release/1d022e01-4da6-387b-8658-8678046e4cef?fmt=json", "id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(ctx, "GET", stubs.MusicBrainzURL+tc.path, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d for %s", resp.StatusCode, tc.path)
			}
			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if _, ok := body[tc.wantKey]; !ok {
				t.Errorf("expected key %q in response, keys: %v", tc.wantKey, keys(body))
			}
		})
	}
}

func TestStopClosesServers(t *testing.T) {
	stubs, stop := devmode.Start()
	tmdbURL := stubs.TMDbURL
	stop()

	client := &http.Client{Timeout: 1 * time.Second}
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, "GET", tmdbURL+"/movie/1", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Error("expected error after stop, got nil")
	}
}

func keys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Ensure fmt is used (IDE satisfaction)
var _ = fmt.Sprintf
