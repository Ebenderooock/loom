// Package torrent is a built-in BitTorrent client for Loom using anacrolix/torrent.
//
// This is the default in-process torrent engine. It implements DownloadClient,
// DetailProvider, and TorrentManager interfaces, supporting add/status/pause/resume/remove
// with global speed limits, seeding lifecycle, and per-item detail (peers/files/trackers).
//
// Configuration keys in Definition.Config (JSON):
//
//	listen_port (int)               - DHT/peer listen port (default 6881)
//	download_dir (string, required) - where to save completed downloads
//	incomplete_dir (string)         - temporary directory for in-progress (empty = use download_dir)
//	seed_ratio_limit (float)        - stop seeding after this ratio (0 = unlimited)
//	seed_time_limit_minutes (int)   - stop seeding after this many minutes (0 = unlimited)
//	max_connections (int)           - per-torrent connection limit (default 200)
//	max_upload_slots (int)          - per-torrent upload slots (default 50)
//	enable_dht (bool)               - enable DHT (default true)
//	enable_pex (bool)               - enable PEX peer exchange (default true)
//	enable_upnp (bool)              - enable UPnP/NAT-PMP (default false)
//	download_speed_limit (int)      - global download limit in bytes/sec (0 = unlimited)
//	upload_speed_limit (int)        - global upload limit in bytes/sec (0 = unlimited)
//	debug_peer_discovery (bool)     - log peer discovery details to server logs
//
// See docs/downloads.md for operator guide.
package torrent
