package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const releaseURL = "https://github.com/" + repository + "/releases"

// Notice describes a verified release or a fail-closed manual-update fallback.
type Notice struct {
	CurrentVersion string    `json:"currentVersion"`
	Version        string    `json:"version,omitempty"`
	Tag            string    `json:"tag,omitempty"`
	SourceSHA      string    `json:"sourceSha,omitempty"`
	Available      bool      `json:"available"`
	Verified       bool      `json:"verified"`
	URL            string    `json:"url"`
	CheckedAt      time.Time `json:"checkedAt"`
	NextCheck      time.Time `json:"nextCheck"`
	Message        string    `json:"message,omitempty"`
	// ManualCheckAfter and Throttled are only set on explicit manual checks.
	ManualCheckAfter time.Time `json:"manualCheckAfter,omitempty"`
	Throttled        bool      `json:"throttled,omitempty"`
}

// ManualCheckCooldown bounds how often explicit checks may bypass the cache for
// one channel, protecting GitHub's unauthenticated API budget.
const ManualCheckCooldown = time.Minute

type release struct {
	Tag         string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

type cachedCheck struct {
	notice    Notice
	etag      string
	body      []byte
	candidate *Candidate
	// lastAttempt is the most recent network check; rateLimitedUntil is the
	// GitHub Retry-After/reset time from the most recent attempt.
	lastAttempt, rateLimitedUntil time.Time
}

// Candidate keeps the publisher's exact signed bytes separate from the GitHub tag.
type Candidate struct {
	Manifest       Manifest
	Tag            string
	Raw, Signature []byte
}

// Checker serializes and throttles demand-driven GitHub release checks.
type Checker struct {
	mu      sync.Mutex
	client  *http.Client
	keys    map[string]ed25519.PublicKey
	current string
	commit  string
	cache   map[string]cachedCheck
	now     func() time.Time
}

// NewChecker uses only the compiled-in publisher trust roots and GitHub URLs.
func NewChecker(current string, commit ...string) *Checker {
	currentCommit := ""
	if len(commit) == 1 {
		currentCommit = commit[0]
	}
	return &Checker{
		client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// GitHub release assets redirect to a dedicated HTTPS download host.
			if len(via) >= 5 || req.URL.Scheme != "https" ||
				(req.URL.Host != "release-assets.githubusercontent.com" && req.URL.Host != "github.com") {
				return errors.New("untrusted update redirect")
			}
			return nil
		}},
		keys: embeddedKeys(), current: current, commit: currentCommit, cache: map[string]cachedCheck{}, now: time.Now,
	}
}

// Check is intentionally demand-driven: there is no startup request or poller.
// Both successes and failures are cached for at least one hour per channel;
// concurrent requests share the same result. Use CheckNow for explicit manual
// checks that may bypass this cache.
func (c *Checker) Check(ctx context.Context, channel string) Notice {
	return c.run(ctx, channel, false)
}

// CheckNow is an explicit administrator check that bypasses the per-channel
// cache. It is still limited to one network check per channel per
// ManualCheckCooldown (shared by concurrent callers), never issues a request
// while GitHub rate limiting is in effect, and reuses ETags.
func (c *Checker) CheckNow(ctx context.Context, channel string) Notice {
	return c.run(ctx, channel, true)
}

func (c *Checker) run(ctx context.Context, channel string, force bool) Notice {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now().UTC()
	if channel != "stable" && channel != "prelease" {
		return Notice{Message: "Invalid update channel.", URL: releaseURL}
	}
	cached := c.cache[channel]
	if force && !cached.notice.CheckedAt.IsZero() {
		if now.Before(cached.rateLimitedUntil) {
			n := c.manual(cached, true)
			n.Message = strings.TrimSpace(n.Message + " GitHub rate limit reached; try again after " +
				cached.rateLimitedUntil.UTC().Format(time.RFC3339) + ".")
			return n
		}
		if now.Before(cached.lastAttempt.Add(ManualCheckCooldown)) {
			return c.manual(cached, true)
		}
	} else if !force && now.Before(cached.notice.NextCheck) {
		n := cached.notice
		n.CurrentVersion = c.current
		return n
	}
	n := Notice{CurrentVersion: c.current, URL: releaseURL, CheckedAt: now, NextCheck: now.Add(time.Hour)}
	var err error
	if len(c.keys) == 0 {
		n.Message = "Signed update notices are unavailable: the publisher verification key has not been provisioned. Check GitHub Releases manually."
	} else {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cached.lastAttempt, cached.rateLimitedUntil = now, time.Time{}
		n, cached, err = c.check(ctx, channel, n, cached)
		if err != nil {
			// Do not return upstream response bodies, redirect URLs, or signed
			// asset URLs (which may contain temporary credentials).
			n.Version, n.Tag, n.SourceSHA, n.Verified, n.Available = "", "", "", false, false
			cached.candidate = nil
			n.URL = releaseURL
			n.Message = "No verified update information is available. Check GitHub Releases manually."
		}
	}
	cached.notice = n
	c.cache[channel] = cached
	if force {
		return c.manual(cached, false)
	}
	return n
}

// manual decorates a cached notice with the earliest next manual check time.
func (c *Checker) manual(cached cachedCheck, throttled bool) Notice {
	n := cached.notice
	n.CurrentVersion = c.current
	n.Throttled = throttled
	if !cached.lastAttempt.IsZero() {
		n.ManualCheckAfter = cached.lastAttempt.Add(ManualCheckCooldown)
	}
	if cached.rateLimitedUntil.After(n.ManualCheckAfter) {
		n.ManualCheckAfter = cached.rateLimitedUntil
	}
	return n
}

func (c *Checker) check(ctx context.Context, channel string, n Notice, cached cachedCheck) (Notice, cachedCheck, error) {
	endpoint := "https://api.github.com/repos/" + repository + "/releases"
	if channel == "stable" {
		endpoint += "/latest"
	} else {
		endpoint += "?per_page=100"
	}
	body, etag, retry, err := c.get(ctx, endpoint, cached.etag, 2<<20)
	cached.noteRetry(retry, &n)
	if err != nil {
		return n, cached, err
	}
	if body == nil {
		body = cached.body
	} else {
		cached.body, cached.etag = body, etag
	}
	var selected release
	if channel == "stable" {
		err = json.Unmarshal(body, &selected)
	} else {
		var releases []release
		err = json.Unmarshal(body, &releases)
		sort.SliceStable(releases, func(i, j int) bool { return releases[i].PublishedAt.After(releases[j].PublishedAt) })
		for _, r := range releases {
			if r.Draft || !r.Prerelease || !validReleaseTag(r.Tag) {
				continue
			}
			selected = r
			break
		}
	}
	if err != nil || !validReleaseTag(selected.Tag) || selected.Draft ||
		(channel == "stable" && selected.Prerelease) ||
		(channel == "prelease" && !selected.Prerelease) {
		return n, cached, errors.New("no matching release")
	}
	assets := map[string]string{}
	for _, a := range selected.Assets {
		if a.Name != "update-manifest.json" && a.Name != "update-manifest.json.sig" {
			continue
		}
		if _, exists := assets[a.Name]; exists {
			return n, cached, errors.New("duplicate manifest asset")
		}
		expected := releaseAssetURL(selected.Tag, a.Name)
		u, parseErr := url.Parse(a.URL)
		if parseErr != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
			u.Path != "/"+repository+"/releases/download/"+selected.Tag+"/"+a.Name {
			return n, cached, errors.New("unexpected manifest URL")
		}
		assets[a.Name] = expected
	}
	raw, _, retry, err := c.get(ctx, assets["update-manifest.json"], "", 64<<10)
	cached.noteRetry(retry, &n)
	if err != nil {
		return n, cached, err
	}
	sig, _, retry, err := c.get(ctx, assets["update-manifest.json.sig"], "", 4<<10)
	cached.noteRetry(retry, &n)
	if err != nil {
		return n, cached, err
	}
	m, err := verifyManifest(raw, sig, c.keys, channel, selected.Tag)
	if err != nil {
		return n, cached, err
	}
	current, known := parseVersion(c.current)
	v, _ := parseVersion(m.Version)
	n.Version, n.Verified = m.Version, true
	n.Tag, n.SourceSHA = selected.Tag, m.SourceSHA
	n.Available = known && (compareVersions(v, current) > 0 ||
		(channel == "prelease" && compareVersions(v, current) == 0 && sourceSHA.MatchString(c.commit) && c.commit != m.SourceSHA))
	n.URL = releaseURL + "/tag/" + url.PathEscape(selected.Tag)
	if !known {
		n.Message = "Development or unrecognized build: automatic version comparison is unavailable."
	}
	cached.candidate = &Candidate{Manifest: m, Tag: selected.Tag, Raw: raw, Signature: sig}
	return n, cached, nil
}

func (cached *cachedCheck) noteRetry(retry time.Time, n *Notice) {
	if retry.After(cached.rateLimitedUntil) {
		cached.rateLimitedUntil = retry
	}
	if retry.After(n.NextCheck) {
		n.NextCheck = retry
	}
}

// Configured reports whether this build contains any publisher trust roots.
func (c *Checker) Configured() bool { return len(c.keys) > 0 }

// Discover returns independently verified publisher metadata, never caller URLs.
func (c *Checker) Discover(ctx context.Context, channel string) (Candidate, error) {
	n := c.Check(ctx, channel)
	c.mu.Lock()
	defer c.mu.Unlock()
	candidate := c.cache[channel].candidate
	if !n.Verified || candidate == nil {
		return Candidate{}, errors.New("no trusted update candidate")
	}
	copy := *candidate
	copy.Raw = append([]byte{}, candidate.Raw...)
	copy.Signature = append([]byte{}, candidate.Signature...)
	copy.Manifest.Assets = append([]Artifact{}, candidate.Manifest.Assets...)
	return copy, nil
}

// Download verifies metadata again and streams one bounded, SHA-256 checked
// archive. Callers must not use partial output when an error is returned.
func (c *Checker) Download(ctx context.Context, candidate Candidate, platform string, out io.Writer) error {
	m, err := verifyManifest(candidate.Raw, candidate.Signature, c.keys, candidate.Manifest.Channel, candidate.Tag)
	if err != nil {
		return err
	}
	var asset Artifact
	for _, a := range m.Assets {
		if a.Platform == platform {
			asset = a
		}
	}
	if asset.Name == "" {
		return errors.New("release has no archive for installed platform")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAssetURL(candidate.Tag, asset.Name), nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.New("archive download failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return errors.New("archive download rejected")
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(resp.Body, (512<<20)+1))
	if err != nil || n > 512<<20 {
		return errors.New("archive download incomplete or exceeds 512 MiB")
	}
	if hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(asset.SHA256) {
		return errors.New("archive digest mismatch")
	}
	return nil
}

func releaseAssetURL(tag, name string) string {
	return releaseURL + "/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
}

// Ahead checks the fixed repository's commit graph for same-version Prelease
// builds. A different SHA alone is not proof of an upgrade.
func (c *Checker) Ahead(ctx context.Context, oldSHA, newSHA string) bool {
	if !sourceSHA.MatchString(oldSHA) || !sourceSHA.MatchString(newSHA) {
		return false
	}
	raw, _, _, err := c.get(ctx, "https://api.github.com/repos/"+repository+"/compare/"+oldSHA+"..."+newSHA, "", 2<<20)
	var result struct {
		Status string `json:"status"`
		Ahead  int    `json:"ahead_by"`
		Behind int    `json:"behind_by"`
	}
	return err == nil && json.Unmarshal(raw, &result) == nil && result.Status == "ahead" && result.Ahead > 0 && result.Behind == 0
}

func (c *Checker) get(ctx context.Context, endpoint, etag string, limit int64) ([]byte, string, time.Time, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || (u.Host != "api.github.com" && u.Host != "github.com") || u.User != nil {
		return nil, "", time.Time{}, errors.New("invalid update URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Loom-update-checker")
	if u.Host == "api.github.com" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified && etag != "" {
		return nil, etag, time.Time{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		var retry time.Time
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			now := c.now()
			retry = now.Add(time.Hour)
			if seconds, err := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 32); err == nil && seconds > 0 {
				retry = now.Add(time.Duration(seconds) * time.Second)
			} else if date, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil {
				retry = date
			} else if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				retry = time.Unix(reset, 0)
			}
			if retry.After(now.Add(24 * time.Hour)) {
				retry = now.Add(24 * time.Hour)
			}
		}
		return nil, "", retry, fmt.Errorf("update HTTP status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, "", time.Time{}, errors.New("update response unreadable or too large")
	}
	return raw, resp.Header.Get("ETag"), time.Time{}, nil
}
