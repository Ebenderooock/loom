package notifications

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscordSenderRetriesAfterRateLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprint(w, `{"retry_after":0.001}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender := &DiscordSender{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := sender.Send(ctx, Notification{Title: "Alert"}, ConnectionSettings{WebhookURL: server.URL})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("expected 2 requests after rate limit, got %d", got)
	}
}

func TestDiscordSenderWaitsForRateLimitReset(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset-After", "0.05")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender := &DiscordSender{}
	settings := ConnectionSettings{WebhookURL: server.URL}
	if err := sender.Send(context.Background(), Notification{Title: "First"}, settings); err != nil {
		t.Fatalf("first Send: %v", err)
	}

	start := time.Now()
	if err := sender.Send(context.Background(), Notification{Title: "Second"}, settings); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("second request was not delayed until reset; elapsed %s", elapsed)
	}
}

func TestDiscordSenderSerializesRequestsForWebhook(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := inFlight.Add(1)
		for current > maxInFlight.Load() && !maxInFlight.CompareAndSwap(maxInFlight.Load(), current) {
		}
		time.Sleep(20 * time.Millisecond)
		inFlight.Add(-1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	settings := ConnectionSettings{WebhookURL: server.URL}
	sender := &DiscordSender{}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sender.Send(context.Background(), Notification{Title: "Alert"}, settings); err != nil {
				t.Errorf("Send: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := maxInFlight.Load(); got != 1 {
		t.Fatalf("expected at most one concurrent request per webhook, got %d", got)
	}
}
