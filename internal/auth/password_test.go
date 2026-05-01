package auth

import (
	"strings"
	"testing"
)

func TestPasswordRoundtrip(t *testing.T) {
	t.Parallel()
	hash, err := HashPassword("hunter2!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash prefix: %q", hash)
	}
	if ok, err := VerifyPassword(hash, "hunter2!"); err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword(hash, "wrong"); ok {
		t.Fatalf("verify wrong password: expected false")
	}
}

func TestPasswordMalformedHash(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"", "not-a-hash", "$argon2id$v=19$m=1,t=1,p=1$bad"} {
		if _, err := VerifyPassword(h, "x"); err == nil {
			t.Errorf("expected error for hash %q", h)
		}
	}
}

func TestPasswordHashesAreSalted(t *testing.T) {
	t.Parallel()
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("expected different salts to produce different hashes")
	}
}
