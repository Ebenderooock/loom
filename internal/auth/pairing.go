package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const pairingTTL = 10 * time.Minute
const pairingInterval = 5 * time.Second
const pairingCapacity = 512

type pairingPeerKey struct{}

// PairingPeer captures the socket peer before middleware such as RealIP can
// replace RemoteAddr with client-supplied forwarding headers.
func PairingPeer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), pairingPeerKey{}, r.RemoteAddr)))
	})
}

type pairingRequest struct {
	code, name, status string
	expires, nextPoll  time.Time
	userID             int64
}

type pairingBucket struct {
	until time.Time
	count int
}

// Only hashes of device secrets are kept. Approval stores a user ID, not a
// plaintext key: the key is minted inside the atomic, one-shot poll operation.
// No goroutine is needed: every access cleans expired entries, with hard bounds.
type pairingStore struct {
	sync.Mutex
	requests map[[32]byte]*pairingRequest
	buckets  map[string]pairingBucket
	now      func() time.Time
}

func newPairingStore() *pairingStore {
	return &pairingStore{requests: make(map[[32]byte]*pairingRequest), buckets: make(map[string]pairingBucket), now: time.Now}
}

func (p *pairingStore) clean(now time.Time) {
	for k, v := range p.requests {
		if !now.Before(v.expires) {
			delete(p.requests, k)
		}
	}
	for k, v := range p.buckets {
		if !now.Before(v.until) {
			delete(p.buckets, k)
		}
	}
}

func (s *Service) pairingReady(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if s.pairingDisabled || !s.appConfig.SetupComplete {
		writeAuthError(w, 503, "pairing unavailable: enable authentication and complete server setup")
		return false
	}
	return true
}

func pairingJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeAuthError(w, 415, "application/json required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeAuthError(w, 400, "invalid json")
		return false
	}
	return true
}

// Rate limits intentionally use the socket peer, never untrusted forwarding
// headers. A reverse proxy can enforce finer per-client limits upstream.
func (p *pairingStore) limit(w http.ResponseWriter, r *http.Request, scope string, max int) bool {
	peer, _ := r.Context().Value(pairingPeerKey{}).(string)
	if peer == "" {
		peer = r.RemoteAddr
	}
	ip, _, err := net.SplitHostPort(peer)
	if err != nil {
		ip = peer
	}
	key := scope + ":" + ip
	now := p.now()
	p.clean(now)
	b := p.buckets[key]
	if b.until.IsZero() {
		if len(p.buckets) >= 2048 {
			w.Header().Set("Retry-After", "60")
			writeAuthError(w, 429, "pairing rate limit")
			return false
		}
		b.until = now.Add(time.Minute)
	}
	if b.count >= max {
		w.Header().Set("Retry-After", "60")
		writeAuthError(w, 429, "pairing rate limit")
		return false
	}
	b.count++
	p.buckets[key] = b
	return true
}

func (s *Service) handlePairingStart(w http.ResponseWriter, r *http.Request) {
	if !s.pairingReady(w) {
		return
	}
	var req struct {
		DeviceName string `json:"device_name"`
	}
	if !pairingJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.DeviceName)
	if name == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n\t") {
		writeAuthError(w, 400, "device_name required (maximum 80 bytes)")
		return
	}
	p := s.pairing
	p.Lock()
	defer p.Unlock()
	if !p.limit(w, r, "start", 10) {
		return
	}
	if len(p.requests) >= pairingCapacity {
		writeAuthError(w, 503, "pairing busy; retry later")
		return
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		writeAuthError(w, 500, "generate pairing")
		return
	}
	// Eight characters from a 32-symbol alphabet: 40 bits, distinct from the
	// 256-bit device secret. Collision checked under the same lock.
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	var code string
	for {
		var raw [8]byte
		if _, err := rand.Read(raw[:]); err != nil {
			writeAuthError(w, 500, "generate pairing")
			return
		}
		for i := range raw {
			raw[i] = alphabet[int(raw[i])%len(alphabet)]
		}
		code = string(raw[:4]) + "-" + string(raw[4:])
		found := false
		for _, v := range p.requests {
			if v.code == code {
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	deviceSecret := base64.RawURLEncoding.EncodeToString(secret[:])
	p.requests[sha256.Sum256([]byte(deviceSecret))] = &pairingRequest{
		code: code, name: name, status: "pending", expires: p.now().Add(pairingTTL),
	}
	writeJSONStatus(w, 201, map[string]any{
		"device_secret": deviceSecret, "user_code": code,
		"verification_path": "/pair?code=" + code, "expires_in": 600, "interval": 5,
	})
}

func (s *Service) handlePairingPoll(w http.ResponseWriter, r *http.Request) {
	s.pairingDevice(w, r, false)
}

func (s *Service) handlePairingCancel(w http.ResponseWriter, r *http.Request) {
	s.pairingDevice(w, r, true)
}

func (s *Service) pairingDevice(w http.ResponseWriter, r *http.Request, cancel bool) {
	if !s.pairingReady(w) {
		return
	}
	var req struct {
		Secret string `json:"device_secret"`
	}
	if !pairingJSON(w, r, &req) {
		return
	}
	p := s.pairing
	p.Lock()
	defer p.Unlock()
	if !p.limit(w, r, "device", 120) {
		return
	}
	hash := sha256.Sum256([]byte(req.Secret))
	v := p.requests[hash]
	if v == nil {
		writeAuthError(w, 410, "pairing expired or unavailable; start again")
		return
	}
	if cancel {
		delete(p.requests, hash)
		w.WriteHeader(204)
		return
	}
	now := p.now()
	if now.Before(v.nextPoll) {
		w.Header().Set("Retry-After", "5")
		writeAuthError(w, 429, "poll too fast")
		return
	}
	v.nextPoll = now.Add(pairingInterval)
	if v.status == "denied" {
		delete(p.requests, hash)
		writeJSONStatus(w, 200, map[string]string{"status": "denied"})
		return
	}
	if v.status != "approved" {
		writeJSONStatus(w, 200, map[string]string{"status": "pending"})
		return
	}
	if _, err := s.store.GetUserByID(r.Context(), v.userID); err != nil {
		delete(p.requests, hash)
		writeAuthError(w, 410, "authorizing user unavailable; start again")
		return
	}
	key, digest, prefix, err := GenerateAPIKey()
	if err != nil {
		writeAuthError(w, 500, "generate key")
		return
	}
	_, err = s.store.CreateAPIKey(r.Context(), CreateAPIKeyParams{
		UserID: v.userID, Name: "Loom TV: " + v.name, KeyHash: digest, Prefix: prefix,
	})
	if err != nil {
		writeAuthError(w, 500, "create key")
		return
	}
	delete(p.requests, hash)
	writeJSONStatus(w, 200, map[string]string{"status": "approved", "api_key": key})
}

// Resolve from the actual request, not middleware context: auth-off synthetic
// identities and API keys must never authorize a new device.
func (s *Service) pairingUser(w http.ResponseWriter, r *http.Request) *Identity {
	if !s.pairingReady(w) {
		return nil
	}
	if peer, ok := r.Context().Value(pairingPeerKey{}).(string); ok {
		r = r.Clone(r.Context())
		r.RemoteAddr = peer
	}
	id, err := s.resolveIdentity(r)
	if err != nil || id == nil {
		writeAuthError(w, 401, "sign in to authorize a device")
		return nil
	}
	if id.UserID <= 0 || (id.AuthMethod != MethodSession && id.AuthMethod != MethodProxy) {
		writeAuthError(w, 403, "browser session or trusted authenticated proxy required")
		return nil
	}
	if _, err := s.store.GetUserByID(r.Context(), id.UserID); err != nil {
		writeAuthError(w, 403, "authorizing user unavailable")
		return nil
	}
	return id
}

func (s *Service) handlePairingRequest(w http.ResponseWriter, r *http.Request) {
	if s.pairingUser(w, r) == nil {
		return
	}
	p := s.pairing
	p.Lock()
	defer p.Unlock()
	if !p.limit(w, r, "browser", 30) {
		return
	}
	for _, v := range p.requests {
		if v.code == r.URL.Query().Get("code") {
			writeJSONStatus(w, 200, map[string]any{
				"user_code": v.code, "device_name": v.name, "status": v.status,
				"expires_in": int(v.expires.Sub(p.now()).Seconds()),
			})
			return
		}
	}
	writeAuthError(w, 410, "code expired or unavailable; start again on your TV")
}

func (s *Service) handlePairingDecision(w http.ResponseWriter, r *http.Request) {
	id := s.pairingUser(w, r)
	if id == nil {
		return
	}
	// Existing sessions use SameSite cookies. This endpoint additionally
	// requires JSON + a non-simple header (not allowed by Loom CORS), and
	// rejects cross-origin browser requests, including trusted-proxy auth.
	if r.Header.Get("X-Loom-Pairing") != "1" {
		writeAuthError(w, 403, "pairing confirmation header required")
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "https" && u.Scheme != "http") {
			writeAuthError(w, 403, "cross-origin pairing confirmation refused")
			return
		}
	}
	var req struct {
		Code     string `json:"user_code"`
		Decision string `json:"decision"`
	}
	if !pairingJSON(w, r, &req) {
		return
	}
	if req.Decision != "approve" && req.Decision != "deny" {
		writeAuthError(w, 400, "decision must be approve or deny")
		return
	}
	p := s.pairing
	p.Lock()
	defer p.Unlock()
	if !p.limit(w, r, "browser", 30) {
		return
	}
	for _, v := range p.requests {
		if v.code != req.Code {
			continue
		}
		if v.status != "pending" {
			writeAuthError(w, 409, "pairing already decided")
			return
		}
		v.status = "denied"
		if req.Decision == "approve" {
			v.status = "approved"
			v.userID = id.UserID
		}
		writeJSONStatus(w, 200, map[string]string{"status": v.status})
		return
	}
	writeAuthError(w, 410, "code expired or unavailable; start again on your TV")
}
