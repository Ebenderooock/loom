#!/bin/sh
# Install or upgrade using a locally downloaded, checksum-verified binary.
set -eu
if [ "$(id -u)" -ne 0 ]; then
    echo "Run with sudo: sudo sh $0 /path/to/loom" >&2
    exit 1
fi
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ] || [ ! -f "$1" ] || [ ! -x "$1" ]; then
    echo "Supply a Linux Loom binary and optionally its matching loom-updater binary." >&2
    exit 1
fi
source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
for command in systemctl getent useradd groupadd install; do
    command -v "$command" >/dev/null || { echo "Missing $command" >&2; exit 1; }
done
if [ "$#" -eq 2 ]; then
    [ -f "$2" ] && [ -x "$2" ] || { echo "Invalid updater binary" >&2; exit 1; }
    "$2" preflight
    for file in loom-updater.service loom-updater-recover.service loom-updater.timer updater.conf; do
        [ -f "$source_dir/$file" ] || { echo "Missing updater installer file: $file" >&2; exit 1; }
    done
fi
if systemctl cat loom-updater.service >/dev/null 2>&1 ||
   [ -e /etc/systemd/system/loom-updater.service ] ||
   [ -L /etc/systemd/system/loom-updater.service ] ||
   [ -e /usr/local/lib/loom/updater-managed ]; then
    [ "$#" -eq 2 ] || {
        echo "This updater-enrolled install requires the matching loom-updater binary as the second argument." >&2
        exit 1
    }
    systemctl stop loom-updater.timer loom-updater.service
fi
if [ -e /var/lib/loom-updater/journal.json ]; then
    echo "An update needs recovery; start loom-updater-recover.service before manual installation." >&2
    exit 1
fi
for file in loom.service loom.yaml; do
    [ -r "$source_dir/$file" ] || { echo "Missing installer file: $file" >&2; exit 1; }
done
if systemctl cat loom.service >/dev/null 2>&1 ||
   [ -e /etc/systemd/system/loom.service ] || [ -L /etc/systemd/system/loom.service ]; then
    if [ ! -f /etc/systemd/system/loom.service ] ||
       ! grep -q '^# Managed by Loom native installer$' /etc/systemd/system/loom.service; then
        echo "Existing loom.service is not managed by this installer; migrate manually." >&2
        exit 1
    fi
fi
if getent passwd loom >/dev/null; then
    if [ "$(getent passwd loom | cut -d: -f6)" != /var/lib/loom ]; then
        echo "Existing loom account has a different home; migrate manually." >&2
        exit 1
    fi
    case "$(getent passwd loom | cut -d: -f7)" in
        /usr/sbin/nologin|/sbin/nologin|/bin/false|/usr/bin/false) ;;
        *) echo "Existing loom account is not a non-login service account." >&2; exit 1 ;;
    esac
else
    getent group loom >/dev/null || groupadd --system loom
    useradd --system --gid loom --home-dir /var/lib/loom --no-create-home --shell /usr/sbin/nologin loom
fi
getent group loom >/dev/null || { echo "Missing loom group" >&2; exit 1; }
# Stage before stopping: missing/corrupt source must not interrupt a running service.
install -d -o root -g root -m 0755 /usr/local/lib/loom
install -o root -g root -m 0755 "$1" /usr/local/lib/loom/loom.new
/usr/local/lib/loom/loom.new version
if [ -f /etc/systemd/system/loom.service ]; then
    systemctl stop loom.service
fi
if [ -f /usr/local/lib/loom/loom ]; then
    cp -p /usr/local/lib/loom/loom /usr/local/lib/loom/loom.previous
fi
mv /usr/local/lib/loom/loom.new /usr/local/lib/loom/loom
install -d -o loom -g loom -m 0750 /etc/loom /var/lib/loom
if [ ! -e /etc/loom/loom.yaml ]; then
    install -o loom -g loom -m 0640 "$source_dir/loom.yaml" /etc/loom/loom.yaml
fi
install -o root -g root -m 0644 "$source_dir/loom.service" /etc/systemd/system/loom.service
if [ "$#" -eq 2 ]; then
    install -o root -g root -m 0755 "$2" /usr/local/lib/loom/loom-updater
    install -d -o root -g root -m 0700 /var/lib/loom-updater
    install -d -o root -g root -m 0755 /etc/systemd/system/loom.service.d
    for file in loom-updater.service loom-updater-recover.service loom-updater.timer; do
        install -o root -g root -m 0644 "$source_dir/$file" "/etc/systemd/system/$file"
    done
    install -o root -g root -m 0644 "$source_dir/updater.conf" /etc/systemd/system/loom.service.d/updater.conf
fi
systemctl daemon-reload
systemctl enable --now loom.service
if [ "$#" -eq 2 ]; then
    /usr/local/lib/loom/loom-updater enroll
    systemctl enable --now loom-updater.timer
    echo "Updater helper enrolled. Automatic installation stays OFF until enabled by a Loom administrator."
fi
echo "Installed. Check: systemctl status loom; journalctl -u loom"
echo "Open http://127.0.0.1:1925 and complete setup. See docs/linux.md."
