package qbittorrent

import "errors"

// ErrAuthFailed is returned when /api/v2/auth/login does not respond
// with a 200 + "Ok." body. Wrapping callers should preserve the
// underlying status text where available.
var ErrAuthFailed = errors.New("qbittorrent: authentication failed; check the username and password")

// ErrUnknownHash is returned when an item id (infohash) is not
// recognised by the qBittorrent server. qBittorrent itself does not
// distinguish "unknown hash" from "no hashes match the filter" on
// /torrents/info, so the client elevates an empty result to this
// error only when the caller asked about specific hashes.
var ErrUnknownHash = errors.New("qbittorrent: torrent hash is not known to the server")

// ErrServer wraps any non-2xx response that is not specifically
// modelled. The wrapped error includes the request path and HTTP
// status code so logs are actionable without re-running the request.
var ErrServer = errors.New("qbittorrent: server returned an unexpected response")

// ErrNotConfigured is returned by the factory when the persisted
// Definition is missing fields required to talk to qBittorrent
// (e.g. host / port).
var ErrNotConfigured = errors.New("qbittorrent: client definition is missing required configuration fields")
