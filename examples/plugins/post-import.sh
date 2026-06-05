#!/usr/bin/env sh
#
# Example Loom plugin: logs every event it receives.
#
# Loom invokes this script directly (no shell wrapping) when a subscribed event
# fires. The event payload arrives two ways:
#   1. As a JSON document on stdin.
#   2. As LOOM_* environment variables (see docs/plugins.md).
#
# Exit 0 for success; any non-zero exit is recorded as a failed run.
#
# Register it under Settings -> Plugins with command: /scripts/post-import.sh
# (after enabling the "Plugins (Custom Scripts)" feature flag).

set -eu

LOG="${LOOM_PLUGIN_LOG:-/tmp/loom-plugin.log}"

# Read the full JSON payload from stdin.
payload="$(cat)"

{
  echo "----- $(date -u '+%Y-%m-%dT%H:%M:%SZ') -----"
  echo "event:  ${LOOM_EVENT:-?}"
  echo "topic:  ${LOOM_TOPIC:-?}"
  echo "title:  ${LOOM_TITLE:-?}"
  echo "payload: ${payload}"
} >> "$LOG"

# Also echo to stdout so it shows up in the plugin run history.
echo "handled ${LOOM_EVENT:-unknown} for '${LOOM_TITLE:-unknown}'"

# If you have `jq` available you could pull individual fields, e.g.:
#   media_title="$(printf '%s' "$payload" | jq -r '.data.title // empty')"

exit 0
