# Plugins (Custom Post-Processing Scripts)

Loom can run **custom scripts** when domain events fire — for example, after a
download is grabbed, after an import completes, or when playback starts/stops.
This is Loom's equivalent of the Sonarr/Radarr "Custom Script" connection.

A plugin is simply an executable command that you configure. Loom invokes it as
a child process, hands it the event payload, captures its output, and records
the result.

> **Admin only & opt-in.** Plugins execute arbitrary commands, so the feature is
> gated by the **Plugins (Custom Scripts)** feature flag (disabled by default)
> and every endpoint requires an admin. Enable it under **Settings → Features**.

## Security & trust model

**Plugins are NOT run in an OS-level sandbox.** A plugin runs as the Loom server
process user, with that user's privileges and access to the same filesystem,
network, and (where granted) configuration.

What Loom *does* provide is **failure isolation and resource bounding**:

| Control | Behaviour |
| --- | --- |
| Separate process | Each run is a child process; a crash can't take down Loom. |
| Timeout | Each run has a wall-clock budget (default 30s, max 300s). On expiry the **entire process group** is killed (`SIGKILL`). |
| No host env | The server environment is **not** inherited, so server secrets aren't leaked. Only a minimal `PATH`, your configured variables, and `LOOM_*` vars are passed. |
| Output caps | `stdout`/`stderr` are captured up to 64 KiB each; the rest is discarded. |
| Concurrency cap | Runs execute in a bounded worker pool and never block Loom's event bus. |
| Panic recovery | A failure in the runner is logged, never propagated. |
| Run history | Every execution is recorded (success, exit code, duration, output), pruned per-plugin and by age. |

For real isolation, run **Loom itself** under container/Kubernetes controls
(read-only mounts, dropped capabilities, network policies, a dedicated user).

## How a plugin is invoked

- The command is executed **directly — there is no shell**. Arguments are passed
  as an argv list (space-separated in the UI). Use an **absolute path** to the
  executable. If you need shell features, make your command
  `/bin/sh -c "…"` explicitly.
- The working directory defaults to `/` (configurable per plugin).
- The event payload is written to the process **stdin** as a single JSON
  document, and also exposed via environment variables.

### Environment variables

| Variable | Description |
| --- | --- |
| `LOOM_PAYLOAD_VERSION` | Payload schema version (currently `1`). |
| `LOOM_EVENT` | Event key (e.g. `grab`, `import_complete`). |
| `LOOM_TOPIC` | Internal bus topic (e.g. `downloads.queued`). |
| `LOOM_TITLE` | Human-readable title for the event subject. |
| `LOOM_DATA_JSON` | The `data` object, as a JSON string. |
| `LOOM_PAYLOAD_JSON` | The full payload, as a JSON string (same as stdin). |

`LOOM_*` keys are reserved — you cannot override them with your own env vars.

### stdin payload (schema v1)

```json
{
  "version": 1,
  "event": "import_complete",
  "topic": "imports.completed",
  "title": "The Matrix (1999)",
  "data": {
    "title": "The Matrix (1999)",
    "...": "event-specific fields"
  },
  "timestamp": "2026-06-05T11:49:45Z"
}
```

The `data` map is populated from the event's structured fields. The exact keys
depend on the event; `title` is always present when known.

### Exit codes & result

- **Exit `0`** → the run is recorded as **success**.
- **Any non-zero exit** → recorded as **failure** with the captured exit code.
- **Timeout** → recorded as failure with exit code `-1` and a "timed out" error.

Loom does not act on a plugin's exit code beyond recording it — a failing plugin
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

See [`examples/plugins/post-import.sh`](../examples/plugins/post-import.sh) for a
minimal, dependency-free example that reads the payload from stdin and logs it.

To register it in the UI:

1. **Settings → Features** → enable **Plugins (Custom Scripts)**.
2. **Settings → Plugins → Add Plugin**.
   - **Command:** `/scripts/post-import.sh`
   - **Events:** check *On Import Complete*.
   - Optionally set **env vars**, a **timeout**, and a **working directory**.
3. Save, then click **▶ (test)** to run it once with a synthetic payload and
   inspect the captured output under **History**.
