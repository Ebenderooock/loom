package throttle

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Unix(1700000000, 0)
	got := parseRetryAfter("30", now)
	if got != 30*time.Second {
		t.Fatalf("want 30s, got %v", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	header := now.Add(15 * time.Second).Format(http.TimeFormat)
	got := parseRetryAfter(header, now)
	if got < 14*time.Second || got > 16*time.Second {
		t.Fatalf("want ~15s, got %v", got)
	}
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	header := now.Add(-30 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(header, now); got != 0 {
		t.Fatalf("past dates must return 0, got %v", got)
	}
}

func TestParseRetryAfter_Empty(t *testing.T) {
	if got := parseRetryAfter("", time.Now()); got != 0 {
		t.Fatalf("empty must be 0, got %v", got)
	}
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	if got := parseRetryAfter("not-a-number", time.Now()); got != 0 {
		t.Fatalf("invalid must be 0, got %v", got)
	}
}

func TestParseRetryAfter_NegativeSeconds(t *testing.T) {
	if got := parseRetryAfter("-5", time.Now()); got != 0 {
		t.Fatalf("negative must be 0, got %v", got)
	}
}

func TestShouldRetry_Matrix(t *testing.T) {
	cases := []struct {
		name    string
		resp    *http.Response
		err     error
		wantOK  bool
		wantRsn Reason
	}{
		{"429", &http.Response{StatusCode: 429}, nil, true, ReasonRateLimited},
		{"503", &http.Response{StatusCode: 503}, nil, true, ReasonUnavailable},
		{"200", &http.Response{StatusCode: 200}, nil, false, ""},
		{"404", &http.Response{StatusCode: 404}, nil, false, ""},
		{"500", &http.Response{StatusCode: 500}, nil, false, ""},
		{"ctx canceled", nil, context.Canceled, false, ""},
		{"ctx deadline", nil, context.DeadlineExceeded, false, ""},
		{"wrapped ctx canceled", nil, &url.Error{Op: "Get", Err: context.Canceled}, false, ""},
		{"unexpected eof", nil, io.ErrUnexpectedEOF, true, ReasonNetwork},
		{"plain err", nil, errors.New("kaboom"), true, ReasonNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRsn, gotOK := shouldRetry(tc.resp, tc.err)
			if gotOK != tc.wantOK {
				t.Fatalf("retry=%v want %v", gotOK, tc.wantOK)
			}
			if gotRsn != tc.wantRsn {
				t.Fatalf("reason=%q want %q", gotRsn, tc.wantRsn)
			}
		})
	}
}

func TestBackoff_BoundedAndJittered(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for attempt := 1; attempt <= 10; attempt++ {
		got := backoff(attempt, rng)
		if got < BaseBackoff/2 {
			t.Fatalf("attempt %d below floor: %v", attempt, got)
		}
		if got > MaxBackoff {
			t.Fatalf("attempt %d above cap: %v", attempt, got)
		}
	}
}

func TestBackoff_GrowsWithAttempt(t *testing.T) {
	// Average a few rolls so we sidestep the noise band of jitter.
	rng := rand.New(rand.NewSource(42))
	avg := func(attempt int, n int) time.Duration {
		var total time.Duration
		for i := 0; i < n; i++ {
			total += backoff(attempt, rng)
		}
		return total / time.Duration(n)
	}
	a1 := avg(1, 50)
	a3 := avg(3, 50)
	if a3 <= a1 {
		t.Fatalf("expected attempt 3 (%v) > attempt 1 (%v)", a3, a1)
	}
}
