package musicsearch

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/ebenderooock/loom/internal/customformats"
	"github.com/ebenderooock/loom/internal/downloads"
	"github.com/ebenderooock/loom/internal/indexers"
	"github.com/ebenderooock/loom/internal/music"
)

type acquisitionMusicRepo struct {
	music.Repository
	album          *music.Album
	owner          *music.Artist
	definitions    []*music.AudioQualityDefinition
	profile        *music.AudioQualityProfile
	profileLookups []string
}

func (r *acquisitionMusicRepo) GetAlbum(_ context.Context, id string) (*music.Album, error) {
	if r.album != nil && r.album.ID == id {
		return r.album, nil
	}
	return nil, nil
}

func (r *acquisitionMusicRepo) GetArtist(_ context.Context, id string) (*music.Artist, error) {
	if r.owner != nil && r.owner.ID == id {
		return r.owner, nil
	}
	return nil, nil
}

func (r *acquisitionMusicRepo) ListAudioQualityDefinitions(context.Context) ([]*music.AudioQualityDefinition, error) {
	return r.definitions, nil
}

func (r *acquisitionMusicRepo) GetAudioQualityProfile(_ context.Context, id string) (*music.AudioQualityProfile, error) {
	r.profileLookups = append(r.profileLookups, id)
	if r.profile != nil && r.profile.ID == id {
		return r.profile, nil
	}
	return nil, nil
}

type acquisitionIndexerRepo struct{ indexers.Repository }

func (acquisitionIndexerRepo) List(context.Context) ([]indexers.Definition, error) {
	return []indexers.Definition{{ID: "music-test"}}, nil
}

type acquisitionTestIndexer struct {
	mu      sync.Mutex
	queries []string
	results []indexers.Result
}

type acquisitionTestClient struct {
	downloads.DownloadClient
	id string
}

func (c *acquisitionTestClient) ID() string                 { return c.id }
func (*acquisitionTestClient) Name() string                 { return "Acquisition test client" }
func (*acquisitionTestClient) Kind() downloads.Kind         { return downloads.KindNull }
func (*acquisitionTestClient) Protocol() downloads.Protocol { return downloads.ProtocolTorrent }
func (c *acquisitionTestClient) Add(_ context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	return downloads.AddResult{ClientID: c.id, ItemID: req.MediaID + "-download"}, nil
}

func (*acquisitionTestIndexer) ID() string   { return "music-test" }
func (*acquisitionTestIndexer) Name() string { return "Music test indexer" }
func (*acquisitionTestIndexer) Caps() indexers.Caps {
	return indexers.Caps{Categories: []indexers.Category{indexers.CategoryAudio}}
}
func (i *acquisitionTestIndexer) Test(context.Context) error { return nil }
func (i *acquisitionTestIndexer) Search(_ context.Context, q indexers.Query) (*indexers.Results, error) {
	i.mu.Lock()
	i.queries = append(i.queries, q.Term)
	i.mu.Unlock()
	return &indexers.Results{IndexerID: i.ID(), Items: i.results}, nil
}

func (i *acquisitionTestIndexer) queryTerms() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string(nil), i.queries...)
}

func TestInvalidReleaseLinkUsesBadRequestStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeSearchErr(recorder, errors.Join(ErrInvalidReleaseLink, errors.New("bad link")))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestBuildAudioQueries(t *testing.T) {
	qs := buildAudioQueries("Pink Floyd", "Animals", 1977)
	if len(qs) != 2 {
		t.Fatalf("want 2 specific queries, got %d", len(qs))
	}
	if qs[0].Term != "Pink Floyd Animals" {
		t.Errorf("tier0 = %q", qs[0].Term)
	}
	if qs[1].Term != "Pink Floyd Animals 1977" {
		t.Errorf("tier1 = %q", qs[1].Term)
	}
	for _, q := range qs {
		if q.Mode != indexers.ModeSearch {
			t.Errorf("mode = %q, want search", q.Mode)
		}
		if len(q.Categories) == 0 || q.Categories[0] != indexers.CategoryAudio {
			t.Errorf("categories = %v", q.Categories)
		}
	}
}

func TestBuildAudioQueriesNoYear(t *testing.T) {
	qs := buildAudioQueries("Artist", "Album", 0)
	if len(qs) != 1 {
		t.Fatalf("want 1 specific query without year, got %d", len(qs))
	}
}

func TestStandaloneListReleasesUsesCreditsAndConfiguredProfile(t *testing.T) {
	tests := []struct {
		name        string
		album       *music.Album
		title       string
		wantQueries []string
	}{
		{
			name: "single credited artist",
			album: &music.Album{
				ID: "solo", Title: "Solo Record", ReleaseDate: "2021",
				QualityProfileID: "solo-profile",
				Artists:          music.AlbumArtistCredits{{Name: "Solo Performer"}},
			},
			title:       "Solo Performer - Solo Record FLAC",
			wantQueries: []string{"Solo Performer Solo Record"},
		},
		{
			name: "compilation queries various artists and contributors",
			album: &music.Album{
				ID: "compilation", Title: "Great Soundtrack", AlbumType: "Album",
				SecondaryTypes: music.StringSlice{"Compilation"}, QualityProfileID: "comp-profile",
				Artists: music.AlbumArtistCredits{{Name: "First Composer"}, {Name: "Second Composer"}},
			},
			title: "Various Artists - Great Soundtrack FLAC",
			wantQueries: []string{
				"Various Artists Great Soundtrack",
				"First Composer Great Soundtrack",
				"Second Composer Great Soundtrack",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indexer := &acquisitionTestIndexer{results: []indexers.Result{{
				IndexerID: "music-test", GUID: "result-1", Title: tt.title,
				Link: "https://downloads.example/test.torrent",
			}}}
			registry := indexers.NewRegistry()
			if err := registry.Register(indexer); err != nil {
				t.Fatal(err)
			}
			indexerSvc, err := indexers.NewService(indexers.ServiceOptions{
				Repository: &acquisitionIndexerRepo{}, Registry: registry,
			})
			if err != nil {
				t.Fatal(err)
			}
			repo := &acquisitionMusicRepo{
				album: tt.album,
				definitions: []*music.AudioQualityDefinition{
					{ID: "aq_flac", Name: "FLAC", Format: "flac", Lossless: true, TierOrder: 8},
					{ID: "aq_mp3_320", Name: "MP3 320", Format: "mp3", Bitrate: 320, TierOrder: 4},
				},
				profile: &music.AudioQualityProfile{
					ID:    tt.album.QualityProfileID,
					Items: []byte(`[{"definition_id":"aq_mp3_320","allowed":true}]`),
				},
			}
			engine := NewEngine(indexerSvc, downloads.NewRegistry(), repo, slog.Default())
			candidates, err := engine.ListReleases(context.Background(), tt.album.ID)
			if err != nil {
				t.Fatalf("ListReleases: %v", err)
			}
			if len(candidates) != 1 || candidates[0].Allowed {
				t.Fatalf("candidates = %+v, parsed=%+v terms=%v; want one profile-rejected FLAC candidate",
					candidates, music.ParseMusic(tt.title), indexer.queryTerms())
			}
			if len(repo.profileLookups) != 1 || repo.profileLookups[0] != tt.album.QualityProfileID {
				t.Fatalf("profile lookups = %v, want %q", repo.profileLookups, tt.album.QualityProfileID)
			}
			terms := indexer.queryTerms()
			for _, wanted := range tt.wantQueries {
				found := false
				for _, term := range terms {
					if term == wanted {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("search terms %v do not include %q", terms, wanted)
				}
			}
		})
	}
}

func TestGrabReleaseAllowsStandaloneAlbum(t *testing.T) {
	album := &music.Album{
		ID: "standalone", Title: "Solo Record", Artists: music.AlbumArtistCredits{{Name: "Solo Performer"}},
		QualityProfileID: "album-profile",
	}
	repo := &acquisitionMusicRepo{album: album}
	dlRegistry := downloads.NewRegistry()
	if err := dlRegistry.Register(&acquisitionTestClient{id: "torrent-client"}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(nil, dlRegistry, repo, slog.Default())
	seeders := 1
	result, err := engine.GrabRelease(context.Background(), album.ID, GrabRequest{
		Title: "Solo Performer - Solo Record FLAC", IndexerID: "indexer",
		Infohash: "0123456789abcdef0123456789abcdef01234567", Seeders: &seeders,
	})
	if err != nil {
		t.Fatalf("GrabRelease: %v", err)
	}
	if result.AlbumID != album.ID || result.DownloadID != album.ID+"-download" {
		t.Fatalf("grab result = %+v", result)
	}
}

func TestAlbumQualityProfileRespectsClearedOwnerProfile(t *testing.T) {
	album := &music.Album{QualityProfileID: "previous-owner-profile"}
	if got := albumQualityProfileID(album, &music.Artist{}); got != "" {
		t.Fatalf("cleared owner profile fell back to old album profile: %q", got)
	}
	if got := albumQualityProfileID(album, nil); got != album.QualityProfileID {
		t.Fatalf("standalone album profile = %q, want %q", got, album.QualityProfileID)
	}
}

func TestBuildTrackQueriesAndIdentity(t *testing.T) {
	queries := buildTrackQueries("Pink Floyd", "Animals", "Dogs")
	if len(queries) != 2 || queries[0].Term != "Pink Floyd Animals Dogs" ||
		queries[1].Term != "Pink Floyd Dogs" {
		t.Fatalf("track queries = %+v", queries)
	}
	if !trackIdentityMatch("Pink Floyd - Animals - Dogs (FLAC)", "Pink Floyd", "Dogs") {
		t.Fatal("expected release matching artist and track")
	}
	if trackIdentityMatch("Pink Floyd - Animals - Pigs (FLAC)", "Pink Floyd", "Dogs") {
		t.Fatal("unrelated track should not match")
	}
	if trackIdentityMatch("Some Artist - Dogs (FLAC)", "Pink Floyd", "Dogs") {
		t.Fatal("unrelated artist should not match")
	}
}

func TestIdentityMatch(t *testing.T) {
	tests := []struct {
		title  string
		artist string
		album  string
		want   bool
	}{
		{"Pink Floyd - Animals (1977) [FLAC]", "Pink Floyd", "Animals", true},
		{"Pink.Floyd-Animals-1977-FLAC", "Pink Floyd", "Animals", true},
		{"Led Zeppelin - IV [FLAC]", "Pink Floyd", "Animals", false},
		{"Pink Floyd - The Wall [FLAC]", "Pink Floyd", "Animals", false},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			parsed := music.ParseMusic(tt.title)
			got := identityMatch(tt.title, parsed, tt.artist, tt.album)
			if got != tt.want {
				t.Errorf("identityMatch = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdentityMatchUsesUnicodeAndWholePhrases(t *testing.T) {
	tests := []struct {
		title, artist, album string
		want                 bool
	}{
		{"Björk - Debut (1993) FLAC", "Bjork", "Debut", true},
		{"Sigur Rós — Álbum FLAC", "Sigur Ros", "Album", true},
		{"The Artist - Art of Noise FLAC", "Art", "Art of Noise", false},
		{"Pink Floyd - Animals Remastered FLAC", "Pink Floyd", "Animal", false},
	}
	for _, tt := range tests {
		parsed := music.ParseMusic(tt.title)
		got := identityMatch(tt.title, parsed, tt.artist, tt.album)
		if got != tt.want {
			t.Errorf("identityMatch(%q, %q, %q) = %v, want %v; parsed=%+v",
				tt.title, tt.artist, tt.album, got, tt.want, parsed)
		}
	}
}

func TestAlbumYear(t *testing.T) {
	tests := []struct {
		date string
		want int
	}{
		{"1977-01-23", 1977},
		{"2020", 2020},
		{"", 0},
		{"unknown", 0},
	}
	for _, tt := range tests {
		got := albumYear(&music.Album{ReleaseDate: tt.date})
		if got != tt.want {
			t.Errorf("albumYear(%q) = %d, want %d", tt.date, got, tt.want)
		}
	}
}

func TestInferProtocol(t *testing.T) {
	seeders := 5
	torrent := &indexers.Result{Seeders: &seeders}
	if inferProtocol(torrent) != downloads.ProtocolTorrent {
		t.Error("expected torrent for seeded result")
	}
	usenet := &indexers.Result{Link: "https://x/file.nzb"}
	if inferProtocol(usenet) != downloads.ProtocolUsenet {
		t.Error("expected usenet for .nzb link")
	}
	if got := inferProtocol(&indexers.Result{Link: "https://x/download?id=123"}); got != "" {
		t.Errorf("ambiguous link protocol = %q, want unknown", got)
	}
	if got := inferProtocol(&indexers.Result{Link: "file:///tmp/file.torrent"}); got != "" {
		t.Errorf("non-web link protocol = %q, want unknown", got)
	}
}

func TestBuildDownloadRequest(t *testing.T) {
	req := buildDownloadRequest(&indexers.Result{
		Title:    "Artist - Album [FLAC]",
		Infohash: "0123456789abcdef0123456789abcdef01234567",
	})
	if req.Magnet == "" {
		t.Error("expected magnet built from infohash")
	}
	if req.Title == "" {
		t.Error("expected title carried through")
	}
	u, err := url.Parse(req.Magnet)
	if err != nil {
		t.Fatalf("parse magnet: %v", err)
	}
	if got := u.Query().Get("dn"); got != "Artist - Album [FLAC]" {
		t.Fatalf("dn = %q", got)
	}
}

func TestNormalize(t *testing.T) {
	if got := normalize("Pink Floyd!"); got != "pinkfloyd" {
		t.Errorf("normalize = %q", got)
	}
}

func TestFormatScore(t *testing.T) {
	cf := customformats.CustomFormat{
		ID:   "cf_flac",
		Name: "Prefer FLAC",
		Specifications: []customformats.Specification{
			{Implementation: customformats.ImplAudio, Fields: map[string]any{"value": "FLAC"}},
		},
	}
	eng := NewEngine(nil, nil, nil, slog.Default())
	eng.SetCustomFormats(customformats.NewEngine([]customformats.CustomFormat{cf}))

	profile := &music.AudioQualityProfile{
		FormatItems:    []music.AudioFormatItem{{FormatID: "cf_flac", Score: 50}},
		MinFormatScore: 10,
	}

	// FLAC release matches → score 50.
	flac := &music.MusicRelease{Format: "FLAC"}
	res := indexers.Result{Title: "Artist - Album 2020 FLAC", IndexerID: "x"}
	score, matches := eng.formatScore(profile, flac, &res)
	if score != 50 {
		t.Fatalf("FLAC score = %d, want 50", score)
	}
	if len(matches) != 1 || matches[0].CustomFormatID != "cf_flac" {
		t.Fatalf("matches = %+v, want one cf_flac match", matches)
	}

	// MP3 release does not match → score 0.
	mp3 := &music.MusicRelease{Format: "MP3"}
	res2 := indexers.Result{Title: "Artist - Album 2020 MP3 320", IndexerID: "x"}
	if s, _ := eng.formatScore(profile, mp3, &res2); s != 0 {
		t.Fatalf("MP3 score = %d, want 0", s)
	}
}

func TestFormatScoreNoEngine(t *testing.T) {
	eng := NewEngine(nil, nil, nil, slog.Default())
	rel := &music.MusicRelease{Format: "FLAC"}
	res := indexers.Result{Title: "x FLAC", IndexerID: "x"}
	if s, m := eng.formatScore(nil, rel, &res); s != 0 || m != nil {
		t.Fatalf("no-engine score = %d matches=%v, want 0/nil", s, m)
	}
}
