# Example Loom plugins

These are minimal, dependency-free example scripts for Loom's
[Plugins (custom post-processing scripts)](../../docs/plugins.md) feature.

| File | Description |
| --- | --- |
| [`post-import.sh`](./post-import.sh) | Logs every event it receives (stdin JSON + `LOOM_*` env vars) to a file and to stdout. |

## Using an example

1. Make the script executable and place it where the Loom server can reach it
   (for the container deployment, mount it into the pod, e.g. `/scripts`):

   ```sh
   chmod +x post-import.sh
   ```

2. In Loom: **Settings → Features** → enable **Plugins (Custom Scripts)**.

3. **Settings → Plugins → Add Plugin**:
   - **Command:** absolute path to the script, e.g. `/scripts/post-import.sh`
   - **Events:** choose which events trigger it.

4. Click the **▶ test** button to run it once with a synthetic payload and view
   the captured output under **History**.

See [`docs/plugins.md`](../../docs/plugins.md) for the full SDK contract and the
security/trust model.
