package logging

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// LogEntry represents a single captured log record.
type LogEntry struct {
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	Source     string `json:"source,omitempty"`
	Attrs      string `json:"attrs,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

// RingBuffer is a thread-safe circular buffer for log entries with
// subscriber fan-out for real-time streaming.
type RingBuffer struct {
	mu      sync.Mutex
	entries []LogEntry
	size    int
	head    int // next write position
	count   int
	seq     uint64 // monotonic sequence for cursors

	subMu       sync.Mutex
	subscribers map[uint64]chan LogEntry
	nextSubID   uint64
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 5000
	}
	return &RingBuffer{
		entries:     make([]LogEntry, capacity),
		size:        capacity,
		subscribers: make(map[uint64]chan LogEntry),
	}
}

// Write adds an entry to the buffer and fans out to subscribers.
func (rb *RingBuffer) Write(entry LogEntry) {
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	rb.mu.Lock()
	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
	atomic.AddUint64(&rb.seq, 1)
	rb.mu.Unlock()

	// Fan-out to subscribers (non-blocking, drop on slow consumer).
	rb.subMu.Lock()
	for _, ch := range rb.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	rb.subMu.Unlock()
}

// Read returns up to limit entries. If workflowID is non-empty, only
// entries matching that workflow are returned.
func (rb *RingBuffer) Read(limit int, workflowID string) []LogEntry {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return nil
	}
	if limit <= 0 || limit > rb.count {
		limit = rb.count
	}

	// Walk backwards from most recent to collect entries.
	result := make([]LogEntry, 0, limit)
	pos := (rb.head - 1 + rb.size) % rb.size
	for i := 0; i < rb.count && len(result) < limit; i++ {
		e := rb.entries[pos]
		if workflowID == "" || e.WorkflowID == workflowID {
			result = append(result, e)
		}
		pos = (pos - 1 + rb.size) % rb.size
	}

	// Reverse to chronological order.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// Subscribe returns a channel that receives new log entries in real time.
// Call Unsubscribe with the returned ID when done.
func (rb *RingBuffer) Subscribe(bufSize int) (uint64, <-chan LogEntry) {
	if bufSize <= 0 {
		bufSize = 256
	}
	ch := make(chan LogEntry, bufSize)

	rb.subMu.Lock()
	id := rb.nextSubID
	rb.nextSubID++
	rb.subscribers[id] = ch
	rb.subMu.Unlock()

	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (rb *RingBuffer) Unsubscribe(id uint64) {
	rb.subMu.Lock()
	if ch, ok := rb.subscribers[id]; ok {
		delete(rb.subscribers, id)
		close(ch)
	}
	rb.subMu.Unlock()
}

// Len returns the current number of entries in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}
