package torrent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// metadataTimeout is how long we wait for a magnet's metadata to
// resolve before giving up. Thirty seconds is generous for public
// swarms and tolerable for private trackers.
const metadataTimeout = 30 * time.Second

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
}

// trackedTorrent pairs the anacrolix torrent handle with Loom metadata.
type trackedTorrent struct {
	t            *torrent.Torrent
	title        string
	category     string
	savePath     string
	addedAt      time.Time
	seedStartAt  *time.Time
	seedPolicy   SeedPolicy
	paused       bool
	downloaded   int64 // snapshot for ratio calculation
	uploaded     int64

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
	mu      sync.RWMutex
	client  *torrent.Client
	cfg     Config
	logger  *slog.Logger
	items   map[string]*trackedTorrent // keyed by lowercase infohash hex
	cancel  context.CancelFunc
	dataDir string
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
	)

	return &Engine{
		client:  cl,
		cfg:     cfg,
		logger:  logger,
		items:   make(map[string]*trackedTorrent),
		dataDir: dataDir,
	}, nil
}

// Start launches the seeding supervisor goroutine. It blocks until
// ctx is cancelled.
func (e *Engine) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
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

// AddMagnet adds a torrent via magnet URI. It waits for metadata to
// resolve (up to metadataTimeout or ctx deadline) and returns the
// lowercase infohash as the stable item ID.
func (e *Engine) AddMagnet(ctx context.Context, magnet string, meta torrentMeta) (string, error) {
	t, err := e.client.AddMagnet(magnet)
	if err != nil {
		return "", fmt.Errorf("builtin/torrent: adding magnet: %w", err)
	}

	// Wait for metadata with a timeout.
	waitCtx, waitCancel := context.WithTimeout(ctx, metadataTimeout)
	defer waitCancel()

	select {
	case <-t.GotInfo():
	case <-waitCtx.Done():
		t.Drop()
		return "", fmt.Errorf("%w: %v", ErrMetadataTimeout, waitCtx.Err())
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
		return "", fmt.Errorf("%w: parsing torrent bytes: %v", ErrInvalidInput, err)
	}

	t, err := e.client.AddTorrent(mi)
	if err != nil {
		return "", fmt.Errorf("builtin/torrent: adding torrent: %w", err)
	}

	// Wait for info — for a .torrent file this should be immediate
	// since the metainfo contains the info dict.
	waitCtx, waitCancel := context.WithTimeout(ctx, metadataTimeout)
	defer waitCancel()

	select {
	case <-t.GotInfo():
	case <-waitCtx.Done():
		t.Drop()
		return "", fmt.Errorf("%w: %v", ErrMetadataTimeout, waitCtx.Err())
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
		// Announce to all DHT servers attached to the client.
		for _, s := range e.client.DhtServers() {
			_, _, _ = tt.t.AnnounceToDht(s)
		}
	}
	return nil
}

// FreeSpace returns the bytes available on the engine's download
// directory using syscall.Statfs.
func (e *Engine) FreeSpace() (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(e.cfg.DownloadDir, &stat); err != nil {
		return -1, fmt.Errorf("builtin/torrent: statfs %q: %w", e.cfg.DownloadDir, err)
	}
	// Available blocks × block size gives free bytes for unprivileged users.
	return int64(stat.Bavail) * int64(stat.Bsize), nil
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
			fmt.Sscanf(portStr, "%d", &port)
		}

		numPieces := t.NumPieces()
		peerPieceCount := int(pc.PeerPieces().GetCardinality())
		progress := float64(peerPieceCount) / float64(max(1, numPieces))
		if progress > 1 {
			progress = 1
		}

		peerStats := pc.Peer.Stats()

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
