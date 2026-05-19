package searchdebug

import (
	"sync"
)

// Hub broadcasts StatusUpdate events to connected SSE clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan StatusUpdate]struct{}
}

// NewHub creates a new broadcast hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[chan StatusUpdate]struct{}),
	}
}

// Subscribe registers a new client channel. The caller must call
// Unsubscribe when done. The channel is buffered to avoid blocking
// the publisher on slow clients.
func (h *Hub) Subscribe() chan StatusUpdate {
	ch := make(chan StatusUpdate, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes and closes a client channel.
func (h *Hub) Unsubscribe(ch chan StatusUpdate) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// Publish sends an update to all connected clients. Slow clients
// that can't keep up will have their message dropped (non-blocking send).
func (h *Hub) Publish(u StatusUpdate) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- u:
		default:
			// Client too slow — drop the message.
		}
	}
}

// MakeStatusUpdate creates a lightweight StatusUpdate from a full Entry.
func MakeStatusUpdate(e *Entry) StatusUpdate {
	return StatusUpdate{
		ID:            e.ID,
		Status:        e.Status,
		Outcome:       e.Outcome,
		Title:         e.Title,
		MediaType:     e.MediaType,
		Season:        e.Season,
		Episode:       e.Episode,
		SearchRunID:   e.SearchRunID,
		TotalResults:  e.TotalResults,
		TotalRejected: e.TotalRejected,
		DurationMS:    e.DurationMS,
		ErrorMessage:  e.ErrorMessage,
	}
}
