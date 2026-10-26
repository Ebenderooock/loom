# TV device pairing

Deploy the updated Loom backend **and web UI alongside the updated Android TV
APK**. Older servers do not implement pairing. Manual API-key setup remains
available. No TV/Home/firmware changes are needed for this feature.

On the TV, enter the server URL, select **Pair with phone / browser**, then
open the displayed `/pair?code=XXXX-XXXX` link on your own phone/computer.
Sign in, select **Review request**, check the exact code and device name against
your TV, and explicitly choose **Approve my TV** or **Deny request**. Visiting
a link never authorizes a device. Approve only requests you initiated.

Approval requires a real database-backed browser session (including OIDC
sessions), or a trusted proxy-authenticated user. API keys, unauthenticated
requests and synthetic/default identities cannot approve. Pairing is unavailable
when `auth.mode` is `disabled` or initial setup is incomplete. The TV must reach
the native endpoints without an interactive reverse-proxy login page. Configure
trusted proxy CIDRs/headers correctly; never trust arbitrary client headers.

## Protocol and security

All responses are `Cache-Control: no-store`. Requests containing secrets use
JSON POST bodies, never query strings. Do not log bodies/credentials in a proxy,
APM tool or debug logger. The browser receives only a human code, requested
device name, status and remaining lifetime; no device secret or API key.

| Endpoint under `/api/v1/auth/pairing` | Contract |
| --- | --- |
| `POST /start` | `{device_name}` → 201 `{device_secret,user_code,verification_path,expires_in:600,interval:5}` |
| `POST /poll` | `{device_secret}` → 200 `{status:"pending"}`, `{status:"denied"}`, or one-time `{status:"approved",api_key}` |
| `POST /cancel` | `{device_secret}` → 204, or 410 when already expired/consumed |
| `GET /request?code=XXXX-XXXX` | Authenticated review; no mutation |
| `POST /decision` | Authenticated `{user_code,decision:"approve"\|"deny"}` with JSON content type and `X-Loom-Pairing: 1` |

The device secret has 256 random bits, is retained only as a SHA-256 digest,
and is never part of the approval URL. Human codes have 40 random bits and
must match exactly, including case and hyphen. Each pending request has a fixed
ten-minute expiry; polls/approval do not extend it. Approval stores the real
user ID. An atomic approved poll mints a **new** `Loom TV: <device name>` API
key for that user, stores its digest through the existing API-key store, and
consumes the request. Keys can be listed/revoked using the existing
`/api/v1/auth/apikeys` APIs. The key has the approving account's normal
permissions, not a restricted TV-only scope.

Pending state is single-process, memory-only and capped at 512 entries. Server
restart invalidates every outstanding request; it does not revoke delivered
keys. Multi-replica deployments need sticky routing across start, browser review,
decision and poll (or a future shared pairing store). Expired entries are
cleaned on access, with no plaintext credential persistence or timer goroutines.

Starts are limited to 10/minute per socket peer; review/decisions share
30/minute; poll/cancel share 120/minute. Each device additionally polls no faster
than five seconds. Limiter state is capped at 2,048 buckets and expires after
one minute. `429` includes `Retry-After`. Socket peer capture occurs before
forwarded-IP middleware, preventing forged headers bypassing these limits.
A reverse proxy shares its socket-peer quota; enforce appropriate finer limits
at the trusted edge as needed.

Browser approval uses the existing SameSite session cookies, plus JSON and a
non-simple confirmation header not allowed by Loom's CORS policy, and rejects
cross-origin `Origin` headers. This also protects trusted-proxy authorization.
No GET has approval side effects.

If a delivery response is lost after minting, the credential cannot be polled
again. Start a new attempt and revoke the unused TV key. Cancel before delivery
removes the pending request; it cannot revoke a credential already delivered.
Two simultaneous polls or approve/deny decisions cannot both succeed.

## URLs and transport

The TV supports `https://host`, explicit HTTP, or bare LAN IP/host:port input.
Bare addresses normalize to HTTP **only with explicit trusted-network HTTP
consent**. HTTP exposes pairing secrets/API keys to the network; use HTTPS
outside a trusted LAN. Normal TLS checks remain enabled. The TV never follows
redirects or forwards a saved API key/cookie during pairing.

For `https://host/loom`, the verification path `/pair?...` is **base-prefix
relative**: open `https://host/loom/pair?...`. Android preserves that prefix
for all API requests. The approval route/login calls also retain the prefix.
The reverse proxy must strip the prefix when forwarding, preserve `Host`,
and serve/rewrite the web bundle's existing root-relative assets appropriately;
pairing does not add a general application-wide URL-base configuration.

See [Android TV build and acceptance checks](../android-tv/README.md).
