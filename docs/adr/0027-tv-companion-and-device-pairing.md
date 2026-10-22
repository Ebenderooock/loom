# ADR-0027: Native TV companion and device pairing

- Status: Accepted
- Date: 2026-10-21
- Deciders: @loom-maintainers

## Context

Android/Google TV needs a remote-friendly home interface with media catalogs,
installed apps, settings and inputs. Entering a long API key with a D-pad is
impractical. Loom manages libraries but does not expose a streaming playback
API. Veon firmware capabilities are not yet verified.

## Decision

Ship a lightweight native Android companion, not firmware or a privileged
system application. Retain the original launcher and delegate settings and
inputs to public Android intents/frameworks with explicit failure handling.
Display real library metadata and leave playback to installed player apps.

Use expiring device authorization: the TV retains a high-entropy device
secret, while a signed-in browser user reviews a separate human code and
explicitly approves or denies. The initiating TV alone consumes a newly
issued, revocable user API key, then stores it encrypted with Android
Keystore. Retain manual key setup. Do not put credentials in browser links.

Keep pending pairing state bounded and process-local. Publish this
experimental surface through the isolated `prelease/0.2.9` branch/image
channel; stable/main consumers do not opt in automatically.

## Consequences

### Positive

- No remote-control API-key entry or copy of an existing browser credential.
- Existing API-key ownership/revocation and normal TLS validation are reused.
- No firmware flashing, package disabling, or changes to stable image tags.

### Negative / trade-offs

- Pairing needs an updated server/web UI and a real authenticated authorizer.
- Restart invalidates pending requests; multiple replicas need sticky routing.
- One-time delivery cannot be recovered after a lost response; retry pairing
  and revoke unused keys. Keys retain the approving user's normal permissions.
- OEM Home/input behavior needs physical testing; this is not a player.

### Neutral

- HTTP requires explicit trusted-network consent; HTTPS is preferred.
- Prerelease instances need separate storage and automation targets.

## Alternatives considered

- Firmware replacement: unnecessary device-specific risk and privilege.
- Browser-only launcher: cannot reliably provide native TV Home/app controls.
- Share an existing API key: exposes a reusable credential and complicates
  per-device revocation.
- Persist pending authorizations: defer shared-store/schema complexity until
  multi-replica requirements justify it.
