package analytics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/connect"
	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// streamingProviders are the connection types that report playback sessions.
var streamingProviders = map[connect.ProviderType]bool{
	connect.ProviderPlex:     true,
	connect.ProviderEmby:     true,
	connect.ProviderJellyfin: true,
}

// ConnectionSource supplies the configured media-server connections.
type ConnectionSource interface {
	ListConnections(ctx context.Context) ([]*connect.Connection, error)
}

// SessionFetcher fetches active sessions for a single connection. It is a field
// on the service so tests can substitute a fake.
type SessionFetcher func(ctx context.Context, conn *connect.Connection) ([]connect.Session, error)

// defaultFetcher uses the connect provider registry.
func defaultFetcher(ctx context.Context, conn *connect.Connection) ([]connect.Session, error) {
	p, err := connect.ProviderFor(conn.Provider)
	if err != nil {
		return nil, err
	}
	return p.ActiveSessions(ctx, conn.Settings)
}

// Service is the analytics business logic: it owns the live snapshot and the
// persisted history store.
type Service struct {
	store   *Store
	conns   ConnectionSource
	fetch   SessionFetcher
	bus     eventbus.Bus
	logger  *slog.Logger
	minPlay time.Duration // minimum watched time to count a play in stats

	mu       sync.RWMutex
	snapshot []LiveStream

	// baselined is false until the first Sample completes; the baseline pass
	// suppresses start events for already-running streams.
	baselined bool
}

// NewService creates an analytics service. bus may be nil (playback start/stop
// notifications are then disabled).
func NewService(store *Store, conns ConnectionSource, bus eventbus.Bus, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:    store,
		conns:    conns,
		fetch:    defaultFetcher,
		bus:      bus,
		logger:   logger.With("module", "analytics"),
		minPlay:  60 * time.Second,
		snapshot: []LiveStream{},
	}
}

// ActiveStreams returns the most recent live-stream snapshot.
func (s *Service) ActiveStreams() []LiveStream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LiveStream, len(s.snapshot))
	copy(out, s.snapshot)
	return out
}

// History returns persisted playback rows.
func (s *Service) History(ctx context.Context, f HistoryFilter) ([]HistoryRecord, error) {
	return s.store.ListHistory(ctx, f)
}

// ClearHistory removes all persisted analytics play history.
func (s *Service) ClearHistory(ctx context.Context) error {
	return s.store.ClearHistory(ctx)
}

// Stats returns the analytics report for the given window in days.
func (s *Service) Stats(ctx context.Context, windowDays int) (*Stats, error) {
	if windowDays <= 0 {
		windowDays = 30
	}
	since := time.Now().UTC().AddDate(0, 0, -windowDays)
	return s.store.Stats(ctx, since, windowDays, s.minPlay.Milliseconds())
}

// ResetOrphans closes any open rows left over from a previous run. Call once at
// startup before sampling begins.
func (s *Service) ResetOrphans(ctx context.Context) {
	n, err := s.store.CloseAllOpen(ctx, "startup_reap")
	if err != nil {
		s.logger.Warn("analytics: failed to close orphan rows", "error", err)
		return
	}
	if n > 0 {
		s.logger.Info("analytics: closed orphaned play rows on startup", "count", n)
	}
}

// sessionKeyOf identifies an active session within a connection.
type sessionKey struct {
	session string
	media   string
}

// Sample performs one polling cycle: fetch active sessions from each enabled
// streaming connection, update the live snapshot, and persist history. Sampling
// is isolated per connection so one unreachable server can't corrupt others'
// history or hide their streams.
func (s *Service) Sample(ctx context.Context, interval time.Duration) {
	conns, err := s.conns.ListConnections(ctx)
	if err != nil {
		s.logger.Warn("analytics: list connections failed", "error", err)
		return
	}

	type result struct {
		conn     *connect.Connection
		sessions []connect.Session
		ok       bool
	}

	var targets []*connect.Connection
	for _, c := range conns {
		if c.Enabled && streamingProviders[c.Provider] {
			targets = append(targets, c)
		}
	}

	results := make([]result, len(targets))
	var wg sync.WaitGroup
	for i, c := range targets {
		wg.Add(1)
		go func(i int, c *connect.Connection) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, interval-time.Second)
			defer cancel()
			sessions, err := s.fetch(cctx, c)
			if err != nil {
				s.logger.Debug("analytics: sample connection failed", "connection", c.Name, "error", err)
				results[i] = result{conn: c, ok: false}
				return
			}
			results[i] = result{conn: c, sessions: sessions, ok: true}
		}(i, c)
	}
	wg.Wait()

	now := time.Now().UTC()
	grace := 2 * interval

	// The first sample after startup is a baseline: ResetOrphans has just closed
	// all open rows, so every active session looks "new". Suppress start events
	// for that pass so a restart doesn't spam "Playback Started" for streams that
	// were already running.
	suppressStarts := !s.baselined
	s.baselined = true

	// Preserve previous snapshot entries for connections that failed this tick
	// (mark stale by keeping them) and rebuild from successful ones.
	prev := s.ActiveStreams()
	prevByConn := map[string][]LiveStream{}
	for _, ls := range prev {
		prevByConn[ls.ConnectionID] = append(prevByConn[ls.ConnectionID], ls)
	}

	newSnapshot := []LiveStream{}
	var events []*PlaybackEvent
	for _, res := range results {
		if !res.ok {
			// Keep last-known streams for this connection rather than dropping.
			newSnapshot = append(newSnapshot, prevByConn[res.conn.ID]...)
			continue
		}
		streams, evs := s.persistConnection(ctx, res.conn, res.sessions, now, grace, suppressStarts)
		newSnapshot = append(newSnapshot, streams...)
		events = append(events, evs...)
	}

	s.mu.Lock()
	s.snapshot = newSnapshot
	s.mu.Unlock()

	// Publish playback start/stop notifications after persistence succeeds, so a
	// notification only ever corresponds to a real recorded state change.
	s.publish(ctx, events)
}

// publish emits collected playback events on the bus (best-effort, nil-safe).
func (s *Service) publish(ctx context.Context, events []*PlaybackEvent) {
	if s.bus == nil || len(events) == 0 {
		return
	}
	for _, ev := range events {
		if err := s.bus.Publish(ctx, ev); err != nil {
			s.logger.Warn("analytics: publish playback event failed", "topic", ev.Topic(), "error", err)
		}
	}
}

// persistConnection reconciles one connection's active sessions against its
// open history rows and returns the live streams for the snapshot plus any
// playback start/stop events to publish.
func (s *Service) persistConnection(ctx context.Context, c *connect.Connection, sessions []connect.Session, now time.Time, grace time.Duration, suppressStarts bool) ([]LiveStream, []*PlaybackEvent) {
	open, err := s.store.OpenRowsForConn(ctx, c.ID)
	if err != nil {
		s.logger.Warn("analytics: load open rows failed", "connection", c.Name, "error", err)
	}
	openByKey := map[sessionKey]*HistoryRecord{}
	for i := range open {
		k := sessionKey{session: open[i].SessionKey, media: open[i].MediaID}
		openByKey[k] = &open[i]
	}

	active := map[sessionKey]bool{}
	streams := make([]LiveStream, 0, len(sessions))
	var events []*PlaybackEvent

	for _, sess := range sessions {
		k := sessionKey{session: sess.SessionKey, media: sess.MediaID}
		active[k] = true

		if row, ok := openByKey[k]; ok {
			// Accumulate watched time only while playing, capping the per-tick
			// delta so a missed poll or a long gap can't inflate totals.
			watched := row.WatchedMs
			if sess.State == "playing" {
				delta := now.Sub(row.LastSeenAt)
				if delta < 0 {
					delta = 0
				}
				if delta > grace {
					delta = grace
				}
				watched += delta.Milliseconds()
			}
			if err := s.store.UpdateOpen(ctx, row.ID, now, sess.PositionMs, watched, sess.Transcode, sess.BitrateKbps); err != nil {
				s.logger.Warn("analytics: update row failed", "error", err)
			} else {
				row.WatchedMs = watched
			}
		} else {
			rec := HistoryRecord{
				ID:               uuid.NewString(),
				ConnectionID:     c.ID,
				Provider:         string(c.Provider),
				SessionKey:       sess.SessionKey,
				MediaID:          sess.MediaID,
				User:             sess.User,
				MediaType:        sess.MediaType,
				Title:            sess.Title,
				GrandparentTitle: sess.GrandparentTitle,
				FullTitle:        sess.FullTitle,
				Device:           sess.Device,
				Transcode:        sess.Transcode,
				StartedAt:        now,
				LastSeenAt:       now,
				LastPositionMs:   sess.PositionMs,
				DurationMs:       sess.DurationMs,
				WatchedMs:        0,
				BitrateKbps:      sess.BitrateKbps,
			}
			if err := s.store.InsertOpen(ctx, rec); err != nil {
				s.logger.Warn("analytics: insert row failed", "error", err)
			} else if !suppressStarts {
				events = append(events, newPlaybackEvent(TopicPlaybackStarted, c.Name, rec))
			}
		}

		sess.ConnectionID = c.ID
		sess.Provider = c.Provider
		streams = append(streams, liveStreamFrom(sess, c.Name))
	}

	// Reap: close open rows for this connection that are no longer active and
	// whose last sighting is older than the grace window.
	cutoff := now.Add(-grace)
	for _, row := range open {
		k := sessionKey{session: row.SessionKey, media: row.MediaID}
		if active[k] {
			continue
		}
		if row.LastSeenAt.Before(cutoff) {
			closed, err := s.store.Close(ctx, row.ID, row.LastSeenAt, "disappeared")
			if err != nil {
				s.logger.Warn("analytics: close row failed", "error", err)
			} else if closed {
				events = append(events, newPlaybackEvent(TopicPlaybackStopped, c.Name, row))
			}
		}
	}

	return streams, events
}

func progress(pos, dur int64) float64 {
	if dur <= 0 {
		return 0
	}
	p := float64(pos) / float64(dur) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}
