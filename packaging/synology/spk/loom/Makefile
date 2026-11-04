SPK_NAME = loom
SPK_VERS = 0.4.1
SPK_REV = 6
SPK_ICON = src/loom.png

DEPENDS = cross/loom

# Go 1.26 cannot target these legacy architectures.
UNSUPPORTED_ARCHS = $(PPC_ARCHS) $(ARMv5_ARCHS)

MAINTAINER = Ebenderooock
HOMEPAGE = https://github.com/Ebenderooock/loom
LICENSE = AGPL-3.0
DISPLAY_NAME = Loom

DESCRIPTION = Loom is a unified, self-hosted media automation server. It searches indexers, drives your download clients, imports and organizes movies, TV and music, and ships a built-in web UI and request portal — a single-binary replacement for the Radarr/Sonarr/Lidarr/Prowlarr/Overseerr stack.

CHANGELOG = "0.4.1-6: The install and upgrade wizard asks for the media and downloads shared folders, and DSM grants sc-loom Read/Write on them automatically. Granted shares are offered in the folder picker. 0.4.1-2: Fix DSM installation by deferring runtime directory checks until service startup."

STARTABLE = yes
SERVICE_USER = auto
SERVICE_SETUP = src/service-setup.sh

# The wizard asks for a media and a downloads shared folder. DSM's data-share
# worker grants sc-loom Read/Write on both (creating missing shares) at install
# and on every upgrade; package scripts are unprivileged and cannot do this.
WIZARDS_DIR = src/wizard/
SERVICE_WIZARD_SHARENAME = wizard_media_share
POST_SERVICE_TARGET = loom_extra_shares

# Loom serves its web UI and API over plain HTTP on this port; put it behind the
# DSM reverse proxy for TLS. Default upstream listen port is 1925.
SERVICE_PORT = 1925
SERVICE_PORT_TITLE = Loom Web UI
ADMIN_PORT = $(SERVICE_PORT)
ADMIN_URL = /

FWPORTS = src/loom.sc

include ../../mk/spksrc.spk.mk

.PHONY: loom_extra_shares
loom_extra_shares:
	@jq --arg share "{{wizard_downloads_share}}" --arg user sc-$(SPK_USER) \
		'."data-share".shares += [{"name": $$share, "permission": {"rw": [$$user]}}]' \
		$(DSM_CONF_DIR)/resource | sponge $(DSM_CONF_DIR)/resource
