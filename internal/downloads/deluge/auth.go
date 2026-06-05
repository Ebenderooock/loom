package deluge

import (
	"context"
	"fmt"
)

// login performs the auth.login handshake. On success, Deluge sets
// the _session_id cookie which the http.Client jar persists for
// subsequent calls. Idempotent — repeat calls simply mint a fresh
// session id.
//
// Auth lock semantics: only one login can be in flight at a time so
// a flood of concurrent expired-session retries does not fan out.
func (c *Client) login(ctx context.Context) error {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	var ok bool
	// auth.login bypasses ensureLoggedIn (see call()), so a
	// failure here is the original auth failure rather than a
	// recursive one.
	if err := c.call(ctx, "auth.login", []any{c.cfg.password}, &ok); err != nil {
		return fmt.Errorf("%w: %w", ErrAuthFailed, err)
	}
	if !ok {
		return fmt.Errorf("%w: auth.login returned false", ErrAuthFailed)
	}
	return nil
}

// ensureLoggedIn is a cheap pre-flight: if no session cookie has
// been issued yet, log in. The call() layer transparently refreshes
// on a session-expiry RPC error anyway, so this is purely an
// optimisation that avoids the first request always paying the
// expiry-then-retry cost.
func (c *Client) ensureLoggedIn(ctx context.Context) error {
	if c.http.Jar != nil {
		for _, ck := range c.http.Jar.Cookies(c.cfg.baseURL) {
			if ck.Name == sessionCookieName && ck.Value != "" {
				return nil
			}
		}
	}
	return c.login(ctx)
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
