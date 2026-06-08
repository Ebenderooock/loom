package musicbrainz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetArtist_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify User-Agent header
		if ua := r.Header.Get("User-Agent"); ua != "Loom/1.0 (metadata_service)" {
			t.Errorf("expected User-Agent 'Loom/1.0 (metadata_service)', got %q", ua)
		}

		w.Header().Set("Content-Type", "application/json")
		response := ArtistResponse{
			ID:             "12c6fc9b-c70d-45c0-8aab-75731bde6e56",
			Name:           "The Beatles",
			Disambiguation: "British rock band",
			Area: &AreaResponse{
				ID:   "08310658-51eb-3c20-a0c3-4dac3ebf5d3c",
				Name: "United Kingdom",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond), // Faster throttle for tests
	}

	artist, err := client.GetArtist(context.Background(), "12c6fc9b-c70d-45c0-8aab-75731bde6e56")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if artist.Name != "The Beatles" {
		t.Errorf("expected name 'The Beatles', got %q", artist.Name)
	}
	if artist.MBID != "12c6fc9b-c70d-45c0-8aab-75731bde6e56" {
		t.Errorf("expected MBID, got %q", artist.MBID)
	}
	if artist.Area != "United Kingdom" {
		t.Errorf("expected area 'United Kingdom', got %q", artist.Area)
	}
}

func TestGetArtist_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Not found")
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	_, err := client.GetArtist(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	clientErr := &ClientError{}
	ok := errors.As(err, &clientErr)
	if !ok {
		t.Fatalf("expected ClientError, got %T", err)
	}

	if clientErr.Code != ErrCodeNotFound {
		t.Errorf("expected ErrCodeNotFound, got %s", clientErr.Code)
	}
	if clientErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", clientErr.StatusCode)
	}
}

func TestGetArtist_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Server error")
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	_, err := client.GetArtist(context.Background(), "some-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	clientErr := &ClientError{}
	ok := errors.As(err, &clientErr)
	if !ok {
		t.Fatalf("expected ClientError, got %T", err)
	}

	if clientErr.Code != ErrCodeServerError {
		t.Errorf("expected ErrCodeServerError, got %s", clientErr.Code)
	}
}

func TestSearchArtist_MultipleResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := SearchResponse{
			Count:  2,
			Offset: 0,
			Artists: []ArtistResponse{
				{
					ID:   "artist-1",
					Name: "The Beatles",
				},
				{
					ID:   "artist-2",
					Name: "The Rolling Stones",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	results, err := client.SearchArtist(context.Background(), "Beatles", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results[0].Name != "The Beatles" {
		t.Errorf("expected 'The Beatles', got %q", results[0].Name)
	}
	if results[1].Name != "The Rolling Stones" {
		t.Errorf("expected 'The Rolling Stones', got %q", results[1].Name)
	}
}

func TestSearchArtist_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := SearchResponse{
			Count:   0,
			Offset:  0,
			Artists: []ArtistResponse{},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	results, err := client.SearchArtist(context.Background(), "nonexistent", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestGetRelease_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := ReleaseResponse{
			ID:    "release-1",
			Title: "Abbey Road",
			Date:  "1969-09-26",
			Artists: []ArtistResponse{
				{ID: "artist-1", Name: "The Beatles"},
			},
			Media: []MediaResponse{
				{
					Position: "1",
					Tracks: []TrackResponse{
						{Title: "Come Together"},
						{Title: "Something"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	release, err := client.GetRelease(context.Background(), "release-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if release.Title != "Abbey Road" {
		t.Errorf("expected title 'Abbey Road', got %q", release.Title)
	}
	if release.Year != 1969 {
		t.Errorf("expected year 1969, got %d", release.Year)
	}
	if len(release.Tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(release.Tracks))
	}
}

func TestGetRecording_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := RecordingResponse{
			ID:     "recording-1",
			Title:  "Imagine",
			Length: 183000, // 183 seconds in ms
			Artists: []ArtistResponse{
				{ID: "artist-1", Name: "John Lennon"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	recording, err := client.GetRecording(context.Background(), "recording-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recording.Title != "Imagine" {
		t.Errorf("expected title 'Imagine', got %q", recording.Title)
	}
	if recording.Duration != 183000 {
		t.Errorf("expected duration 183000ms, got %d", recording.Duration)
	}
}

func TestRateLimitHandling(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			// First request: return 429 with Retry-After
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, "Rate limited")
			return
		}

		// Second attempt: success
		w.Header().Set("Content-Type", "application/json")
		response := ArtistResponse{
			ID:   "artist-1",
			Name: "The Beatles",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(50 * time.Millisecond),
	}

	artist, err := client.GetArtist(context.Background(), "artist-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if artist.Name != "The Beatles" {
		t.Errorf("expected 'The Beatles', got %q", artist.Name)
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests (1 failed, 1 retry), got %d", requestCount)
	}
}

func TestThrottling_EnforcesMinimumDelay(t *testing.T) {
	requestTimes := []time.Time{}
	mu := sync.Mutex{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestTimes = append(requestTimes, time.Now())
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		response := ArtistResponse{
			ID:   "artist-1",
			Name: "The Beatles",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	throttle := 100 * time.Millisecond
	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(throttle),
	}

	// Make 3 consecutive requests
	for i := 0; i < 3; i++ {
		_, _ = client.GetArtist(context.Background(), fmt.Sprintf("artist-%d", i))
	}

	// Verify minimum delay between requests. Arrival times are recorded
	// server-side, so per-request scheduling/network jitter can make a measured
	// gap dip a few hundred microseconds below the interval even though the
	// throttler slept correctly; allow a small tolerance. A broken throttler
	// would show gaps near zero, far below this bound.
	const tolerance = 5 * time.Millisecond
	for i := 1; i < len(requestTimes); i++ {
		elapsed := requestTimes[i].Sub(requestTimes[i-1])
		if elapsed < throttle-tolerance {
			t.Errorf("request %d to %d: expected at least %v delay, got %v", i-1, i, throttle, elapsed)
		}
	}
}

func TestUserAgentHeader(t *testing.T) {
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		response := ArtistResponse{
			ID:   "artist-1",
			Name: "The Beatles",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	_, _ = client.GetArtist(context.Background(), "artist-1")

	expected := "Loom/1.0 (metadata_service)"
	if userAgent != expected {
		t.Errorf("expected User-Agent %q, got %q", expected, userAgent)
	}
}

func TestConcurrentRequests_RaceSafe(t *testing.T) {
	requestCount := int64(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		response := ArtistResponse{
			ID:   "artist-1",
			Name: "The Beatles",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(10 * time.Millisecond),
	}

	// Launch 5 concurrent requests
	wg := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.GetArtist(context.Background(), "artist-1")
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&requestCount) != 5 {
		t.Errorf("expected 5 requests, got %d", requestCount)
	}
}

func TestContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		response := ArtistResponse{
			ID:   "artist-1",
			Name: "The Beatles",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 500 * time.Millisecond},
		httpClient: &http.Client{Timeout: 500 * time.Millisecond},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.GetArtist(ctx, "artist-1")
	if err == nil {
		t.Fatal("expected context timeout error")
	}

	clientErr := &ClientError{}
	ok := errors.As(err, &clientErr)
	if !ok {
		t.Fatalf("expected ClientError, got %T", err)
	}

	if clientErr.Code != ErrCodeContextError && clientErr.Code != ErrCodeNetworkError {
		t.Errorf("expected context or network error, got %s", clientErr.Code)
	}
}

func TestSearchRelease_Pagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		offset := query.Get("offset")

		w.Header().Set("Content-Type", "application/json")

		var response SearchResponse
		if offset == "0" {
			response = SearchResponse{
				Count:  1,
				Offset: 0,
				Releases: []ReleaseResponse{
					{
						ID:    "release-1",
						Title: "Abbey Road",
					},
				},
			}
		} else {
			response = SearchResponse{
				Count:    1,
				Offset:   10,
				Releases: []ReleaseResponse{},
			}
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := &Client{
		config:     &Config{BaseURL: server.URL, Timeout: 5 * time.Second},
		httpClient: &http.Client{Timeout: 5 * time.Second},
		throttler:  NewThrottler(100 * time.Millisecond),
	}

	// First page
	results1, err := client.SearchRelease(context.Background(), "Abbey Road", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results1) != 1 {
		t.Errorf("expected 1 result on first page, got %d", len(results1))
	}

	// Second page
	results2, err := client.SearchRelease(context.Background(), "Abbey Road", 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results2) != 0 {
		t.Errorf("expected 0 results on second page, got %d", len(results2))
	}
}

func TestClientErrorMessages(t *testing.T) {
	tests := []struct {
		name           string
		code           ErrorCode
		statusCode     int
		message        string
		retryAfter     int
		expectedString string
	}{
		{
			name:           "not found",
			code:           ErrCodeNotFound,
			statusCode:     404,
			message:        "entity not found",
			expectedString: "musicbrainz: not_found (HTTP 404): entity not found",
		},
		{
			name:           "rate limit with retry",
			code:           ErrCodeRateLimit,
			statusCode:     429,
			message:        "too many requests",
			retryAfter:     30,
			expectedString: "musicbrainz: rate_limit (HTTP 429, retry after 30s): too many requests",
		},
		{
			name:           "server error",
			code:           ErrCodeServerError,
			statusCode:     500,
			message:        "internal server error",
			expectedString: "musicbrainz: server_error (HTTP 500): internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &ClientError{
				Code:       tt.code,
				StatusCode: tt.statusCode,
				Message:    tt.message,
				RetryAfter: tt.retryAfter,
			}

			errStr := err.Error()
			if !strings.Contains(errStr, tt.expectedString) {
				t.Errorf("expected error to contain %q, got %q", tt.expectedString, errStr)
			}
		})
	}
}
