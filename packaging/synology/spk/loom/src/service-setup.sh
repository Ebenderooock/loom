# Loom service definition for SynoCommunity / spksrc.
#
# Loom is a single static binary. All mutable state (loom.yaml config, the
# sqlite database, caches and logs) lives under one writable
# directory. ${SYNOPKG_PKGVAR} is preserved across package upgrades by DSM, so
# pointing both LOOM_CONFIG_DIR and LOOM_DATA_DIR at it gives upgrade-safe,
# self-contained persistence.

SVC_BACKGROUND=y
SVC_WRITE_PID=y

service_prestart ()
{
    # DSM also sources this file during preinst, before these paths are set.
    : "${SYNOPKG_PKGDEST:?DSM package installation directory is not set}"
    : "${SYNOPKG_PKGVAR:?DSM package data directory is not set}"

    SERVICE_COMMAND="${SYNOPKG_PKGDEST}/bin/loom serve"
    export LOOM_CONFIG_DIR="${SYNOPKG_PKGVAR}"
    export LOOM_DATA_DIR="${SYNOPKG_PKGVAR}"
    export LOOM_HTTP_ADDR=":${SERVICE_PORT:-1925}"
    export HOME="${SYNOPKG_PKGVAR}"
    export XDG_CACHE_HOME="${SYNOPKG_PKGVAR}/cache"
    export TMPDIR="${SYNOPKG_PKGVAR}/tmp"

    # Allow download clients in the same group to modify imported media.
    umask 0002

    if ! mkdir -p "${LOOM_DATA_DIR}/data" "${XDG_CACHE_HOME}" "${TMPDIR}"; then
        echo "Loom cannot create its data, cache or temporary directories in ${SYNOPKG_PKGVAR}." >&2
        exit 1
    fi

    for directory in "${SYNOPKG_PKGVAR}" "${LOOM_DATA_DIR}/data" "${XDG_CACHE_HOME}" "${TMPDIR}"; do
        if [ ! -w "${directory}" ] || [ ! -x "${directory}" ]; then
            echo "Loom requires write and search permissions on ${directory}; check the sc-loom package user permissions." >&2
            exit 1
        fi
    done

    loom_configure_shares
}

# Shared folders granted through the wizard's data-share resource appear
# as links in /var/packages/<pkg>/shares. Offer them in the folder picker
# unless loom.yaml or the environment already chooses browse roots.
loom_configure_shares ()
{
    shares_dir="${LOOM_SYNO_SHARES_DIR:-/var/packages/${SYNOPKG_PKGNAME:-loom}/shares}"
    roots=""
    for link in "${shares_dir}"/*; do
        [ -e "${link}" ] || continue
        share=$(realpath "${link}" 2>/dev/null) || continue
        if [ ! -r "${share}" ] || [ ! -w "${share}" ] || [ ! -x "${share}" ]; then
            echo "Warning: sc-loom lacks Read/Write access to ${share}; re-run the package upgrade wizard or grant access in Control Panel > Shared Folder." >&2
        fi
        roots="${roots:+${roots},}${share}"
    done

    if [ -n "${roots}" ] && [ -z "${LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS:-}" ] &&
        ! grep -qs "allowed_browse_roots" "${LOOM_CONFIG_DIR}/loom.yaml"; then
        export LOOM_FILESYSTEM_ALLOWED_BROWSE_ROOTS="${roots}"
    fi
}

# Remember the wizard's shared folders so the upgrade wizard can offer
# them again; DSM re-applies the data-share grant on every upgrade.
service_postinst ()
{
    if [ -n "${wizard_media_share:-}" ] && [ -n "${wizard_downloads_share:-}" ] && [ -n "${SYNOPKG_PKGVAR:-}" ]; then
        printf 'MEDIA_SHARE=%s\nDOWNLOADS_SHARE=%s\n' "${wizard_media_share}" "${wizard_downloads_share}" \
            > "${SYNOPKG_PKGVAR}/synology-shares.conf"
    fi
}

service_postupgrade ()
{
    service_postinst
}
