package series

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ── Mock Repository ──────────────────────────────────────────────────

type mockRepo struct {
	series     map[string]*Series
	seasons    map[string][]*Season
	episodes   map[string][]*Episode
	credits    map[string][]*SeriesCredit
	stats      map[string]*EpisodeStats
	allStats   map[string]*EpisodeStats
	seasonStats map[string]map[string]*EpisodeStats

	createSeriesErr error
	updateSeriesErr error
	deleteSeriesErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		series:      make(map[string]*Series),
		seasons:     make(map[string][]*Season),
		episodes:    make(map[string][]*Episode),
		credits:     make(map[string][]*SeriesCredit),
		stats:       make(map[string]*EpisodeStats),
		allStats:    make(map[string]*EpisodeStats),
		seasonStats: make(map[string]map[string]*EpisodeStats),
	}
}

func (r *mockRepo) ListSeries(_ context.Context) ([]*Series, error) {
	var out []*Series
	for _, s := range r.series {
		out = append(out, s)
	}
	return out, nil
}
func (r *mockRepo) GetSeries(_ context.Context, id string) (*Series, error) {
	s, ok := r.series[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}
func (r *mockRepo) CreateSeries(_ context.Context, s *Series) error {
	if r.createSeriesErr != nil {
		return r.createSeriesErr
	}
	r.series[s.ID] = s
	return nil
}
func (r *mockRepo) UpdateSeries(_ context.Context, s *Series) error {
	if r.updateSeriesErr != nil {
		return r.updateSeriesErr
	}
	r.series[s.ID] = s
	return nil
}
func (r *mockRepo) DeleteSeries(_ context.Context, id string) error {
	if r.deleteSeriesErr != nil {
		return r.deleteSeriesErr
	}
	delete(r.series, id)
	return nil
}
func (r *mockRepo) ListSeasons(_ context.Context, seriesID string) ([]*Season, error) {
	return r.seasons[seriesID], nil
}
func (r *mockRepo) GetSeason(_ context.Context, id string) (*Season, error) { return nil, nil }
func (r *mockRepo) CreateSeason(_ context.Context, s *Season) error {
	r.seasons[s.SeriesID] = append(r.seasons[s.SeriesID], s)
	return nil
}
func (r *mockRepo) UpdateSeason(_ context.Context, _ *Season) error { return nil }
func (r *mockRepo) ListEpisodes(_ context.Context, seriesID string, seasonNum *int) ([]*Episode, error) {
	eps := r.episodes[seriesID]
	if seasonNum == nil {
		return eps, nil
	}
	var filtered []*Episode
	for _, e := range eps {
		// Simple filter by season ID containing the season number
		filtered = append(filtered, e)
	}
	return filtered, nil
}
func (r *mockRepo) GetEpisode(_ context.Context, id string) (*Episode, error) {
	for _, eps := range r.episodes {
		for _, e := range eps {
			if e.ID == id {
				return e, nil
			}
		}
	}
	return nil, errors.New("not found")
}
func (r *mockRepo) CreateEpisode(_ context.Context, e *Episode) error {
	r.episodes[e.SeriesID] = append(r.episodes[e.SeriesID], e)
	return nil
}
func (r *mockRepo) UpdateEpisode(_ context.Context, e *Episode) error {
	for i, ep := range r.episodes[e.SeriesID] {
		if ep.ID == e.ID {
			r.episodes[e.SeriesID][i] = e
			return nil
		}
	}
	return nil
}
func (r *mockRepo) CreateEpisodeFile(_ context.Context, _ *EpisodeFile) error { return nil }
func (r *mockRepo) DeleteSeasonsBySeriesID(_ context.Context, seriesID string) error {
	delete(r.seasons, seriesID)
	return nil
}
func (r *mockRepo) DeleteEpisodesBySeriesID(_ context.Context, seriesID string) error {
	delete(r.episodes, seriesID)
	return nil
}
func (r *mockRepo) DeleteCreditsBySeriesID(_ context.Context, seriesID string) error {
	delete(r.credits, seriesID)
	return nil
}
func (r *mockRepo) GetCredits(_ context.Context, seriesID string) ([]*SeriesCredit, error) {
	return r.credits[seriesID], nil
}
func (r *mockRepo) SaveCredits(_ context.Context, seriesID string, credits []*SeriesCredit) error {
	r.credits[seriesID] = credits
	return nil
}
func (r *mockRepo) GetEpisodeStats(_ context.Context, seriesID string) (*EpisodeStats, error) {
	s, ok := r.stats[seriesID]
	if !ok {
		return &EpisodeStats{}, nil
	}
	return s, nil
}
func (r *mockRepo) GetAllEpisodeStats(_ context.Context) (map[string]*EpisodeStats, error) {
	return r.allStats, nil
}
func (r *mockRepo) GetSeasonEpisodeStats(_ context.Context, seriesID string) (map[string]*EpisodeStats, error) {
	return r.seasonStats[seriesID], nil
}

// ── Tests ────────────────────────────────────────────────────────────

func TestGetSeries_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	_, err := svc.GetSeries(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestGetSeries_PopulatesSeasonsAndEpisodes(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.series["s1"] = &Series{ID: "s1", Title: "Breaking Bad"}
	repo.seasons["s1"] = []*Season{{ID: "s1-s01", SeriesID: "s1", SeasonNumber: 1}}
	repo.episodes["s1"] = []*Episode{{ID: "s1-s01-e001", SeriesID: "s1", SeasonID: "s1-s01", EpisodeNumber: 1}}

	svc := NewService(repo, "")
	s, err := svc.GetSeries(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	if len(s.Seasons) != 1 {
		t.Errorf("expected 1 season, got %d", len(s.Seasons))
	}
	if len(s.Episodes) != 1 {
		t.Errorf("expected 1 episode, got %d", len(s.Episodes))
	}
}

func TestGetSeries_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	_, err := svc.GetSeries(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent series")
	}
}

func TestUpdateSeries_NilSeries(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	if err := svc.UpdateSeries(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil series")
	}
}

func TestUpdateSeries_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	err := svc.UpdateSeries(context.Background(), &Series{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestUpdateSeries_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.series["s1"] = &Series{ID: "s1", Title: "Old"}
	svc := NewService(repo, "")

	err := svc.UpdateSeries(context.Background(), &Series{ID: "s1", Title: "New"})
	if err != nil {
		t.Fatalf("UpdateSeries: %v", err)
	}
	if repo.series["s1"].Title != "New" {
		t.Error("series not updated")
	}
	if repo.series["s1"].UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestDeleteSeries_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	if err := svc.DeleteSeries(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestDeleteSeries_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.series["s1"] = &Series{ID: "s1", Title: "Test"}
	svc := NewService(repo, "")

	if err := svc.DeleteSeries(context.Background(), "s1"); err != nil {
		t.Fatalf("DeleteSeries: %v", err)
	}
	if _, ok := repo.series["s1"]; ok {
		t.Error("series not deleted")
	}
}

func TestSetMonitoringStatus_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	err := svc.SetMonitoringStatus(context.Background(), "", MonitoringAll)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestSetMonitoringStatus_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.series["s1"] = &Series{ID: "s1", Title: "Test", MonitoringStatus: MonitoringAll}
	svc := NewService(repo, "")

	err := svc.SetMonitoringStatus(context.Background(), "s1", MonitoringNone)
	if err != nil {
		t.Fatalf("SetMonitoringStatus: %v", err)
	}
	if repo.series["s1"].MonitoringStatus != MonitoringNone {
		t.Errorf("status = %s, want none", repo.series["s1"].MonitoringStatus)
	}
}

func TestSetMonitoringStatus_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	err := svc.SetMonitoringStatus(context.Background(), "nonexistent", MonitoringAll)
	if err == nil {
		t.Fatal("expected error for non-existent series")
	}
}

func TestListSeries(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.series["s1"] = &Series{ID: "s1", Title: "Show A"}
	repo.series["s2"] = &Series{ID: "s2", Title: "Show B"}
	svc := NewService(repo, "")

	got, err := svc.ListSeries(context.Background())
	if err != nil {
		t.Fatalf("ListSeries: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 series, got %d", len(got))
	}
}

func TestListEpisodes(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.episodes["s1"] = []*Episode{
		{ID: "e1", SeriesID: "s1", EpisodeNumber: 1},
		{ID: "e2", SeriesID: "s1", EpisodeNumber: 2},
	}
	svc := NewService(repo, "")

	got, err := svc.ListEpisodes(context.Background(), "s1", nil)
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 episodes, got %d", len(got))
	}
}

func TestGetEpisode(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.episodes["s1"] = []*Episode{{ID: "e1", SeriesID: "s1", Title: "Pilot"}}
	svc := NewService(repo, "")

	got, err := svc.GetEpisode(context.Background(), "e1")
	if err != nil {
		t.Fatalf("GetEpisode: %v", err)
	}
	if got.Title != "Pilot" {
		t.Errorf("title = %q, want Pilot", got.Title)
	}
}

func TestUpdateEpisode_Nil(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	if err := svc.UpdateEpisode(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil episode")
	}
}

func TestUpdateEpisode_SetsUpdatedAt(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.episodes["s1"] = []*Episode{{ID: "e1", SeriesID: "s1"}}
	svc := NewService(repo, "")

	ep := &Episode{ID: "e1", SeriesID: "s1", Title: "Updated"}
	before := time.Now()
	if err := svc.UpdateEpisode(context.Background(), ep); err != nil {
		t.Fatalf("UpdateEpisode: %v", err)
	}
	if ep.UpdatedAt.Before(before) {
		t.Error("UpdatedAt should be set")
	}
}

func TestCreateEpisodeFile_Nil(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	if err := svc.CreateEpisodeFile(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil episode file")
	}
}

func TestGetCredits(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.credits["s1"] = []*SeriesCredit{{PersonName: "Bryan Cranston", Role: "actor"}}
	svc := NewService(repo, "")

	got, err := svc.GetCredits(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetCredits: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 credit, got %d", len(got))
	}
}

func TestGetEpisodeStats(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.stats["s1"] = &EpisodeStats{TotalEpisodes: 62, DownloadedEpisodes: 50}
	svc := NewService(repo, "")

	got, err := svc.GetEpisodeStats(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetEpisodeStats: %v", err)
	}
	if got.TotalEpisodes != 62 {
		t.Errorf("total = %d, want 62", got.TotalEpisodes)
	}
}

func TestSearchTMDB_EmptyQuery(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "test-key")
	_, err := svc.SearchTMDB(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchTMDB_NoAPIKey(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "")
	_, err := svc.SearchTMDB(context.Background(), "breaking bad")
	if err == nil {
		t.Fatal("expected error when API key is empty")
	}
}

func TestLookupTMDB_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), "test-key")
	_, err := svc.LookupTMDB(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty TMDB ID")
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"Breaking Bad", "breaking-bad"},
		{"The Walking Dead", "the-walking-dead"},
		{"Mr. Robot", "mr-robot"},
		{"Game of Thrones", "game-of-thrones"},
		{"Stranger Things", "stranger-things"},
		{"", ""},
		{"One", "one"},
		{"Hello...World", "hello-world"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := slugify(tt.input)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	t.Parallel()
	m := map[string]interface{}{
		"name": "test",
		"num":  42,
		"nil":  nil,
	}

	if got := getString(m, "name"); got != "test" {
		t.Errorf("getString(name) = %q, want test", got)
	}
	if got := getString(m, "num"); got != "" {
		t.Errorf("getString(num) = %q, want empty", got)
	}
	if got := getString(m, "missing"); got != "" {
		t.Errorf("getString(missing) = %q, want empty", got)
	}
	if got := getString(m, "nil"); got != "" {
		t.Errorf("getString(nil) = %q, want empty", got)
	}
}

func TestGetInt(t *testing.T) {
	t.Parallel()
	m := map[string]interface{}{
		"float":   float64(42),
		"int":     42,
		"array":   []interface{}{float64(30)},
		"empty":   []interface{}{},
		"str":     "not a number",
		"nil_val": nil,
	}

	if got := getInt(m, "float"); got != 42 {
		t.Errorf("getInt(float) = %d, want 42", got)
	}
	if got := getInt(m, "int"); got != 42 {
		t.Errorf("getInt(int) = %d, want 42", got)
	}
	if got := getInt(m, "array"); got != 30 {
		t.Errorf("getInt(array) = %d, want 30", got)
	}
	if got := getInt(m, "empty"); got != 0 {
		t.Errorf("getInt(empty) = %d, want 0", got)
	}
	if got := getInt(m, "str"); got != 0 {
		t.Errorf("getInt(str) = %d, want 0", got)
	}
	if got := getInt(m, "missing"); got != 0 {
		t.Errorf("getInt(missing) = %d, want 0", got)
	}
}

func TestGetFloat(t *testing.T) {
	t.Parallel()
	m := map[string]interface{}{
		"rating":  8.5,
		"str":     "not a float",
		"nil_val": nil,
	}

	if got := getFloat(m, "rating"); got != 8.5 {
		t.Errorf("getFloat(rating) = %f, want 8.5", got)
	}
	if got := getFloat(m, "str"); got != 0 {
		t.Errorf("getFloat(str) = %f, want 0", got)
	}
	if got := getFloat(m, "missing"); got != 0 {
		t.Errorf("getFloat(missing) = %f, want 0", got)
	}
}

func TestParseTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  int // expected year, 0 means zero time
	}{
		{"2024-01-15T10:30:00Z", 2024},
		{"2024-01-15 10:30:00", 2024},
		{"2024-01-15 10:30:00+00:00", 2024},
		{"garbage", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTime(tt.input)
			if tt.want == 0 && !got.IsZero() {
				t.Errorf("parseTime(%q) = %v, want zero", tt.input, got)
			}
			if tt.want != 0 && got.Year() != tt.want {
				t.Errorf("parseTime(%q).Year() = %d, want %d", tt.input, got.Year(), tt.want)
			}
		})
	}
}
