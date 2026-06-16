SPK_NAME = loom
SPK_VERS = 0.1.1
SPK_REV = 1
SPK_ICON = src/loom.png

DEPENDS = cross/loom

# Go 1.26 cannot target these legacy architectures.
UNSUPPORTED_ARCHS = $(PPC_ARCHS) $(ARMv5_ARCHS)

MAINTAINER = Ebenderooock
HOMEPAGE = https://github.com/Ebenderooock/loom
LICENSE = AGPL-3.0
DISPLAY_NAME = Loom

DESCRIPTION = Loom is a unified, self-hosted media automation server. It searches indexers, drives your download clients, imports and organizes movies, TV and music, and ships a built-in web UI and request portal — a single-binary replacement for the Radarr/Sonarr/Lidarr/Prowlarr/Overseerr stack.

CHANGELOG = "0.1.1: Add Music capability (artists/albums/tracks, audio quality profiles, automated and interactive search, music requests via portal and chat bots)."

STARTABLE = yes
SERVICE_USER = auto
SERVICE_SETUP = src/service-setup.sh

# Loom serves its web UI and API over plain HTTP on this port; put it behind the
# DSM reverse proxy for TLS. Default upstream listen port is 1925.
SERVICE_PORT = 1925
SERVICE_PORT_TITLE = Loom Web UI
ADMIN_PORT = $(SERVICE_PORT)
ADMIN_URL = /

FWPORTS = src/loom.sc

include ../../mk/spksrc.spk.mk
