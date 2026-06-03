package series

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeEpisodeProvider is a test EpisodeProvider returning canned data.
type fakeEpisodeProvider struct {
	resolveID   int
	resolveErr  error
	episodes    []ProviderEpisode
	episodesErr error

	resolvedTitle string
	resolvedYear  int
}

func (f *fakeEpisodeProvider) ResolveSeriesID(_ context.Context, title string, year int, _ map[string]string) (int, error) {
	f.resolvedTitle = title
	f.resolvedYear = year
	return f.resolveID, f.resolveErr
}

func (f *fakeEpisodeProvider) SeriesEpisodes(_ context.Context, _ int) ([]ProviderEpisode, error) {
	return f.episodes, f.episodesErr
}

// soloLevelingEpisodes mirrors the issue: TVDB splits into S1 (12) + S2 (13).
func soloLevelingEpisodes() []ProviderEpisode {
	var eps []ProviderEpisode
	for i := 1; i <= 12; i++ {
		eps = append(eps, ProviderEpisode{SeasonNumber: 1, EpisodeNumber: i, AbsoluteNumber: i})
	}
	for i := 1; i <= 13; i++ {
		eps = append(eps, ProviderEpisode{SeasonNumber: 2, EpisodeNumber: i, AbsoluteNumber: 12 + i})
	}
	return eps
}

func TestBuildSeasonsAndEpisodes_MultiCour(t *testing.T) {
	ts := time.Now()
	seasons, episodes := buildSeasonsAndEpisodes("solo-leveling-2024", soloLevelingEpisodes(), ts)

	if len(seasons) != 2 {
		t.Fatalf("expected 2 seasons, got %d", len(seasons))
	}
	if seasons[0].SeasonNumber != 1 || seasons[0].EpisodeCount != 12 {
		t.Fatalf("season 1 wrong: %+v", seasons[0])
	}
	if seasons[1].SeasonNumber != 2 || seasons[1].EpisodeCount != 13 {
		t.Fatalf("season 2 wrong: %+v", seasons[1])
	}
	if len(episodes) != 25 {
		t.Fatalf("expected 25 episodes, got %d", len(episodes))
	}

	// S02E01 must exist and belong to season 2 (this is what release matching needs).
	want := "solo-leveling-2024-s02-e001"
	var found *Episode
	for _, e := range episodes {
		if e.ID == want {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatalf("expected episode id %q (S02E01) to exist", want)
	}
	if found.SeasonID != "solo-leveling-2024-s02" || found.EpisodeNumber != 1 {
		t.Fatalf("S02E01 mapped incorrectly: %+v", found)
	}
}

func TestBuildSeasonsAndEpisodes_SkipsInvalidAndDuplicates(t *testing.T) {
	ts := time.Now()
	eps := []ProviderEpisode{
		{SeasonNumber: 1, EpisodeNumber: 1},
		{SeasonNumber: 1, EpisodeNumber: 1},  // duplicate
		{SeasonNumber: 1, EpisodeNumber: 0},  // invalid episode
		{SeasonNumber: -1, EpisodeNumber: 5}, // invalid season
		{SeasonNumber: 0, EpisodeNumber: 1},  // specials kept
	}
	seasons, episodes := buildSeasonsAndEpisodes("show", eps, ts)
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes (dedup + skip invalid), got %d", len(episodes))
	}
	if len(seasons) != 2 {
		t.Fatalf("expected 2 seasons (1 and specials), got %d", len(seasons))
	}
	// Specials season title.
	for _, se := range seasons {
		if se.SeasonNumber == 0 && se.Title != "Specials" {
			t.Fatalf("expected specials title, got %q", se.Title)
		}
	}
}

func TestPopulateProviderEpisodes_AnimeUsesTVDB(t *testing.T) {
	repo := newMockRepo()
	fp := &fakeEpisodeProvider{resolveID: 999, episodes: soloLevelingEpisodes()}
	svc := NewService(repo, "", WithEpisodeProvider(fp)).(*service)

	tvdbID := "" // none stored -> must resolve via provider
	sr := &Series{ID: "solo-leveling-2024", Title: "Solo Leveling", Year: 2024, SeriesType: TypeAnime}
	_ = tvdbID

	ok, err := svc.populateProviderEpisodes(context.Background(), sr, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected episodes to be stored")
	}
	if got := len(repo.episodes["solo-leveling-2024"]); got != 25 {
		t.Fatalf("expected 25 episodes stored, got %d", got)
	}
	if got := len(repo.seasons["solo-leveling-2024"]); got != 2 {
		t.Fatalf("expected 2 seasons stored, got %d", got)
	}
	// Resolved TVDB ID must be written back for persistence.
	if sr.TVDBID == nil || *sr.TVDBID != "999" {
		t.Fatalf("expected resolved TVDB id 999 to be set, got %v", sr.TVDBID)
	}
	if fp.resolvedTitle != "Solo Leveling" || fp.resolvedYear != 2024 {
		t.Fatalf("resolver received wrong args: %q %d", fp.resolvedTitle, fp.resolvedYear)
	}
}

func TestPopulateProviderEpisodes_UnresolvedReturnsFalse(t *testing.T) {
	repo := newMockRepo()
	fp := &fakeEpisodeProvider{resolveID: 0} // cannot resolve
	svc := NewService(repo, "", WithEpisodeProvider(fp)).(*service)

	sr := &Series{ID: "x", Title: "Unknown", SeriesType: TypeAnime}
	ok, err := svc.populateProviderEpisodes(context.Background(), sr, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false when provider cannot resolve")
	}
	if len(repo.episodes["x"]) != 0 {
		t.Fatalf("expected no episodes stored")
	}
}

func TestPopulateProviderEpisodes_ProviderErrorPropagates(t *testing.T) {
	repo := newMockRepo()
	fp := &fakeEpisodeProvider{resolveID: 5, episodesErr: errors.New("boom")}
	svc := NewService(repo, "", WithEpisodeProvider(fp)).(*service)

	sr := &Series{ID: "x", Title: "Anime", SeriesType: TypeAnime, TVDBID: ptr("5")}
	ok, err := svc.populateProviderEpisodes(context.Background(), sr, time.Now())
	if err == nil {
		t.Fatalf("expected error to propagate")
	}
	if ok {
		t.Fatalf("expected ok=false on provider error")
	}
}

func TestUseTVDBEpisodes(t *testing.T) {
	svcNoProvider := NewService(newMockRepo(), "").(*service)
	if svcNoProvider.useTVDBEpisodes(&Series{SeriesType: TypeAnime}) {
		t.Fatalf("should be false without a provider")
	}

	svc := NewService(newMockRepo(), "", WithEpisodeProvider(&fakeEpisodeProvider{})).(*service)
	if svc.useTVDBEpisodes(&Series{SeriesType: TypeStandard}) {
		t.Fatalf("standard series must not use TVDB")
	}
	if !svc.useTVDBEpisodes(&Series{SeriesType: TypeAnime}) {
		t.Fatalf("anime series with provider must use TVDB")
	}
}

func TestResolveTVDBID_PrefersStored(t *testing.T) {
	fp := &fakeEpisodeProvider{resolveID: 111}
	svc := NewService(newMockRepo(), "", WithEpisodeProvider(fp)).(*service)
	sr := &Series{ID: "x", Title: "T", TVDBID: ptr("42"), SeriesType: TypeAnime}
	if got := svc.resolveTVDBID(context.Background(), sr); got != 42 {
		t.Fatalf("expected stored id 42, got %d", got)
	}
	if fp.resolvedTitle != "" {
		t.Fatalf("resolver should not be called when id is stored")
	}
}

func ptr(s string) *string { return &s }
