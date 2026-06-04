package bots

import (
	"context"
	"log/slog"
	"sync"
)

// Transport is a running bot connection for a single platform.
type Transport interface {
	// Run blocks serving the platform until ctx is cancelled.
	Run(ctx context.Context) error
	// LastError reports the most recent transport error, for health.
	LastError() string
}

// TransportFactory builds a Transport for a platform from a bot token.
type TransportFactory func(token string) Transport

// Supervisor starts and stops platform transports to match the stored config,
// restarting a transport when its token changes and ensuring a stopped
// transport has fully exited before a replacement starts (no duplicate
// consumers).
type Supervisor struct {
	store       *Store
	newTelegram TransportFactory
	newDiscord  TransportFactory
	logger      *slog.Logger

	mu   sync.Mutex
	base context.Context
	tg   *running
	dc   *running
}

type running struct {
	token  string
	cancel context.CancelFunc
	done   chan struct{}
	t      Transport
}

// NewSupervisor constructs a bot supervisor. Transport factories are injected so
// the package does not import the concrete transports (avoiding import cycles)
// and so tests can supply fakes.
func NewSupervisor(store *Store, telegram, discord TransportFactory, logger *slog.Logger) *Supervisor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Supervisor{store: store, newTelegram: telegram, newDiscord: discord, logger: logger}
}

// Start records the base (application-lifetime) context and applies the current
// configuration.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	s.base = ctx
	s.mu.Unlock()
	return s.Reload(ctx)
}

// Reload re-reads the configuration and reconciles the running transports.
func (s *Supervisor) Reload(ctx context.Context) error {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base == nil {
		s.base = ctx
	}
	tgToken := ""
	if cfg.TelegramEnabled {
		tgToken = cfg.TelegramBotToken
	}
	dcToken := ""
	if cfg.DiscordEnabled {
		dcToken = cfg.DiscordBotToken
	}
	s.reconcile(&s.tg, "telegram", tgToken, s.newTelegram)
	s.reconcile(&s.dc, "discord", dcToken, s.newDiscord)
	return nil
}

// reconcile brings a single platform slot in line with the desired token.
// Caller must hold s.mu.
func (s *Supervisor) reconcile(cur **running, name, token string, factory TransportFactory) {
	existing := *cur
	if existing != nil && existing.token == token {
		return
	}
	if existing != nil {
		existing.cancel()
		<-existing.done
		*cur = nil
	}
	if token == "" || factory == nil {
		return
	}
	ctx, cancel := context.WithCancel(s.base)
	tr := factory(token)
	done := make(chan struct{})
	r := &running{token: token, cancel: cancel, done: done, t: tr}
	go func() {
		defer close(done)
		if err := tr.Run(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("bots: transport exited unexpectedly", "platform", name, "err", err)
		}
	}()
	*cur = r
	s.logger.Info("bots: transport started", "platform", name)
}

// Shutdown stops all running transports and waits for them to exit.
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range []*running{s.tg, s.dc} {
		if r != nil {
			r.cancel()
			<-r.done
		}
	}
	s.tg, s.dc = nil, nil
}

// PlatformStatus reports the health of one platform.
type PlatformStatus struct {
	Platform  Platform `json:"platform"`
	Running   bool     `json:"running"`
	LastError string   `json:"last_error,omitempty"`
}

// Status reports the current state of each platform transport.
func (s *Supervisor) Status() []PlatformStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []PlatformStatus{
		statusOf(PlatformTelegram, s.tg),
		statusOf(PlatformDiscord, s.dc),
	}
}

func statusOf(p Platform, r *running) PlatformStatus {
	if r == nil {
		return PlatformStatus{Platform: p, Running: false}
	}
	return PlatformStatus{Platform: p, Running: true, LastError: r.t.LastError()}
}
