package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// API key encoding scheme:
//
//	loom_<24 base62 chars>
//
// The raw secret is presented to clients exactly once. Storage holds
// SHA-256(key) in api_keys.key_hash and the first 8 base62 chars in
// api_keys.prefix to support a "last 8 chars" UI lookup without leaking
// the secret material.
const (
	apiKeyAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	apiKeyBodyLen  = 24
	apiKeyPrefix   = "loom_"
	apiKeyVisible  = 8
)

// ErrInvalidAPIKey is returned by ParseAPIKey for malformed inputs.
var ErrInvalidAPIKey = errors.New("auth: invalid api key format")

// GenerateAPIKey returns the secret string presented to the user, the
// SHA-256 hash stored in api_keys.key_hash, and the prefix stored
// plaintext for UI lookups.
func GenerateAPIKey() (key, hash, prefix string, err error) {
	body, err := randomBase62(apiKeyBodyLen)
	if err != nil {
		return "", "", "", err
	}
	key = apiKeyPrefix + body
	hash = HashAPIKey(key)
	prefix = body[:apiKeyVisible]
	return key, hash, prefix, nil
}

// HashAPIKey returns the SHA-256 hex digest of the full key as stored.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// ParseAPIKey validates the key shape and returns it normalized.
// Currently only verifies prefix + length + alphabet.
func ParseAPIKey(s string) (string, error) {
	if !strings.HasPrefix(s, apiKeyPrefix) {
		return "", ErrInvalidAPIKey
	}
	body := strings.TrimPrefix(s, apiKeyPrefix)
	if len(body) != apiKeyBodyLen {
		return "", ErrInvalidAPIKey
	}
	for _, r := range body {
		if !strings.ContainsRune(apiKeyAlphabet, r) {
			return "", ErrInvalidAPIKey
		}
	}
	return s, nil
}

func randomBase62(n int) (string, error) {
	out := make([]byte, n)
	buf := make([]byte, n*2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: random: %w", err)
	}
	for i := 0; i < n; i++ {
		// 16 bits → reduce modulo 62. The bias is negligible for our
		// use (key is high-entropy) and avoids rejection-sample loops.
		v := (uint16(buf[2*i]) << 8) | uint16(buf[2*i+1])
		out[i] = apiKeyAlphabet[int(v)%len(apiKeyAlphabet)]
	}
	return string(out), nil
}
