package torrent

import "errors"

// ErrNotConfigured is returned by the factory when the persisted
// Definition is missing fields required to run the built-in torrent
// engine (e.g. download_dir).
var ErrNotConfigured = errors.New("builtin/torrent: not configured")

// ErrEngineNotRunning is returned when the shared engine has not been
// started or has already been shut down.
var ErrEngineNotRunning = errors.New("builtin/torrent: engine not running")

// ErrTorrentNotFound is returned when a requested infohash is not
// tracked by the engine.
var ErrTorrentNotFound = errors.New("builtin/torrent: torrent not found")

// ErrMetadataTimeout is returned when waiting for torrent metadata
// (e.g. from a magnet link) exceeds the context deadline.
var ErrMetadataTimeout = errors.New("builtin/torrent: metadata resolution timed out")

// ErrInvalidInput is returned when the caller provides data that
// cannot be parsed as a valid torrent or magnet URI.
var ErrInvalidInput = errors.New("builtin/torrent: invalid input")
