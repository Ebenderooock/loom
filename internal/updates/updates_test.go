package updates

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ebenderooock/loom/internal/storage"
)

var testNow = time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)

func testManifest(tag, channel string) Manifest {
	ref := "refs/tags/v" + strings.TrimPrefix(tag, "v")
	if channel == "prelease" {
		ref = "refs/heads/prelease/" + strings.Split(strings.TrimPrefix(tag, "v"), "-")[0]
	}
	m := Manifest{
		SchemaVersion: 1, Channel: channel, Version: tag, SourceRef: ref, SourceSHA: strings.Repeat("a", 40),
	}
	for _, target := range []struct {
		platform, os, arch, arm string
	}{
		{"linux-amd64", "linux", "amd64", ""},
		{"linux-arm64", "linux", "arm64", ""},
		{"linux-armv7", "linux", "arm", "7"},
		{"darwin-amd64", "darwin", "amd64", ""},
		{"darwin-arm64", "darwin", "arm64", ""},
		{"windows-amd64", "windows", "amd64", ""},
		{"windows-arm64", "windows", "arm64", ""},
	} {
		m.Assets = append(m.Assets, Artifact{
			Name: artifactName(tag, target.os, target.arch, target.arm), Platform: target.platform,
			SHA256: strings.Repeat("a", 64),
		})
	}
	return m
}

func signingKey(t *testing.T) (map[string]ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]ed25519.PublicKey{"test": pub}, priv
}

func sign(t *testing.T, m Manifest, key ed25519.PrivateKey) ([]byte, []byte) {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)))
	return raw, sig
}

func TestManifestTrustAndBindings(t *testing.T) {
	keys, key := signingKey(t)
	m := testManifest("v1.2.3", "stable")
	raw, sig := sign(t, m, key)
	if _, err := verifyManifest(raw, sig, keys, "stable", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*Manifest)
	}{
		{"schema", func(m *Manifest) { m.SchemaVersion = 2 }},
		{"channel", func(m *Manifest) { m.Channel = "prelease" }},
		{"version", func(m *Manifest) { m.Version = "v1.2.4" }},
		{"source ref", func(m *Manifest) { m.SourceRef = "refs/heads/main" }},
		{"source SHA", func(m *Manifest) { m.SourceSHA = "bad" }},
		{"traversal", func(m *Manifest) { m.Assets[0].Name = "../loom" }},
		{"hash", func(m *Manifest) { m.Assets[0].SHA256 = "abc" }},
		{"target", func(m *Manifest) { m.Assets[0].Platform = "linux/amd64" }},
		{"duplicate target", func(m *Manifest) { m.Assets = append(m.Assets, m.Assets[0]) }},
		{"missing target", func(m *Manifest) { m.Assets = m.Assets[:len(m.Assets)-1] }},
		{"empty artifacts", func(m *Manifest) { m.Assets = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := testManifest("v1.2.3", "stable")
			tc.edit(&changed)
			raw, sig := sign(t, changed, key)
			if _, err := verifyManifest(raw, sig, keys, "stable", "v1.2.3"); err == nil {
				t.Fatal("accepted invalid signed manifest")
			}
		})
	}
	for _, tc := range []struct {
		name     string
		raw, sig []byte
		keys     map[string]ed25519.PublicKey
	}{
		{"tampered", append(append([]byte{}, raw...), ' '), sig, keys},
		{"signed invalid UTF-8", []byte{0xff}, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte{0xff}))), keys},
		{"missing key", raw, sig, nil},
		{"bad signature", raw, []byte(`{"keyId":"test","signature":"AAAA"}`), keys},
		{"trailing envelope", raw, append(append([]byte{}, sig...), []byte(`{}`)...), keys},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifyManifest(tc.raw, tc.sig, tc.keys, "stable", "v1.2.3"); err == nil {
				t.Fatal("accepted untrusted manifest")
			}
		})
	}
	pre := testManifest("v1.2.4-prelease.20261117000000.gabcdef123456", "prelease")
	raw, sig = sign(t, pre, key)
	if _, err := verifyManifest(raw, sig, keys, "prelease", "prelease-1.2.4-build-abcdef123456"); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyManifest(raw, sig, keys, "stable", "prelease-1.2.4-build-abcdef123456"); err == nil {
		t.Fatal("accepted Prelease on Stable")
	}
}

func TestVersionOrdering(t *testing.T) {
	ordered := []string{
		"v1.0.0-prelease.2", "v1.0.0-prelease.10", "v1.0.0-prelease.9999999999999999999999",
		"v1.0.0-prelease.a", "v1.0.0-prelease.a.1", "v1.0.0", "v1.0.1", "v1.1.0", "v2.0.0",
	}
	for i, a := range ordered {
		av, ok := parseVersion(a)
		if !ok {
			t.Fatalf("invalid fixture %s", a)
		}
		if compareVersions(av, av) != 0 {
			t.Fatal("unequal self")
		}
		for _, b := range ordered[i+1:] {
			bv, _ := parseVersion(b)
			if compareVersions(av, bv) >= 0 || compareVersions(bv, av) <= 0 {
				t.Fatalf("incorrect order: %s vs %s", a, b)
			}
		}
	}
	for _, s := range []string{"dev", "v01.2.3", "v1.2.3-01", "v1.2.3/evil", "v1.2.3-", "v18446744073709551616.0.0"} {
		if _, ok := parseVersion(s); ok {
			t.Fatalf("accepted %q", s)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(code int, body []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}
}

func fixtureChecker(t *testing.T, tag, channel string) (*Checker, *atomic.Int32) {
	t.Helper()
	keys, key := signingKey(t)
	raw, sig := sign(t, testManifest(tag, channel), key)
	c := NewChecker("1.0.0")
	c.keys, c.now = keys, func() time.Time { return testNow }
	calls := new(atomic.Int32)
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		switch {
		case r.URL.Host == "api.github.com":
			if r.Header.Get("If-None-Match") == `"test-etag"` {
				return response(304, nil), nil
			}
			rel := release{Tag: tag, Prerelease: channel == "prelease"}
			for _, name := range []string{"update-manifest.json", "update-manifest.json.sig"} {
				rel.Assets = append(rel.Assets, struct {
					Name string `json:"name"`
					URL  string `json:"browser_download_url"`
				}{name, releaseURL + "/download/" + tag + "/" + name})
			}
			var body []byte
			if channel == "stable" {
				body, _ = json.Marshal(rel)
			} else {
				body, _ = json.Marshal([]release{{Tag: "v9.0.0"}, rel, {Tag: "v99.0.0-beta.1", Prerelease: true, Draft: true}})
			}
			resp := response(200, body)
			resp.Header.Set("ETag", `"test-etag"`)
			return resp, nil
		case strings.HasSuffix(r.URL.Path, ".sig"):
			return response(200, sig), nil
		default:
			return response(200, raw), nil
		}
	})
	return c, calls
}

func TestCheckerCacheChannelsAndETag(t *testing.T) {
	for _, tc := range []struct{ tag, channel string }{
		{"v1.2.3", "stable"}, {"v1.2.4-prelease.2", "prelease"},
	} {
		t.Run(tc.channel, func(t *testing.T) {
			c, calls := fixtureChecker(t, tc.tag, tc.channel)
			var wg sync.WaitGroup
			for range 10 {
				wg.Go(func() {
					n := c.Check(context.Background(), tc.channel)
					if !n.Verified || !n.Available || n.Version != tc.tag {
						t.Errorf("notice = %+v", n)
					}
				})
			}
			wg.Wait()
			if calls.Load() != 3 {
				t.Fatalf("concurrent checks made %d requests", calls.Load())
			}
			c.now = func() time.Time { return testNow.Add(time.Hour) }
			if n := c.Check(context.Background(), tc.channel); !n.Verified {
				t.Fatalf("304 failed: %+v", n)
			}
			if calls.Load() != 6 {
				t.Fatalf("expected conditional refresh, calls=%d", calls.Load())
			}
		})
	}
}

func TestCheckerFailsClosedAndThrottles(t *testing.T) {
	c, calls := fixtureChecker(t, "v1.2.3", "stable")
	c.keys = nil
	if n := c.Check(context.Background(), "stable"); n.Verified || n.Available || calls.Load() != 0 {
		t.Fatalf("missing key did not fail closed: %+v", n)
	}
	c, _ = fixtureChecker(t, "v1.2.3", "stable")
	var count int
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		count++
		resp := response(429, []byte("upstream body must not leak"))
		resp.Header.Set("Retry-After", "7200")
		return resp, nil
	})
	n := c.Check(context.Background(), "stable")
	if n.Verified || n.Available || !n.NextCheck.Equal(testNow.Add(2*time.Hour)) {
		t.Fatalf("rate limit = %+v", n)
	}
	c.now = func() time.Time { return testNow.Add(time.Hour) }
	_ = c.Check(context.Background(), "stable")
	if count != 1 {
		t.Fatal("rate limit not cached")
	}
}

func TestCheckerRejectsTamper(t *testing.T) {
	c, _ := fixtureChecker(t, "v1.2.3", "stable")
	base := c.client.Transport
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, ".sig") {
			return response(200, []byte(`{"keyId":"test","signature":"AAAA"}`)), nil
		}
		return base.RoundTrip(r)
	})
	if n := c.Check(context.Background(), "stable"); n.Verified || n.Available || n.Version != "" {
		t.Fatalf("accepted tampered signature: %+v", n)
	}
}

func TestCheckerRejectsReleaseMetadataMismatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*release)
	}{
		{"draft", func(r *release) { r.Draft = true }},
		{"prerelease on stable", func(r *release) { r.Prerelease = true }},
		{"wrong tag", func(r *release) { r.Tag = "v1.2.4" }},
		{"untrusted asset URL", func(r *release) { r.Assets[0].URL = "https://evil.example/loom-update.json" }},
		{"duplicate manifest", func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) }},
		{"missing signature", func(r *release) { r.Assets = r.Assets[:1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fixtureChecker(t, "v1.2.3", "stable")
			base := c.client.Transport
			c.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				resp, err := base.RoundTrip(req)
				if err != nil || req.URL.Host != "api.github.com" {
					return resp, err
				}
				var r release
				if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
				tc.edit(&r)
				raw, _ := json.Marshal(r)
				return response(200, raw), nil
			})
			if n := c.Check(context.Background(), "stable"); n.Verified || n.Available || n.Version != "" {
				t.Fatalf("accepted mismatched release metadata: %+v", n)
			}
		})
	}
}

func TestCheckerDoesNotOfferDowngradesOrCompareDev(t *testing.T) {
	for _, current := range []string{"1.2.3", "v1.2.3", "1.2.4", "dev", "1.2.3-snapshot-local"} {
		c, _ := fixtureChecker(t, "v1.2.3", "stable")
		c.current = current
		n := c.Check(context.Background(), "stable")
		// A semver prerelease of 1.2.3 is older than its stable release.
		want := current == "1.2.3-snapshot-local"
		if !n.Verified || n.Available != want {
			t.Fatalf("current=%s notice=%+v", current, n)
		}
	}
}

func TestNetworkBounds(t *testing.T) {
	c := NewChecker("1.0.0")
	for _, endpoint := range []string{"", "http://github.com/x", "https://evil.example/x", "https://user@github.com/x"} {
		if _, _, _, err := c.get(context.Background(), endpoint, "", 10); err == nil {
			t.Fatalf("accepted %q", endpoint)
		}
	}
	c.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(200, []byte(strings.Repeat("x", 11))), nil
	})
	if _, _, _, err := c.get(context.Background(), releaseURL, "", 10); err == nil {
		t.Fatal("accepted oversized response")
	}
	for _, endpoint := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x"} {
		req := httptest.NewRequest("GET", endpoint, nil)
		if err := c.client.CheckRedirect(req, nil); err == nil {
			t.Fatalf("accepted redirect %s", endpoint)
		}
	}
}

type testDB struct {
	raw    *sql.DB
	engine storage.Engine
}

func (d testDB) DB() *sql.DB                    { return d.raw }
func (d testDB) Engine() storage.Engine         { return d.engine }
func (d testDB) Migrate(context.Context) error  { return nil }
func (d testDB) Ping(ctx context.Context) error { return d.raw.PingContext(ctx) }
func (d testDB) Close() error                   { return d.raw.Close() }

func testService(t *testing.T) *Service {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	return NewService(testDB{raw: db, engine: storage.EngineSQLite}, "1.0.0")
}

func TestSettingsDefaultsPersistenceAndFailClosed(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	defaults := Settings{Channel: "stable"}
	got, err := s.get(ctx)
	if err != nil || got != defaults {
		t.Fatalf("defaults=%+v %v", got, err)
	}
	for _, invalid := range []Settings{
		{Channel: "beta"}, {Channel: "prelease"}, {Channel: "stable", PreleaseOptIn: true},
		{Channel: "stable", AutoInstall: true},
	} {
		if err := s.set(ctx, invalid); err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
	want := Settings{Channel: "prelease", PreleaseOptIn: true}
	if err := s.set(ctx, want); err != nil {
		t.Fatal(err)
	}
	reloaded := NewService(s.db, "1.0.0")
	if got, err := reloaded.get(ctx); err != nil || got != want {
		t.Fatalf("reloaded=%+v %v", got, err)
	}
	if _, err := s.db.DB().Exec(`UPDATE schema_meta SET value = '{"channel":"stable","autoInstall":true}'`); err != nil {
		t.Fatal(err)
	}
	if got, err := s.get(ctx); err != nil || got.AutoInstall {
		t.Fatalf("DB edit enabled installation: %+v %v", got, err)
	}
	for _, engine := range []storage.Engine{storage.EngineSQLite, storage.EnginePostgres, "unknown"} {
		if InstallationUnavailable(engine) == "" {
			t.Fatal("missing installation refusal")
		}
	}
	if !strings.Contains(InstallationUnavailable(storage.EnginePostgres), "disabled") {
		t.Fatal("missing PostgreSQL fallback")
	}
}

func TestSettingsHTTPRejectsInstallAndInvalidInput(t *testing.T) {
	s := testService(t)
	router := s.Router(func(next http.Handler) http.Handler { return next })
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"channel":"stable","autoInstall":true}`, 409},
		{`{"channel":"prelease"}`, 400},
		{`{"channel":"stable","command":"sh"}`, 400},
		{`{"channel":"stable"}{}`, 400},
		{strings.Repeat("x", 4097), 400},
		{`{"channel":"prelease","preleaseOptIn":true}`, 200},
		{`{"channel":"stable"}`, 200},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("PUT", "/", strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("body=%s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/", "/check", "/apply"} {
		w := httptest.NewRecorder()
		s.Router(nil).ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code == 200 {
			t.Fatalf("unauthenticated route %s", path)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/apply", nil))
	if w.Code != 404 {
		t.Fatalf("apply endpoint unexpectedly exists: %d", w.Code)
	}
}

func TestPostgresIsNoticeOnly(t *testing.T) {
	s := testService(t)
	s.db = testDB{raw: s.db.DB(), engine: storage.EnginePostgres}
	router := s.Router(func(next http.Handler) http.Handler { return next })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "disabled") ||
		!strings.Contains(w.Body.String(), `"canAutoInstall":false`) {
		t.Fatalf("PostgreSQL status = %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("PUT", "/", strings.NewReader(`{"channel":"stable","autoInstall":true}`)))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("PostgreSQL auto-install = %d %s", w.Code, w.Body.String())
	}
	c, _ := fixtureChecker(t, "v1.2.3", "stable")
	s.checker = c
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/check", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"available":true`) ||
		!strings.Contains(w.Body.String(), `"verified":true`) {
		t.Fatalf("PostgreSQL signed notice = %d %s", w.Code, w.Body.String())
	}
}

func TestEligibleSQLiteRequiresExplicitInstallationConsent(t *testing.T) {
	s := testService(t)
	s.ConfigureInstallation(func() (bool, string) { return true, "enrolled Linux/systemd helper" })
	// A helper without a publisher key is not enough.
	s.checker.keys = nil
	if err := s.set(context.Background(), Settings{Channel: "stable", AutoInstall: true}); err == nil {
		t.Fatal("enabled without signing key")
	}
	s.checker.keys, _ = signingKey(t)
	got, err := s.get(context.Background())
	if err != nil || got.AutoInstall {
		t.Fatal("enrollment implicitly opted in")
	}
	if err := s.set(context.Background(), Settings{Channel: "stable", AutoInstall: true}); err != nil {
		t.Fatal(err)
	}
	got, err = s.get(context.Background())
	if err != nil || !got.AutoInstall {
		t.Fatal("explicit opt-in not retained", err)
	}
	s.ConfigureInstallation(func() (bool, string) { return false, "unmanaged install" })
	got, err = s.get(context.Background())
	if err != nil || got.AutoInstall {
		t.Fatal("lost eligibility did not fail closed")
	}
}

func TestUniquePreleaseTagIsNotTheVersion(t *testing.T) {
	c, _ := fixtureChecker(t, "0.4.2-prelease", "prelease")
	base := c.client.Transport
	tag := "prelease/0.4.2+build-20261117-aaaaaaaaaaaa"
	c.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(req)
		if err != nil || req.URL.Host != "api.github.com" {
			return resp, err
		}
		var releases []release
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		for i := range releases {
			if releases[i].Tag != "0.4.2-prelease" {
				continue
			}
			releases[i].Tag = tag
			for j := range releases[i].Assets {
				a := &releases[i].Assets[j]
				a.URL = releaseURL + "/download/" + tag + "/" + a.Name
			}
		}
		raw, _ := json.Marshal(releases)
		return response(200, raw), nil
	})
	n := c.Check(context.Background(), "prelease")
	if !n.Verified || n.Version != "0.4.2-prelease" || n.Tag != tag {
		t.Fatalf("tag/version contract mismatch: %+v", n)
	}
}

func TestPublisherRawSignatureAndArchiveDigest(t *testing.T) {
	keys, key := signingKey(t)
	m := testManifest("1.2.3", "stable")
	archive := []byte("signed archive bytes")
	hash := sha256.Sum256(archive)
	m.Assets[0].SHA256 = hex.EncodeToString(hash[:])
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)) + "\n")
	if _, err := verifyManifest(raw, sig, keys, "stable", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	oldSig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte("loom-update-manifest-v1\n"), raw...))))
	if _, err := verifyManifest(raw, oldSig, keys, "stable", "v1.2.3"); err == nil {
		t.Fatal("obsolete domain-prefixed signature accepted")
	}
	c := NewChecker("1.0.0")
	c.keys = keys
	c.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { return response(200, archive), nil })
	candidate := Candidate{Manifest: m, Tag: "v1.2.3", Raw: raw, Signature: sig}
	var out bytes.Buffer
	if err := c.Download(context.Background(), candidate, "linux-amd64", &out); err != nil || !bytes.Equal(out.Bytes(), archive) {
		t.Fatal("download failed", err)
	}
	archive = []byte("tampered archive")
	if err := c.Download(context.Background(), candidate, "linux-amd64", io.Discard); err == nil {
		t.Fatal("bad digest accepted")
	}
	if err := c.Download(context.Background(), candidate, "windows-amd64", io.Discard); err == nil {
		t.Fatal("missing platform accepted")
	}
}
