package torrent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"github.com/ebenderooock/loom/internal/diskspace"
	"github.com/ebenderooock/loom/internal/downloads/torrentutil"
)

// metadataTimeout is how long we wait for a .torrent's metadata to
// resolve during the initial Add. For raw .torrent bytes this should be
// effectively immediate because the info dict is already embedded.
const metadataTimeout = 60 * time.Second

// defaultAnnounceList wraps each tracker in its own tier so anacrolix
// announces to all of them in parallel.
func defaultAnnounceList() [][]string {
	defaultTrackers := torrentutil.PublicTrackers()
	list := make([][]string, len(defaultTrackers))
	for i, tr := range defaultTrackers {
		list[i] = []string{tr}
	}
	return list
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

func announceListFromMagnet(magnet string) [][]string {
	u, err := url.Parse(magnet)
	if err != nil {
		return nil
	}
	values := u.Query()["tr"]
	if len(values) == 0 {
		return nil
	}
	list := make([][]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, tr := range values {
		tr = strings.TrimSpace(tr)
		if tr == "" {
			continue
		}
		if _, ok := seen[tr]; ok {
			continue
		}
		seen[tr] = struct{}{}
		list = append(list, []string{tr})
	}
	return list
}

func shouldAugmentTrackers(announceList [][]string) bool {
	if len(announceList) == 0 {
		return false
	}
	for _, tier := range announceList {
		for _, tr := range tier {
			lower := strings.ToLower(strings.TrimSpace(tr))
			// Private trackers commonly include passkey/auth tokens in path/query.
			for _, marker := range []string{"passkey", "auth", "token", "apikey", "api_key"} {
				if strings.Contains(lower, marker) {
					return false
				}
			}

			u, err := url.Parse(lower)
			if err != nil {
				continue
			}
			if u.User != nil {
				return false
			}
			if u.RawQuery != "" {
				return false
			}
		}
	}
	return true
}

func mergeAnnounceLists(primary, fallback [][]string) [][]string {
	out := make([][]string, 0, len(primary)+len(fallback))
	seen := make(map[string]struct{}, len(primary)+len(fallback))
	add := func(list [][]string) {
		for _, tier := range list {
			for _, tr := range tier {
				tr = strings.TrimSpace(tr)
				if tr == "" {
					continue
				}
				if _, ok := seen[tr]; ok {
					continue
				}
				seen[tr] = struct{}{}
				out = append(out, []string{tr})
			}
		}
	}
	add(primary)
	add(fallback)
	return out
}

func (e *Engine) nudgePeerDiscovery(t *torrent.Torrent, announceList [][]string) {
	if len(announceList) > 0 {
		if e.cfg.DebugPeerDiscovery {
			e.logger.Info("nudging peer discovery: adding trackers",
				"hash", strings.ToLower(t.InfoHash().HexString()),
				"num_trackers", len(announceList),
			)
		}
		t.AddTrackers(announceList)
	}
	for _, s := range e.client.DhtServers() {
		if e.cfg.DebugPeerDiscovery {
			e.logger.Info("nudging peer discovery: announcing to DHT",
				"hash", strings.ToLower(t.InfoHash().HexString()),
				"dht_server", s.Addr().String(),
			)
		}
		_, _, _ = t.AnnounceToDht(s)
	}
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
	t            *torrent.Torrent
	title        string
	category     string
	savePath     string
	announceList [][]string
	addedAt      time.Time
	seedStartAt  *time.Time
	seedPolicy   SeedPolicy
	paused       bool
	movedToDest  bool // true once files have been moved from IncompleteDir → DownloadDir

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
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("builtin/torrent: creating directory %q: %w", d, err)
		}
	}

	// Piece completion tracking. We use bolt DB for persistent state
	// across restarts. The state directory lives alongside the data.
	stateDir := filepath.Join(dataDir, ".torrent-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
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
	tcfg.DisablePEX = !cfg.EnablePEX
	tcfg.NoDefaultPortForwarding = !cfg.EnableUPnP

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
// resolve (up to metadataTimeout) and returns the lowercase infohash as
// the stable item ID. If metadata does not resolve in time, the torrent
// is dropped and the add fails instead of remaining queued indefinitely.
func (e *Engine) AddMagnet(_ context.Context, magnet string, meta torrentMeta) (string, error) {
	t, err := e.client.AddMagnet(magnet)
	if err != nil {
		return "", fmt.Errorf("builtin/torrent: adding magnet: %w", err)
	}

	announceList := announceListFromMagnet(magnet)
	// Feed tracker URIs into the client explicitly. This wakes the regular
	// tracker announcers immediately for tracker-bearing magnets, and falls
	// back to a public bootstrap set for infohash-only magnets.
	if len(announceList) == 0 {
		announceList = defaultAnnounceList()
	} else if shouldAugmentTrackers(announceList) {
		announceList = mergeAnnounceLists(announceList, defaultAnnounceList())
	}
	t.AddTrackers(announceList)

	hash := strings.ToLower(t.InfoHash().HexString())
	title := meta.Title
	if title == "" {
		title = t.Name()
	}

	e.mu.Lock()
	e.items[hash] = &trackedTorrent{
		t:            t,
		title:        title,
		category:     meta.Category,
		savePath:     meta.SavePath,
		announceList: announceList,
		addedAt:      time.Now(),
		seedPolicy:   meta.SeedPolicy,
	}
	e.mu.Unlock()

	// Announce to trackers immediately for fast peer discovery.
	// Critical for NAT/container scenarios where DHT alone is unreliable.
	e.nudgePeerDiscovery(t, announceList)
	// Start the torrent session immediately so the client opens peer
	// handshakes while metadata resolution is in-flight.
	t.DownloadAll()

	waitCtx, waitCancel := context.WithTimeout(e.lifecycleCtx(), metadataTimeout)
	defer waitCancel()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-t.GotInfo():
			goto added
		case <-waitCtx.Done():
			e.mu.Lock()
			tracked := e.items[hash]
			delete(e.items, hash)
			e.mu.Unlock()

			t.Drop()
			if tracked != nil {
				e.logger.Warn("dropping magnet: metadata resolution timeout",
					"hash", hash,
					"title", tracked.title,
					"timeout", metadataTimeout,
				)
			}
			return "", fmt.Errorf("%w: %w", ErrMetadataTimeout, waitCtx.Err())
		case <-ticker.C:
			e.nudgePeerDiscovery(t, announceList)
		}
	}

added:
	e.logger.Info("magnet added",
		"hash", hash,
		"title", title,
		"size", t.Length(),
	)

	if e.cfg.DebugPeerDiscovery {
		e.logger.Info("magnet details for peer discovery",
			"hash", hash,
			"title", title,
			"num_trackers", len(announceList),
			"has_metadata", t.Info() != nil,
			"dht_enabled", e.cfg.EnableDHT,
			"pex_enabled", e.cfg.EnablePEX,
		)
	}

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

	// Wait for info — for a .torrent file this should be immediate since
	// the metainfo contains the info dict. Use the engine's lifecycle
	// context (not the caller's) so a cancelled HTTP request context does
	// not abort an otherwise-instant wait and return a spurious error.
	waitCtx, waitCancel := context.WithTimeout(e.lifecycleCtx(), metadataTimeout)
	defer waitCancel()

	select {
	case <-t.GotInfo():
	case <-waitCtx.Done():
		t.Drop()
		return "", fmt.Errorf("%w: %w", ErrMetadataTimeout, waitCtx.Err())
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

	t.DownloadAll()

	e.logger.Info("torrent added",
		"hash", hash,
		"title", title,
		"size", t.Length(),
	)

	return hash, nil
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
func (e *Engine) Recheck(hashes ...string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, h := range hashes {
		tt, ok := e.items[strings.ToLower(h)]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTorrentNotFound, h)
		}
		tt.t.VerifyData()
	}
	return nil
}

// Reannounce forces a tracker re-announce for the requested torrents.
// For DHT-enabled torrents it also triggers an announce to the DHT.
func (e *Engine) Reannounce(hashes ...string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, h := range hashes {
		tt, ok := e.items[strings.ToLower(h)]
		if !ok {
			return fmt.Errorf("%w: %s", ErrTorrentNotFound, h)
		}
		e.nudgePeerDiscovery(tt.t, tt.announceList)
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

func trackerInfosFromAnnounceList(announceList [][]string, status string) []TrackerInfo {
	trackers := make([]TrackerInfo, 0)
	for tier, tierURLs := range announceList {
		for _, u := range tierURLs {
			trackers = append(trackers, TrackerInfo{
				URL:    u,
				Tier:   tier,
				Status: status,
			})
		}
	}
	return trackers
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
			fmt.Sscanf(portStr, "%d", &port)
		}

		progress := 0.0
		if t.Info() != nil {
			numPieces := t.NumPieces()
			peerPieceCount := int(pc.PeerPieces().GetCardinality())
			progress = float64(peerPieceCount) / float64(max(1, numPieces))
			if progress > 1 {
				progress = 1
			}
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

	if e.cfg.DebugPeerDiscovery {
		e.logger.Info("torrent detail peer discovery info",
			"hash", key,
			"title", detail.Title,
			"status", detail.Status,
			"total_peers", detail.TotalPeers,
			"total_seeds", totalSeeds,
			"has_metadata", t.Info() != nil,
			"bytes_completed", detail.Downloaded,
			"total_size", detail.SizeBytes,
		)
	}

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
		detail.Trackers = trackerInfosFromAnnounceList(announces, "working")
	} else if len(tt.announceList) > 0 {
		detail.Trackers = trackerInfosFromAnnounceList(tt.announceList, "queued")
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
