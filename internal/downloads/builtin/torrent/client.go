package torrent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/time/rate"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Client is a built-in BitTorrent client using anacrolix/torrent.
type Client struct {
	id     string
	name   string
	config Config
	client *torrent.Client
	logger *slog.Logger
	mu     sync.RWMutex
	items  map[string]*torrentItem // hash -> torrent state
	paused map[string]bool          // hash -> paused state (anacrolix has no built-in pause)

	// Rate limiters are created once and mutated via SetLimit so changes
	// apply without recreating the torrent client.
	downLimiter *rate.Limiter
	upLimiter   *rate.Limiter
}

type torrentItem struct {
	Hash     string
	Title    string
	Category string
	SavePath string
	Added    time.Time
	Torrent  *torrent.Torrent

	// Rate sampling: track cumulative byte counters and timestamps so we
	// can compute instantaneous down/up rates between Status calls.
	lastBytesRead    int64
	lastBytesWritten int64
	lastSampledAt    time.Time
	downloadRate     int64 // bytes/sec
	uploadRate       int64 // bytes/sec
}

// New creates a new built-in torrent client from a Definition.
func New(def downloads.Definition) (*Client, error) {
	var cfg Config
	if len(def.Config) > 0 {
		if err := json.Unmarshal(def.Config, &cfg); err != nil {
			return nil, fmt.Errorf("torrent: parse config: %w", err)
		}
	} else {
		cfg = DefaultConfig()
	}

	if cfg.DownloadDir == "" {
		return nil, fmt.Errorf("torrent: download_dir is required")
	}

	logger := slog.Default().With("module", "downloads/torrent", "client_id", def.ID)

	// Build rate limiters — hold pointers so SetSpeedLimits can mutate them live.
	downLimiter := unlimitedRateLimiter()
	upLimiter := unlimitedRateLimiter()
	if cfg.DownloadSpeedLimit > 0 {
		applyLimit(downLimiter, cfg.DownloadSpeedLimit)
	}
	if cfg.UploadSpeedLimit > 0 {
		applyLimit(upLimiter, cfg.UploadSpeedLimit)
	}

	// Instantiate anacrolix/torrent client
	tcfg := torrent.NewDefaultClientConfig()
	tcfg.ListenPort = cfg.ListenPort
	tcfg.DataDir = cfg.DownloadDir
	tcfg.NoDHT = !cfg.EnableDHT
	tcfg.DisablePEX = !cfg.EnablePEX
	tcfg.NoDefaultPortForwarding = !cfg.EnableUPnP
	tcfg.DownloadRateLimiter = downLimiter
	tcfg.UploadRateLimiter = upLimiter

	t, err := torrent.NewClient(tcfg)
	if err != nil {
		return nil, fmt.Errorf("torrent: create client: %w", err)
	}

	c := &Client{
		id:          def.ID,
		name:        def.Name,
		config:      cfg,
		client:      t,
		logger:      logger,
		items:       make(map[string]*torrentItem),
		paused:      make(map[string]bool),
		downLimiter: downLimiter,
		upLimiter:   upLimiter,
	}

	logger.Info("created", "listen_port", cfg.ListenPort, "download_dir", cfg.DownloadDir)
	return c, nil
}

// ID implements DownloadClient.ID.
func (c *Client) ID() string {
	return c.id
}

// Name implements DownloadClient.Name.
func (c *Client) Name() string {
	return c.name
}

// Kind implements DownloadClient.Kind.
func (c *Client) Kind() downloads.Kind {
	return Kind
}

// Protocol implements DownloadClient.Protocol.
func (c *Client) Protocol() downloads.Protocol {
	return downloads.ProtocolTorrent
}

// Add implements DownloadClient.Add.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	req.Normalize()
	if req.Magnet == "" && req.TorrentURL == "" && len(req.RawBytes) == 0 {
		return downloads.AddResult{}, fmt.Errorf("torrent: add requires magnet, torrent_url, or raw bytes")
	}

	// For MVP, only support magnet URIs and raw .torrent data
	if req.Magnet != "" {
		return c.addMagnet(ctx, req)
	}
	if len(req.RawBytes) > 0 {
		return c.addRaw(ctx, req)
	}

	return downloads.AddResult{}, fmt.Errorf("torrent: torrent_url not yet supported; use magnet or raw bytes")
}

func (c *Client) addMagnet(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	t, err := c.client.AddMagnet(req.Magnet)
	if err != nil {
		return downloads.AddResult{}, fmt.Errorf("torrent: add magnet: %w", err)
	}

	c.mu.Lock()
	item := &torrentItem{
		Hash:     t.InfoHash().HexString(),
		Title:    req.Title,
		Category: req.Category,
		SavePath: req.SavePath,
		Added:    time.Now(),
		Torrent:  t,
	}
	c.items[item.Hash] = item
	c.mu.Unlock()

	c.startDownload(item)
	c.logger.Debug("added torrent", "hash", item.Hash, "title", item.Title)
	return downloads.AddResult{
		ClientID:    c.id,
		ItemID:      item.Hash,
		ContentPath: "", // unknown until completed
		SavePath:    item.SavePath,
	}, nil
}

func (c *Client) addRaw(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	mi, err := metainfo.Load(bytes.NewReader(req.RawBytes))
	if err != nil {
		return downloads.AddResult{}, fmt.Errorf("torrent: parse raw bytes: %w", err)
	}
	t, err := c.client.AddTorrent(mi)
	if err != nil {
		return downloads.AddResult{}, fmt.Errorf("torrent: add raw bytes: %w", err)
	}
	title := req.Title
	if title == "" {
		title = t.Name()
	}
	c.mu.Lock()
	item := &torrentItem{
		Hash:     t.InfoHash().HexString(),
		Title:    title,
		Category: req.Category,
		SavePath: req.SavePath,
		Added:    time.Now(),
		Torrent:  t,
	}
	c.items[item.Hash] = item
	c.mu.Unlock()

	c.startDownload(item)
	c.logger.Debug("added raw torrent", "hash", item.Hash, "title", item.Title)
	return downloads.AddResult{
		ClientID:    c.id,
		ItemID:      item.Hash,
		ContentPath: "",
		SavePath:    item.SavePath,
	}, nil
}

func (c *Client) startDownload(item *torrentItem) {
	start := func(t *torrent.Torrent) {
		t.AllowDataDownload()
		t.DownloadAll()
	}

	if item.Torrent.Info() != nil {
		start(item.Torrent)
		return
	}

	go func(hash string, t *torrent.Torrent) {
		select {
		case <-t.GotInfo():
			start(t)
			c.logger.Info("started torrent download after metadata", "hash", hash, "name", t.Name())
		case <-time.After(2 * time.Minute):
			c.logger.Warn("timed out waiting for torrent metadata", "hash", hash)
		}
	}(item.Hash, item.Torrent)
}

// Status implements DownloadClient.Status.
func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var items []downloads.Item

	if len(ids) == 0 {
		// Return status of all torrents
		for _, item := range c.items {
			status := c.torrentToItem(item)
			items = append(items, status)
		}
	} else {
		// Return status of specific torrents
		for _, id := range ids {
			if item, ok := c.items[id]; ok {
				status := c.torrentToItem(item)
				items = append(items, status)
			}
		}
	}

	return items, nil
}

func (c *Client) torrentToItem(item *torrentItem) downloads.Item {
	t := item.Torrent
	status := downloads.StatusItemUnknown

	// Map anacrolix/torrent state to downloads.ItemStatus
	if c.paused[item.Hash] {
		status = downloads.StatusItemPaused
	} else if t.Complete().Bool() && !t.Seeding() {
		status = downloads.StatusItemCompleted
	} else if t.Seeding() {
		status = downloads.StatusItemSeeding
	} else if t.BytesCompleted() > 0 {
		status = downloads.StatusItemDownloading
	} else {
		status = downloads.StatusItemQueued
	}

	progress := 0.0
	if t.Length() > 0 {
		progress = float64(t.BytesCompleted()) / float64(t.Length())
	}

	// Sample cumulative stats and derive instantaneous rates.
	stats := t.Stats()
	nowBytes := stats.BytesReadData.Int64()
	upBytes := stats.BytesWrittenData.Int64()
	now := time.Now()

	if !item.lastSampledAt.IsZero() {
		elapsed := now.Sub(item.lastSampledAt).Seconds()
		if elapsed > 0 {
			item.downloadRate = int64(float64(nowBytes-item.lastBytesRead) / elapsed)
			item.uploadRate = int64(float64(upBytes-item.lastBytesWritten) / elapsed)
			if item.downloadRate < 0 {
				item.downloadRate = 0
			}
			if item.uploadRate < 0 {
				item.uploadRate = 0
			}
		}
	}
	item.lastBytesRead = nowBytes
	item.lastBytesWritten = upBytes
	item.lastSampledAt = now

	// Ratio: uploaded bytes / downloaded bytes
	var ratio float64
	if t.BytesCompleted() > 0 {
		ratio = float64(upBytes) / float64(t.BytesCompleted())
	}

	// ContentPath: full filesystem path to downloaded content
	savePath := item.SavePath
	if savePath == "" {
		savePath = c.config.DownloadDir
	}
	contentName := t.Name()
	var contentPath string
	if contentName != "" {
		contentPath = filepath.Join(savePath, contentName)
	}

	return downloads.Item{
		ID:              item.Hash,
		Title:           item.Title,
		Category:        item.Category,
		Status:          status,
		Progress:        progress,
		SizeBytes:       t.Length(),
		DownloadedBytes: t.BytesCompleted(),
		DownloadRate:    item.downloadRate,
		UploadRate:      item.uploadRate,
		Ratio:           ratio,
		SavePath:        savePath,
		ContentPath:     contentPath,
	}
}

// Pause implements DownloadClient.Pause.
func (c *Client) Pause(ctx context.Context, ids ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(ids) == 0 {
		// Pause all
		for hash, item := range c.items {
			c.paused[hash] = true
			item.Torrent.DisallowDataDownload()
		}
		return nil
	}

	for _, id := range ids {
		if item, ok := c.items[id]; ok {
			c.paused[id] = true
			item.Torrent.DisallowDataDownload()
		}
	}
	return nil
}

// Resume implements DownloadClient.Resume.
func (c *Client) Resume(ctx context.Context, ids ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(ids) == 0 {
		// Resume all
		for hash, item := range c.items {
			c.paused[hash] = false
			item.Torrent.AllowDataDownload()
		}
		return nil
	}

	for _, id := range ids {
		if item, ok := c.items[id]; ok {
			c.paused[id] = false
			item.Torrent.AllowDataDownload()
		}
	}
	return nil
}

// Remove implements DownloadClient.Remove.
func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, id := range ids {
		if item, ok := c.items[id]; ok {
			item.Torrent.Drop()
			delete(c.items, id)
			delete(c.paused, id)
		}
	}
	return nil
}

// SetPriority implements DownloadClient.SetPriority (stub for MVP).
func (c *Client) SetPriority(ctx context.Context, priority downloads.Priority, ids ...string) error {
	return fmt.Errorf("torrent: SetPriority not yet implemented")
}

// SetSpeedLimit implements DownloadClient.SetSpeedLimit (stub for MVP).
func (c *Client) SetSpeedLimit(ctx context.Context, limitBytesPerSec int64, ids ...string) error {
	return fmt.Errorf("torrent: SetSpeedLimit not yet implemented")
}

// ForceStart implements DownloadClient.ForceStart (stub for MVP).
func (c *Client) ForceStart(ctx context.Context, ids ...string) error {
	return fmt.Errorf("torrent: ForceStart not yet implemented")
}

// Recheck implements DownloadClient.Recheck — triggers a data verify on all pieces.
func (c *Client) Recheck(ctx context.Context, ids ...string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	targets := c.resolveIDs(ids)
	for _, item := range targets {
		if err := item.Torrent.VerifyDataContext(ctx); err != nil {
			return fmt.Errorf("torrent: recheck %s: %w", item.Hash, err)
		}
	}
	return nil
}

// Reannounce implements DownloadClient.Reannounce — re-announces to all DHT servers.
func (c *Client) Reannounce(ctx context.Context, ids ...string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	targets := c.resolveIDs(ids)
	for _, item := range targets {
		// anacrolix doesn't have an explicit tracker-reannounce API but we
		// can trigger via AddTrackers with no new trackers to poke the announce loop.
		// For DHT, we get announce by just calling AnnounceToDht on each server.
		for _, srv := range c.client.DhtServers() {
			_, stop, err := item.Torrent.AnnounceToDht(srv)
			if err == nil {
				// Non-blocking: start announce then immediately cancel it —
				// this triggers a fresh announce cycle on the DHT.
				stop()
			}
		}
	}
	return nil
}

// Categories implements DownloadClient.Categories.
func (c *Client) Categories(ctx context.Context) ([]downloads.Category, error) {
	return []downloads.Category{}, nil
}

// FreeSpace implements DownloadClient.FreeSpace — reports available bytes on the download dir's filesystem.
func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	return diskFreeBytes(c.config.DownloadDir)
}

// Test implements DownloadClient.Test.
func (c *Client) Test(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("torrent: client not initialized")
	}
	return nil
}

// EngineSummary implements TorrentManager.EngineSummary.
func (c *Client) EngineSummary() downloads.TorrentEngineSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var downloading, seeding, paused, queued int
	var totalDown, totalUp int64
	for hash, item := range c.items {
		t := item.Torrent
		if c.paused[hash] {
			paused++
		} else if t.Seeding() {
			seeding++
		} else if t.BytesCompleted() > 0 {
			downloading++
		} else {
			queued++
		}
		totalDown += item.downloadRate
		totalUp += item.uploadRate
	}

	return downloads.TorrentEngineSummary{
		TotalTorrents: len(c.items),
		Downloading:   downloading,
		Seeding:       seeding,
		Paused:        paused,
		DownloadRate:  totalDown,
		UploadRate:    totalUp,
		DownloadLimit: c.config.DownloadSpeedLimit,
		UploadLimit:   c.config.UploadSpeedLimit,
		ListenPort:    c.config.ListenPort,
		DHT:           c.config.EnableDHT,
		PEX:           c.config.EnablePEX,
		UPnP:          c.config.EnableUPnP,
		SavePath:      c.config.DownloadDir,
	}
}

// SetSpeedLimits implements TorrentManager.SetSpeedLimits.
func (c *Client) SetSpeedLimits(downBytesPerSec, upBytesPerSec int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.DownloadSpeedLimit = downBytesPerSec
	c.config.UploadSpeedLimit = upBytesPerSec

	if downBytesPerSec > 0 {
		applyLimit(c.downLimiter, downBytesPerSec)
	} else {
		c.downLimiter.SetLimit(rate.Inf)
	}
	if upBytesPerSec > 0 {
		applyLimit(c.upLimiter, upBytesPerSec)
	} else {
		c.upLimiter.SetLimit(rate.Inf)
	}
	c.logger.Debug("speed limits set", "down", downBytesPerSec, "up", upBytesPerSec)
}

// resolveIDs returns the torrentItems for the given IDs, or all items if ids is empty.
// Caller must hold c.mu for reading.
func (c *Client) resolveIDs(ids []string) []*torrentItem {
	if len(ids) == 0 {
		all := make([]*torrentItem, 0, len(c.items))
		for _, item := range c.items {
			all = append(all, item)
		}
		return all
	}
	var out []*torrentItem
	for _, id := range ids {
		if item, ok := c.items[id]; ok {
			out = append(out, item)
		}
	}
	return out
}
