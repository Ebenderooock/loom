package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/metadata"
)

func TestGetMovieReleases(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []metadata.MovieRelease
	}{
		{
			name: "official notes and dates across countries",
			body: `{"id":299534,"results":[
				{"iso_3166_1":"US","release_dates":[
					{"type":3,"release_date":"2019-04-26T00:00:00.000Z","note":""},
					{"type":3,"release_date":"2026-09-25T00:00:00.000Z","note":"Encore"},
					{"type":5,"release_date":"2026-10-01","note":"Collector's Edition"},
					{"type":3,"release_date":"2026-09-26T00:00:00Z","note":"Encore"}
				]},
				{"iso_3166_1":"GB","release_dates":[
					{"type":3,"release_date":"2026-09-25T20:00:00+12:00","note":"Encore"},
					{"type":4,"release_date":"2026-09-25T00:00:00Z","note":"  ENCORE  "},
					{"type":5,"release_date":"2026-10-01T00:00:00Z","note":"collector's   edition"},
					{"type":3,"release_date":"2026-11-01T00:00:00Z","note":"  IMAX Re-release  "}
				]}
			]}`,
			want: []metadata.MovieRelease{
				{Title: "Encore", ReleaseDate: "2026-09-25"},
				{Title: "Collector's Edition", ReleaseDate: "2026-10-01"},
				{Title: "Encore", ReleaseDate: "2026-09-26"},
				{Title: "  IMAX Re-release  ", ReleaseDate: "2026-11-01"},
			},
		},
		{
			name: "blank notes cannot authorize an edition",
			body: `{"id":299534,"results":[{"iso_3166_1":"US","release_dates":[
				{"release_date":"2026-09-25T00:00:00Z"},
				{"release_date":"2026-09-25T00:00:00Z","note":""},
				{"release_date":"2026-09-25T00:00:00Z","note":" \t\n "}
			]}]}`,
			want: []metadata.MovieRelease{},
		},
		{
			name: "invalid dates cannot authorize an edition",
			body: `{"id":299534,"results":[{"iso_3166_1":"US","release_dates":[
				{"release_date":"","note":"Encore"},
				{"release_date":"2026","note":"Encore"},
				{"release_date":"2026-02-30T00:00:00Z","note":"Encore"},
				{"release_date":"2026-13-25","note":"Encore"},
				{"release_date":"2026-09-25junk","note":"Encore"},
				{"release_date":"2026-09-25T99:00:00Z","note":"Encore"}
			]}]}`,
			want: []metadata.MovieRelease{},
		},
		{
			name: "empty response has no synthetic releases",
			body: `{"id":299534,"results":[]}`,
			want: []metadata.MovieRelease{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/movie/299534/release_dates" || r.URL.Query().Get("api_key") != "test" {
					t.Errorf("unexpected request %s", r.URL)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			client := NewClient(Config{APIKey: "test", BaseURL: server.URL})
			got, err := client.GetMovieReleases(context.Background(), 299534)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("GetMovieReleases = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestGetMovieReleasesErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, message string
		status              int
	}{
		{"not found", `{}`, "", http.StatusNotFound},
		{"unauthorized", `{}`, "", http.StatusUnauthorized},
		{"forbidden", `{}`, "", http.StatusForbidden},
		{"invalid JSON", `{`, "unmarshal", http.StatusOK},
		{"wrong identity", `{"id":1825468,"results":[]}`, "ID 1825468 does not match requested movie ID 299534", http.StatusOK},
		{"missing identity", `{"results":[]}`, "ID 0 does not match requested movie ID 299534", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()

			client := NewClient(Config{BaseURL: server.URL})
			got, err := client.GetMovieReleases(context.Background(), 299534)
			if err == nil || got != nil {
				t.Fatalf("GetMovieReleases = (%#v, %v), want nil results and error", got, err)
			}
			if tc.status != http.StatusOK {
				var clientErr *ClientError
				if !errors.As(err, &clientErr) || clientErr.StatusCode != tc.status {
					t.Fatalf("expected HTTP %d ClientError, got %v", tc.status, err)
				}
			} else if !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected error containing %q, got %v", tc.message, err)
			}
			if tc.name == "invalid JSON" {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("JSON error not preserved: %v", err)
				}
			}
		})
	}
}

func TestGetMovieReleasesContextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled context should not issue a request")
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := client.GetMovieReleases(ctx, 299534)
	if got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("GetMovieReleases = (%#v, %v), want nil and context.Canceled", got, err)
	}
}

func TestGetReleaseDatesPreservesCountryAndTypeSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":299534,"results":[
			{"iso_3166_1":"GB","release_dates":[
				{"type":3,"release_date":"2019-04-25T00:00:00.000Z","note":"Original"}
			]},
			{"iso_3166_1":"US","release_dates":[
				{"type":3,"release_date":"2019-04-26T00:00:00.000Z","note":""},
				{"type":3,"release_date":"2026-09-25T00:00:00.000Z","note":"Encore"},
				{"type":4,"release_date":"2019-07-30T00:00:00.000Z","note":""}
			]}
		]}`)
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL})
	theatrical, digital, err := client.GetReleaseDates(context.Background(), 299534)
	if err != nil || theatrical != "2019-04-26" || digital != "2019-07-30" {
		t.Fatalf("GetReleaseDates = (%q, %q, %v)", theatrical, digital, err)
	}
}
