// Package plugins implements custom post-processing plugins: an admin configures
// a JavaScript program that Loom runs whenever a subscribed domain event fires
// (a grab, an import, a playback, ...). It is the rough equivalent of
// Sonarr/Radarr "Custom Scripts", but scripts are written in JavaScript and run
// in an embedded interpreter (goja) rather than as arbitrary OS commands.
//
// The script is given an `event` object (the payload), an `env` object (the
// plugin's configured key/values), a `console` (captured into run history), and
// a synchronous `fetch(url, opts)` for HTTP calls. There is intentionally no way
// to spawn processes or read arbitrary files: the host API surface is the
// boundary.
//
// Trust model: scripts run IN-PROCESS as the Loom server. CPU is bounded (the VM
// is interrupted on timeout/cancel), recursion is capped, output is size-capped,
// and host panics are recovered, so a buggy or looping script cannot hang or
// crash the server. However, because scripts share the server's heap, a script
// that allocates without bound can still pressure memory. Plugins are a
// convenience for trusted admins, not a security sandbox; for hard isolation,
// run Loom itself under container/Kubernetes resource limits.
package plugins

import "time"

// Plugin is an admin-configured JavaScript program invoked on subscribed events.
type Plugin struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Source      string            `json:"source"`       // JavaScript source executed on each event.
	Events      []string          `json:"events"`       // SupportedEvent keys this plugin subscribes to.
	Env         map[string]string `json:"env"`          // values exposed to the script as the `env` object.
	TimeoutSecs int               `json:"timeout_secs"` // per-run wall-clock budget.
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Run is a single recorded execution of a plugin (history).
type Run struct {
	ID         string    `json:"id"`
	PluginID   string    `json:"plugin_id"`
	PluginName string    `json:"plugin_name"`
	Topic      string    `json:"topic"`
	Success    bool      `json:"success"`
	ExitCode   int       `json:"exit_code"`
	DurationMs int64     `json:"duration_ms"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	ErrorMsg   string    `json:"error_msg"`
	StartedAt  time.Time `json:"started_at"`
}

// EventDef is a curated event a plugin may subscribe to. Keys are stable and
// map to an internal event bus topic; the data exposed for each is treated as
// a stable contract for plugin authors.
type EventDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Topic string `json:"topic"`
}

// Bus topic strings are duplicated here (as in the notifications dispatcher) to
// avoid import cycles with the downloads/imports/analytics packages.
const (
	topicDownloadQueued    = "downloads.queued"
	topicDownloadCompleted = "downloads.completed"
	topicDownloadFailed    = "downloads.failed"
	topicImportCompleted   = "imports.completed"
	topicImportFailed      = "imports.failed"
	topicPlaybackStarted   = "analytics.playback.started"
	topicPlaybackStopped   = "analytics.playback.stopped"
)

// SupportedEvents is the curated allow-list of events plugins may run on. We do
// not expose arbitrary internal topics, both to keep the contract stable and to
// avoid leaking event data that was never reviewed for external consumption.
var SupportedEvents = []EventDef{
	{Key: "grab", Label: "On Grab (download queued)", Topic: topicDownloadQueued},
	{Key: "download_complete", Label: "On Download Complete", Topic: topicDownloadCompleted},
	{Key: "download_failed", Label: "On Download Failed", Topic: topicDownloadFailed},
	{Key: "import_complete", Label: "On Import Complete", Topic: topicImportCompleted},
	{Key: "import_failed", Label: "On Import Failed", Topic: topicImportFailed},
	{Key: "playback_started", Label: "On Playback Started", Topic: topicPlaybackStarted},
	{Key: "playback_stopped", Label: "On Playback Stopped", Topic: topicPlaybackStopped},
}

// eventByTopic maps a bus topic back to its supported-event key.
func eventByTopic(topic string) (EventDef, bool) {
	for _, e := range SupportedEvents {
		if e.Topic == topic {
			return e, true
		}
	}
	return EventDef{}, false
}

// eventByKey returns the EventDef for a supported-event key.
func eventByKey(key string) (EventDef, bool) {
	for _, e := range SupportedEvents {
		if e.Key == key {
			return e, true
		}
	}
	return EventDef{}, false
}

// Payload is the JSON document delivered to a plugin on stdin.
type Payload struct {
	Version   int            `json:"version"`
	Event     string         `json:"event"` // supported-event key
	Topic     string         `json:"topic"`
	Title     string         `json:"title"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
}

// PayloadVersion is the schema version embedded in every Payload.
const PayloadVersion = 1
