#!/bin/sh
# Remove only the managed service and executable; retain configuration/data/user.
set -eu
[ "$(id -u)" -eq 0 ] || { echo "Run with sudo." >&2; exit 1; }
unit=/etc/systemd/system/loom.service
if [ -e "$unit" ] || [ -L "$unit" ]; then
    grep -q '^# Managed by Loom native installer$' "$unit" ||
        { echo "Refusing to remove an unmanaged loom.service." >&2; exit 1; }
    for file in /etc/systemd/system/loom-updater.service /etc/systemd/system/loom-updater-recover.service /etc/systemd/system/loom-updater.timer /etc/systemd/system/loom.service.d/updater.conf; do
        if [ -e "$file" ] || [ -L "$file" ]; then
            [ ! -L "$file" ] && grep -q '^# Managed by Loom native updater installer$' "$file" ||
                { echo "Refusing to remove unmanaged updater configuration." >&2; exit 1; }
        fi
    done
    [ ! -e /var/lib/loom-updater/journal.json ] ||
        { echo "Recover the pending update before uninstalling." >&2; exit 1; }
    if systemctl cat loom-updater.service >/dev/null 2>&1; then
        systemctl disable --now loom-updater.timer
        systemctl stop loom-updater.service
        [ ! -e /var/lib/loom-updater/journal.json ] ||
            { echo "Recover the pending update before uninstalling." >&2; exit 1; }
    fi
    systemctl disable --now loom.service
    rm -f "$unit"
    systemctl daemon-reload
    rm -f /usr/local/lib/loom/loom /usr/local/lib/loom/loom.previous /usr/local/lib/loom/loom.new
    for file in loom-updater.service loom-updater-recover.service loom-updater.timer; do
        if [ -f "/etc/systemd/system/$file" ]; then
            grep -q '^# Managed by Loom native updater installer$' "/etc/systemd/system/$file" ||
                { echo "Refusing to remove unmanaged updater unit." >&2; exit 1; }
            rm -f "/etc/systemd/system/$file"
        fi
    done
    rm -f /etc/systemd/system/loom.service.d/updater.conf
    rm -f /usr/local/lib/loom/loom-updater /usr/local/lib/loom/updater-token /usr/local/lib/loom/updater-managed /usr/local/lib/loom/updater-enrolled
    systemctl daemon-reload
fi
echo "Service removed. Preserved /etc/loom, /var/lib/loom, /var/lib/loom-updater backups, journal logs and loom account."
