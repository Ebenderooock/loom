#!/bin/sh
set -eu
[ "$(uname -s)" = Darwin ] || { echo "This script requires macOS." >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || { echo "Run with sudo." >&2; exit 1; }
plist=/Library/LaunchDaemons/dev.loom.server.plist
if [ -e "$plist" ] || [ -L "$plist" ]; then
    grep -q 'Managed by Loom native installer' "$plist" ||
        { echo "Refusing to remove an unmanaged launch daemon." >&2; exit 1; }
    if launchctl print system/dev.loom.server >/dev/null 2>&1; then
        launchctl bootout system/dev.loom.server
    fi
    rm -f "$plist"
    rm -f "/Library/Application Support/Loom/bin/loom" \
          "/Library/Application Support/Loom/bin/loom.previous" \
          "/Library/Application Support/Loom/bin/loom.new"
fi
echo "Preserved /Library/Application Support/Loom/{config,data}, /Library/Logs/Loom and _loom account."
