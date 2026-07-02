package torrent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/rain/v2/rainrpc"

	"github.com/ebenderooock/loom/internal/downloads"
)

const maxTorrentFetchSize = 10 << 20 // 10 MiB

type clientMeta struct {
	Category string
	SavePath string
	AddedAt  time.Time
	Title    string
}

// Client implements downloads.DownloadClient by talking to a Rain sidecar
// over JSON-RPC.
type Client struct {
	id   string
	name string
	cfg  Config
	rpc  *rainrpc.Client

	mu     sync.RWMutex
	meta   map[string]clientMeta
	limits struct {
		down int64
		up   int64
	}
}

var _ downloads.DownloadClient = (*Client)(nil)
var _ downloads.DetailProvider = (*Client)(nil)
var _ downloads.TorrentManager = (*Client)(nil)

func New(def downloads.Definition, cfg Config) (*Client, error) {
	rpcClient := rainrpc.NewClient(cfg.rpcURL())
	rpcClient.SetTimeout(timeout(cfg))

	c := &Client{
		id:   def.ID,
		name: def.Name,
		cfg:  cfg,
		rpc:  rpcClient,
		meta: make(map[string]clientMeta),
	}
	c.limits.down = cfg.DownloadSpeedLimit
	c.limits.up = cfg.UploadSpeedLimit
	return c, nil
}

func (c *Client) ID() string                   { return c.id }
func (c *Client) Name() string                 { return c.name }
func (c *Client) Kind() downloads.Kind         { return Kind }
func (c *Client) Protocol() downloads.Protocol { return downloads.ProtocolTorrent }

// storageDir returns the on-disk directory where Rain stores a torrent's files.
// Rain defaults to data-dir-includes-torrent-id=true, which nests each torrent
// under {base}/{torrentID}/. base is the configured download directory (or a
// per-item override); id is the Rain torrent ID.
func (c *Client) storageDir(base, id string) string {
	if strings.TrimSpace(base) == "" {
		base = c.cfg.DownloadDir
	}
	if c.cfg.DataDirIncludesTorrentID && strings.TrimSpace(id) != "" {
		return filepath.Join(base, id)
	}
	return base
}

func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	req.Normalize()
	category := req.Category
	if category == "" {
		category = c.cfg.DownloadDir
	}
	savePath := req.SavePath
	if savePath == "" {
		savePath = c.cfg.DownloadDir
	}

	var (
		id    string
		title string
		err   error
	)
	switch {
	case len(req.RawBytes) > 0:
		id, title, err = c.addTorrentBytes(req.RawBytes)
	case req.TorrentURL != "":
		var data []byte
		data, err = fetchTorrentURL(ctx, req.TorrentURL)
		if err == nil {
			id, title, err = c.addTorrentBytes(data)
		} else if strings.TrimSpace(req.Magnet) != "" {
			id, title, err = c.addURI(req.Magnet)
		}
	case req.Magnet != "":
		id, title, err = c.addURI(req.Magnet)
	default:
		return downloads.AddResult{}, fmt.Errorf("%w: AddRequest has none of RawBytes, Magnet, TorrentURL, or Infohash", ErrInvalidInput)
	}
	if err != nil {
		return downloads.AddResult{}, err
	}

	if req.Title != "" {
		title = req.Title
	}
	now := time.Now()
	c.mu.Lock()
	c.meta[id] = clientMeta{
		Category: category,
		SavePath: savePath,
		AddedAt:  now,
		Title:    title,
	}
	c.mu.Unlock()

	contentPath := ""
	if title != "" {
		// Rain stores files under {savePath}/{id}/{name}. Report the actual
		// on-disk location so the import pipeline can resolve it.
		contentPath = filepath.Join(c.storageDir(savePath, id), title)
	} else {
		contentPath = c.storageDir(savePath, id)
	}

	return downloads.AddResult{
		ClientID:    c.id,
		ItemID:      id,
		ContentPath: contentPath,
		SavePath:    c.storageDir(savePath, id),
	}, nil
}

func (c *Client) addURI(uri string) (id string, title string, err error) {
	t, err := c.rpc.AddURI(uri, nil)
	if err != nil {
		return "", "", fmt.Errorf("builtin/torrent(rain): add uri: %w", err)
	}
	return t.ID, strings.TrimSpace(t.Name), nil
}

func (c *Client) addTorrentBytes(data []byte) (id string, title string, err error) {
	t, err := c.rpc.AddTorrent(bytes.NewReader(data), nil)
	if err != nil {
		return "", "", fmt.Errorf("builtin/torrent(rain): add torrent: %w", err)
	}
	return t.ID, strings.TrimSpace(t.Name), nil
}

func fetchTorrentURL(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent(rain): building fetch request for %q: %w", rawURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent(rain): fetching %q: %w", rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("builtin/torrent(rain): fetching %q: HTTP %d", rawURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentFetchSize+1))
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent(rain): reading %q body: %w", rawURL, err)
	}
	if int64(len(data)) > maxTorrentFetchSize {
		return nil, fmt.Errorf("builtin/torrent(rain): %q exceeds %d byte limit", rawURL, maxTorrentFetchSize)
	}
	return data, nil
}

func (c *Client) Status(ctx context.Context, ids ...string) ([]downloads.Item, error) {
	_ = ctx
	targetIDs := ids
	if len(targetIDs) == 0 {
		torrents, err := c.rpc.ListTorrents()
		if err != nil {
			return nil, fmt.Errorf("builtin/torrent(rain): list torrents: %w", err)
		}
		targetIDs = make([]string, 0, len(torrents))
		for _, t := range torrents {
			targetIDs = append(targetIDs, t.ID)
		}
	}

	items := make([]downloads.Item, 0, len(targetIDs))
	for _, id := range targetIDs {
		st, err := c.rpc.GetTorrentStats(id)
		if err != nil {
			continue
		}
		meta := c.getMeta(id)
		title := strings.TrimSpace(st.Name)
		if meta.Title != "" {
			title = meta.Title
		}
		if title == "" {
			title = id
		}

		size := st.Bytes.Total
		completed := st.Bytes.Completed
		progress := 0.0
		if size > 0 {
			progress = float64(completed) / float64(size)
		}
		ratio := 0.0
		if st.Bytes.Downloaded > 0 {
			ratio = float64(st.Bytes.Uploaded) / float64(st.Bytes.Downloaded)
		}
		savePath := meta.SavePath
		if savePath == "" {
			savePath = c.cfg.DownloadDir
		}
		// Rain stores each torrent under {savePath}/{id}/. Report that
		// directory as the save path and the full content path inside it so
		// the import pipeline resolves the real on-disk location.
		storage := c.storageDir(savePath, id)
		contentPath := storage
		if name := strings.TrimSpace(st.Name); name != "" {
			contentPath = filepath.Join(storage, name)
		}

		items = append(items, downloads.Item{
			ID:              id,
			ClientID:        c.id,
			Title:           title,
			Category:        meta.Category,
			Status:          mapStatus(st.Status),
			Progress:        progress,
			SizeBytes:       size,
			DownloadedBytes: completed,
			ETA:             int64(st.ETA),
			DownloadRate:    int64(st.Speed.Download),
			UploadRate:      int64(st.Speed.Upload),
			Ratio:           ratio,
			Message:         st.Error,
			SavePath:        storage,
			ContentPath:     contentPath,
		})
	}
	return items, nil
}

func mapStatus(s string) downloads.ItemStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "downloading":
		return downloads.StatusItemDownloading
	case "seeding":
		return downloads.StatusItemSeeding
	case "stopped":
		return downloads.StatusItemPaused
	case "downloading metadata", "allocating", "verifying", "stopping":
		return downloads.StatusItemQueued
	default:
		return downloads.StatusItemUnknown
	}
}

func (c *Client) Pause(ctx context.Context, ids ...string) error {
	_ = ctx
	if len(ids) == 0 {
		return c.rpc.StopAllTorrents()
	}
	for _, id := range ids {
		if err := c.rpc.StopTorrent(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Resume(ctx context.Context, ids ...string) error {
	_ = ctx
	if len(ids) == 0 {
		return c.rpc.StartAllTorrents()
	}
	for _, id := range ids {
		if err := c.rpc.StartTorrent(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Remove(ctx context.Context, ids []string, deleteFiles bool) error {
	_ = ctx
	keepData := !deleteFiles
	for _, id := range ids {
		if err := c.rpc.RemoveTorrent(id, keepData); err != nil {
			return err
		}
		c.mu.Lock()
		delete(c.meta, id)
		c.mu.Unlock()
	}
	return nil
}

func (c *Client) SetPriority(_ context.Context, _ downloads.Priority, _ ...string) error {
	return nil
}

func (c *Client) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error {
	return nil
}

func (c *Client) ForceStart(ctx context.Context, ids ...string) error {
	return c.Resume(ctx, ids...)
}

func (c *Client) Recheck(ctx context.Context, ids ...string) error {
	_ = ctx
	for _, id := range ids {
		if err := c.rpc.VerifyTorrent(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Reannounce(ctx context.Context, ids ...string) error {
	_ = ctx
	for _, id := range ids {
		if err := c.rpc.AnnounceTorrent(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Categories(_ context.Context) ([]downloads.Category, error) {
	return []downloads.Category{}, nil
}

func (c *Client) FreeSpace(ctx context.Context) (int64, error) {
	return diskFreeSpace(c.cfg.DownloadDir)
}

func (c *Client) Test(_ context.Context) error {
	if _, err := c.rpc.ServerVersion(); err != nil {
		return fmt.Errorf("builtin/torrent(rain): rpc test failed: %w", err)
	}
	info, err := os.Stat(c.cfg.DownloadDir)
	if err != nil {
		return fmt.Errorf("builtin/torrent(rain): download_dir %q: %w", c.cfg.DownloadDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("builtin/torrent(rain): download_dir %q is not a directory", c.cfg.DownloadDir)
	}
	return nil
}

func (c *Client) EngineSummary() downloads.TorrentEngineSummary {
	items, err := c.Status(context.Background())
	if err != nil {
		return downloads.TorrentEngineSummary{
			DHT:      c.cfg.EnableDHT,
			PEX:      c.cfg.EnablePEX,
			SavePath: c.cfg.DownloadDir,
		}
	}
	var summary downloads.TorrentEngineSummary
	for _, it := range items {
		summary.TotalTorrents++
		summary.DownloadRate += it.DownloadRate
		summary.UploadRate += it.UploadRate
		switch it.Status {
		case downloads.StatusItemQueued:
			summary.Queued++
		case downloads.StatusItemDownloading:
			summary.Downloading++
		case downloads.StatusItemSeeding:
			summary.Seeding++
		case downloads.StatusItemPaused:
			summary.Paused++
		}
	}
	c.mu.RLock()
	summary.DownloadLimit = c.limits.down
	summary.UploadLimit = c.limits.up
	c.mu.RUnlock()
	summary.DHT = c.cfg.EnableDHT
	summary.PEX = c.cfg.EnablePEX
	summary.SavePath = c.cfg.DownloadDir
	return summary
}

func (c *Client) SetSpeedLimits(downBytesPerSec, upBytesPerSec int64) {
	if downBytesPerSec < 0 {
		downBytesPerSec = 0
	}
	if upBytesPerSec < 0 {
		upBytesPerSec = 0
	}
	c.mu.Lock()
	c.limits.down = downBytesPerSec
	c.limits.up = upBytesPerSec
	c.mu.Unlock()
}

func (c *Client) Close() error {
	return nil
}

type PeerInfo struct {
	IP       string  `json:"ip"`
	Port     int     `json:"port"`
	Client   string  `json:"client"`
	Flags    string  `json:"flags"`
	Progress float64 `json:"progress"`
	DownRate int64   `json:"down_rate"`
	UpRate   int64   `json:"up_rate"`
}

type FileInfo struct {
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority string  `json:"priority"`
}

type TrackerInfo struct {
	URL    string `json:"url"`
	Tier   int    `json:"tier"`
	Status string `json:"status"`
	Peers  int    `json:"peers"`
}

type TorrentDetail struct {
	Hash         string  `json:"Hash"`
	Title        string  `json:"Title"`
	Category     string  `json:"Category"`
	SavePath     string  `json:"SavePath"`
	ContentPath  string  `json:"ContentPath"`
	Status       string  `json:"Status"`
	Progress     float64 `json:"Progress"`
	SizeBytes    int64   `json:"SizeBytes"`
	Downloaded   int64   `json:"Downloaded"`
	Uploaded     int64   `json:"Uploaded"`
	DownloadRate int64   `json:"DownloadRate"`
	UploadRate   int64   `json:"UploadRate"`
	Ratio        float64 `json:"Ratio"`
	Paused       bool    `json:"Paused"`

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

func (c *Client) Detail(_ context.Context, id string) (any, error) {
	st, err := c.rpc.GetTorrentStats(id)
	if err != nil {
		return nil, fmt.Errorf("builtin/torrent(rain): get stats: %w", err)
	}
	meta := c.getMeta(id)

	peersRaw, _ := c.rpc.GetTorrentPeers(id)
	peers := make([]PeerInfo, 0, len(peersRaw))
	totalSeeds := 0
	for _, p := range peersRaw {
		host, port := splitHostPort(p.Addr)
		flags := strings.TrimSpace(p.Source)
		if strings.EqualFold(flags, "incoming") || strings.EqualFold(flags, "outgoing") {
			flags = strings.ToUpper(flags[:1])
		}
		peer := PeerInfo{
			IP:       host,
			Port:     port,
			Client:   p.Client,
			Flags:    flags,
			Progress: 0,
			DownRate: int64(p.DownloadSpeed),
			UpRate:   int64(p.UploadSpeed),
		}
		if p.UploadSpeed > 0 && p.DownloadSpeed == 0 {
			totalSeeds++
		}
		peers = append(peers, peer)
	}

	filesRaw, _ := c.rpc.GetTorrentFiles(id)
	fileStatsRaw, _ := c.rpc.GetTorrentFileStats(id)
	progressByPath := make(map[string]float64, len(fileStatsRaw))
	for _, fs := range fileStatsRaw {
		length := fs.File.Length
		p := 0.0
		if length > 0 {
			p = float64(fs.BytesCompleted) / float64(length)
		}
		progressByPath[fs.File.Path] = p
	}
	files := make([]FileInfo, 0, len(filesRaw))
	for _, f := range filesRaw {
		files = append(files, FileInfo{
			Path:     f.Path,
			Size:     f.Length,
			Progress: progressByPath[f.Path],
			Priority: "normal",
		})
	}

	trackersRaw, _ := c.rpc.GetTorrentTrackers(id)
	trackers := make([]TrackerInfo, 0, len(trackersRaw))
	for i, tr := range trackersRaw {
		trackers = append(trackers, TrackerInfo{
			URL:    tr.URL,
			Tier:   i,
			Status: tr.Status,
			Peers:  tr.Seeders + tr.Leechers,
		})
	}

	title := strings.TrimSpace(st.Name)
	if meta.Title != "" {
		title = meta.Title
	}
	if title == "" {
		title = id
	}
	savePath := meta.SavePath
	if savePath == "" {
		savePath = c.cfg.DownloadDir
	}
	storage := c.storageDir(savePath, id)

	size := st.Bytes.Total
	progress := 0.0
	if size > 0 {
		progress = float64(st.Bytes.Completed) / float64(size)
	}
	ratio := 0.0
	if st.Bytes.Downloaded > 0 {
		ratio = float64(st.Bytes.Uploaded) / float64(st.Bytes.Downloaded)
	}
	addedAt := meta.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now()
	}

	return &TorrentDetail{
		Hash:         id,
		Title:        title,
		Category:     meta.Category,
		SavePath:     storage,
		ContentPath:  filepath.Join(storage, st.Name),
		Status:       st.Status,
		Progress:     progress,
		SizeBytes:    size,
		Downloaded:   st.Bytes.Completed,
		Uploaded:     st.Bytes.Uploaded,
		DownloadRate: int64(st.Speed.Download),
		UploadRate:   int64(st.Speed.Upload),
		Ratio:        ratio,
		Paused:       mapStatus(st.Status) == downloads.StatusItemPaused,
		Peers:        peers,
		Files:        files,
		Trackers:     trackers,
		TotalPeers:   st.Peers.Total,
		TotalSeeds:   totalSeeds,
		AddedAt:      addedAt,
		Comment:      "",
		CreatedBy:    "Rain",
		InfoHash:     st.InfoHash,
	}, nil
}

func (c *Client) getMeta(id string) clientMeta {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.meta[id]
}

func splitHostPort(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return addr, 0
	}
	p, _ := strconv.Atoi(portStr)
	return host, p
}
