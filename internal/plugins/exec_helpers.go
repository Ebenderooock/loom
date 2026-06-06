package plugins

import (
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// Optional interfaces an event may implement to expose structured data, mirrored
// from the notifications dispatcher so we stay decoupled from source packages.
type dataProvider interface{ NotificationData() map[string]any }
type titler interface{ GetTitle() string }

// buildPayload converts a bus event into the stdin payload for plugins.
func buildPayload(def EventDef, ev eventbus.Event) Payload {
	data := map[string]any{}
	if dp, ok := ev.(dataProvider); ok {
		for k, v := range dp.NotificationData() {
			data[k] = v
		}
	}
	title := ""
	if t, ok := ev.(titler); ok {
		title = t.GetTitle()
		if title != "" {
			data["title"] = title
		}
	}
	if title == "" {
		if s, ok := data["title"].(string); ok {
			title = s
		}
	}
	return Payload{
		Version:   PayloadVersion,
		Event:     def.Key,
		Topic:     def.Topic,
		Title:     title,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
}

// capWriter accumulates up to limit bytes and silently discards the rest, while
// always reporting a full write so the output copier never blocks.
type capWriter struct {
	mu    sync.Mutex
	buf   []byte
	limit int
	over  bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if remaining := w.limit - len(w.buf); remaining > 0 {
		if len(p) <= remaining {
			w.buf = append(w.buf, p...)
		} else {
			w.buf = append(w.buf, p[:remaining]...)
			w.over = true
		}
	} else if len(p) > 0 {
		w.over = true
	}
	return len(p), nil
}

func (w *capWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := string(w.buf)
	if w.over {
		s += "\n…(truncated)"
	}
	return s
}
