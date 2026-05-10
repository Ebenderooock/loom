// Package cloudflare provides 3-layer detection of Cloudflare
// challenge pages, matching the strategy used by Prowlarr.
//
// Layer 1 — Server header: CF sets "Server: cloudflare" on challenge responses.
// Layer 2 — Status + HTML patterns: 403/503 with CF-specific page content.
// Layer 3 — Custom header probes: cf-ray, cf-mitigated, __cf_bm cookies.
package cloudflare

import (
	"bytes"
	"net/http"
	"strings"
)

// IsChallenge inspects an HTTP response and returns true when the
// response appears to be a Cloudflare challenge (JS/browser check,
// CAPTCHA, or block page).  The caller must NOT have consumed
// resp.Body yet; this function only inspects headers and, when body
// is provided, its prefix.
//
// body may be nil for a header-only check.
func IsChallenge(resp *http.Response, body []byte) bool {
	if resp == nil {
		return false
	}
	// Layer 1: Server header.
	server := strings.ToLower(resp.Header.Get("Server"))
	isCFServer := strings.Contains(server, "cloudflare")

	// Layer 3: CF-specific headers.
	hasCFRay := resp.Header.Get("CF-RAY") != ""
	hasCFMitigated := resp.Header.Get("cf-mitigated") != ""

	// Quick win: CF server header + challenge status code.
	if isCFServer && (resp.StatusCode == 403 || resp.StatusCode == 503) {
		return true
	}

	// Layer 2: Status + body pattern matching.
	if len(body) > 0 && (resp.StatusCode == 403 || resp.StatusCode == 503) {
		if matchesChallengeBody(body) {
			return true
		}
	}

	// CF Ray + non-2xx is a strong signal even without body match.
	if hasCFRay && resp.StatusCode >= 400 {
		if hasCFMitigated {
			return true
		}
	}

	return false
}

// matchesChallengeBody checks for known Cloudflare challenge page
// fragments in the response body.
func matchesChallengeBody(body []byte) bool {
	lower := bytes.ToLower(body[:min(8192, len(body))])
	for _, sig := range challengeSignatures {
		if bytes.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// challengeSignatures are byte-lowered fragments found in various CF
// challenge page versions.  Kept as []byte to avoid repeated
// allocations.
var challengeSignatures = [][]byte{
	// JS challenge page
	[]byte("cf-browser-verification"),
	[]byte("jschl_vc"),
	[]byte("jschl-answer"),
	[]byte("cdn-cgi/challenge-platform"),
	// Turnstile CAPTCHA
	[]byte("cf_chl_opt"),
	[]byte("challenges.cloudflare.com"),
	// Generic challenge page
	[]byte("cf-challenge-running"),
	[]byte("__cf_chl_"),
	// Older "attention required" page
	[]byte("attention required! | cloudflare"),
	// "just a moment" interstitial
	[]byte("just a moment"),
}
