# Loom TV — native companion home (experimental)

A small Android TV launcher for a Loom media hub: **Movies, Series, Apps,
Inputs and Settings**. It runs on top of Android/Google TV, not instead of it.
It does not flash firmware, remove Google services, debloat packages, install
an accessibility service, or disable the original launcher.

**Veon/Skyworth compatibility is not verified.** Android 14 was reported, but
the exact 65-inch and 32-inch models, input permissions, and Home/startup
behaviour need testing on both TVs. Android 8/API 26 is the minimum; target
SDK is 34 (Android 14), compile SDK is 35. This is a sideloadable prototype,
not a Play Store release.

## What works in this version

- Real read-only movie and series catalogs from the configured Loom server.
  Select a text tile to read its overview, year, runtime, rating and status.
  No generated catalog, pretend posters or playback-success fallbacks.
- Movie pages of 24 titles; series is fetched using the existing unpaginated
  API and displayed in 24-title pages. No catalog is persisted on disk.
- Separate loading, empty, authentication, connection and server-error states
  for each shelf, with Refresh / Retry. Previously loaded data is explicitly
  labelled as potentially out of date after a failed refresh.
- TV apps discovered through `MAIN` + `LEANBACK_LAUNCHER`, launched by their
  public activity. Non-TV apps are intentionally excluded.
- Visible passthrough inputs (HDMI/AV) from `TvInputManager`, including custom
  user labels and connected/standby/disconnected state. Direct switching uses
  `TvContract.buildChannelUriForPassthroughInput` and `ACTION_VIEW`.
- System input picker, input settings, general TV settings, and Home settings
  fallbacks. Tuner/hidden inputs are left to the system picker.
- Apps, inputs and settings remain usable without configuring a server or
  while the server is offline.

**Not a video player or streaming server.** Loom automates media libraries.
The catalog endpoints do not expose playable URLs. Even an “available” title
does not imply it is playable from this TV. Open your separately configured
Jellyfin/Plex/VLC/etc. app from Apps; this launcher does not deep-link a
selected title or send a server API key to a player.

The interface uses native controls, density-independent layout, 32dp safe
gutters, warm white type on ink/navy surfaces, and a **3dp blue focus border**.
Horizontal shelves use D-pad left/right; vertical scrolling keeps all TV
controls reachable. Text details wrap and scroll; Back closes dialogs.
The cross mark follows the existing `web/public/loom.svg` Loom identity.
No remote fonts, remote images, decorative video or background animation.
This first version is English-only. Android lint currently retains nonblocking
warnings for untranslated strings, the deliberate Android-14 target,
TV-only landscape orientation and the TV banner's 320×180 vector size.

## Build

Prerequisites: **JDK 17**, Android SDK command-line tools, Android platform 35,
build-tools 35.0.0, `curl`, `unzip`, and `sha256sum` or `shasum`. Install those
explicitly using Android Studio or the official tools; this project never
installs host prerequisites automatically.

```sh
export JAVA_HOME=/path/to/jdk-17
export ANDROID_HOME=/path/to/android-sdk
"$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" \
  "platforms;android-35" "build-tools;35.0.0"
cd android-tv
sh ./gradlew --no-daemon :app:testDebugUnitTest :app:lintDebug :app:assembleDebug
```

`gradlew` here is a small POSIX bootstrap rather than the generated Gradle
wrapper JAR. It downloads **Gradle 8.10.2**, verifies its published SHA-256,
then runs it from `$GRADLE_USER_HOME/loom-bootstrap` (default `~/.gradle`).
AGP is pinned to 8.7.3. Runtime dependencies: Android SDK only.
JVM test dependencies: JUnit 4.13.2 and JSON-java 20240303.
First builds require internet access to Google's and Maven's repositories.
On Windows use WSL, or installed Gradle 8.10.2 with the same JDK and SDK.

Output: `app/build/outputs/apk/debug/app-debug.apk`. The debug key is for
testing, not distribution. Do not commit a signing key. Any later release
needs its own privately managed signing setup and release review.
Preserve `~/.android/debug.keystore` privately between builds if you want
in-place updates (`adb install -r`). In an isolated Docker build, persist
`/root/.android` as well as the SDK and `/root/.gradle` caches. Deleting an
ephemeral signing container without exporting/persisting its private key loses
that update identity; a signed APK contains the public certificate, **not** a
recoverable private signing key. Do not silently replace an installed build's
key. A different signer requires an explicit reinstall/data-loss decision.

### Pairing build signing notice

The pairing APK built on 2026-10-21 uses a **new debug signing identity**:
the previous build's ephemeral private key was not available. Its public
certificate cannot recover the old key. Consequently this APK **cannot update
the previously sideloaded build with `adb install -r`**. Do not uninstall the
existing app without deciding how to handle its app data; no TV was changed by
this build. The output path below now contains the new pairing build, not the
original signed APK.

New certificate SHA-256:
`48fc244193f9f7cd2aed7b0db0ef800df5be24dcf05dda791f40af665ffc7088`.
The new private debug key is retained only in the Docker volume
`loom-android-home` at `/root/.android/debug.keystore`; keep that volume private
and do not remove it. SDK and Gradle volumes are also retained.

Exact successful isolated build from the repository root (JDK 17.0.20.1,
Gradle 8.10.2, AGP 8.7.3, Android platform 35/build-tools 35.0.0):

```sh
docker run --rm --platform linux/amd64 \
  -v "$PWD/android-tv:/workspace/android-tv" \
  -v loom-android-sdk:/tmp/android-sdk \
  -v loom-android-gradle:/root/.gradle \
  -v loom-android-home:/root/.android \
  -w /workspace/android-tv -e ANDROID_HOME=/tmp/android-sdk \
  eclipse-temurin:17-jdk-jammy sh -c \
  'sh ./gradlew --no-daemon --no-watch-fs --no-configuration-cache \
    :app:testDebugUnitTest :app:lintDebug :app:assembleDebug'
```

This produced the updated signed `app/build/outputs/apk/debug/app-debug.apk`,
passed 32 JVM tests and Android lint (0 errors, 25 nonblocking warnings).

The separate [Android TV workflow](../.github/workflows/android-tv.yml)
performs unit tests, Android lint and debug APK assembly on affected pushes,
pull requests or manual dispatch and uploads the APK and reports. Existing
Go/web workflows are unchanged. A successful APK build is **not** hardware
certification or proof that an OEM allows a replacement Home app.

## Install and try — before changing Home

1. Keep the stock launcher and system apps installed. Ensure you can still
   reach system Settings using the TV remote.
2. Enable Developer options and USB/wireless debugging in TV settings, if
   the firmware offers them. Pair ADB only on your trusted LAN; do not expose
   ADB to the internet. Authorize your own computer on the TV.
3. With the Android SDK platform-tools on PATH:

   ```sh
   adb devices
   adb install -r app/build/outputs/apk/debug/app-debug.apk
   adb shell am start -n dev.loom.tv/.HomeActivity
   ```

   If multiple devices are connected, use `adb -s DEVICE` for every command.
   Do not publish `adb devices` output containing device identifiers.
4. D-pad through Apps, TV settings and Inputs first. Test an actual HDMI
   source. If OEM switching is denied, the launcher displays an explanation
   and offers input/settings alternatives; the remote's Source/Input button
   may still be necessary.
5. Open **Server** and configure the connection below.

## Connect to Loom

Use the server root address reachable **from the TV**, for example
`https://loom.example` or `http://192.168.1.20:1925`. `localhost` on a TV is
the TV itself. A reverse-proxy prefix is supported, for example
`https://host.example/loom`. Do not append `/api/v1`, query parameters,
credentials, or a fragment. Prefixes support letters, numbers, `/`, `_`, `-`.
Encoded/special-character prefixes are deliberately rejected in this version.
Bare IP/host addresses such as `192.168.1.20:1925` or `loom.local:1925/loom`
are intentionally normalized to **HTTP**, and are rejected until you explicitly
check the trusted-network HTTP consent box. They do not silently opt you into
unencrypted credentials; prefer an explicit HTTPS address where available.

### Pair with a phone or browser

Update/deploy the Loom **backend and web UI alongside this APK** before pairing.
See [server pairing protocol and deployment requirements](../docs/device-pairing.md).

1. In **Server**, enter the server address and, only for trusted HTTP LAN
   setups, explicitly allow unencrypted HTTP.
2. Select **Pair with phone / browser**. Leave the manual API key blank;
   pairing never sends an existing saved API key.
3. On your own phone or computer, open the complete address displayed on the
   TV, sign in to Loom, verify the `XXXX-XXXX` code, and approve this TV.
   For `https://host.example/loom`, that address is
   `https://host.example/loom/pair?code=XXXX-XXXX`, **not** the origin's `/pair`.
4. Keep the pairing dialog open on the TV. It polls no faster than the server's
   interval (normally 5 seconds), shows remaining expiry and the next check,
   and honors `429 Retry-After` with at least a five-second wait.
   The normal code lifetime is 600 seconds.
5. Only an approved poll containing a valid API key saves the connection.
   The key is encrypted with Android Keystore as described below. The code
   and device secret remain memory-only, never in preferences or logs.

Pairing needs a Loom server with this protocol and **genuine authentication
configured**. A server returning 503 cannot pair while auth is disabled or
unconfigured; use manual setup if you deliberately run without auth. A 404/405
explains that the server may be too old or the URL prefix wrong, with the
manual-key alternative. Denial, expiry/consumption/invalid secret (410),
connection/TLS errors and invalid responses are visible failures, never an
empty library or silent success. **Start again** creates a fresh attempt.
The TV does not show upstream error bodies, follow redirects, open an external
browser itself, accept foreign verification URLs, or bypass TLS validation.
The browser must reach the same server and prefix.

**Cancel / manual setup**, Back, replacing the dialog, or leaving the app stops
polling and best-effort cancels the server attempt. An in-flight request has the
same bounded timeouts as catalog requests; if a start response arrives after
cancellation, its secret is still cancelled. Dialog/attempt identity and active
lifecycle checks prevent an old callback saving credentials after cancellation,
retry, manual setup, Forget, or app stop. A cancelled/stopped flow is not resumed
automatically. Apps, inputs and settings remain independent of pairing.

Approval records the user on the server; the API key is minted only when the TV
polls approval, and is delivered **once**. If the network fails after the server
consumes that poll, or the TV leaves before saving its response, the TV cannot
recover that key by polling again. Start a new pairing; remove any unused
TV key in Loom web Settings. Cancellation cannot revoke a key already minted.
This is a protocol limitation, not permission to retry a consumed secret.

### Manual API key setup (still available)

In the existing Loom web Settings **API Key** section, generate/copy an API
key while authenticated. Prefer a key dedicated to this TV and revoke it
when the TV is retired. Existing server code resolves API keys to the
issuing user's identity; the TV does not introduce a narrower server-side
scope. Treat the key as a server credential even though catalog access only
issues GET requests. The native manual key issuance endpoint is
`POST /api/v1/auth/apikeys` with `{"name":"Loom TV"}` using an existing
authenticated session; its response includes `key` **once**. Do not put
keys into shell history, screenshots, issues or this repository.

Enter that key in Server and select **Save & connect**. It is masked on
screen, encrypted using AES-GCM with an Android Keystore key, and stored in
app-private preferences; backups are disabled and the configuration dialog
blocks screenshots. It is not protected against root or compromised TV
firmware. The URL and HTTP-consent flag are stored in private preferences.
**Forget server** clears the saved configuration and displayed catalog.
Uninstalling clears app data; revoke the server key separately.

Leave the key blank only when your server really has authentication disabled.
The launcher does not implement browser cookies, password login, OIDC or a
reverse-proxy login page. A proxy must allow native API-key requests through.
Loom's actual middleware (`internal/auth/middleware.go`) accepts `X-Api-Key`
even for a forms-login installation. Expired/revoked/incorrect keys show 401;
403 identifies access denial rather than an empty library.

HTTPS uses Android's normal certificate validation: no trust-all certificates
or hostname bypass. For a plain HTTP LAN server, explicitly enable
**Allow unencrypted HTTP on my trusted network**. This sends the API key
unencrypted and is inappropriate on untrusted networks. The manifest allows
cleartext for self-hosted LAN setups, but validation prevents HTTP requests
unless this consent is saved. Changing to another URL requires saving and
confirming its configuration again.

Requests run off the UI thread, use 8-second connect and 12-second read
timeouts plus a 30-second elapsed check between reads, never follow redirects,
and accept at most 8 MiB per catalog
response. No error response bodies, keys or transport exception details are
logged/displayed. No artwork is fetched, so credentials cannot leak to
image hosts. A 3xx asks for the final URL instead of forwarding the key.
Navigating away invalidates callbacks and cancels outstanding work;
blocked reads are bounded by the read timeout.

### API contracts used

| Request | Response / use |
| --- | --- |
| `GET {base}/api/v1/movies?limit=24&offset=N` | `{data: [...], total: number}`; server pagination |
| `GET {base}/api/v1/series` | `{data: [...], total: number}`; local display pagination |
| `POST {base}/api/v1/auth/pairing/start` | `{"device_name":"Loom TV"}` → 201 `{device_secret,user_code,verification_path,expires_in,interval}` |
| `POST {base}/api/v1/auth/pairing/poll` | `{device_secret}` → 200 `{status:"pending"}`, `{status:"denied"}`, or one-time `{status:"approved",api_key}` |
| `POST {base}/api/v1/auth/pairing/cancel` | `{device_secret}` → 204; best-effort cleanup |

Catalog authentication: `X-Api-Key` header, never query parameters. Pairing
POSTs are unauthenticated JSON with no saved key/cookie forwarding; secrets are
only in the POST body. Pairing responses are capped at 64 KiB, codes/verification
paths are validated, and HTTPS uses the platform's normal TLS trust.
Required catalog record
fields: `id`, `title`. Optional detail fields: `year`, `overview`, `runtime`,
`rating`, `network`, `status`, `monitoringStatus` (camelCase as encoded by
the existing movie/series handlers). Details use this real list metadata;
no fabricated additional detail requests. API documentation has historical
stubs, so implementation was checked against the actual handlers/middleware.

## Make Loom Home — separate, reversible step

Only after testing, open **Home setup → Open Home settings**. If the firmware
offers a default Home app selector, choose Loom TV. Alternatively, pressing
Home may offer a chooser: first try “Just once” if available.

This declares both `HOME`/`DEFAULT` and `LEANBACK_LAUNCHER`; it does not force
the Home role. Some Veon/Skyworth firmware locks the Home key, default-home
selector or power-on screen. Android/Google TV startup can also be a
quick-resume of the previous app or input. Being the default home does not
guarantee Loom appears at every power-on. There is **no boot receiver or
automatic reboot launch bypass**. If the TV will not offer home selection,
use Loom as a regular TV app rather than modifying packages.

### Roll back

1. Open TV **Settings → Apps → Default apps / Home app** (names vary), or
   Loom **Home setup → Open Home settings**.
2. Select the original Google TV/Android TV/OEM launcher.
3. Press Home to verify the stock home works. Then uninstall Loom TV from
   Apps settings, or:

   ```sh
   adb uninstall dev.loom.tv
   ```

If the default-home selector is unavailable, use the remote's Settings
button and the system Apps screen to uninstall Loom. Keep an authorized ADB
connection during initial testing as an additional recovery route. No
disabled packages need re-enabling because Loom never disables any.
Turn debugging off/revoke computer authorizations when testing finishes.

## Identify the actual TVs safely

These are **read-only** discovery commands after ADB pairing. Run on each TV,
separately. Model/build strings and installed package names can reveal
details about your installation; inspect and redact output before sharing.
Do not dump all properties, account data, preferences or logcat.

```sh
adb shell getprop ro.product.manufacturer
adb shell getprop ro.product.model
adb shell getprop ro.product.device
adb shell getprop ro.product.board
adb shell getprop ro.hardware
adb shell getprop ro.build.version.release
adb shell getprop ro.build.version.sdk
adb shell pm list packages
adb shell cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.HOME
adb shell cmd package query-activities --brief -a android.intent.action.MAIN -c android.intent.category.HOME
adb shell dumpsys tv_input
```

Some OEMs omit/restrict the package-query or TV-input dump commands. A denied
command is information about the firmware, not a reason to root it. Nothing
here assumes a Skyworth package name or supplies a debloat list.

## Verification and hardware acceptance

JVM tests cover URL/HTTP consent/header validation, actual JSON envelope and
field parsing, series page boundaries, error vs empty semantics, real HTTP
request paths and API headers, redirect rejection, and safe auth errors.
Pairing tests additionally cover prefixed POST paths, JSON bodies with no
saved API key, verification-address prefix preservation and hostile-path
rejection, response bounds/schema, pending/approved/denied, 404/405/410/503,
429 delay, expiry, cancellation during start/poll/wait, and late approvals.

Before treating this as your daily home, test on **both actual TVs**:

- 720p, 1080p and 4K output/UI scaling where supported, plus increased system
  font size: no clipped key controls, readable titles/details at TV distance.
- Only a D-pad/OK/Back remote: all controls, next/previous, dialogs, long
  titles, shelves and TV settings; focus border always visible; Back returns
  from players/settings to Loom when Android allows it.
- Zero server configuration, Wi-Fi disconnected, unreachable/slow server,
  invalid/revoked key, 403, redirected login, malformed response, empty
  catalogs, >24 movies and >24 series; no silent “success”.
- Save then force-stop/reopen or restart: URL/key restored. Forget clears
  the connection. Leave during a load and return: no stale callbacks or
  frozen device controls.
- Pair from a phone with real browser approval: correct prefixed address,
  D-pad/Back/Cancel/Start again, denial, expiry, old/unconfigured server, HTTP
  consent, leaving during start/poll, and no saved key from a stale attempt.
  Pair successfully, reopen the TV app and confirm encrypted key restoration;
  cancel before approval and confirm no key was created. Hardware/browser
  pairing is not verified by the loopback JVM tests.
- Launch installed TV apps and return; install/uninstall an app, then
  Refresh apps & inputs.
- Real HDMI sources: custom labels, hotplug states, direct switching, denied
  access and system/remote fallbacks. Tuner/input permissions vary by OEM.
- Test stock-home recovery **before** home assignment; then Home, standby/
  wake, quick resume and full power cycle. Record actual startup behavior.

No emulator screenshots or physical-TV results are implied by the JVM
tests/build. See the implementation handoff for commands actually run.

## Reference and licensing

[CleanYourTV](https://github.com/Gathin23/cleanyourtv) was supplied as an
architectural reference for a native Java launcher and Android TV Input
framework. This implementation is written for Loom; no launcher source or
font assets were copied. It remains under this repository's
[AGPL-3.0 license](../LICENSE). The original launcher/system packages remain
installed and are not part of this distribution.
