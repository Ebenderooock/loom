package deluge

import "errors"

// ErrAuthFailed is returned when auth.login does not return true,
// or when auth.check_session reports the cookie has expired and a
// fresh login also fails. Wrapping callers should preserve the
// underlying RPC error message where available.
var ErrAuthFailed = errors.New("deluge: authentication failed; check the Web UI password")

// ErrDaemonNotConnected is returned by Test when the Web UI is
// reachable but is not currently connected to a Deluge daemon.
// Operators see this when running deluge-web standalone without
// auto-connect configured.
var ErrDaemonNotConnected = errors.New("deluge: Web UI is not connected to a daemon; configure auto-connect or pick a host in the UI")

// ErrLabelPluginMissing is returned by Categories when the Label
// plugin is not enabled on the daemon. Categories surfaces this as
// an empty list rather than an error so dashboards do not break;
// Test surfaces it as a soft warning when present.
var ErrLabelPluginMissing = errors.New("deluge: Label plugin is not enabled; enable it in the daemon to use categories")

// ErrRPC wraps a non-nil JSON-RPC error envelope. The message
// includes the RPC method name, error code, and the daemon's
// description so log lines are actionable without re-running.
var ErrRPC = errors.New("deluge: RPC call returned an error")

// ErrServer wraps any HTTP-level failure that does not fit the
// auth or RPC categories (e.g. 5xx, malformed JSON envelope).
var ErrServer = errors.New("deluge: server returned an unexpected response")

// ErrUnknownHash is returned when Status is asked about specific
// hashes and the server reports none of them exist.
var ErrUnknownHash = errors.New("deluge: torrent hash is not known to the server")

// ErrNotConfigured is returned by the factory when the persisted
// Definition is missing fields required to talk to Deluge
// (e.g. host, password).
var ErrNotConfigured = errors.New("deluge: client definition is missing required configuration fields")
