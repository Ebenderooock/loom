package deluge

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Failed-login cooldown bounds.
const (
	loginBaseBackoff = 5 * time.Second
	loginMaxBackoff  = 5 * time.Minute
)

// login performs the auth.login handshake unconditionally. On success,
// Deluge sets the _session_id cookie which the http.Client jar
// persists for subsequent calls. Used by Test(), which must always
// probe the credentials; operational calls go through refreshSession.
func (c *Client) login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	return c.loginLocked(ctx)
}

// loginLocked runs auth.login. Caller must hold loginMu.
func (c *Client) loginLocked(ctx context.Context) error {
	var ok bool
	// auth.login bypasses ensureLoggedIn (see call()), so a
	// failure here is the original auth failure rather than a
	// recursive one.
	err := c.call(ctx, "auth.login", []any{c.cfg.password}, &ok)
	if err == nil && !ok {
		err = errors.New("auth.login returned false")
	}
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrAuthFailed, err)
		c.loginFailures++
		backoff := loginBaseBackoff
		for i := 1; i < c.loginFailures && backoff < loginMaxBackoff; i++ {
			backoff *= 2
		}
		c.loginRetryAt = time.Now().Add(min(backoff, loginMaxBackoff))
		c.loginErr = err
		return err
	}
	c.loginFailures = 0
	c.loginRetryAt = time.Time{}
	c.loginErr = nil
	c.sessionGen.Add(1)
	return nil
}

// refreshSession logs in on behalf of a caller that observed an
// expired or missing session at generation seen. Concurrent callers
// collapse onto one login, and recent login failures are returned
// without contacting Deluge until the cooldown expires.
func (c *Client) refreshSession(ctx context.Context, seen uint64) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.sessionGen.Load() != seen {
		return nil
	}
	if c.loginErr != nil && time.Now().Before(c.loginRetryAt) {
		return c.loginErr
	}
	return c.loginLocked(ctx)
}

func (c *Client) hasSessionCookie() bool {
	if c.http.Jar == nil {
		return false
	}
	for _, ck := range c.http.Jar.Cookies(c.cfg.baseURL) {
		if ck.Name == sessionCookieName && ck.Value != "" {
			return true
		}
	}
	return false
}

// ensureLoggedIn is a cheap pre-flight: if no session cookie has
// been issued yet, log in. The call() layer transparently refreshes
// on a session-expiry RPC error anyway, so this is purely an
// optimisation that avoids the first request always paying the
// expiry-then-retry cost.
func (c *Client) ensureLoggedIn(ctx context.Context) error {
	if c.hasSessionCookie() {
		return nil
	}
	return c.refreshSession(ctx, c.sessionGen.Load())
}

// checkSession asks Deluge whether the current session cookie is
// still valid. Used by Test() to surface session-expiry as a soft
// signal rather than waiting for the next operational call to
// re-login. Returns nil if the session is good; an error wrapping
// ErrAuthFailed when the daemon reports it is not.
func (c *Client) checkSession(ctx context.Context) error {
	var ok bool
	if err := c.call(ctx, "auth.check_session", nil, &ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: auth.check_session returned false", ErrAuthFailed)
	}
	return nil
}
