package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func pairingCall(h http.Handler, method, path, body, cookie string, confirm bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://loom.test/api/v1/auth/pairing/"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookie})
	}
	if confirm {
		r.Header.Set("X-Loom-Pairing", "1")
		r.Header.Set("Origin", "http://loom.test")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func pairingFixture(t *testing.T) (*auth.Service, auth.Store, int64, http.Handler, string) {
	t.Helper()
	s, store, uid := newTestService(t)
	// Complete setup using the actual handler, preserving the original user's
	// identity for assertions rather than relying on any default admin.
	r := chi.NewRouter()
	s.Mount(r)
	setup := httptest.NewRequest("POST", "/api/v1/auth/initialize", strings.NewReader(`{"username":"alice","password":"hunter2!"}`))
	r.ServeHTTP(httptest.NewRecorder(), setup)
	token, err := auth.SignSession(s.SessionSecret(), auth.SessionPayload{UID: uid, IAT: time.Now().Unix(), EXP: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return s, store, uid, r, token
}

func pairingStart(t *testing.T, h http.Handler) (string, string) {
	t.Helper()
	w := pairingCall(h, "POST", "start", `{"device_name":"Living room"}`, "", false)
	if w.Code != 201 {
		t.Fatalf("start %d %s", w.Code, w.Body.String())
	}
	var v map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	secret, code := v["device_secret"].(string), v["user_code"].(string)
	if len(secret) < 40 || len(code) != 9 || strings.Contains(v["verification_path"].(string), secret) {
		t.Fatal("unsafe start response")
	}
	return secret, code
}

func TestPairingEndToEndOneShotBoundAndRevocable(t *testing.T) {
	s, store, uid, h, cookie := pairingFixture(t)
	secret, code := pairingStart(t, h)
	body := `{"device_secret":"` + secret + `"}`
	w := pairingCall(h, "GET", "request?code="+code, "", cookie, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	decision := `{"user_code":"` + code + `","decision":"approve"}`
	if w = pairingCall(h, "POST", "decision", decision, cookie, true); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if keys, _ := store.ListAPIKeysForUser(context.Background(), uid); len(keys) != 1 {
		t.Fatal("approval must not mint a key before device polls", len(keys))
	}
	// Concurrent polls may deliver the credential to exactly one caller.
	var wg sync.WaitGroup
	var mu sync.Mutex
	keys := []string{}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := pairingCall(h, "POST", "poll", body, "", false)
			var v struct {
				Key string `json:"api_key"`
			}
			_ = json.Unmarshal(resp.Body.Bytes(), &v)
			if v.Key != "" {
				mu.Lock()
				keys = append(keys, v.Key)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(keys) != 1 {
		t.Fatalf("delivered %d keys", len(keys))
	}
	key := keys[0]
	row, err := store.GetAPIKeyByHash(context.Background(), auth.HashAPIKey(key))
	if err != nil || row.UserID != uid || row.Name != "Loom TV: Living room" || row.KeyHash == key {
		t.Fatal("key binding/digest", row, err)
	}
	if err := s.VerifyAPIKey(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeAPIKey(context.Background(), row.ID, uid); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyAPIKey(context.Background(), key); err == nil {
		t.Fatal("revoked key accepted")
	}
	if w = pairingCall(h, "POST", "poll", body, "", false); w.Code != 410 {
		t.Fatal("replay", w.Code)
	}
}

func TestPairingDecisionsRequireGenuineUserAndCSRF(t *testing.T) {
	_, _, _, h, cookie := pairingFixture(t)
	_, code := pairingStart(t, h)
	body := `{"user_code":"` + code + `","decision":"approve"}`
	for _, tc := range []struct {
		cookie string
		header bool
		want   int
	}{
		{"", true, 401}, {cookie, false, 403},
	} {
		if w := pairingCall(h, "POST", "decision", body, tc.cookie, tc.header); w.Code != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := pairingCall(h, "GET", "decision?user_code="+code, "", cookie, true); w.Code != 405 {
		t.Fatal("GET approval", w.Code)
	}
	if w := pairingCall(h, "POST", "decision", strings.Replace(body, code, strings.ToLower(code), 1), cookie, true); w.Code != 410 {
		t.Fatal("inexact code", w.Code)
	}
	r := httptest.NewRequest("POST", "http://loom.test/api/v1/auth/pairing/decision", strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookie})
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Loom-Pairing", "1")
	r.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin", w.Code)
	}
}

func TestPairingPendingDenyCancelInvalidAndRateLimit(t *testing.T) {
	_, _, _, h, cookie := pairingFixture(t)
	secret, code := pairingStart(t, h)
	body := `{"device_secret":"` + secret + `"}`
	w := pairingCall(h, "POST", "poll", body, "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "pending") {
		t.Fatal(w.Code)
	}
	if w = pairingCall(h, "POST", "poll", body, "", false); w.Code != 429 || w.Header().Get("Retry-After") != "5" {
		t.Fatal("poll rate", w.Code)
	}
	if w = pairingCall(h, "POST", "poll", `{"device_secret":"wrong"}`, "", false); w.Code != 410 {
		t.Fatal("invalid secret", w.Code)
	}
	if w = pairingCall(h, "POST", "decision", `{"user_code":"`+code+`","decision":"deny"}`, cookie, true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	secret2, code2 := pairingStart(t, h)
	if w = pairingCall(h, "POST", "decision", `{"user_code":"`+code2+`","decision":"deny"}`, cookie, true); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = pairingCall(h, "POST", "poll", `{"device_secret":"`+secret2+`"}`, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "denied") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = pairingCall(h, "POST", "cancel", body, "", false); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w = pairingCall(h, "POST", "poll", body, "", false); w.Code != 410 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 9; i++ {
		w = pairingCall(h, "POST", "start", `{"device_name":"TV"}`, "", false)
	}
	if w.Code != 429 {
		t.Fatal("start rate", w.Code)
	}
}

func TestPairingUnavailableBeforeSetup(t *testing.T) {
	s, _, _ := newTestService(t)
	h := chi.NewRouter()
	s.Mount(h)
	w := pairingCall(h, "POST", "start", `{"device_name":"TV"}`, "", false)
	if w.Code != 503 {
		t.Fatal(fmt.Sprint(w.Code, w.Body.String()))
	}
}

func TestPairingConcurrentDecisionExactlyOnce(t *testing.T) {
	_, store, uid, h, cookie := pairingFixture(t)
	_, code := pairingStart(t, h)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for _, decision := range []string{"approve", "deny"} {
		wg.Add(1)
		go func(decision string) {
			defer wg.Done()
			w := pairingCall(h, "POST", "decision", `{"user_code":"`+code+`","decision":"`+decision+`"}`, cookie, true)
			if w.Code == 200 {
				mu.Lock()
				success++
				mu.Unlock()
			} else if w.Code != 409 {
				t.Errorf("decision: %d", w.Code)
			}
		}(decision)
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("decisions accepted", success)
	}
	keys, _ := store.ListAPIKeysForUser(context.Background(), uid)
	if len(keys) != 1 {
		t.Fatal("decision minted key", len(keys))
	}
}

func TestPairingAPIKeyCannotAuthorize(t *testing.T) {
	_, store, uid, h, _ := pairingFixture(t)
	_, code := pairingStart(t, h)
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAPIKey(context.Background(), auth.CreateAPIKeyParams{UserID: uid, Name: "test", KeyHash: hash, Prefix: prefix}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "http://loom.test/api/v1/auth/pairing/decision", strings.NewReader(`{"user_code":"`+code+`","decision":"approve"}`))
	r.Header.Set("X-Api-Key", key)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Loom-Pairing", "1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPairingProxyUsesSocketPeerNotForwardedIP(t *testing.T) {
	_, _, _, routes, _ := pairingFixture(t)
	h := auth.PairingPeer(middleware.RealIP(routes))
	r := httptest.NewRequest("GET", "http://loom.test/api/v1/auth/pairing/request?code=ABCD-2345", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	r.Header.Set("X-Real-IP", "127.0.0.1")
	r.Header.Set("Remote-User", "mallory")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("untrusted proxy identity accepted", w.Code, w.Body.String())
	}

	// A genuinely trusted socket peer may authorize (unknown code, not 401).
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Real-IP", "192.0.2.10")
	r.Header.Set("Remote-User", "alice")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 410 {
		t.Fatal("trusted proxy identity refused", w.Code, w.Body.String())
	}
}
