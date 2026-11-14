package connect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlexRefresh(t *testing.T) {
	for _, tc := range []struct {
		name       string
		sections   string
		listStatus int
		scanStatus int
		wantError  string
		wantScans  int
	}{
		{"all sections", `<MediaContainer><Directory key="1"/><Directory key="2"/></MediaContainer>`, 200, 200, "", 2},
		{"duplicate sections", `<MediaContainer><Directory key="1"/><Directory key="1"/></MediaContainer>`, 200, 204, "", 1},
		{"unauthorized listing", "", 401, 200, "plex sections: unexpected status 401", 0},
		{"unauthorized refresh", `<MediaContainer><Directory key="1"/></MediaContainer>`, 200, 401, "plex refresh 1: unexpected status 401", 1},
		{"failed refresh", `<MediaContainer><Directory key="1"/></MediaContainer>`, 200, 500, "plex refresh 1: unexpected status 500", 1},
		{"empty sections", `<MediaContainer/>`, 200, 200, "no library sections", 0},
		{"missing ID", `<MediaContainer><Directory key="1"/><Directory/></MediaContainer>`, 200, 200, "invalid library section ID", 0},
		{"invalid ID", `<MediaContainer><Directory key="../identity"/></MediaContainer>`, 200, 200, "invalid library section ID", 0},
		{"malformed sections", `<MediaContainer`, 200, 200, "plex sections parse", 0},
		{"malformed response containing token", `<MediaContainer><synthetic-token></MediaContainer>`, 200, 200, "plex sections parse", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var scans []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("X-Plex-Token") != "synthetic-token" {
					t.Errorf("incorrect Plex request: method=%s path=%s query=%s", r.Method, r.URL.Path, r.URL.RawQuery)
				}
				if r.URL.Path == "/prefix/library/sections" {
					if r.Header.Get("Accept") != "application/xml" {
						t.Error("sections request must ask for XML")
					}
					w.WriteHeader(tc.listStatus)
					_, _ = fmt.Fprint(w, tc.sections)
					return
				}
				if r.URL.Path != "/prefix/library/sections/1/refresh" && r.URL.Path != "/prefix/library/sections/2/refresh" {
					t.Errorf("unexpected refresh path %s", r.URL.Path)
				}
				scans = append(scans, r.URL.Path)
				w.WriteHeader(tc.scanStatus)
				_, _ = fmt.Fprint(w, "synthetic-token")
			}))
			defer server.Close()

			err := (&plexProvider{}).NotifyLibraryUpdate(context.Background(), ProviderSettings{
				Host: server.URL + "/prefix/", APIKey: "synthetic-token",
			})
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantError)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-token") {
				t.Fatalf("error exposes token: %v", err)
			}
			if len(scans) != tc.wantScans {
				t.Errorf("refreshes = %d, want %d", len(scans), tc.wantScans)
			}
		})
	}
}

func TestPlexConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings ProviderSettings
		want     string
	}{
		{"missing host", ProviderSettings{APIKey: "synthetic-token"}, "host"},
		{"missing token", ProviderSettings{Host: "http://127.0.0.1:1"}, "api_key"},
		{"invalid token", ProviderSettings{Host: "http://127.0.0.1:1", APIKey: "synthetic-token\x00"}, "api_key"},
		{"relative host", ProviderSettings{Host: "localhost:32400", APIKey: "synthetic-token"}, "host"},
		{"invalid URL", ProviderSettings{Host: "http://%synthetic-token", APIKey: "synthetic-token"}, "host"},
		{"embedded credentials", ProviderSettings{Host: "http://user:synthetic-token@localhost", APIKey: "synthetic-token"}, "host"},
		{"token query", ProviderSettings{Host: "http://localhost?X-Plex-Token=synthetic-token", APIKey: "synthetic-token"}, "host"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &plexProvider{}
			for _, call := range []func(context.Context, ProviderSettings) error{p.Test, p.NotifyLibraryUpdate} {
				err := call(context.Background(), tc.settings)
				if err == nil || !strings.Contains(err.Error(), "plex config: "+tc.want) {
					t.Fatalf("got %v, want config error for %s", err, tc.want)
				}
				if strings.Contains(err.Error(), "synthetic-token") {
					t.Fatalf("error exposes token: %v", err)
				}
			}
		})
	}
}

func TestPlexIdentity(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/identity" || r.URL.RawQuery != "" ||
					r.Header.Get("X-Plex-Token") != "synthetic-token" || r.Header.Get("Accept") != "application/json" {
					t.Error("incorrect Plex identity request")
				}
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, "synthetic-token")
			}))
			defer server.Close()
			err := (&plexProvider{}).Test(context.Background(), ProviderSettings{Host: server.URL, APIKey: "synthetic-token"})
			if status == http.StatusOK && err != nil {
				t.Fatal(err)
			}
			if status == http.StatusUnauthorized && (err == nil || !strings.Contains(err.Error(), "plex test: unexpected status 401")) {
				t.Fatalf("expected unauthorized error, got %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-token") {
				t.Fatal("identity error exposes token")
			}
		})
	}
}

func TestPlexNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	err := (&plexProvider{}).NotifyLibraryUpdate(context.Background(), ProviderSettings{
		Host: server.URL, APIKey: "synthetic-token",
	})
	if err == nil || !strings.Contains(err.Error(), "plex sections:") {
		t.Fatalf("expected network error, got %v", err)
	}
}

func TestPlexDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected = true
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()
	p := &plexProvider{}
	for _, call := range []func(context.Context, ProviderSettings) error{p.Test, p.NotifyLibraryUpdate} {
		err := call(context.Background(), ProviderSettings{Host: server.URL, APIKey: "synthetic-token"})
		if err == nil || !strings.Contains(err.Error(), "unexpected status 302") {
			t.Fatalf("expected redirect error, got %v", err)
		}
	}
	if redirected {
		t.Fatal("Plex followed a redirect, potentially forwarding the token")
	}
}
