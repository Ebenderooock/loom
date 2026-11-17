#!/bin/sh
set -eu
[ "$(uname -s)" = Darwin ] || { echo "This script requires macOS." >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || { echo "Run with sudo." >&2; exit 1; }
plist=/Library/LaunchDaemons/dev.loom.server.plist
base="/Library/Application Support/Loom"
updater_plist=/Library/LaunchDaemons/dev.loom.updater.plist
if [ -e "$updater_plist" ] || [ -L "$updater_plist" ]; then
    [ ! -L "$updater_plist" ] && grep -q 'Managed by Loom native updater installer' "$updater_plist" ||
        { echo "Refusing to remove an unmanaged privileged daemon." >&2; exit 1; }
fi
[ ! -e "$base/updater/journal.json" ] ||
    { echo "Recover the pending native update before uninstalling." >&2; exit 1; }
if [ -e "$plist" ] || [ -L "$plist" ]; then
    [ ! -L "$plist" ] && grep -q 'Managed by Loom native installer' "$plist" ||
        { echo "Refusing to remove an unmanaged launch daemon." >&2; exit 1; }
fi
if launchctl print system/dev.loom.updater >/dev/null 2>&1; then
    launchctl bootout system/dev.loom.updater
fi
[ ! -e "$base/updater/journal.json" ] ||
    { echo "Recovery remains pending; installation retained." >&2; exit 1; }
if [ -e "$plist" ]; then
    if launchctl print system/dev.loom.server >/dev/null 2>&1; then
        launchctl bootout system/dev.loom.server
    fi
    rm -f "$plist"
    rm -f "/Library/Application Support/Loom/bin/loom" \
          "/Library/Application Support/Loom/bin/loom.previous" \
          "/Library/Application Support/Loom/bin/loom.new"
fi
rm -f "$updater_plist" "$base/bin/loom-updater" "$base/bin/updater-managed" \
      "$base/bin/updater-enrolled" "$base/bin/updater-token" "$base/bin/updater.sock"
echo "Preserved /Library/Application Support/Loom/{config,data,updater}, /Library/Logs/Loom and _loom account."
