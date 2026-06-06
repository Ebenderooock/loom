# Plugins (Custom JavaScript)

Loom can run **custom JavaScript** when domain events fire — for example, after a
download is grabbed, after an import completes, or when playback starts/stops.
This is Loom's equivalent of the Sonarr/Radarr "Custom Script" connection, but
the script is JavaScript that Loom runs itself (no external interpreter, no
subprocess).

A plugin is a snippet of JavaScript plus the set of events it should react to.
Loom compiles and runs it in an embedded interpreter
([goja](https://github.com/dop251/goja), ES5.1+), hands it the event payload,
captures `console` output, and records the result.

> **Admin only & opt-in.** Plugins execute arbitrary JavaScript with the Loom
> server's network access, so the feature is gated by the **Plugins (Custom
> Scripts)** feature flag (disabled by default) and every endpoint requires an
> admin. Enable it under **Settings → Features**.

## Security & trust model

**Plugins are NOT run in an OS-level sandbox.** A plugin runs **in-process**,
inside the Loom server, with that process's privileges and network access.

What Loom *does* provide is **failure isolation and resource bounding**:

| Control | Behaviour |
| --- | --- |
| Fresh interpreter per run | Each run gets its own goja runtime; nothing is shared between runs. |
| Timeout | Each run has a wall-clock budget (default 30s, max 300s). On expiry the VM is **interrupted** and the run recorded as failed. |
| CPU/recursion bounds | Long loops are interruptible; the call stack is bounded so a script can't overflow the host stack. |
| Panic recovery | A host panic in the runner is recovered and recorded, never propagated. |
| `fetch` limits | Only `http`/`https`; request body, response body and header count are size-capped; a per-request timeout applies. |
| Output caps | `console` output is captured up to 64 KiB per stream; the rest is discarded. |
| Concurrency cap | Runs execute in a bounded worker pool and never block Loom's event bus. |
| Run history | Every execution is recorded (success, duration, output), pruned per-plugin and by age. |

> **Caveat:** because plugins run in-process, a single huge allocation
> (e.g. `new Array(1e9)`) can pressure the shared server heap — there is no cheap
> per-script heap cap. Only configure plugins you trust. For real isolation, run
> **Loom itself** under container/Kubernetes controls (dropped capabilities,
> network policies, a dedicated user).

## The JavaScript runtime API

Each run exposes four globals:

### `event`

A detached object describing what happened (mutating it does not affect Loom):

```js
event = {
  version: 1,                       // payload schema version
  event: "import_complete",         // event key
  topic: "imports.completed",       // internal bus topic
  title: "The Matrix (1999)",       // human-readable subject (when known)
  data: { title: "The Matrix (1999)", /* event-specific fields */ },
  timestamp: "2026-06-05T11:49:45Z" // RFC3339 UTC
};
```

The `data` map is populated from the event's structured fields. The exact keys
depend on the event; `title` is present when known.

### `env`

An object of the environment variables you configured for the plugin, e.g.
`env.WEBHOOK_URL`. The host server environment is **not** exposed.

### `console`

`console.log` / `info` / `debug` write to captured **stdout**; `console.warn` /
`error` write to captured **stderr**. Non-string arguments are rendered as
compact JSON. Captured output is visible under **History**.

### `fetch(url, opts)`

A minimal, **synchronous** HTTP client bound to the run's timeout:

```js
var res = fetch("https://example.com/hook", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ title: event.title }),
});
console.log("status", res.status, "ok", res.ok);
console.log("body", res.body);
```

- Only `http`/`https` URLs are allowed.
- `opts` (optional): `{ method, headers: {}, body }`. `body` is a string.
- Returns `{ status, ok, statusText, body, headers }`. `body` is a string,
  truncated to the response size cap.
- Failures (network error, invalid URL, disallowed scheme, body too large)
  throw a JavaScript exception, which records the run as failed.

## Result

- The script **returning normally** → the run is recorded as **success**.
- A **thrown exception** or **compile error** → recorded as **failure** with the
  error message captured.
- **Timeout / cancellation** → recorded as failure ("timed out…").

Loom does not act on a plugin's result beyond recording it — a failing plugin
does not block or revert the underlying import/grab.

## Supported events

| Key | When it fires |
| --- | --- |
| `grab` | A release was grabbed and queued to a download client. |
| `download_complete` | A download finished in the client. |
| `download_failed` | A download failed. |
| `import_complete` | A file was imported into the library. |
| `import_failed` | An import failed. |
| `playback_started` | A stream started on a connected media server. |
| `playback_stopped` | A stream stopped. |

## Example plugin

See [`examples/plugins/post-import.js`](../examples/plugins/post-import.js) for a
minimal example that logs the event and posts a webhook.

To register it in the UI:

1. **Settings → Features** → enable **Plugins (Custom Scripts)**.
2. **Settings → Plugins → Add Plugin**.
   - **Script:** paste your JavaScript (a starter template is prefilled).
   - **Events:** check *On Import Complete*.
   - Optionally set **env vars** and a **timeout**.
3. Save, then click **▶ (test)** to run it once with a synthetic payload and
   inspect the captured output under **History**.
