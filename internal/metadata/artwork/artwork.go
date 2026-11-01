// Package artwork fetches music artwork and biographies that MusicBrainz does
// not carry, from fanart.tv (API key required) and TheAudioDB (a free shared
// key is used when none is configured). Every lookup is best-effort: callers
// must treat errors as "no artwork" and never fail an add or refresh on them.
package artwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/metadata"
)

const (
	// DefaultFanartBaseURL is the fanart.tv v3 API.
	DefaultFanartBaseURL = "https://webservice.fanart.tv/v3"
	// DefaultAudioDBBaseURL is TheAudioDB v1 JSON API.
	DefaultAudioDBBaseURL = "https://www.theaudiodb.com/api/v1/json"
	// FreeAudioDBKey is TheAudioDB's public test key (lookups by MBID work).
	FreeAudioDBKey = "123"

	defaultTimeout  = 8 * time.Second
	defaultCacheTTL = 12 * time.Hour
	maxCacheEntries = 2000
	maxBodyBytes    = 4 << 20
)

// Config configures the artwork provider.
type Config struct {
	FanartAPIKey   string
	AudioDBAPIKey  string // empty uses FreeAudioDBKey
	FanartBaseURL  string
	AudioDBBaseURL string
	HTTPClient     *http.Client
	UserAgent      string
	CacheTTL       time.Duration
}

// Provider implements metadata.MusicArtworkProvider.
type Provider struct {
	http      *http.Client
	userAgent string
	fanartURL string
	dbURL     string
	ttl       time.Duration

	mu         sync.RWMutex
	fanartKey  string
	audioDBKey string

	cacheMu sync.Mutex
	cache   map[string]cacheEntry
}

type cacheEntry struct {
	value   any
	expires time.Time
}

var _ metadata.MusicArtworkProvider = (*Provider)(nil)

// New builds a provider.
func New(cfg Config) *Provider {
	p := &Provider{
		http:      cfg.HTTPClient,
		userAgent: cfg.UserAgent,
		fanartURL: strings.TrimRight(cfg.FanartBaseURL, "/"),
		dbURL:     strings.TrimRight(cfg.AudioDBBaseURL, "/"),
		ttl:       cfg.CacheTTL,
		cache:     make(map[string]cacheEntry),
	}
	if p.http == nil {
		p.http = &http.Client{Timeout: defaultTimeout}
	}
	if p.userAgent == "" {
		p.userAgent = "Loom (https://github.com/ebenderooock/loom)"
	}
	if p.fanartURL == "" {
		p.fanartURL = DefaultFanartBaseURL
	}
	if p.dbURL == "" {
		p.dbURL = DefaultAudioDBBaseURL
	}
	if p.ttl <= 0 {
		p.ttl = defaultCacheTTL
	}
	p.SetKeys(cfg.FanartAPIKey, cfg.AudioDBAPIKey)
	return p
}

// SetKeys replaces the API keys and drops cached results.
func (p *Provider) SetKeys(fanartKey, audioDBKey string) {
	fanartKey = strings.TrimSpace(fanartKey)
	audioDBKey = strings.TrimSpace(audioDBKey)
	if audioDBKey == "" {
		audioDBKey = FreeAudioDBKey
	}
	p.mu.Lock()
	p.fanartKey, p.audioDBKey = fanartKey, audioDBKey
	p.mu.Unlock()
	p.cacheMu.Lock()
	p.cache = make(map[string]cacheEntry)
	p.cacheMu.Unlock()
}

func (p *Provider) keys() (fanart, audioDB string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.fanartKey, p.audioDBKey
}

// ArtistArtwork merges fanart.tv (preferred) and TheAudioDB artwork. It returns
// an error only when every source failed; partial results are returned as-is.
func (p *Provider) ArtistArtwork(ctx context.Context, mbid string) (*metadata.ArtistArtwork, error) {
	mbid = strings.TrimSpace(mbid)
	if mbid == "" {
		return nil, nil
	}
	if v, ok := p.cached("artist:" + mbid); ok {
		return v.(*metadata.ArtistArtwork), nil
	}
	fanartKey, dbKey := p.keys()

	var (
		wg            sync.WaitGroup
		fan           *fanartArtist
		db            *audioDBArtist
		fanErr, dbErr error
	)
	if fanartKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fan, fanErr = p.fetchFanartArtist(ctx, fanartKey, mbid)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		db, dbErr = p.fetchAudioDBArtist(ctx, dbKey, mbid)
	}()
	wg.Wait()

	if dbErr != nil && (fanartKey == "" || fanErr != nil) {
		return nil, errors.Join(fanErr, dbErr)
	}
	out := &metadata.ArtistArtwork{}
	if fan != nil {
		out.ThumbURL = firstImage(fan.ArtistThumb)
		out.BackgroundURL = firstImage(fan.ArtistBackground)
		out.LogoURL = firstImage(fan.HDMusicLogo, fan.MusicLogo)
		for rg, al := range fan.Albums {
			if cover := firstImage(al.AlbumCover); cover != "" {
				if out.AlbumCovers == nil {
					out.AlbumCovers = make(map[string]string)
				}
				out.AlbumCovers[rg] = cover
			}
		}
	}
	if db != nil {
		out.ThumbURL = firstNonEmpty(out.ThumbURL, db.Thumb)
		out.BackgroundURL = firstNonEmpty(out.BackgroundURL, db.Fanart, db.WideThumb)
		out.LogoURL = firstNonEmpty(out.LogoURL, db.Logo)
		out.Biography = strings.TrimSpace(firstNonEmpty(db.BiographyEN, db.Biography))
	}
	// Only cache complete answers so a transient failure is retried.
	if fanErr == nil && dbErr == nil {
		p.store("artist:"+mbid, out)
	}
	return out, nil
}

// AlbumArtwork returns TheAudioDB's cover and description for a release-group.
func (p *Provider) AlbumArtwork(ctx context.Context, mbid string) (*metadata.AlbumArtwork, error) {
	mbid = strings.TrimSpace(mbid)
	if mbid == "" {
		return nil, nil
	}
	if v, ok := p.cached("album:" + mbid); ok {
		return v.(*metadata.AlbumArtwork), nil
	}
	_, dbKey := p.keys()
	var resp struct {
		Album []audioDBAlbum `json:"album"`
	}
	endpoint := fmt.Sprintf("%s/%s/album-mb.php?i=%s", p.dbURL, url.PathEscape(dbKey), url.QueryEscape(mbid))
	if err := p.getJSON(ctx, endpoint, nil, &resp); err != nil && !errors.Is(err, errNotFound) {
		return nil, fmt.Errorf("theaudiodb album: %w", err)
	}
	out := &metadata.AlbumArtwork{}
	if len(resp.Album) > 0 {
		a := resp.Album[0]
		out.CoverURL = a.Thumb
		out.Description = strings.TrimSpace(firstNonEmpty(a.DescriptionEN, a.Description))
	}
	p.store("album:"+mbid, out)
	return out, nil
}

// TestKeys checks that the given keys are accepted. An empty fanart.tv key is
// skipped; an empty TheAudioDB key tests the shared free key.
func (p *Provider) TestKeys(ctx context.Context, fanartKey, audioDBKey string) (fanartErr, audioDBErr error) {
	// Linkin Park is present in both services.
	const probe = "f59c5520-5f46-4d2c-b2c4-822eabf53419"
	if k := strings.TrimSpace(fanartKey); k != "" {
		var fan *fanartArtist
		fan, fanartErr = p.fetchFanartArtist(ctx, k, probe)
		if fanartErr == nil && fan == nil {
			fanartErr = errors.New("key returned no data")
		}
	}
	k := strings.TrimSpace(audioDBKey)
	if k == "" {
		k = FreeAudioDBKey
	}
	db, audioDBErr := p.fetchAudioDBArtist(ctx, k, probe)
	if audioDBErr == nil && db == nil {
		audioDBErr = errors.New("key returned no data")
	}
	return fanartErr, audioDBErr
}

// --- fanart.tv ---

type fanartImage struct {
	URL string `json:"url"`
}

type fanartAlbum struct {
	AlbumCover []fanartImage `json:"albumcover"`
}

type fanartArtist struct {
	ArtistThumb      []fanartImage          `json:"artistthumb"`
	ArtistBackground []fanartImage          `json:"artistbackground"`
	HDMusicLogo      []fanartImage          `json:"hdmusiclogo"`
	MusicLogo        []fanartImage          `json:"musiclogo"`
	Albums           map[string]fanartAlbum `json:"albums"`
}

func (p *Provider) fetchFanartArtist(ctx context.Context, key, mbid string) (*fanartArtist, error) {
	endpoint := fmt.Sprintf("%s/music/%s", p.fanartURL, url.PathEscape(mbid))
	out := &fanartArtist{}
	err := p.getJSON(ctx, endpoint, map[string]string{"api-key": key}, out)
	if errors.Is(err, errNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fanart.tv: %w", err)
	}
	return out, nil
}

// --- TheAudioDB ---

type audioDBArtist struct {
	Thumb       string `json:"strArtistThumb"`
	Fanart      string `json:"strArtistFanart"`
	WideThumb   string `json:"strArtistWideThumb"`
	Logo        string `json:"strArtistLogo"`
	BiographyEN string `json:"strBiographyEN"`
	Biography   string `json:"strBiography"`
}

type audioDBAlbum struct {
	Thumb         string `json:"strAlbumThumb"`
	DescriptionEN string `json:"strDescriptionEN"`
	Description   string `json:"strDescription"`
}

func (p *Provider) fetchAudioDBArtist(ctx context.Context, key, mbid string) (*audioDBArtist, error) {
	var resp struct {
		Artists []audioDBArtist `json:"artists"`
	}
	endpoint := fmt.Sprintf("%s/%s/artist-mb.php?i=%s", p.dbURL, url.PathEscape(key), url.QueryEscape(mbid))
	err := p.getJSON(ctx, endpoint, nil, &resp)
	if errors.Is(err, errNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("theaudiodb: %w", err)
	}
	if len(resp.Artists) == 0 {
		return nil, nil
	}
	return &resp.Artists[0], nil
}

// --- HTTP + cache ---

var errNotFound = errors.New("not found")

func (p *Provider) getJSON(ctx context.Context, endpoint string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := p.http.Do(req) //nolint:gosec // Endpoint is built from configured artwork API base URLs.
	if err != nil {
		return redactURL(err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("API key rejected (HTTP %d)", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	// TheAudioDB answers unknown IDs with an empty body or a null list.
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// redactURL drops the request URL, which may embed an API key, from transport
// errors so keys never reach logs.
func redactURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("%s request failed: %w", uerr.Op, uerr.Err)
	}
	return err
}

func (p *Provider) cached(key string) (any, bool) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	e, ok := p.cache[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (p *Provider) store(key string, v any) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	if len(p.cache) >= maxCacheEntries {
		p.cache = make(map[string]cacheEntry)
	}
	p.cache[key] = cacheEntry{value: v, expires: time.Now().Add(p.ttl)}
}

func firstImage(lists ...[]fanartImage) string {
	for _, l := range lists {
		for _, img := range l {
			if img.URL != "" {
				return img.URL
			}
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
