# Authentication

> **Stub.** Authentication is being implemented in Phase 1c. This page
> will document forms login (argon2id), API keys, OIDC, and reverse-proxy
> header trust once the Phase 1c PR lands. Until then, see ADR-0004 for
> the design and `internal/auth/` for the in-progress implementation.

Planned content:

- The four auth modes: `forms`, `apikey`, `oidc`, `proxy`, `disabled`.
- Session cookies (HTTP-only, SameSite=Lax, Secure when `cookie_secure: true`).
- API-key creation, rotation, scopes, at-rest hashing (SHA-256 with a
  short cleartext prefix for UI display).
- OIDC provider setup (Authelia, Authentik, Keycloak, Entra) with the
  exact claim / group mapping.
- Reverse-proxy header trust with CIDR allow-list.
