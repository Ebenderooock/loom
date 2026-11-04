#!/bin/sh
# DSM install/upgrade wizard. The two shared-folder names feed the
# "data-share" resource in conf/resource, so DSM itself grants sc-loom
# Read/Write on them (creating missing shares) on every install and upgrade.

SAVED="/var/packages/${SYNOPKG_PKGNAME:-loom}/var/synology-shares.conf"
MEDIA_SHARE=media
DOWNLOADS_SHARE=downloads
if [ -r "${SAVED}" ]; then
    saved_media=$(sed -n 's/^MEDIA_SHARE=//p' "${SAVED}" | head -n 1)
    saved_downloads=$(sed -n 's/^DOWNLOADS_SHARE=//p' "${SAVED}" | head -n 1)
    MEDIA_SHARE=${saved_media:-${MEDIA_SHARE}}
    DOWNLOADS_SHARE=${saved_downloads:-${DOWNLOADS_SHARE}}
fi

quote_json ()
{
    printf '%s' "$1" | sed -e 's|\\|\\\\|g' -e 's|"|\\"|g'
}

share_field ()
{
    cat <<FIELD
{
    "key": "$1",
    "desc": "$2",
    "defaultValue": "$(quote_json "$3")",
    "validator": {
        "allowBlank": false,
        "regex": {
            "expr": "/^[\\\\w.][\\\\w. -]{0,30}[\\\\w.-][\\\\\$]?\$|^[\\\\w][\\\\\$]?\$/",
            "errorText": "Enter a shared folder name, not a path or subfolder."
        }
    }
}
FIELD
}

cat > "${SYNOPKG_TEMP_LOGFILE:-/dev/stdout}" <<PAGE
[{
    "step_title": "Shared folders",
    "invalid_next_disabled_v2": true,
    "items": [{
        "type": "textfield",
        "desc": "Loom runs as the system internal user <b>sc-loom</b>. DSM will grant it Read/Write on these shared folders and create any that do not exist. Use two different shared folders. Other folders can still be granted later in Control Panel &gt; Shared Folder.",
        "subitems": [
$(share_field wizard_media_share "Media library shared folder" "${MEDIA_SHARE}"),
$(share_field wizard_downloads_share "Completed downloads shared folder" "${DOWNLOADS_SHARE}")
        ]
    }]
}]
PAGE
