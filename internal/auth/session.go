package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SessionCookieName is the cookie name used for HMAC-signed sessions.
const SessionCookieName = "loom_session"

// SessionPayload is the JSON body of a session cookie.
type SessionPayload struct {
	UID int64 `json:"uid"`
	IAT int64 `json:"iat"`
	EXP int64 `json:"exp"`
}

// ErrSessionInvalid covers tampered, malformed, or expired sessions.
var ErrSessionInvalid = errors.New("auth: invalid session")

// SignSession returns a base64url-encoded "<payloadB64>.<sigB64>" cookie
// value with the payload signed by HMAC-SHA256(secret).
func SignSession(secret []byte, p SessionPayload) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("auth: marshal session: %w", err)
	}
	bodyB64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(bodyB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return bodyB64 + "." + sig, nil
}

// VerifySession parses and validates a cookie value produced by
// SignSession. It rejects bad signatures and expired payloads.
func VerifySession(secret []byte, value string, now time.Time) (SessionPayload, error) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return SessionPayload{}, ErrSessionInvalid
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(got, expected) {
		return SessionPayload{}, ErrSessionInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return SessionPayload{}, ErrSessionInvalid
	}
	var p SessionPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return SessionPayload{}, ErrSessionInvalid
	}
	if p.EXP > 0 && now.Unix() >= p.EXP {
		return SessionPayload{}, ErrSessionInvalid
	}
	return p, nil
}

// IssueSessionCookie writes a signed session cookie for uid onto w. The
// secure flag is set when the request was forwarded over TLS or when
// cookieSecure is true in config.
func IssueSessionCookie(w http.ResponseWriter, r *http.Request, secret []byte, uid int64, ttl time.Duration, cookieSecure bool) error {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	now := time.Now()
	payload := SessionPayload{
		UID: uid,
		IAT: now.Unix(),
		EXP: now.Add(ttl).Unix(),
	}
	value, err := SignSession(secret, payload)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  now.Add(ttl),
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   cookieSecure || requestIsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// ClearSessionCookie writes an expired loom_session cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request, cookieSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookieSecure || requestIsTLS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func requestIsTLS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}
