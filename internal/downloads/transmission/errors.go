package transmission

import "errors"

// ErrAuth is returned when Transmission rejects the supplied HTTP
// Basic credentials with a 401. Operators see this most often after
// rotating the RPC password without updating Loom.
var ErrAuth = errors.New("transmission: authentication failed; check the RPC username and password")

// ErrConfig is returned by the factory and parseConfig when the
// persisted Definition is missing fields required to talk to
// Transmission (host, or an unparseable Definition.Config blob).
var ErrConfig = errors.New("transmission: invalid client config")

// ErrUnknownTorrent is returned when an item id (hashString or
// Transmission torrent id) is supplied to Status/Pause/Resume/Remove
// but the daemon reports no matching torrent.
var ErrUnknownTorrent = errors.New("transmission: torrent id is not known to the daemon")

// ErrServer wraps any non-success RPC envelope or non-2xx HTTP
// response that is not specifically modelled. The wrapped error
// includes the RPC method and the upstream "result" string verbatim
// where available so logs are actionable without re-running the call.
var ErrServer = errors.New("transmission: server returned an unexpected response")

// ErrUpstream is a transport- or framing-level failure: connection
// refused, 5xx response, malformed JSON. Distinct from ErrServer so
// callers can retry transport flakes without retrying logical errors.
var ErrUpstream = errors.New("transmission: upstream request failed")

// ErrMissingPayload is returned when an AddRequest carries neither
// Magnet, TorrentURL, nor RawBytes. Transmission's torrent-add accepts
// any of those three, so we surface a typed sentinel rather than
// letting the daemon reject an empty payload.
var ErrMissingPayload = errors.New("transmission: AddRequest has no magnet, torrent URL, or raw bytes")
