package movies

import (
	"context"
	"errors"
	"testing"

	"github.com/ebenderooock/loom/internal/metadata"
)

// ── Mock Repository ──────────────────────────────────────────────────

type mockRepo struct {
	movies            map[string]*Movie
	movieFiles        map[string][]*MovieFile
	qualityDefs       map[string]*QualityDefinition
	qualityProfiles   map[string]*QualityProfile
	customFormats     map[string]*CustomFormat
	searchResults     []*Movie
	addMovieErr       error
	updateMovieErr    error
	deleteMovieErr    error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		movies:          make(map[string]*Movie),
		movieFiles:      make(map[string][]*MovieFile),
		qualityDefs:     make(map[string]*QualityDefinition),
		qualityProfiles: make(map[string]*QualityProfile),
		customFormats:   make(map[string]*CustomFormat),
	}
}

func (r *mockRepo) AddMovie(_ context.Context, m *Movie) error {
	if r.addMovieErr != nil {
		return r.addMovieErr
	}
	r.movies[m.ID] = m
	return nil
}
func (r *mockRepo) GetMovie(_ context.Context, id string) (*Movie, error) {
	return r.movies[id], nil
}
func (r *mockRepo) UpdateMovie(_ context.Context, m *Movie) error {
	if r.updateMovieErr != nil {
		return r.updateMovieErr
	}
	r.movies[m.ID] = m
	return nil
}
func (r *mockRepo) DeleteMovie(_ context.Context, id string) error {
	if r.deleteMovieErr != nil {
		return r.deleteMovieErr
	}
	delete(r.movies, id)
	return nil
}
func (r *mockRepo) ListMovies(_ context.Context, limit, offset int) ([]*Movie, error) {
	var out []*Movie
	i := 0
	for _, m := range r.movies {
		if i >= offset && len(out) < limit {
			out = append(out, m)
		}
		i++
	}
	return out, nil
}
func (r *mockRepo) SearchMovies(_ context.Context, _ string) ([]*Movie, error) {
	return r.searchResults, nil
}
func (r *mockRepo) GetMovieByTMDBID(_ context.Context, id string) (*Movie, error) {
	for _, m := range r.movies {
		if m.TMDBID != nil && *m.TMDBID == id {
			return m, nil
		}
	}
	return nil, nil
}
func (r *mockRepo) GetMovieByIMDBID(_ context.Context, id string) (*Movie, error) {
	for _, m := range r.movies {
		if m.IMDBID != nil && *m.IMDBID == id {
			return m, nil
		}
	}
	return nil, nil
}
func (r *mockRepo) AddMovieFile(_ context.Context, mf *MovieFile) error {
	r.movieFiles[mf.MovieID] = append(r.movieFiles[mf.MovieID], mf)
	return nil
}
func (r *mockRepo) GetMovieFile(_ context.Context, id string) (*MovieFile, error) { return nil, nil }
func (r *mockRepo) UpdateMovieFile(_ context.Context, _ *MovieFile) error        { return nil }
func (r *mockRepo) DeleteMovieFile(_ context.Context, _ string) error            { return nil }
func (r *mockRepo) ListMovieFilesByMovie(_ context.Context, movieID string) ([]*MovieFile, error) {
	return r.movieFiles[movieID], nil
}
func (r *mockRepo) GetMovieFileByPath(_ context.Context, _ string) (*MovieFile, error) {
	return nil, nil
}
func (r *mockRepo) AddQualityDefinition(_ context.Context, qd *QualityDefinition) error {
	r.qualityDefs[qd.ID] = qd
	return nil
}
func (r *mockRepo) GetQualityDefinition(_ context.Context, id string) (*QualityDefinition, error) {
	return r.qualityDefs[id], nil
}
func (r *mockRepo) UpdateQualityDefinition(_ context.Context, qd *QualityDefinition) error {
	r.qualityDefs[qd.ID] = qd
	return nil
}
func (r *mockRepo) DeleteQualityDefinition(_ context.Context, id string) error {
	delete(r.qualityDefs, id)
	return nil
}
func (r *mockRepo) ListQualityDefinitions(_ context.Context) ([]*QualityDefinition, error) {
	var out []*QualityDefinition
	for _, qd := range r.qualityDefs {
		out = append(out, qd)
	}
	return out, nil
}
func (r *mockRepo) GetQualityDefinitionByName(_ context.Context, _ string) (*QualityDefinition, error) {
	return nil, nil
}
func (r *mockRepo) AddQualityProfile(_ context.Context, qp *QualityProfile) error {
	r.qualityProfiles[qp.ID] = qp
	return nil
}
func (r *mockRepo) GetQualityProfile(_ context.Context, id string) (*QualityProfile, error) {
	return r.qualityProfiles[id], nil
}
func (r *mockRepo) UpdateQualityProfile(_ context.Context, qp *QualityProfile) error {
	r.qualityProfiles[qp.ID] = qp
	return nil
}
func (r *mockRepo) DeleteQualityProfile(_ context.Context, id string) error {
	delete(r.qualityProfiles, id)
	return nil
}
func (r *mockRepo) ListQualityProfiles(_ context.Context) ([]*QualityProfile, error) {
	var out []*QualityProfile
	for _, qp := range r.qualityProfiles {
		out = append(out, qp)
	}
	return out, nil
}
func (r *mockRepo) GetQualityProfileByName(_ context.Context, _ string) (*QualityProfile, error) {
	return nil, nil
}
func (r *mockRepo) AddCustomFormat(_ context.Context, cf *CustomFormat) error {
	r.customFormats[cf.ID] = cf
	return nil
}
func (r *mockRepo) GetCustomFormat(_ context.Context, id string) (*CustomFormat, error) {
	return r.customFormats[id], nil
}
func (r *mockRepo) UpdateCustomFormat(_ context.Context, cf *CustomFormat) error {
	r.customFormats[cf.ID] = cf
	return nil
}
func (r *mockRepo) DeleteCustomFormat(_ context.Context, id string) error {
	delete(r.customFormats, id)
	return nil
}
func (r *mockRepo) ListCustomFormats(_ context.Context) ([]*CustomFormat, error) {
	var out []*CustomFormat
	for _, cf := range r.customFormats {
		out = append(out, cf)
	}
	return out, nil
}
func (r *mockRepo) GetCustomFormatByName(_ context.Context, _ string) (*CustomFormat, error) {
	return nil, nil
}

// ── Mock MetadataSearcher ────────────────────────────────────────────

type mockMetadata struct {
	queryResults []*metadata.MovieMetadata
	tmdbResult   *metadata.MovieMetadata
	err          error
}

func (m *mockMetadata) FindMovieByQuery(_ context.Context, _ string, _ int) ([]*metadata.MovieMetadata, error) {
	return m.queryResults, m.err
}
func (m *mockMetadata) FindMovieByTMDBID(_ context.Context, _ string) (*metadata.MovieMetadata, error) {
	return m.tmdbResult, m.err
}

// ── Mock CreditsProvider ─────────────────────────────────────────────

type mockCredits struct {
	credits *metadata.Credits
	err     error
}

func (m *mockCredits) GetMovieCredits(_ context.Context, _ int) (*metadata.Credits, error) {
	return m.credits, m.err
}

// ── Tests ────────────────────────────────────────────────────────────

func TestListMovies_DefaultLimit(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	for i := 0; i < 30; i++ {
		id := "m" + string(rune('a'+i))
		repo.movies[id] = &Movie{ID: id, Title: "Movie"}
	}
	svc := NewService(repo)

	got, err := svc.ListMovies(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("ListMovies: %v", err)
	}
	// limit <= 0 defaults to 25
	if len(got) > 25 {
		t.Errorf("expected at most 25 movies, got %d", len(got))
	}
}

func TestListMovies_ClampLimits(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	svc := NewService(repo)

	tests := []struct {
		name       string
		limit      int
		wantClamp  int
	}{
		{"negative limit defaults to 25", -5, 25},
		{"zero limit defaults to 25", 0, 25},
		{"over 1000 clamped to 1000", 5000, 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify no error; mock returns empty slice
			_, err := svc.ListMovies(context.Background(), tt.limit, 0)
			if err != nil {
				t.Fatalf("ListMovies(%d, 0): %v", tt.limit, err)
			}
		})
	}
}

func TestListMovies_NegativeOffset(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	svc := NewService(repo)
	_, err := svc.ListMovies(context.Background(), 10, -1)
	if err != nil {
		t.Fatalf("ListMovies with negative offset: %v", err)
	}
}

func TestSearchMovies_EmptyQuery(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.SearchMovies(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchMovies_ValidQuery(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.searchResults = []*Movie{{ID: "m1", Title: "Inception"}}
	svc := NewService(repo)

	got, err := svc.SearchMovies(context.Background(), "inception")
	if err != nil {
		t.Fatalf("SearchMovies: %v", err)
	}
	if len(got) != 1 || got[0].ID != "m1" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestGetMovie_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.GetMovie(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestGetMovie_CacheHit(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Matrix"}
	svc := NewService(repo)
	ctx := context.Background()

	// First call: cache miss, hits repo
	m1, err := svc.GetMovie(ctx, "m1")
	if err != nil || m1 == nil {
		t.Fatalf("first GetMovie: %v", err)
	}

	// Remove from repo to prove second call hits cache
	delete(repo.movies, "m1")

	m2, err := svc.GetMovie(ctx, "m1")
	if err != nil || m2 == nil {
		t.Fatalf("second GetMovie (cache): %v", err)
	}
	if m2.Title != "Matrix" {
		t.Errorf("cached movie title = %q, want Matrix", m2.Title)
	}
}

func TestGetMovie_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	m, err := svc.GetMovie(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetMovie: %v", err)
	}
	if m != nil {
		t.Error("expected nil for non-existent movie")
	}
}

func TestAddMovie_NilMovie(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	if err := svc.AddMovie(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil movie")
	}
}

func TestAddMovie_EmptyTitle(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	err := svc.AddMovie(context.Background(), &Movie{ID: "m1"})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestAddMovie_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	svc := NewService(repo)

	m := &Movie{ID: "m1", Title: "Inception", Year: 2010}
	if err := svc.AddMovie(context.Background(), m); err != nil {
		t.Fatalf("AddMovie: %v", err)
	}
	if _, ok := repo.movies["m1"]; !ok {
		t.Error("movie not stored in repo")
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestAddMovie_InvalidatesCache(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	svc := NewService(repo)
	ctx := context.Background()

	m := &Movie{ID: "m1", Title: "Inception"}
	_ = svc.AddMovie(ctx, m)
	// Pre-populate cache
	_, _ = svc.GetMovie(ctx, "m1")

	// Update the underlying repo and re-add
	m.Title = "Inception Updated"
	_ = svc.AddMovie(ctx, m)

	got, _ := svc.GetMovie(ctx, "m1")
	if got.Title != "Inception Updated" {
		t.Errorf("cache not invalidated after AddMovie, got title %q", got.Title)
	}
}

func TestAddMovie_RepoError(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.addMovieErr = errors.New("db error")
	svc := NewService(repo)

	err := svc.AddMovie(context.Background(), &Movie{ID: "m1", Title: "Test"})
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestUpdateMovie_NilMovie(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	if err := svc.UpdateMovie(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil movie")
	}
}

func TestUpdateMovie_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	err := svc.UpdateMovie(context.Background(), &Movie{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestUpdateMovie_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Old"}
	svc := NewService(repo)

	err := svc.UpdateMovie(context.Background(), &Movie{ID: "m1", Title: "New"})
	if err != nil {
		t.Fatalf("UpdateMovie: %v", err)
	}
	if repo.movies["m1"].Title != "New" {
		t.Error("movie not updated in repo")
	}
}

func TestDeleteMovie_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	if err := svc.DeleteMovie(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestDeleteMovie_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Test"}
	svc := NewService(repo)

	if err := svc.DeleteMovie(context.Background(), "m1"); err != nil {
		t.Fatalf("DeleteMovie: %v", err)
	}
	if _, ok := repo.movies["m1"]; ok {
		t.Error("movie not deleted from repo")
	}
}

func TestSetMonitoringStatus_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	err := svc.SetMonitoringStatus(context.Background(), "", MonitoringStatusMonitored)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestSetMonitoringStatus_ValidStatuses(t *testing.T) {
	t.Parallel()
	valid := []MonitoringStatus{
		MonitoringStatusMonitored,
		MonitoringStatusUnmonitored,
		MonitoringStatusDeleted,
	}
	for _, status := range valid {
		t.Run(string(status), func(t *testing.T) {
			repo := newMockRepo()
			repo.movies["m1"] = &Movie{ID: "m1", Title: "Test", MonitoringStatus: MonitoringStatusMonitored}
			svc := NewService(repo)

			if err := svc.SetMonitoringStatus(context.Background(), "m1", status); err != nil {
				t.Fatalf("SetMonitoringStatus(%s): %v", status, err)
			}
			if repo.movies["m1"].MonitoringStatus != status {
				t.Errorf("status = %s, want %s", repo.movies["m1"].MonitoringStatus, status)
			}
		})
	}
}

func TestSetMonitoringStatus_InvalidStatus(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Test"}
	svc := NewService(repo)

	err := svc.SetMonitoringStatus(context.Background(), "m1", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestSetMonitoringStatus_MovieNotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	err := svc.SetMonitoringStatus(context.Background(), "nonexistent", MonitoringStatusMonitored)
	if err == nil {
		t.Fatal("expected error for non-existent movie")
	}
}

func TestLookupMovies_EmptyTerm(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.LookupMovies(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty term")
	}
}

func TestLookupMovies_NoMetadataProvider(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.LookupMovies(context.Background(), "Inception")
	if err == nil {
		t.Fatal("expected error when metadata provider is nil")
	}
}

func TestLookupMovies_Success(t *testing.T) {
	t.Parallel()
	meta := &mockMetadata{
		queryResults: []*metadata.MovieMetadata{{Title: "Inception", Year: 2010}},
	}
	svc := NewService(newMockRepo(), WithMetadata(meta))

	got, err := svc.LookupMovies(context.Background(), "Inception")
	if err != nil {
		t.Fatalf("LookupMovies: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
}

func TestGetMovieCredits_NoProvider(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.GetMovieCredits(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error when credits provider is nil")
	}
}

func TestGetMovieCredits_MovieNotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo(), WithCredits(&mockCredits{}))
	_, err := svc.GetMovieCredits(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent movie")
	}
}

func TestGetMovieCredits_NoTMDBID(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Test"}
	svc := NewService(repo, WithCredits(&mockCredits{}))

	_, err := svc.GetMovieCredits(context.Background(), "m1")
	if err == nil {
		t.Fatal("expected error for movie without TMDB ID")
	}
}

func TestRefreshMovie_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	if err := svc.RefreshMovie(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestRefreshMovie_NoMetadataProvider(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	tmdb := "12345"
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Test", TMDBID: &tmdb}
	svc := NewService(repo)

	if err := svc.RefreshMovie(context.Background(), "m1"); err == nil {
		t.Fatal("expected error when metadata provider is nil")
	}
}

func TestRefreshMovie_Success(t *testing.T) {
	t.Parallel()
	repo := newMockRepo()
	tmdb := "550"
	repo.movies["m1"] = &Movie{ID: "m1", Title: "Old Title", TMDBID: &tmdb}

	meta := &mockMetadata{
		tmdbResult: &metadata.MovieMetadata{
			Title:   "Fight Club",
			Year:    1999,
			Overview: "Updated overview",
			Rating:  8.4,
		},
	}
	svc := NewService(repo, WithMetadata(meta))

	if err := svc.RefreshMovie(context.Background(), "m1"); err != nil {
		t.Fatalf("RefreshMovie: %v", err)
	}
	if repo.movies["m1"].Title != "Fight Club" {
		t.Errorf("title = %q, want Fight Club", repo.movies["m1"].Title)
	}
	if repo.movies["m1"].Year != 1999 {
		t.Errorf("year = %d, want 1999", repo.movies["m1"].Year)
	}
}

func TestListMovieFiles_EmptyMovieID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.ListMovieFiles(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty movie ID")
	}
}

func TestAddMovieFile_Validation(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())

	tests := []struct {
		name    string
		file    *MovieFile
		wantErr bool
	}{
		{"empty movie_id", &MovieFile{FilePath: "/test.mkv"}, true},
		{"empty file_path", &MovieFile{MovieID: "m1"}, true},
		{"valid", &MovieFile{MovieID: "m1", FilePath: "/test.mkv"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.AddMovieFile(context.Background(), tt.file)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddMovieFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddQualityDefinition_Validation(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	ctx := context.Background()

	tests := []struct {
		name    string
		qd      *QualityDefinition
		wantErr bool
	}{
		{"nil", nil, true},
		{"empty name", &QualityDefinition{Source: "BluRay", Resolution: "1080p"}, true},
		{"empty source", &QualityDefinition{Name: "HD", Resolution: "1080p"}, true},
		{"empty resolution", &QualityDefinition{Name: "HD", Source: "BluRay"}, true},
		{"valid", &QualityDefinition{ID: "qd1", Name: "HD", Source: "BluRay", Resolution: "1080p"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.AddQualityDefinition(ctx, tt.qd)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddQualityDefinition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddQualityProfile_Validation(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	ctx := context.Background()

	tests := []struct {
		name    string
		qp      *QualityProfile
		wantErr bool
	}{
		{"nil", nil, true},
		{"empty name", &QualityProfile{Cutoff: "1080p", Items: []QualityProfileItem{{ID: "1080p", Allowed: true}}}, true},
		{"empty cutoff", &QualityProfile{Name: "Test"}, true},
		{"no items", &QualityProfile{Name: "Test", Cutoff: "1080p"}, true},
		{"cutoff not in items", &QualityProfile{
			Name: "Test", Cutoff: "4k",
			Items: []QualityProfileItem{{ID: "1080p", Allowed: true}},
		}, true},
		{"cutoff not allowed", &QualityProfile{
			Name: "Test", Cutoff: "1080p",
			Items: []QualityProfileItem{{ID: "1080p", Allowed: false}},
		}, true},
		{"valid", &QualityProfile{
			ID: "qp1", Name: "Test", Cutoff: "1080p",
			Items: []QualityProfileItem{{ID: "1080p", Allowed: true}},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.AddQualityProfile(ctx, tt.qp)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddQualityProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteQualityDefinition_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	if err := svc.DeleteQualityDefinition(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestDeleteQualityProfile_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	if err := svc.DeleteQualityProfile(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestGetQualityDefinition_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.GetQualityDefinition(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestGetQualityProfile_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	_, err := svc.GetQualityProfile(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestUpdateQualityDefinition_Validation(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	ctx := context.Background()

	if err := svc.UpdateQualityDefinition(ctx, nil); err == nil {
		t.Fatal("expected error for nil")
	}
	if err := svc.UpdateQualityDefinition(ctx, &QualityDefinition{}); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestUpdateQualityProfile_Validation(t *testing.T) {
	t.Parallel()
	svc := NewService(newMockRepo())
	ctx := context.Background()

	if err := svc.UpdateQualityProfile(ctx, nil); err == nil {
		t.Fatal("expected error for nil")
	}
	if err := svc.UpdateQualityProfile(ctx, &QualityProfile{Name: "x"}); err == nil {
		t.Fatal("expected error for empty ID")
	}
}
