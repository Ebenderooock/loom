# Security

This page summarises Loom's security posture today (Phase 1) and points
at the policy documents that govern handling of vulnerabilities.

## Threat model (summary)

Loom is a self-hosted service that:

- Holds **secrets** for indexers and download clients (API keys,
  cookies, basic-auth passwords).
- Stores **user credentials** for forms login (argon2id-hashed) and
  OIDC client secrets.
- Issues **API keys** that downstream apps (Overseerr, Bazarr, …) use
  to call Loom.
- Exposes an HTTP listener that, in many home deployments, is reachable
  from outside the LAN through a reverse proxy.

The primary adversaries we plan against are:

1. **Unauthenticated network attackers** reaching the HTTP listener.
2. **Authenticated low-privilege users** attempting to escalate.
3. **Supply-chain compromise** of a dependency.
4. **Credential theft** via log spillage or filesystem read.

The non-goals (out of scope) are: defeating root on the host, hardware
attacks, side-channel attacks against the JIT/GC.

## Controls in place today

| Concern | Control |
|---|---|
| TLS | Terminated at the edge (reverse proxy). Loom listens on HTTP behind it. |
| Log spillage | A redaction pass replaces sensitive attribute values with `[REDACTED]` before any record is emitted. See [observability.md](observability.md#logging). |
| Panic safety | Every HTTP handler runs under chi's `Recoverer` plus a Loom-specific recovery middleware that logs the stack and returns 500 without crashing the process. |
| Pprof exposure | `/debug/pprof/*` is **off by default** (`debug.pprof: false`). |
| Image hardening | `gcr.io/distroless/static-debian12:nonroot`; runs as UID `nonroot`; no shell, no package manager, no setuid binaries. |
| Cross-origin | CORS is off by default; `cors.allowed_origins` is an explicit allow-list. |
| Dependency hygiene | `govulncheck` runs in CI. SBOMs and image signing land in Phase 11. |

## Secrets handling

- **`auth.session_secret`** — used to sign the session cookie. Must be
  set to a random ≥32-byte value in production. Treat as a password;
  rotate by setting a new value and restarting (existing sessions
  invalidate).
- **API keys (at rest)** — stored as **SHA-256** hashes plus a
  4-character non-secret prefix used for UI display. The cleartext key
  is shown to the user exactly once at creation time.
- **OIDC client secret** — read from config; never logged.
- **Indexer / download-client credentials** — Phase 2/3; will be encrypted
  at rest with a key derived from `auth.session_secret`. Documented
  there once landed.

Secrets in YAML files should be loaded from a secret manager (Docker /
Compose secrets, Kubernetes Secrets) and mounted as files; reference
them via env var indirection (`LOOM_AUTH_SESSION_SECRET`) rather than
inlining into a checked-in config.

## Reporting a vulnerability

Please **do not** open a public issue. See [`SECURITY.md`](../SECURITY.md)
for the coordinated disclosure process and the contact address.

## Relationship to other documents

- [auth.md](auth.md) — auth modes, sessions, API keys.  _(Phase 1c stub.)_
- [ADR-0004](adr/0004-auth.md) — auth design decision.
- [ADR-0005](adr/0005-observability.md) — observability decision (covers
  the log redaction list).
- [`SECURITY.md`](../SECURITY.md) — supported versions and disclosure
  policy.
