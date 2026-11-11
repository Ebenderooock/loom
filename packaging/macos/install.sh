#!/bin/sh
set -eu
[ "$(uname -s)" = Darwin ] || { echo "This installer requires macOS." >&2; exit 1; }
[ "$(id -u)" -eq 0 ] || { echo "Run with sudo." >&2; exit 1; }
if [ "$#" -ne 1 ] || [ ! -f "$1" ] || [ ! -x "$1" ]; then
    echo "Supply one executable macOS Loom release binary." >&2
    exit 1
fi
source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
base="/Library/Application Support/Loom"
plist=/Library/LaunchDaemons/dev.loom.server.plist
if { [ -e "$plist" ] || [ -L "$plist" ]; } && ! grep -q 'Managed by Loom native installer' "$plist"; then
    echo "Existing launch daemon is unmanaged; migrate manually." >&2
    exit 1
fi
/usr/bin/plutil -lint "$source_dir/dev.loom.server.plist"
[ -r "$source_dir/loom.yaml" ] || { echo "Missing installer loom.yaml." >&2; exit 1; }
# Dedicated non-login account; do not reuse a normal interactive user's identity.
if dscl . -read /Users/_loom >/dev/null 2>&1; then
    [ "$(dscl . -read /Users/_loom NFSHomeDirectory | sed 's/^NFSHomeDirectory: //')" = "$base/data" ] ||
        { echo "Existing _loom user has a different home." >&2; exit 1; }
    [ "$(dscl . -read /Users/_loom UserShell | sed 's/^UserShell: //')" = /usr/bin/false ] ||
        { echo "Existing _loom user is not a non-login service account." >&2; exit 1; }
    user_group=$(dscl . -read /Users/_loom PrimaryGroupID | awk '{print $NF}')
    group_id=$(dscl . -read /Groups/_loom PrimaryGroupID | awk '{print $NF}')
    [ "$user_group" = "$group_id" ] ||
        { echo "Existing _loom user/group mismatch." >&2; exit 1; }
else
    if dscl . -read /Groups/_loom >/dev/null 2>&1; then
        echo "Existing _loom group without _loom user; migrate manually." >&2
        exit 1
    fi
    service_id=400
    while dscl . -list /Users UniqueID | awk '{print $NF}' | grep -qx "$service_id" ||
          dscl . -list /Groups PrimaryGroupID | awk '{print $NF}' | grep -qx "$service_id"; do
        service_id=$((service_id + 1))
        [ "$service_id" -lt 500 ] || { echo "No free system account ID." >&2; exit 1; }
    done
    dscl . -create /Groups/_loom
    dscl . -create /Groups/_loom PrimaryGroupID "$service_id"
    dscl . -create /Users/_loom
    dscl . -create /Users/_loom UniqueID "$service_id"
    dscl . -create /Users/_loom PrimaryGroupID "$service_id"
    dscl . -create /Users/_loom NFSHomeDirectory "$base/data"
    dscl . -create /Users/_loom UserShell /usr/bin/false
    dscl . -create /Users/_loom IsHidden 1
    dscl . -create /Users/_loom Password '*'
fi
install -d -o root -g wheel -m 0755 "$base" "$base/bin"
install -o root -g wheel -m 0755 "$1" "$base/bin/loom.new"
"$base/bin/loom.new" version
if launchctl print system/dev.loom.server >/dev/null 2>&1; then
    launchctl bootout system/dev.loom.server
fi
if [ -f "$base/bin/loom" ]; then
    cp -p "$base/bin/loom" "$base/bin/loom.previous"
fi
mv "$base/bin/loom.new" "$base/bin/loom"
install -d -o _loom -g _loom -m 0750 "$base/config" "$base/data" /Library/Logs/Loom
if [ ! -e "$base/config/loom.yaml" ]; then
    install -o _loom -g _loom -m 0640 "$source_dir/loom.yaml" "$base/config/loom.yaml"
fi
install -o root -g wheel -m 0644 "$source_dir/dev.loom.server.plist" "$plist"
launchctl enable system/dev.loom.server
launchctl bootstrap system "$plist"
echo "Installed. Check: sudo launchctl print system/dev.loom.server"
echo "Open http://127.0.0.1:1925 and complete setup. See docs/macos.md."
