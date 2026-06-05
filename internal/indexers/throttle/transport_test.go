package throttle

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fixedNow returns a deterministic clock for the transport tests so
// retries don't depend on wall-clock time.
func fixedNow() func() time.Time {
	t := time.Unix(1_700_000_000, 0)
	return func() time.Time { return t }
}

// fastBucketCfg keeps the rate limiter out of the way so transport
// tests don't double-test bucket math.
func fastBucketCfg() Config {
	return Config{PerMinute: 6000, Burst: 100, MaxRetries: 2}
}

func TestTransport_PassesThroughOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	rt := Wrap(http.DefaultTransport, "idx-1", "newznab", fastBucketCfg(), Options{
		Now:  fixedNow(),
		Rand: rand.New(rand.NewSource(1)),
	})
	client := &http.Client{Transport: rt}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body=%q", body)
	}
}

func TestTransport_RetriesOn429ThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := Wrap(http.DefaultTransport, "idx-2", "newznab", fastBucketCfg(), Options{
		Now:  fixedNow(),
		Rand: rand.New(rand.NewSource(1)),
	})
	client := &http.Client{Transport: rt}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits, got %d", hits.Load())
	}
}

func TestTransport_GivesUpAfterMaxRetries(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cfg := fastBucketCfg()
	cfg.MaxRetries = 2
	rt := Wrap(http.DefaultTransport, "idx-3", "newznab", cfg, Options{
		Now:  fixedNow(),
		Rand: rand.New(rand.NewSource(1)),
	})
	client := &http.Client{Transport: rt}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 final, got %d", resp.StatusCode)
	}
	// Initial + MaxRetries attempts == 3.
	if got := hits.Load(); got != 3 {
		t.Fatalf("expected 3 hits, got %d", got)
	}
}

func TestTransport_NonRetriable4xxPassesThrough(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	rt := Wrap(http.DefaultTransport, "idx-4", "newznab", fastBucketCfg(), Options{
		Now:  fixedNow(),
		Rand: rand.New(rand.NewSource(1)),
	})
	client := &http.Client{Transport: rt}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected single hit, got %d", hits.Load())
	}
}

func TestTransport_CtxCancelDuringBackoff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// No Retry-After header → exponential backoff kicks in
		// (~250ms+ on attempt 1), giving us a window to cancel.
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	rt := Wrap(http.DefaultTransport, "idx-5", "newznab", fastBucketCfg(), Options{
		Now:  fixedNow(),
		Rand: rand.New(rand.NewSource(1)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error after ctx cancel")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestTransport_BodyReplayedOnRetry(t *testing.T) {
	var hits atomic.Int32
	var bodies atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if string(body) == "payload" {
			bodies.Add(1)
		}
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rt := Wrap(http.DefaultTransport, "idx-6", "newznab", fastBucketCfg(), Options{
		Now:  fixedNow(),
		Rand: rand.New(rand.NewSource(1)),
	})
	req, _ := http.NewRequest("POST", srv.URL, strings.NewReader("payload"))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if bodies.Load() != 2 {
		t.Fatalf("expected body replayed twice, got %d", bodies.Load())
	}
}
