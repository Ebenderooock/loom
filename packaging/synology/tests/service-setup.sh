#!/bin/sh
set -eu

SERVICE_SETUP="$(cd "$(dirname "$0")/../spk/loom/src" && pwd)/service-setup.sh"
TEST_DIR="$(mktemp -d)"
trap 'chmod -R u+rwX "${TEST_DIR}"; rm -rf "${TEST_DIR}"' EXIT

fail()
{
    echo "FAIL: $*" >&2
    exit 1
}

run_service()
{
    env SYNOPKG_PKGDEST="${TEST_DIR}/package" SYNOPKG_PKGVAR="$1" \
        sh -eu -c '. "$1"; service_prestart' sh "${SERVICE_SETUP}"
}

env -u SYNOPKG_PKGDEST -u SYNOPKG_PKGVAR sh -eu -c '
    original_home="${HOME}"
    original_tmp="${TMPDIR-}"
    original_umask="$(umask)"
    . "$1"
    [ "${HOME}" = "${original_home}" ]
    [ "${TMPDIR-}" = "${original_tmp}" ]
    [ "$(umask)" = "${original_umask}" ]
' sh "${SERVICE_SETUP}" || fail "installer sourcing requires runtime paths or changes its environment"

env SYNOPKG_PKGDEST="${TEST_DIR}/package" SYNOPKG_PKGVAR="${TEST_DIR}/package data" \
    SERVICE_PORT=2925 sh -eu -c '
        . "$1"
        service_prestart
        [ "${LOOM_CONFIG_DIR}" = "${SYNOPKG_PKGVAR}" ]
        [ "${LOOM_DATA_DIR}" = "${SYNOPKG_PKGVAR}" ]
        [ "${HOME}" = "${SYNOPKG_PKGVAR}" ]
        [ "${XDG_CACHE_HOME}" = "${SYNOPKG_PKGVAR}/cache" ]
        [ "${TMPDIR}" = "${SYNOPKG_PKGVAR}/tmp" ]
        [ "${LOOM_HTTP_ADDR}" = ":2925" ]
        [ "${SERVICE_COMMAND}" = "${SYNOPKG_PKGDEST}/bin/loom serve" ]
        for directory in "${LOOM_CONFIG_DIR}" "${LOOM_DATA_DIR}/data" "${XDG_CACHE_HOME}" "${TMPDIR}"; do
            [ -d "${directory}" ] && [ -w "${directory}" ] && [ -x "${directory}" ]
        done
        [ "$(umask)" = "0002" ]
        printf "preserve\n" > "${LOOM_DATA_DIR}/data/existing.db"
        service_prestart
        [ "$(cat "${LOOM_DATA_DIR}/data/existing.db")" = "preserve" ]
    ' sh "${SERVICE_SETUP}" || fail "writable paths, environment or restart preservation"

mkdir -p "${TEST_DIR}/blocked"
touch "${TEST_DIR}/blocked/cache"
if run_service "${TEST_DIR}/blocked" > "${TEST_DIR}/blocked.log" 2>&1; then
    fail "startup accepted a cache path that is a file"
fi
grep -q "Loom cannot create" "${TEST_DIR}/blocked.log" || fail "missing directory-creation error"

if env SYNOPKG_PKGDEST="${TEST_DIR}/package" SYNOPKG_PKGVAR= \
    sh -c '. "$1"; service_prestart' sh "${SERVICE_SETUP}" > "${TEST_DIR}/missing.log" 2>&1; then
    fail "startup accepted a missing package data directory"
fi
grep -q "DSM package data directory is not set" "${TEST_DIR}/missing.log" || fail "missing environment error"

if env SYNOPKG_PKGDEST= SYNOPKG_PKGVAR="${TEST_DIR}/state" \
    sh -c '. "$1"; service_prestart' sh "${SERVICE_SETUP}" > "${TEST_DIR}/missing-dest.log" 2>&1; then
    fail "startup accepted a missing package installation directory"
fi
grep -q "DSM package installation directory is not set" "${TEST_DIR}/missing-dest.log" || fail "missing installation-directory error"

if [ "$(id -u)" -ne 0 ]; then
    run_service "${TEST_DIR}/readonly"
    chmod a-w "${TEST_DIR}/readonly/data"
    if run_service "${TEST_DIR}/readonly" > "${TEST_DIR}/readonly.log" 2>&1; then
        fail "startup accepted a read-only database directory"
    fi
    grep -q "Loom requires write and search permissions" "${TEST_DIR}/readonly.log" || fail "missing permission error"
    chmod u+w "${TEST_DIR}/readonly/data"
else
    echo "SKIP: read-only permission check requires a non-root user"
fi

# Wizard-granted shares become folder-picker roots unless loom.yaml chooses.
mkdir -p "${TEST_DIR}/volume1/media" "${TEST_DIR}/volume1/my downloads" "${TEST_DIR}/shares"
ln -s "${TEST_DIR}/volume1/media" "${TEST_DIR}/shares/media"
ln -s "${TEST_DIR}/volume1/my downloads" "${TEST_DIR}/shares/my downloads"
ln -s "${TEST_DIR}/volume1/missing" "${TEST_DIR}/shares/missing"
real_volume="$(cd "${TEST_DIR}/volume1" && pwd -P)"
env -u LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS SYNOPKG_PKGDEST="${TEST_DIR}/package" SYNOPKG_PKGVAR="${TEST_DIR}/shares-state" \
    LOOM_SYNO_SHARES_DIR="${TEST_DIR}/shares" EXPECTED="${real_volume}/media,${real_volume}/my downloads" sh -eu -c '
        . "$1"
        service_prestart
        [ "${LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS}" = "${EXPECTED}" ]
    ' sh "${SERVICE_SETUP}" || fail "granted shares were not offered as browse roots"

mkdir -p "${TEST_DIR}/yaml-state"
printf 'filesystem:\n  allowed_browse_roots:\n    - /custom\n' > "${TEST_DIR}/yaml-state/loom.yaml"
env -u LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS SYNOPKG_PKGDEST="${TEST_DIR}/package" SYNOPKG_PKGVAR="${TEST_DIR}/yaml-state" \
    LOOM_SYNO_SHARES_DIR="${TEST_DIR}/shares" sh -eu -c '
        . "$1"
        service_prestart
        [ -z "${LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS-}" ]
    ' sh "${SERVICE_SETUP}" || fail "share roots overrode loom.yaml allowed_browse_roots"

env -u LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS SYNOPKG_PKGDEST="${TEST_DIR}/package" SYNOPKG_PKGVAR="${TEST_DIR}/no-shares" \
    LOOM_SYNO_SHARES_DIR="${TEST_DIR}/absent" sh -eu -c '
        . "$1"
        service_prestart
        [ -z "${LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS-}" ]
    ' sh "${SERVICE_SETUP}" || fail "missing shares directory set browse roots"

# The wizard choices are saved for the upgrade wizard to prefill.
mkdir -p "${TEST_DIR}/pkgvar"
env SYNOPKG_PKGVAR="${TEST_DIR}/pkgvar" wizard_media_share="Media" wizard_downloads_share="My Downloads" \
    sh -eu -c '. "$1"; service_postinst' sh "${SERVICE_SETUP}"
grep -qx "MEDIA_SHARE=Media" "${TEST_DIR}/pkgvar/synology-shares.conf" || fail "media share not saved"
grep -qx "DOWNLOADS_SHARE=My Downloads" "${TEST_DIR}/pkgvar/synology-shares.conf" || fail "downloads share not saved"

if command -v python3 > /dev/null 2>&1; then
    WIZARD_DIR="$(dirname "${SERVICE_SETUP}")/wizard"
    cmp -s "${WIZARD_DIR}/install_uifile.sh" "${WIZARD_DIR}/upgrade_uifile.sh" || fail "install and upgrade wizards differ"
    mkdir -p "${TEST_DIR}/packages/loomtest/var"
    cp "${TEST_DIR}/pkgvar/synology-shares.conf" "${TEST_DIR}/packages/loomtest/var/"
    for expect in "media downloads" "Media|My Downloads"; do
        case "${expect}" in
            "media downloads") pkg=loom-none ;;
            *) pkg=loomtest ;;
        esac
        sed "s|/var/packages/|${TEST_DIR}/packages/|" "${WIZARD_DIR}/install_uifile.sh" > "${TEST_DIR}/wizard.sh"
        SYNOPKG_PKGNAME="${pkg}" SYNOPKG_TEMP_LOGFILE="${TEST_DIR}/wizard.json" sh "${TEST_DIR}/wizard.sh"
        python3 - "${TEST_DIR}/wizard.json" "${expect}" <<'PY' || fail "wizard output for ${expect}"
import json, re, sys
page = json.load(open(sys.argv[1]))
fields = {f["key"]: f for f in page[0]["items"][0]["subitems"]}
want = sys.argv[2].split("|") if "|" in sys.argv[2] else sys.argv[2].split()
assert [fields["wizard_media_share"]["defaultValue"], fields["wizard_downloads_share"]["defaultValue"]] == want, fields
rx = re.compile(fields["wizard_media_share"]["validator"]["regex"]["expr"].strip("/"))
assert all(rx.match(n) for n in ("media", "My Downloads", "a")), "valid names rejected"
assert not any(rx.match(n) for n in ("media/movies", "/volume1/media", "")), "paths accepted"
PY
    done
else
    echo "SKIP: wizard JSON check requires python3"
fi

echo "PASS: Synology service setup"
