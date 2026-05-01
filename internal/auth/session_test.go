package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSessionSignVerify(t *testing.T) {
	t.Parallel()
	secret := []byte("super-secret-session-key-32bytes")
	now := time.Now()
	p := SessionPayload{UID: 42, IAT: now.Unix(), EXP: now.Add(time.Hour).Unix()}
	tok, err := SignSession(secret, p)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := VerifySession(secret, tok, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.UID != 42 {
		t.Fatalf("uid: want 42 got %d", got.UID)
	}
}

func TestSessionTamper(t *testing.T) {
	t.Parallel()
	secret := []byte("super-secret-session-key-32bytes")
	now := time.Now()
	tok, _ := SignSession(secret, SessionPayload{UID: 1, IAT: now.Unix(), EXP: now.Add(time.Hour).Unix()})
	parts := strings.SplitN(tok, ".", 2)
	tampered := parts[0] + "x." + parts[1]
	if _, err := VerifySession(secret, tampered, now); err == nil {
		t.Fatal("expected error for tampered payload")
	}
	wrongSig := parts[0] + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if _, err := VerifySession(secret, wrongSig, now); err == nil {
		t.Fatal("expected error for forged sig")
	}
	if _, err := VerifySession([]byte("different-secret-of-32-bytes!!!!"), tok, now); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestSessionExpiry(t *testing.T) {
	t.Parallel()
	secret := []byte("super-secret-session-key-32bytes")
	past := time.Now().Add(-2 * time.Hour)
	tok, _ := SignSession(secret, SessionPayload{UID: 1, IAT: past.Unix(), EXP: past.Add(time.Hour).Unix()})
	if _, err := VerifySession(secret, tok, time.Now()); err == nil {
		t.Fatal("expected error for expired session")
	}
}

func TestSessionMalformed(t *testing.T) {
	t.Parallel()
	secret := []byte("super-secret-session-key-32bytes")
	for _, v := range []string{"", "no-dot", "a.b.c"} {
		if _, err := VerifySession(secret, v, time.Now()); err == nil {
			t.Errorf("expected error for %q", v)
		}
	}
}
