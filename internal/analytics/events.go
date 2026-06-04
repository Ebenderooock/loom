package analytics

import "github.com/ebenderooock/loom/internal/connect"

// Event bus topics for playback activity. The notifications dispatcher maps both
// of these to the on_playback notification event type.
const (
	TopicPlaybackStarted = "analytics.playback.started"
	TopicPlaybackStopped = "analytics.playback.stopped"
)

// PlaybackEvent is emitted when a stream starts or stops. It implements
// eventbus.Event plus the optional NotificationData/GetTitle interfaces the
// notification dispatcher uses to render messages.
type PlaybackEvent struct {
	topic          string
	ConnectionName string
	Provider       string
	User           string
	MediaType      string
	FullTitle      string
	Device         string
	Transcode      bool
	BitrateKbps    int64
	WatchedMs      int64
}

func newPlaybackEvent(topic, connectionName string, rec HistoryRecord) *PlaybackEvent {
	return &PlaybackEvent{
		topic:          topic,
		ConnectionName: connectionName,
		Provider:       rec.Provider,
		User:           rec.User,
		MediaType:      rec.MediaType,
		FullTitle:      rec.FullTitle,
		Device:         rec.Device,
		Transcode:      rec.Transcode,
		BitrateKbps:    rec.BitrateKbps,
		WatchedMs:      rec.WatchedMs,
	}
}

// Topic implements eventbus.Event.
func (e *PlaybackEvent) Topic() string { return e.topic }

// GetTitle implements the dispatcher's optional titler interface.
func (e *PlaybackEvent) GetTitle() string { return e.FullTitle }

// NotificationData implements the dispatcher's optional dataProvider interface.
func (e *PlaybackEvent) NotificationData() map[string]any {
	return map[string]any{
		"title":        e.FullTitle,
		"user":         e.User,
		"device":       e.Device,
		"server":       e.ConnectionName,
		"provider":     e.Provider,
		"media_type":   e.MediaType,
		"transcode":    e.Transcode,
		"bitrate_kbps": e.BitrateKbps,
		"watched_ms":   e.WatchedMs,
	}
}

// liveStreamFrom builds a snapshot LiveStream from a connect session.
func liveStreamFrom(sess connect.Session, connName string) LiveStream {
	return LiveStream{
		Session:        sess,
		ConnectionName: connName,
		Progress:       progress(sess.PositionMs, sess.DurationMs),
	}
}
