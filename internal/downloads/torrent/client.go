package torrent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/ebenderooock/loom/internal/downloads"
)

// maxTorrentFetchSize caps the response body when downloading a
// .torrent file via TorrentURL. 10 MiB is generous for any real
// torrent file.
const maxTorrentFetchSize = 10 << 20 // 10 MiB

// torrentFetchTimeout bounds the HTTP request for fetching a .torrent
// file from a URL.
const torrentFetchTimeout = 30 * time.Second

// Client implements downloads.DownloadClient for the builtin/torrent
// kind. It wraps the singleton Engine and carries per-Definition
// configuration (category, save path, seed policy).
type Client struct {
	id        string
	name      string
	defConfig Config
	engine    *Engine
}

// Compile-time guard: keep the Client honest about implementing the
// downloads contract.
var _ downloads.DownloadClient = (*Client)(nil)

// New constructs a Client from a Definition and a shared Engine.
func New(def downloads.Definition, engine *Engine) (*Client, error) {
	cfg, err := parseConfig(def)
	if err != nil {
		return nil, err
	}

	return &Client{
		id:        def.ID,
		name:      def.Name,
		defConfig: cfg,
		engine:    engine,
	}, nil
}

// Engine returns the underlying torrent engine for direct access
// to detailed torrent information.
func (c *Client) Engine() *Engine { return c.engine }

// Detail implements downloads.DetailProvider. It returns rich torrent
// detail including peers, files, and trackers.
func (c *Client) Detail(_ context.Context, id string) (any, error) {
	return c.engine.Detail(id)
}

// EngineSummary implements downloads.TorrentManager.
func (c *Client) EngineSummary() downloads.TorrentEngineSummary {
	s := c.engine.Summary()
	return downloads.TorrentEngineSummary{
		TotalTorrents: s.TotalTorrents,
		Downloading:   s.Downloading,
		Seeding:       s.Seeding,
		Paused:        s.Paused,
		DownloadRate:  s.DownloadRate,
		UploadRate:    s.UploadRate,
		DownloadLimit: s.DownloadLimit,
		UploadLimit:   s.UploadLimit,
		ListenPort:    s.ListenPort,
		DHT:           s.DHT,
		PEX:           s.PEX,
		UPnP:          s.UPnP,
		SavePath:      s.SavePath,
	}
}

// SetSpeedLimits implements downloads.TorrentManager.
func (c *Client) SetSpeedLimits(downBytesPerSec, upBytesPerSec int64) {
	c.engine.SetSpeedLimits(downBytesPerSec, upBytesPerSec)
}

// ID implements downloads.DownloadClient.
func (c *Client) ID() string { return c.id }

// Name implements downloads.DownloadClient.
func (c *Client) Name() string { return c.name }

// Kind implements downloads.DownloadClient.
func (c *Client) Kind() downloads.Kind { return Kind }

// Protocol implements downloads.DownloadClient.
func (c *Client) Protocol() downloads.Protocol { return downloads.ProtocolTorrent }

// Add implements downloads.DownloadClient. It handles magnet URIs,
// torrent URLs, raw bytes, and bare infohashes. The returned ItemID
// is the lowercase infohash hex string.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	req.Normalize()

	category := req.Category
	if category == "" {
		category = c.defConfig.DownloadDir // fall back to engine dir
	}

	savePath := req.SavePath
	if savePath == "" {
		savePath = c.defConfig.DownloadDir
	}

	seedPolicy := SeedPolicy{
		RatioLimit:       c.defConfig.SeedRatioLimit,
		TimeLimitMinutes: c.defConfig.SeedTimeLimitMinutes,
	}
	// Per-indexer overrides take precedence over client defaults.
	if req.SeedRatioLimit != nil {
		seedPolicy.RatioLimit = *req.SeedRatioLimit
	}
	if req.SeedTimeLimitMinutes != nil {
		seedPolicy.TimeLimitMinutes = *req.SeedTimeLimitMinutes
	}

	meta := torrentMeta{
		Title:            req.Title,
		Category:         category,
		SavePath:         savePath,
		SeedPolicy:       seedPolicy,
		ExpectedInfohash: req.Infohash,
	}

	var hash string
	var err error

	switch {
	case len(req.RawBytes) > 0:
		hash, err = c.engine.AddTorrentBytes(ctx, req.RawBytes, meta)

	case req.TorrentURL != "":
		// Prefer fetching the .torrent file over adding a magnet. A
		// .torrent file embeds the metainfo, so metadata resolves
		// instantly without any peer/DHT/tracker discovery. Magnets —
		// especially the bare infohash magnets synthesised from an
		// indexer's hash, which carry no trackers — depend on peer
		// discovery, and DHT is effectively dead in NAT'd/containerised
		// deployments. Using the .torrent avoids spurious
		// "metadata resolution timed out" grab failures.
		data, fetchErr := fetchTorrentURL(ctx, req.TorrentURL)
		if fetchErr == nil {
			hash, err = c.engine.AddTorrentBytes(ctx, data, meta)
		} else {
			err = fetchErr
		}
		// Fall back to the magnet only when the .torrent is
		// unreachable/invalid AND the magnet was explicitly supplied by
		// the indexer (it carries trackers). We never fall back to a
		// synthesised bare-infohash magnet: it would fail the same way
		// and could leak a private infohash to public trackers.
		if err != nil && magnetHasTrackers(req.Magnet) {
			hash, err = c.engine.AddMagnet(ctx, req.Magnet, meta)
		}

	case req.Magnet != "":
		hash, err = c.engine.AddMagnet(ctx, req.Magnet, meta)

	default:
		return downloads.AddResult{}, fmt.Errorf(
			"%w: AddRequest has none of RawBytes, Magnet, TorrentURL, or Infohash",
			ErrInvalidInput,
		)
	}

	if err != nil {
		return downloads.AddResult{}, err
	}

	return downloads.AddResult{
		ClientID:    c.id,
		ItemID:      hash,
		ContentPath: c.engine.ContentPathByHash(hash),
		SavePath:    savePath,
	}, nil
}

// fetchTorrentURL downloads a .torrent file from a URL with size and
// timeout limits.
func fetchTorrentURL(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, torrentFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent: building fetch request for %q: %w", rawURL, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent: fetching %q: %w", rawURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("builtin/torrent: fetching %q: HTTP %d", rawURL, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentFetchSize+1))
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent: reading %q body: %w", rawURL, err)
	}
	if int64(len(data)) > maxTorrentFetchSize {
		return nil, fmt.Errorf("builtin/torrent: %q exceeds %d byte limit", rawURL, maxTorrentFetchSize)
	}

	// Validate that this is actually a torrent file.
	if _, err := metainfo.Load(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%w: URL %q did not return a valid torrent file: %w", ErrInvalidInput, rawURL, err)
	}

	return data, nil
}

// Status implements downloads.DownloadClient.
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	statuses := c.engine.Status(ids...)
	items := make([]downloads.Item, 0, len(statuses))
	for _, s := range statuses {
		items = append(items, downloads.Item{
			ID:              s.Hash,
			ClientID:        c.id,
			Title:           s.Title,
			Category:        s.Category,
			Status:          mapEngineStatus(s.Status),
			Progress:        s.Progress,
			SizeBytes:       s.SizeBytes,
			DownloadedBytes: s.Downloaded,
			DownloadRate:    s.DownloadRate,
			UploadRate:      s.UploadRate,
			Ratio:           s.Ratio,
			SavePath:        s.SavePath,
			ContentPath:     s.ContentPath,
		})
	}
	return items, nil
}

// mapEngineStatus maps the engine's status string to the downloads
// ItemStatus enum.
func mapEngineStatus(s string) downloads.ItemStatus {
	switch s {
	case "queued":
		return downloads.StatusItemQueued
	case "downloading":
		return downloads.StatusItemDownloading
	case "seeding":
		return downloads.StatusItemSeeding
	case "paused":
		return downloads.StatusItemPaused
	case "completed":
		return downloads.StatusItemCompleted
	default:
		return downloads.StatusItemUnknown
	}
}

// Pause implements downloads.DownloadClient.
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return c.pauseAll()
	}
	return c.engine.Pause(ids...)
}

func (c *Client) pauseAll() error {
	statuses := c.engine.Status()
	hashes := make([]string, 0, len(statuses))
	for _, s := range statuses {
		hashes = append(hashes, s.Hash)
	}
	if len(hashes) == 0 {
		return nil
	}
	return c.engine.Pause(hashes...)
}

// Resume implements downloads.DownloadClient.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return c.resumeAll()
	}
	return c.engine.Resume(ids...)
}

func (c *Client) resumeAll() error {
	statuses := c.engine.Status()
	hashes := make([]string, 0, len(statuses))
	for _, s := range statuses {
		hashes = append(hashes, s.Hash)
	}
	if len(hashes) == 0 {
		return nil
	}
	return c.engine.Resume(hashes...)
}

// Remove implements downloads.DownloadClient.
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	return c.engine.Remove(ids, deleteFiles)
}

// SetPriority implements downloads.DownloadClient. The built-in
// torrent engine does not maintain a queue with position semantics,
// so this is a no-op that returns nil.
func (c *Client) SetPriority(_ context.Context, _ downloads.Priority, _ ...string) error {
	return nil
}

// SetSpeedLimit implements downloads.DownloadClient. Per-torrent rate
// limiting is not directly supported by the anacrolix engine; this is
// a best-effort no-op. The engine-level speed limits configured in
// Config are applied globally.
func (c *Client) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error {
	return nil
}

// ForceStart implements downloads.DownloadClient. In the built-in
// engine there is no queue backlog; torrents start immediately on
// add. ForceStart is equivalent to Resume.
func (c *Client) ForceStart(ctx context.Context, ids ...string) error {
	return c.Resume(ctx, ids...)
}

// Recheck implements downloads.DownloadClient.
func (c *Client) Recheck(ctx context.Context, ids ...string) error {
	return c.engine.Recheck(ctx, ids...)
}

// Reannounce implements downloads.DownloadClient.
func (c *Client) Reannounce(ctx context.Context, ids ...string) error {
	return c.engine.Reannounce(ctx, ids...)
}

// Categories implements downloads.DownloadClient. The built-in engine
// does not have server-side categories; we return the Definition's
// default category if set.
func (c *Client) Categories(_ context.Context) ([]downloads.Category, error) {
	// No server-side category management; return empty or default.
	return []downloads.Category{}, nil
}

// FreeSpace implements downloads.DownloadClient.
func (c *Client) FreeSpace(_ context.Context) (int64, error) {
	return c.engine.FreeSpace()
}

// Test implements downloads.DownloadClient. It verifies that the
// download directory exists and is writable.
func (c *Client) Test(_ context.Context) error {
	dir := c.defConfig.DownloadDir

	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("builtin/torrent: download_dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("builtin/torrent: download_dir %q is not a directory", dir)
	}

	// Probe write access by creating and removing a temp file.
	probe := filepath.Join(dir, ".loom-probe-"+filepath.Base(c.id))
	if !strings.HasPrefix(probe, dir) {
		return fmt.Errorf("builtin/torrent: probe path escape detected")
	}
	probeClean := filepath.Clean(probe)
	f, err := os.Create(probeClean)
	if err != nil {
		return fmt.Errorf("builtin/torrent: download_dir %q is not writable: %w", dir, err)
	}
	_ = f.Close()
	_ = os.Remove(probeClean)

	return nil
}
