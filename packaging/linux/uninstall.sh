#!/bin/sh
# Remove only the managed service and executable; retain configuration/data/user.
set -eu
[ "$(id -u)" -eq 0 ] || { echo "Run with sudo." >&2; exit 1; }
unit=/etc/systemd/system/loom.service
if [ -e "$unit" ] || [ -L "$unit" ]; then
    grep -q '^# Managed by Loom native installer$' "$unit" ||
        { echo "Refusing to remove an unmanaged loom.service." >&2; exit 1; }
    systemctl disable --now loom.service
    rm -f "$unit"
    systemctl daemon-reload
    rm -f /usr/local/lib/loom/loom /usr/local/lib/loom/loom.previous /usr/local/lib/loom/loom.new
fi
echo "Service removed. Preserved /etc/loom, /var/lib/loom, journal logs and loom account."
