package auth

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/appconfig"
)

func TestPairingExpiryCleanupAndBoundedLimiter(t *testing.T) {
	p := newPairingStore()
	now := time.Now()
	hash := sha256.Sum256([]byte("secret"))
	p.requests[hash] = &pairingRequest{code: "ABCD-2345", status: "approved", expires: now}
	p.buckets["old"] = pairingBucket{until: now}
	p.clean(now)
	if len(p.requests) != 0 || len(p.buckets) != 0 {
		t.Fatal("expired pairing retained")
	}
	for i := 0; i < 2048; i++ {
		p.buckets[string(rune(i))] = pairingBucket{until: now.Add(time.Hour)}
	}
	w := httptest.NewRecorder()
	if p.limit(w, httptest.NewRequest("POST", "/", nil), "new", 10) || len(p.buckets) != 2048 {
		t.Fatal("unbounded limiter")
	}
}

func TestPairingExpiredApprovedSecretCannotMintKey(t *testing.T) {
	p := newPairingStore()
	now := time.Now()
	p.now = func() time.Time { return now }
	p.requests[sha256.Sum256([]byte("secret"))] = &pairingRequest{
		code: "ABCD-2345", status: "approved", userID: 1, expires: now,
	}
	// A nil store makes any accidental mint/lookup fail the test immediately.
	s := &Service{pairing: p, appConfig: &appconfig.Config{SetupComplete: true}}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"device_secret":"secret"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePairingPoll(w, r)
	if w.Code != 410 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPairingDisabledAndSocketPeer(t *testing.T) {
	p := newPairingStore()
	s := &Service{pairing: p, appConfig: &appconfig.Config{SetupComplete: true}, pairingDisabled: true}
	w := httptest.NewRecorder()
	s.handlePairingStart(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	// Simulate RealIP overwriting RemoteAddr after the capture middleware.
	h := PairingPeer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = "1.2.3.4:1234"
		p.limit(w, r, "test", 10)
	}))
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if _, ok := p.buckets["test:127.0.0.1"]; !ok {
		t.Fatal("socket peer lost")
	}
}
