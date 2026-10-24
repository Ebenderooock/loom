package indexers

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProviderBudgetOutOfOrderSignals(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	migration, err := os.ReadFile("../storage/migrations/sqlite/0085_provider_budgets.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Execute only Up; migration lifecycle is covered by repository tests.
	up := strings.Split(string(migration), "-- +goose Down")[0]
	if _, err := db.Exec(up); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s := NewProviderBudgetStore(db, nil)
	if err := s.Configure(ctx, ProviderBudget{ID: "test", Allowance: 100, WindowSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO provider_budget_members VALUES ('one','test')`); err != nil {
		t.Fatal(err)
	}
	first, err := s.reserve(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.reserve(ctx, "one")
	if err != nil {
		t.Fatal(err)
	}
	reset := time.Now().Unix() + 300
	if err := s.observe(ctx, *second, 4, reset); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := s.Reserve(ctx, "one"); err != nil {
			t.Fatal(err)
		}
	}
	// A stale response reporting 5 must account for the four later
	// reservations; it must not restore the previously decremented allowance.
	if err := s.observe(ctx, *first, 5, reset); err != nil {
		t.Fatal(err)
	}
	budgets, err := s.List(ctx)
	if err != nil || *budgets[0].ProviderRemaining != 1 {
		t.Fatalf("stale response granted extra calls: %+v %v", budgets, err)
	}
	if err := s.Reserve(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, "one"); err == nil {
		t.Fatal("provider allowance exceeded")
	}
	// A late local-window response still persists an exhaustion signal.
	first.ResetAt--
	if err := s.observe(ctx, *first, 0, 0); err != nil {
		t.Fatal(err)
	}
	budgets, err = s.List(ctx)
	if err != nil || budgets[0].ProviderResetAt != 0 || *budgets[0].ProviderRemaining != 0 {
		t.Fatalf("unknown reset did not fail closed: %+v %v", budgets, err)
	}
}

func TestProviderSignalValidation(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, tc := range []struct {
		name      string
		headers   map[string]string
		status    int
		remaining int64
		reset     int64
		ok        bool
	}{
		{"absent", nil, 200, 0, 0, false},
		{"negative", map[string]string{"X-RateLimit-Remaining": "-1"}, 200, 0, 0, false},
		{"invalid", map[string]string{"X-RateLimit-Remaining": "secret text"}, 200, 0, 0, false},
		{"stale reset", map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1"}, 200, 0, 0, true},
		{"absolute", map[string]string{"X-RateLimit-Remaining": "3", "X-RateLimit-Reset": strconv.FormatInt(now.Unix()+60, 10)}, 200, 3, now.Unix() + 60, true},
		{"relative", map[string]string{"RateLimit-Remaining": "2", "RateLimit-Reset": "60"}, 200, 2, now.Unix() + 60, true},
		{"exhaustion wins", map[string]string{"X-RateLimit-Remaining": "500", "Retry-After": "60"}, 429, 0, now.Unix() + 60, true},
		{"date", map[string]string{"Retry-After": now.Add(time.Minute).UTC().Format(http.TimeFormat)}, 429, 0, now.Unix() + 60, true},
		{"invalid retry", map[string]string{"Retry-After": "-1"}, 429, 0, 0, true},
		{"unknown exhaustion", nil, 429, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status, Header: http.Header{}}
			for name, value := range tc.headers {
				resp.Header.Set(name, value)
			}
			n, reset, ok := providerSignal(resp, now)
			if n != tc.remaining || reset != tc.reset || ok != tc.ok {
				t.Fatalf("signal=(%d,%d,%v), want (%d,%d,%v)", n, reset, ok, tc.remaining, tc.reset, tc.ok)
			}
		})
	}
}
