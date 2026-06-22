package torrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	dht "github.com/anacrolix/dht/v2"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"github.com/ebenderooock/loom/internal/diskspace"
)

// metadataTimeout is deprecated; use Config.MetadataTimeoutSecs instead.
// This constant is kept for reference; the default is now 180 seconds
// in DefaultConfig() to better support Kubernetes/NAT environments.
// Can be overridden via LOOM_TORRENT_METADATA_TIMEOUT_SECS env var.
const metadataTimeout = 60 * time.Second

// defaultTrackers is a curated list of reliable public BitTorrent
// trackers used to bootstrap peer discovery for magnet links. In
// NAT'd/containerised environments (e.g. Kubernetes) inbound peer
// connectivity and DHT responsiveness are limited, so announcing to
// well-known trackers is the most reliable way to find peers and pull
// metadata. anacrolix dedups these against any trackers already present
// in the magnet, so injecting them is safe.
//
// HTTP/HTTPS (TCP) trackers are listed first and deliberately
// prioritised: many container/NAT networks silently drop outbound UDP
// (and the UDP-based DHT along with it), which leaves UDP-only tracker
// announces timing out and magnets stuck "queued" with zero peers
// because metadata can never be pulled. TCP-based HTTP(S) trackers
// announce reliably over the same egress that already serves normal
// HTTPS traffic, so they find peers even when UDP is unavailable. The
// UDP entries are kept as a best-effort fallback for deployments where
// UDP egress does work.
var defaultTrackers = []string{
	// TCP (HTTP/HTTPS) trackers — work even when outbound UDP is blocked.
	// Verified reachable + returning live swarms from a UDP-blocked
	// Kubernetes cluster (opentrackr 443/1337 and bt4g responded with
	// real seeder/leecher counts).
	"https://tracker.opentrackr.org:443/announce",
	"http://tracker.opentrackr.org:1337/announce",
	"https://tracker.bt4g.com:443/announce",
	"https://tracker.tamersunion.org:443/announce",
	"https://tracker1.520.jp:443/announce",
	"https://opentracker.i2p.rocks:443/announce",
	"https://tracker.gbitt.info:443/announce",
	"http://tracker.gbitt.info:80/announce",
	"http://open.acgnxtracker.com:80/announce",
	"http://bt.okmp3.ru:2710/announce",

	// UDP trackers — best-effort fallback when UDP egress is available.
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://tracker.dler.org:6969/announce",
	"udp://p4p.arenabg.com:1337/announce",
}

// defaultDHTBootstrapHostPorts augments the library defaults with
// additional stable public bootstrap routers.
var defaultDHTBootstrapHostPorts = []string{
	"router.bittorrent.com:6881",
	"router.utorrent.com:6881",
	"dht.transmissionbt.com:6881",
	"dht.libtorrent.org:25401",
}

// defaultAnnounceList wraps each tracker in its own tier so anacrolix
// announces to all of them in parallel.
func defaultAnnounceList() [][]string {
	list := make([][]string, len(defaultTrackers))
	for i, tr := range defaultTrackers {
		list[i] = []string{tr}
	}
	return list
}

func dhtStartingNodes(network string) dht.StartingNodesGetter {
	return func() ([]dht.Addr, error) {
		hostPorts := append([]string{}, dht.DefaultGlobalBootstrapHostPorts...)
		hostPorts = append(hostPorts, defaultDHTBootstrapHostPorts...)
		addrs, err := dht.ResolveHostPorts(hostPorts)
		if err == nil && len(addrs) > 0 {
			return addrs, nil
		}
		return dht.GlobalBootstrapAddrs(network)
	}
}

// magnetHasTrackers reports whether a magnet URI already carries one or
// more tracker (tr) parameters. Private-tracker magnets always do, so we
// use this to avoid announcing private infohashes to public trackers.
func magnetHasTrackers(magnet string) bool {
	u, err := url.Parse(magnet)
	if err != nil {
		return false
	}
	return len(u.Query()["tr"]) > 0
}

// credentialQueryKeys are tracker-announce query parameters that carry a
// per-user secret. Their presence marks a tracker (and therefore the
// torrent) as belonging to a private tracker, which must never be
// announced to public trackers.
var credentialQueryKeys = []string{
	"passkey", "authkey", "auth", "token", "secret",
	"torrent_pass", "apikey", "api_key", "pid",
}

// magnetLikelyPrivate reports whether a magnet's existing trackers
// indicate a private tracker. It is deliberately conservative: any
// signal of an embedded credential causes the magnet to be treated as
// private. A false result means "safe to augment with public trackers".
// An unparseable magnet is treated as private so we never inject blindly.
func magnetLikelyPrivate(magnet string) bool {
	u, err := url.Parse(magnet)
	if err != nil {
		return true
	}
	for _, tr := range u.Query()["tr"] {
		if trackerHasCredential(tr) {
			return true
		}
	}
	return false
}

// trackerHasCredential reports whether a single tracker announce URL
// embeds a per-user secret, either as a known credential query
// parameter or as a passkey-like path segment. Unparseable URLs are
// treated as credentialed (private) to stay on the safe side.
func trackerHasCredential(tracker string) bool {
	tu, err := url.Parse(tracker)
	if err != nil {
		return true
	}
	q := tu.Query()
	for _, k := range credentialQueryKeys {
		if q.Get(k) != "" {
			return true
		}
	}
	for _, seg := range strings.Split(tu.Path, "/") {
		if looksLikePasskey(seg) {
			return true
		}
	}
	return false
}

// looksLikePasskey reports whether a URL path segment looks like an
// embedded private-tracker passkey: a long, purely alphanumeric token.
// Public announce paths use short words ("announce"), so a long
// alphanumeric segment is a strong private-tracker signal.
func looksLikePasskey(seg string) bool {
	if len(seg) < 20 {
		return false
	}
	for _, r := range seg {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// seedCheckInterval controls how often the seeding supervisor scans
// all tracked torrents to enforce ratio/time policies.
const seedCheckInterval = 30 * time.Second

// pausedConns is the connection count assigned to paused torrents.
// Setting it to zero effectively stops all peer traffic.
const pausedConns = 0

// activeConns is restored when a torrent is resumed. Matches a
// reasonable per-torrent default.
const activeConns = 50

// SeedPolicy captures per-torrent seeding limits.
type SeedPolicy struct {
	RatioLimit       float64
	TimeLimitMinutes int
}

// torrentMeta carries Loom-specific metadata that does not come from
// the torrent itself.
type torrentMeta struct {
	Title      string
	Category   string
	SavePath   string
	SeedPolicy SeedPolicy

	// ExpectedInfohash, when set, is compared against the infohash of a
	// fetched .torrent file before it is added. This guards against an
	// indexer download URL that resolves to the wrong (stale, redirected,
	// or malicious) torrent. Empty disables the check.
	ExpectedInfohash string
}

// trackedTorrent pairs the anacrolix torrent handle with Loom metadata.
type trackedTorrent struct {
	t           *torrent.Torrent
	title       string
	category    string
	savePath    string
	addedAt     time.Time
	seedStartAt *time.Time
	seedPolicy  SeedPolicy
	paused      bool
	movedToDest bool // true once files have been moved from IncompleteDir → DownloadDir
	// nolint:unused // Fields kept for future ratio calculation implementation
	downloaded int64 // snapshot for ratio calculation
	uploaded   int64 // nolint:unused

	// Speed tracking — computed from byte deltas between Status() calls.
	lastSpeedSampleAt time.Time
	lastBytesRead     int64
	lastBytesWritten  int64
	downloadRate      int64 // bytes/sec
	uploadRate        int64 // bytes/sec
}

// Engine wraps a single anacrolix/torrent.Client and manages its
// lifecycle, including a seeding supervisor goroutine.
type Engine struct {
	mu     sync.RWMutex
	client *torrent.Client
	cfg    Config
	logger *slog.Logger
	items  map[string]*trackedTorrent // keyed by lowercase infohash hex
	cancel context.CancelFunc
	// engineCtx is the lifecycle context started by Start(). It is used
	// for metadata-resolution waits so they are tied to the engine's
	// lifetime rather than to the caller's (e.g. HTTP request) context.
	// nil until Start() is called; falls back to context.Background().
	engineCtx context.Context
	dataDir   string

	// Live rate limiters shared with the anacrolix client config so the
	// global download/upload caps can be changed at runtime.
	downLimiter *rate.Limiter
	upLimiter   *rate.Limiter
}

// rateLimiterBurst is the minimum token-bucket burst applied to a finite
// rate limiter so individual piece requests are never starved.
const rateLimiterBurst = 1 << 20 // 1 MiB

// newRateLimiter builds a rate limiter for the given cap in bytes/sec.
// A value <= 0 means unlimited.
func newRateLimiter(bytesPerSec int64) *rate.Limiter {
	if bytesPerSec <= 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	burst := bytesPerSec
	if burst < rateLimiterBurst {
		burst = rateLimiterBurst
	}
	return rate.NewLimiter(rate.Limit(bytesPerSec), int(burst))
}

// applyRateLimit updates a live limiter to the given cap in bytes/sec.
func applyRateLimit(l *rate.Limiter, bytesPerSec int64) {
	if l == nil {
		return
	}
	if bytesPerSec <= 0 {
		l.SetLimit(rate.Inf)
		l.SetBurst(0)
		return
	}
	burst := bytesPerSec
	if burst < rateLimiterBurst {
		burst = rateLimiterBurst
	}
	l.SetLimit(rate.Limit(bytesPerSec))
	l.SetBurst(int(burst))
}

// NewEngine creates the anacrolix torrent client with the supplied
// Config. The engine is not yet running its seeding supervisor; call
// Start() to begin background work.
func NewEngine(cfg Config, logger *slog.Logger) (*Engine, error) {
	if cfg.DownloadDir == "" {
		return nil, fmt.Errorf("%w: download_dir is required", ErrNotConfigured)
	}

	dataDir := cfg.DownloadDir
	if cfg.IncompleteDir != "" {
		dataDir = cfg.IncompleteDir
	}

	// Ensure directories exist.
	for _, d := range []string{cfg.DownloadDir, dataDir} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o750); err != nil {
			return nil, fmt.Errorf("builtin/torrent: creating directory %q: %w", d, err)
		}
	}

	// Piece completion tracking. We use bolt DB for persistent state
	// across restarts. The state directory lives alongside the data.
	stateDir := filepath.Join(dataDir, ".torrent-state")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		return nil, fmt.Errorf("builtin/torrent: creating state dir %q: %w", stateDir, err)
	}

	pc, err := storage.NewBoltPieceCompletion(stateDir)
	if err != nil {
		// Fall back to in-memory piece completion if bolt is
		// unavailable (e.g. build constraints).
		logger.Warn("bolt piece completion unavailable, using in-memory fallback", "error", err)
		pc = storage.NewMapPieceCompletion()
	}

	tcfg := torrent.NewDefaultClientConfig()
	tcfg.ListenPort = cfg.ListenPort
	tcfg.DataDir = dataDir
	tcfg.DefaultStorage = storage.NewFileWithCompletion(dataDir, pc)
	tcfg.NoDHT = !cfg.EnableDHT
	if cfg.EnableDHT {
		tcfg.DhtStartingNodes = dhtStartingNodes
	}
	tcfg.DisablePEX = !cfg.EnablePEX
	tcfg.NoDefaultPortForwarding = !cfg.EnableUPnP

	// Configure external IP addresses for peer announcements. This is critical
	// in NAT/container environments (e.g. Kubernetes LoadBalancer) where the
	// internal pod IP differs from the external address peers should connect to.
	if cfg.PublicIP4 != "" {
		if ip := net.ParseIP(cfg.PublicIP4); ip != nil {
			tcfg.PublicIp4 = ip
			logger.Info("anacrolix public IPv4 configured", "ip", cfg.PublicIP4)
		} else {
			logger.Warn("invalid PublicIP4 address", "ip", cfg.PublicIP4)
		}
	}
	if cfg.PublicIP6 != "" {
		if ip := net.ParseIP(cfg.PublicIP6); ip != nil {
			tcfg.PublicIp6 = ip
			logger.Info("anacrolix public IPv6 configured", "ip", cfg.PublicIP6)
		} else {
			logger.Warn("invalid PublicIP6 address", "ip", cfg.PublicIP6)
		}
	}

	if cfg.MaxConnections > 0 {
		tcfg.EstablishedConnsPerTorrent = cfg.MaxConnections
	}

	tcfg.SetListenAddr(net.JoinHostPort("", fmt.Sprintf("%d", cfg.ListenPort)))

	// Global speed caps. anacrolix throttles using these limiters; we keep
	// references so the caps can be changed at runtime via SetSpeedLimits.
	downLimiter := newRateLimiter(cfg.DownloadSpeedLimit)
	upLimiter := newRateLimiter(cfg.UploadSpeedLimit)
	tcfg.DownloadRateLimiter = downLimiter
	tcfg.UploadRateLimiter = upLimiter

	cl, err := torrent.NewClient(tcfg)
	if err != nil && cfg.ListenPort > 0 && isAddrInUseErr(err) {
		logger.Warn("listen port in use; retrying with ephemeral port",
			"listen_port", cfg.ListenPort,
			"error", err,
		)
		tcfg.ListenPort = 0
		tcfg.SetListenAddr("")
		cl, err = torrent.NewClient(tcfg)
	}
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("builtin/torrent: creating client: %w", err)
	}

	logger.Info("engine created",
		"listen_port", cfg.ListenPort,
		"data_dir", dataDir,
		"dht", cfg.EnableDHT,
		"pex", cfg.EnablePEX,
		"upnp", cfg.EnableUPnP,
		"download_limit", cfg.DownloadSpeedLimit,
		"upload_limit", cfg.UploadSpeedLimit,
	)

	return &Engine{
		client:      cl,
		cfg:         cfg,
		logger:      logger,
		items:       make(map[string]*trackedTorrent),
		dataDir:     dataDir,
		downLimiter: downLimiter,
		upLimiter:   upLimiter,
	}, nil
}

func isAddrInUseErr(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
}

// Start launches the seeding supervisor goroutine. It blocks until
// ctx is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.engineCtx = ctx
	e.mu.Unlock()

	e.logger.Info("seeding supervisor started")
	defer e.logger.Info("seeding supervisor stopped")

	ticker := time.NewTicker(seedCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			e.enforceSeedPolicies()
		}
	}
}

// Close shuts down the engine: cancels the supervisor and closes the
// anacrolix client.
func (e *Engine) Close() error {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	cl := e.client
	e.mu.Unlock()

	if cl != nil {
		cl.Close()
	}
	e.logger.Info("engine closed")
	return nil
}

// lifecycleCtx returns the engine's lifecycle context, falling back to
// context.Background() when Start() has not yet been called (e.g. in
// tests). Callers must NOT hold e.mu when calling this.
func (e *Engine) lifecycleCtx() context.Context {
	e.mu.RLock()
	ctx := e.engineCtx
	e.mu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// AddMagnet adds a torrent via magnet URI. It waits for metadata to
// resolve (up to the configured MetadataTimeoutSecs or ctx deadline) and returns the
// lowercase infohash as the stable item ID.
func (e *Engine) AddMagnet(ctx context.Context, magnet string, meta torrentMeta) (string, error) {
	t, err := e.client.AddMagnet(magnet)
	if err != nil {
		return "", fmt.Errorf("builtin/torrent: adding magnet: %w", err)
	}

	// Reliable public trackers are the difference between a magnet
	// resolving metadata in seconds and timing out: many magnets (e.g.
	// EZTV, YTS) ship with weak or dead bundled trackers, and DHT alone
	// is frequently insufficient in NAT'd/containerised deployments.
	// We therefore augment any non-private magnet with the shared public
	// tracker list, whether or not it already lists trackers. Magnets
	// that carry a private-tracker credential are left untouched so a
	// private infohash is never announced to public trackers.
	switch {
	case !magnetHasTrackers(magnet):
		e.logger.Info("injecting public trackers into magnet (no trackers present)",
			"infohash", strings.ToLower(t.InfoHash().HexString()),
		)
		t.AddTrackers(defaultAnnounceList())
		e.logger.Info("public trackers injected",
			"trackerCount", len(defaultTrackers),
			"infohash", strings.ToLower(t.InfoHash().HexString()),
		)
	case !magnetLikelyPrivate(magnet):
		e.logger.Info("augmenting public magnet with reliable public trackers",
			"infohash", strings.ToLower(t.InfoHash().HexString()),
			"trackerCount", len(defaultTrackers),
		)
		t.AddTrackers(defaultAnnounceList())
	default:
		e.logger.Info("magnet appears private; not injecting public trackers",
			"infohash", strings.ToLower(t.InfoHash().HexString()),
		)
	}

	// Wait for metadata with a timeout. Use the engine's lifecycle context
	// rather than the caller's context so that a short-lived HTTP request
	// context being cancelled (e.g. client disconnect, write timeout) does
	// not abort metadata resolution mid-flight.
	timeout := time.Duration(e.cfg.MetadataTimeoutSecs) * time.Second
	waitCtx, waitCancel := context.WithTimeout(e.lifecycleCtx(), timeout)
	defer waitCancel()

	e.logger.Info("waiting for magnet metadata",
		"hash", strings.ToLower(t.InfoHash().HexString()),
		"timeout", timeout.String(),
	)

	select {
	case <-t.GotInfo():
		e.logger.Info("magnet metadata resolved",
			"hash", strings.ToLower(t.InfoHash().HexString()),
			"name", t.Name(),
			"size", t.Length(),
		)
	case <-waitCtx.Done():
		// Do not fail the add when metadata is slow. Keep the torrent in the
		// engine and let metadata continue resolving asynchronously; status()
		// reports it as queued while Info() is still nil.
		e.logger.Warn("magnet metadata not yet available; keeping torrent queued",
			"error", waitCtx.Err(),
			"timeout", timeout,
			"hash", strings.ToLower(t.InfoHash().HexString()),
		)
	}

	hash := strings.ToLower(t.InfoHash().HexString())
	title := meta.Title
	if title == "" {
		title = t.Name()
	}

	e.mu.Lock()
	e.items[hash] = &trackedTorrent{
		t:          t,
		title:      title,
		category:   meta.Category,
		savePath:   meta.SavePath,
		addedAt:    time.Now(),
		seedPolicy: meta.SeedPolicy,
	}
	e.mu.Unlock()

	if t.Info() != nil {
		e.logger.Info("torrent metadata resolved immediately",
			"hash", strings.ToLower(t.InfoHash().HexString()),
			"name", t.Name(),
			"size", t.Length(),
		)
		e.maybeAddPublicTrackers(t)
		t.DownloadAll()
	} else {
		// Metadata was not available within the configured timeout. Keep the
		// torrent tracked and start downloading automatically once metadata
		// eventually resolves.
		e.logger.Info("torrent added; waiting for async metadata resolution",
			"hash", strings.ToLower(t.InfoHash().HexString()),
			"timeout", timeout.String(),
		)
		go func(t *torrent.Torrent) {
			select {
			case <-t.GotInfo():
				e.logger.Info("async metadata resolution completed",
					"hash", strings.ToLower(t.InfoHash().HexString()),
					"name", t.Name(),
					"size", t.Length(),
				)
				e.maybeAddPublicTrackers(t)
				t.DownloadAll()
			case <-e.lifecycleCtx().Done():
				e.logger.Debug("engine shutting down before async metadata resolution")
			}
		}(t)
	}

	e.logger.Info("magnet added",
		"hash", hash,
		"title", title,
		"size", t.Length(),
	)

	return hash, nil
}

// AddTorrentBytes adds a torrent from raw .torrent file bytes. Returns
// the lowercase infohash as the stable item ID.
func (e *Engine) AddTorrentBytes(ctx context.Context, data []byte, meta torrentMeta) (string, error) {
	mi, err := metainfo.Load(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%w: parsing torrent bytes: %w", ErrInvalidInput, err)
	}

	// Verify the fetched torrent matches the infohash the indexer
	// advertised. A mismatch means the download URL served the wrong
	// content, so we refuse it rather than grab something unexpected.
	if meta.ExpectedInfohash != "" {
		got := strings.ToLower(mi.HashInfoBytes().HexString())
		if want := strings.ToLower(strings.TrimSpace(meta.ExpectedInfohash)); got != want {
			return "", fmt.Errorf("%w: torrent infohash %s does not match expected %s", ErrInvalidInput, got, want)
		}
	}

	t, err := e.client.AddTorrent(mi)
	if err != nil {
		return "", fmt.Errorf("builtin/torrent: adding torrent: %w", err)
	}
	e.maybeAddPublicTrackers(t)

	// Wait for info — for a .torrent file this should be immediate since
	// the metainfo contains the info dict. Use the engine's lifecycle
	// context (not the caller's) so a cancelled HTTP request context does
	// not abort an otherwise-instant wait and return a spurious error.
	timeout := time.Duration(e.cfg.MetadataTimeoutSecs) * time.Second
	waitCtx, waitCancel := context.WithTimeout(e.lifecycleCtx(), timeout)
	defer waitCancel()
	hash := strings.ToLower(t.InfoHash().HexString())

	e.logger.Info("waiting for torrent metadata",
		"hash", hash,
		"timeout", timeout.String(),
	)

	metadataReady := false
	select {
	case <-t.GotInfo():
		metadataReady = true
		e.logger.Info("torrent metadata resolved",
			"hash", hash,
			"name", t.Name(),
			"size", t.Length(),
		)
	case <-waitCtx.Done():
		// Keep the torrent queued and let metadata resolve asynchronously.
		e.logger.Warn("torrent metadata not yet available; keeping torrent queued",
			"error", waitCtx.Err(),
			"timeout", timeout.String(),
			"hash", hash,
		)
	}

	title := meta.Title
	if title == "" {
		title = t.Name()
	}

	e.mu.Lock()
	e.items[hash] = &trackedTorrent{
		t:          t,
		title:      title,
		category:   meta.Category,
		savePath:   meta.SavePath,
		addedAt:    time.Now(),
		seedPolicy: meta.SeedPolicy,
	}
	e.mu.Unlock()

	if metadataReady || t.Info() != nil {
		t.DownloadAll()
	} else {
		e.logger.Info("torrent added; waiting for async metadata resolution",
			"hash", hash,
			"timeout", timeout.String(),
		)
		go func(t *torrent.Torrent) {
			select {
			case <-t.GotInfo():
				e.logger.Info("async metadata resolution completed",
					"hash", strings.ToLower(t.InfoHash().HexString()),
					"name", t.Name(),
					"size", t.Length(),
				)
				t.DownloadAll()
			case <-e.lifecycleCtx().Done():
				e.logger.Debug("engine shutting down before async metadata resolution")
			}
		}(t)
	}

	e.logger.Info("torrent added",
		"hash", hash,
		"title", title,
		"size", t.Length(),
	)

	return hash, nil
}

// maybeAddPublicTrackers appends the shared public tracker list for
// non-private torrents. It never modifies private torrents.
func (e *Engine) maybeAddPublicTrackers(t *torrent.Torrent) {
	mi := t.Metainfo()
	info, err := mi.UnmarshalInfo()
	if err != nil {
		e.logger.Debug("could not inspect torrent privacy; skipping public tracker bootstrap",
			"hash", strings.ToLower(t.InfoHash().HexString()),
			"error", err,
		)
		return
	}
	if info.Private != nil && *info.Private {
		return
	}
	t.AddTrackers(defaultAnnounceList())
}

// TorrentStatus is the engine-level view of a tracked torrent.
type TorrentStatus struct {
	Hash         string
	Title        string
	Category     string
	SavePath     string
	ContentPath  string // actual filesystem path to the torrent's content
	Status       string // "queued", "downloading", "seeding", "paused", "completed"
	Progress     float64
	SizeBytes    int64
	Downloaded   int64
	Uploaded     int64
	DownloadRate int64
	UploadRate   int64
	Ratio        float64
	AddedAt      time.Time
	Paused       bool
}

// Status returns the current state of the requested torrents. An
// empty hashes slice returns all tracked torrents.
func (e *Engine) Status(hashes ...string) []TorrentStatus {
	e.mu.Lock()
	defer e.mu.Unlock()

	var targets []*trackedTorrent
	if len(hashes) == 0 {
		targets = make([]*trackedTorrent, 0, len(e.items))
		for _, tt := range e.items {
			targets = append(targets, tt)
		}
	} else {
		targets = make([]*trackedTorrent, 0, len(hashes))
		for _, h := range hashes {
			if tt, ok := e.items[strings.ToLower(h)]; ok {
				targets = append(targets, tt)
			}
		}
	}

	out := make([]TorrentStatus, 0, len(targets))
	for _, tt := range targets {
		out = append(out, e.buildStatus(tt))
	}
	return out
}

// EngineSummary aggregates the engine's live state for the management UI.
type EngineSummary struct {
	TotalTorrents int
	Queued        int
	Downloading   int
	Seeding       int
	Paused        int
	DownloadRate  int64 // aggregate bytes/sec
	UploadRate    int64 // aggregate bytes/sec
	DownloadLimit int64 // bytes/sec, 0 = unlimited
	UploadLimit   int64 // bytes/sec, 0 = unlimited
	ListenPort    int
	DHT           bool
	PEX           bool
	UPnP          bool
	SavePath      string
}

// Summary returns an aggregate snapshot of the engine state.
func (e *Engine) Summary() EngineSummary {
	statuses := e.Status() // refreshes per-torrent rates; takes the lock itself

	e.mu.RLock()
	defer e.mu.RUnlock()

	sum := EngineSummary{
		TotalTorrents: len(statuses),
		DownloadLimit: e.cfg.DownloadSpeedLimit,
		UploadLimit:   e.cfg.UploadSpeedLimit,
		ListenPort:    e.cfg.ListenPort,
		DHT:           e.cfg.EnableDHT,
		PEX:           e.cfg.EnablePEX,
		UPnP:          e.cfg.EnableUPnP,
		SavePath:      e.cfg.DownloadDir,
	}
	for _, s := range statuses {
		sum.DownloadRate += s.DownloadRate
		sum.UploadRate += s.UploadRate
		switch s.Status {
		case "queued":
			sum.Queued++
		case "paused":
			sum.Paused++
		case "seeding":
			sum.Seeding++
		case "downloading":
			sum.Downloading++
		}
	}
	return sum
}

// SetSpeedLimits updates the global download/upload caps (bytes/sec, 0 =
// unlimited) on the running engine. The change takes effect immediately.
func (e *Engine) SetSpeedLimits(downBytesPerSec, upBytesPerSec int64) {
	if downBytesPerSec < 0 {
		downBytesPerSec = 0
	}
	if upBytesPerSec < 0 {
		upBytesPerSec = 0
	}
	e.mu.Lock()
	e.cfg.DownloadSpeedLimit = downBytesPerSec
	e.cfg.UploadSpeedLimit = upBytesPerSec
	dl, ul := e.downLimiter, e.upLimiter
	e.mu.Unlock()

	applyRateLimit(dl, downBytesPerSec)
	applyRateLimit(ul, upBytesPerSec)
	e.logger.Info("engine speed limits updated",
		"download_limit", downBytesPerSec, "upload_limit", upBytesPerSec)
}

// buildStatus creates a TorrentStatus snapshot from a trackedTorrent.
// Must be called with e.mu held (write lock for speed tracking).
func (e *Engine) buildStatus(tt *trackedTorrent) TorrentStatus {
	t := tt.t
	stats := t.Stats()

	downloaded := stats.BytesReadUsefulData.Int64()
	uploaded := stats.BytesWrittenData.Int64()
	bytesCompleted := t.BytesCompleted()
	size := t.Length()

	// Compute speed from byte deltas since last sample.
	now := time.Now()
	if !tt.lastSpeedSampleAt.IsZero() {
		elapsed := now.Sub(tt.lastSpeedSampleAt).Seconds()
		if elapsed >= 0.5 {
			tt.downloadRate = int64(float64(downloaded-tt.lastBytesRead) / elapsed)
			tt.uploadRate = int64(float64(uploaded-tt.lastBytesWritten) / elapsed)
			if tt.downloadRate < 0 {
				tt.downloadRate = 0
			}
			if tt.uploadRate < 0 {
				tt.uploadRate = 0
			}
			tt.lastSpeedSampleAt = now
			tt.lastBytesRead = downloaded
			tt.lastBytesWritten = uploaded
		}
	} else {
		// First sample — seed the values, rate stays 0.
		tt.lastSpeedSampleAt = now
		tt.lastBytesRead = downloaded
		tt.lastBytesWritten = uploaded
	}

	var progress float64
	if size > 0 {
		progress = float64(bytesCompleted) / float64(size)
		if progress > 1.0 {
			progress = 1.0
		}
	}
	if t.Complete().Bool() {
		progress = 1.0
	}

	var ratio float64
	if downloaded > 0 {
		ratio = float64(uploaded) / float64(downloaded)
	}

	status := "downloading"
	switch {
	case tt.paused:
		status = "paused"
	case t.Complete().Bool():
		status = "seeding"
	case t.Info() == nil:
		// Waiting for metadata.
		status = "queued"
	}

	return TorrentStatus{
		Hash:         strings.ToLower(t.InfoHash().HexString()),
		Title:        tt.title,
		Category:     tt.category,
		SavePath:     tt.savePath,
		ContentPath:  e.contentPath(t),
		Status:       status,
		Progress:     progress,
		SizeBytes:    size,
		Downloaded:   downloaded,
		Uploaded:     uploaded,
		DownloadRate: tt.downloadRate,
		UploadRate:   tt.uploadRate,
		Ratio:        ratio,
		AddedAt:      tt.addedAt,
		Paused:       tt.paused,
	}
}

// contentPath returns the absolute filesystem path where the torrent's
// data actually lives. This uses the torrent's real content name (from
// metadata) rather than Loom's title, which may differ. After move-on-
// complete, files are in DownloadDir; before that, they're in dataDir
// (which may be IncompleteDir).
func (e *Engine) contentPath(t *torrent.Torrent) string {
	name := t.Name()
	if name == "" {
		return "" // metadata not yet available
	}
	hash := strings.ToLower(t.InfoHash().HexString())
	tt, ok := e.items[hash]
	if ok && tt.movedToDest && e.cfg.IncompleteDir != "" {
		return filepath.Join(e.cfg.DownloadDir, name)
	}
	return filepath.Join(e.dataDir, name)
}

// ContentPathByHash returns the on-disk content path for a tracked torrent by its
// lowercase infohash. Returns empty string if the hash is unknown or metadata is
// not yet available. Callers should use this immediately after Add to cache the
// real folder name before any restart can lose the in-memory state.
func (e *Engine) ContentPathByHash(hash string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	tt, ok := e.items[strings.ToLower(hash)]
	if !ok {
		return ""
	}
	return e.contentPath(tt.t)
}

// SavePath returns the configured download directory (effective save path).
func (e *Engine) SavePath() string {
	return e.cfg.DownloadDir
}

// Pause stops peer traffic for the given torrents by zeroing their
// max established connections.
func (e *Engine) Pause(hashes ...string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, h := range hashes {
		tt, ok := e.items[strings.ToLower(h)]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTorrentNotFound, h)
		}
		tt.paused = true
		tt.t.SetMaxEstablishedConns(pausedConns)
	}
	return nil
}

// Resume restores peer connections for the given torrents.
func (e *Engine) Resume(hashes ...string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, h := range hashes {
		tt, ok := e.items[strings.ToLower(h)]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTorrentNotFound, h)
		}
		tt.paused = false
		conns := activeConns
		if e.cfg.MaxConnections > 0 {
			conns = e.cfg.MaxConnections
		}
		tt.t.SetMaxEstablishedConns(conns)
	}
	return nil
}

// Remove drops torrents from the engine and optionally deletes their
// files from disk.
func (e *Engine) Remove(hashes []string, deleteFiles bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, h := range hashes {
		key := strings.ToLower(h)
		tt, ok := e.items[key]
		if !ok {
			continue
		}
		tt.t.Drop()
		delete(e.items, key)

		if deleteFiles {
			dir := filepath.Join(e.dataDir, key)
			if err := os.RemoveAll(dir); err != nil {
				e.logger.Warn("failed to delete torrent files",
					"hash", key,
					"dir", dir,
					"error", err,
				)
			}
		}
		e.logger.Info("torrent removed",
			"hash", key,
			"delete_files", deleteFiles,
		)
	}
	return nil
}

// Recheck triggers a data integrity verification for the requested
// torrents.
func (e *Engine) Recheck(ctx context.Context, hashes ...string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, h := range hashes {
		tt, ok := e.items[strings.ToLower(h)]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTorrentNotFound, h)
		}
		if err := tt.t.VerifyDataContext(ctx); err != nil {
			return fmt.Errorf("builtin/torrent: verify data for %s: %w", h, err)
		}
	}
	return nil
}

// Reannounce forces a tracker re-announce for the requested torrents.
// For DHT-enabled torrents it also triggers an announce to the DHT.
func (e *Engine) Reannounce(_ context.Context, hashes ...string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, h := range hashes {
		tt, ok := e.items[strings.ToLower(h)]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTorrentNotFound, h)
		}
		// Announce to all DHT servers attached to the client.
		for _, s := range e.client.DhtServers() {
			_, _, _ = tt.t.AnnounceToDht(s)
		}
	}
	return nil
}

// FreeSpace returns the bytes available on the engine's download
// directory.
func (e *Engine) FreeSpace() (int64, error) {
	_, free, err := diskspace.Get(e.cfg.DownloadDir)
	if err != nil {
		return -1, fmt.Errorf("builtin/torrent: statfs %q: %w", e.cfg.DownloadDir, err)
	}
	// Available bytes for unprivileged users. Disk sizes fit comfortably in int64.
	return int64(free), nil //nolint:gosec // disk byte counts fit in int64
}

// PeerInfo describes a single connected peer.
type PeerInfo struct {
	IP       string  `json:"ip"`
	Port     int     `json:"port"`
	Client   string  `json:"client"`
	Flags    string  `json:"flags"`
	Progress float64 `json:"progress"`
	DownRate int64   `json:"down_rate"`
	UpRate   int64   `json:"up_rate"`
}

// FileInfo describes a single file within the torrent.
type FileInfo struct {
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority string  `json:"priority"`
}

// TrackerInfo describes a single tracker.
type TrackerInfo struct {
	URL    string `json:"url"`
	Tier   int    `json:"tier"`
	Status string `json:"status"`
	Peers  int    `json:"peers"`
}

// TorrentDetail carries full detail for a single torrent.
type TorrentDetail struct {
	TorrentStatus
	Peers      []PeerInfo    `json:"peers"`
	Files      []FileInfo    `json:"files"`
	Trackers   []TrackerInfo `json:"trackers"`
	TotalPeers int           `json:"total_peers"`
	TotalSeeds int           `json:"total_seeds"`
	AddedAt    time.Time     `json:"added_at"`
	Comment    string        `json:"comment"`
	CreatedBy  string        `json:"created_by"`
	InfoHash   string        `json:"info_hash"`
}

// Detail returns comprehensive detail for the torrent identified by hash.
func (e *Engine) Detail(hash string) (*TorrentDetail, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := strings.ToLower(hash)
	tt, ok := e.items[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTorrentNotFound, hash)
	}

	detail := &TorrentDetail{
		TorrentStatus: e.buildStatus(tt),
		InfoHash:      key,
		AddedAt:       tt.addedAt,
	}

	t := tt.t

	// Metadata fields from the info dict.
	if info := t.Info(); info != nil {
		detail.Comment = t.Metainfo().Comment
		detail.CreatedBy = t.Metainfo().CreatedBy
	}

	// Peers.
	conns := t.PeerConns()
	peers := make([]PeerInfo, 0, len(conns))
	totalSeeds := 0
	for _, pc := range conns {
		ra := pc.RemoteAddr
		host, portStr := "", ""
		if ra != nil {
			host, portStr, _ = net.SplitHostPort(ra.String())
		}
		port := 0
		if portStr != "" {
			_, _ = fmt.Sscanf(portStr, "%d", &port)
		}

		numPieces := t.NumPieces()
		//nolint:gosec // Safe: piece count is always non-negative
		numPiecesU64 := uint64(numPieces)
		cardinality := pc.PeerPieces().GetCardinality() // already uint64
		peerPieceCount := cardinality
		if peerPieceCount > numPiecesU64 {
			peerPieceCount = numPiecesU64
		}
		progress := float64(peerPieceCount) / float64(max(1, int64(numPieces)))
		if progress > 1 {
			progress = 1
		}

		peerStats := pc.Stats()

		// Client name may not be set yet.
		clientName := ""
		if v := pc.PeerClientName.Load(); v != nil {
			clientName, _ = v.(string)
		}

		if progress >= 1.0 {
			totalSeeds++
		}
		peers = append(peers, PeerInfo{
			IP:       host,
			Port:     port,
			Client:   clientName,
			Progress: progress,
			DownRate: peerStats.BytesReadUsefulData.Int64(),
			UpRate:   peerStats.BytesWrittenData.Int64(),
		})
	}
	detail.Peers = peers
	detail.TotalPeers = len(conns)
	detail.TotalSeeds = totalSeeds

	// Files.
	if t.Info() != nil {
		tFiles := t.Files()
		files := make([]FileInfo, 0, len(tFiles))
		for _, f := range tFiles {
			var fprog float64
			if f.Length() > 0 {
				fprog = float64(f.BytesCompleted()) / float64(f.Length())
				if fprog > 1 {
					fprog = 1
				}
			}
			priority := "normal"
			if f.Priority() == torrent.PiecePriorityNone {
				priority = "skip"
			}
			files = append(files, FileInfo{
				Path:     f.DisplayPath(),
				Size:     f.Length(),
				Progress: fprog,
				Priority: priority,
			})
		}
		detail.Files = files
	}

	// Trackers.
	if t.Info() != nil {
		mi := t.Metainfo()
		announces := mi.UpvertedAnnounceList()
		trackers := make([]TrackerInfo, 0)
		for tier, tierURLs := range announces {
			for _, u := range tierURLs {
				trackers = append(trackers, TrackerInfo{
					URL:    u,
					Tier:   tier,
					Status: "working",
				})
			}
		}
		detail.Trackers = trackers
	}

	return detail, nil
}

// enforceSeedPolicies is called periodically by the seeding supervisor.
// It scans all tracked torrents and pauses any that have exceeded their
// seed ratio or seed time limit.
func (e *Engine) enforceSeedPolicies() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for hash, tt := range e.items {
		if tt.paused || !tt.t.Complete().Bool() {
			continue
		}

		// Move completed files from IncompleteDir → DownloadDir.
		if !tt.movedToDest && e.cfg.IncompleteDir != "" {
			name := tt.t.Name()
			if name != "" {
				src := filepath.Join(e.cfg.IncompleteDir, name)
				dst := filepath.Join(e.cfg.DownloadDir, name)
				if src != dst {
					if err := os.Rename(src, dst); err != nil {
						e.logger.Error("move-on-complete failed",
							"hash", hash, "src", src, "dst", dst, "error", err)
					} else {
						e.logger.Info("moved completed torrent to download dir",
							"hash", hash, "src", src, "dst", dst)
						tt.movedToDest = true
					}
				} else {
					tt.movedToDest = true
				}
			}
		}

		// Track when seeding started.
		if tt.seedStartAt == nil {
			t := now
			tt.seedStartAt = &t
		}

		stats := tt.t.Stats()
		downloaded := stats.BytesReadUsefulData.Int64()
		uploaded := stats.BytesWrittenData.Int64()

		// Check ratio limit.
		if tt.seedPolicy.RatioLimit > 0 && downloaded > 0 {
			ratio := float64(uploaded) / float64(downloaded)
			if ratio >= tt.seedPolicy.RatioLimit {
				e.logger.Info("seed ratio reached, pausing",
					"hash", hash,
					"ratio", ratio,
					"limit", tt.seedPolicy.RatioLimit,
				)
				tt.paused = true
				tt.t.SetMaxEstablishedConns(pausedConns)
				continue
			}
		}

		// Check time limit.
		if tt.seedPolicy.TimeLimitMinutes > 0 && tt.seedStartAt != nil {
			elapsed := now.Sub(*tt.seedStartAt)
			limit := time.Duration(tt.seedPolicy.TimeLimitMinutes) * time.Minute
			if elapsed >= limit {
				e.logger.Info("seed time limit reached, pausing",
					"hash", hash,
					"elapsed", elapsed,
					"limit", limit,
				)
				tt.paused = true
				tt.t.SetMaxEstablishedConns(pausedConns)
				continue
			}
		}
	}
}
