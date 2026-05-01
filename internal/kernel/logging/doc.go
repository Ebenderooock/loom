// Package logging wires Loom's structured logging. It returns a
// *log/slog.Logger configured for JSON or text output and applies a
// redaction pass over a fixed set of sensitive attribute keys
// (password, secret, token, api_key, authorization, cookie, …) so
// credentials never reach the log stream.
//
// Log level and format are read from config.LogConfig and are
// hot-reloadable. See docs/observability.md and ADR-0005.
package logging
