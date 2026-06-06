# Example Loom plugins

These are minimal example **JavaScript** plugins for Loom's
[Plugins (custom scripts)](../../docs/plugins.md) feature. Plugins run inside the
Loom server in an embedded JS interpreter — there's nothing to install or mount.

| File | Description |
| --- | --- |
| [`post-import.js`](./post-import.js) | Logs the event and, if `env.WEBHOOK_URL` is set, POSTs a small JSON notification via `fetch`. |

## Using an example

1. In Loom: **Settings → Features** → enable **Plugins (Custom Scripts)**.

2. **Settings → Plugins → Add Plugin**:
   - **Script:** paste the contents of the `.js` file (a starter template is
     prefilled for new plugins).
   - **Events:** choose which events trigger it.
   - **Environment variables (optional):** e.g. `WEBHOOK_URL=https://example.com/hook`.

3. Click the **▶ test** button to run it once with a synthetic payload and view
   the captured output under **History**.

See [`docs/plugins.md`](../../docs/plugins.md) for the full runtime API and the
security/trust model.
