package cloudflare

import (
	"net/http"
	"testing"
)

func TestIsChallenge_ServerHeader(t *testing.T) {
	resp := &http.Response{
		StatusCode: 403,
		Header:     http.Header{"Server": {"cloudflare"}},
	}
	if !IsChallenge(resp, nil) {
		t.Error("expected CF challenge for Server:cloudflare + 403")
	}
}

func TestIsChallenge_ServerHeader503(t *testing.T) {
	resp := &http.Response{
		StatusCode: 503,
		Header:     http.Header{"Server": {"cloudflare"}},
	}
	if !IsChallenge(resp, nil) {
		t.Error("expected CF challenge for Server:cloudflare + 503")
	}
}

func TestIsChallenge_BodyPattern(t *testing.T) {
	body := []byte(`<html><head><title>Just a moment...</title></head><body>Please wait</body></html>`)
	resp := &http.Response{
		StatusCode: 503,
		Header:     http.Header{},
	}
	if !IsChallenge(resp, body) {
		t.Error("expected CF challenge for 'Just a moment' body pattern")
	}
}

func TestIsChallenge_CFRayPlusMitigated(t *testing.T) {
	h := make(http.Header)
	h.Set("CF-RAY", "abc123")
	h.Set("cf-mitigated", "challenge")
	resp := &http.Response{
		StatusCode: 403,
		Header:     h,
	}
	if !IsChallenge(resp, nil) {
		t.Error("expected CF challenge for CF-RAY + cf-mitigated + 403")
	}
}

func TestIsChallenge_NormalResponse(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Server": {"nginx"}},
	}
	if IsChallenge(resp, []byte(`{"results": []}`)) {
		t.Error("200 from nginx should not be CF challenge")
	}
}

func TestIsChallenge_NonCF403(t *testing.T) {
	resp := &http.Response{
		StatusCode: 403,
		Header:     http.Header{"Server": {"nginx"}},
	}
	body := []byte(`<html><body>Forbidden</body></html>`)
	if IsChallenge(resp, body) {
		t.Error("normal 403 from nginx should not be CF challenge")
	}
}

func TestIsChallenge_NilResp(t *testing.T) {
	if IsChallenge(nil, nil) {
		t.Error("nil response should return false")
	}
}

func TestIsChallenge_ChallengeBodySignatures(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"jschl", `<form id="challenge-form"><input name="jschl_vc" value="x"/></form>`},
		{"cdn-cgi", `<script src="/cdn-cgi/challenge-platform/scripts/abc.js"></script>`},
		{"turnstile", `<script>window._cf_chl_opt={}</script>`},
		{"attention", `<title>Attention Required! | Cloudflare</title>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: 403, Header: http.Header{}}
			if !IsChallenge(resp, []byte(tt.body)) {
				t.Errorf("expected CF challenge for body containing %s pattern", tt.name)
			}
		})
	}
}
