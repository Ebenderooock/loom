package movies

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service defines the business logic interface for the movies module.
type Service interface {
	ListMovies(ctx context.Context, limit, offset int) ([]*Movie, error)
	SearchMovies(ctx context.Context, query string) ([]*Movie, error)
	GetMovie(ctx context.Context, id string) (*Movie, error)
	AddMovie(ctx context.Context, movie *Movie) error
	UpdateMovie(ctx context.Context, movie *Movie) error
	DeleteMovie(ctx context.Context, id string) error
	SetMonitoringStatus(ctx context.Context, movieID string, status MonitoringStatus) error

	GetRootFolder(ctx context.Context, id string) (*RootFolder, error)
	AddRootFolder(ctx context.Context, path string) (*RootFolder, error)
	ListRootFolders(ctx context.Context) ([]*RootFolder, error)
	DeleteRootFolder(ctx context.Context, id string) error

	ListMovieFiles(ctx context.Context, movieID string) ([]*MovieFile, error)

	// Quality definitions
	AddQualityDefinition(ctx context.Context, qd *QualityDefinition) error
	GetQualityDefinition(ctx context.Context, id string) (*QualityDefinition, error)
	UpdateQualityDefinition(ctx context.Context, qd *QualityDefinition) error
	DeleteQualityDefinition(ctx context.Context, id string) error
	ListQualityDefinitions(ctx context.Context) ([]*QualityDefinition, error)

	// Quality profiles
	AddQualityProfile(ctx context.Context, qp *QualityProfile) error
	GetQualityProfile(ctx context.Context, id string) (*QualityProfile, error)
	UpdateQualityProfile(ctx context.Context, qp *QualityProfile) error
	DeleteQualityProfile(ctx context.Context, id string) error
	ListQualityProfiles(ctx context.Context) ([]*QualityProfile, error)

	// Custom formats
	AddCustomFormat(ctx context.Context, cf *CustomFormat) error
	GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error)
	UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error
	DeleteCustomFormat(ctx context.Context, id string) error
	ListCustomFormats(ctx context.Context) ([]*CustomFormat, error)
}

// service implements the Service interface.
type service struct {
	repo  Repository
	cache sync.Map // map[string]*Movie with expiry
	ttl   time.Duration
	mu    sync.RWMutex
}

// cacheEntry holds a cached movie with expiry time.
type cacheEntry struct {
	value   *Movie
	expiry  time.Time
}

// NewService creates a new Service instance with in-memory caching.
func NewService(repo Repository) Service {
	return &service{
		repo: repo,
		ttl:  5 * time.Minute,
	}
}

// ListMovies retrieves all movies with pagination.
func (s *service) ListMovies(ctx context.Context, limit, offset int) ([]*Movie, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.ListMovies(ctx, limit, offset)
}

// SearchMovies searches for movies by title or other fields.
func (s *service) SearchMovies(ctx context.Context, query string) ([]*Movie, error) {
	if query == "" {
		return nil, fmt.Errorf("movies: search query required")
	}

	return s.repo.SearchMovies(ctx, query)
}

// GetMovie retrieves a movie by ID with caching.
func (s *service) GetMovie(ctx context.Context, id string) (*Movie, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: movie ID required")
	}

	// Check cache first
	if entry, ok := s.cache.Load(id); ok {
		e := entry.(cacheEntry)
		if time.Now().Before(e.expiry) {
			return e.value, nil
		}
		s.cache.Delete(id)
	}

	movie, err := s.repo.GetMovie(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("movies: get movie: %w", err)
	}

	if movie != nil {
		s.cache.Store(id, cacheEntry{
			value:  movie,
			expiry: time.Now().Add(s.ttl),
		})
	}

	return movie, nil
}

// AddMovie adds a new movie to the library.
func (s *service) AddMovie(ctx context.Context, movie *Movie) error {
	if movie == nil {
		return fmt.Errorf("movies: movie required")
	}
	if movie.Title == "" {
		return fmt.Errorf("movies: movie title required")
	}

	if err := s.repo.AddMovie(ctx, movie); err != nil {
		return fmt.Errorf("movies: add movie: %w", err)
	}

	// Invalidate cache
	s.cache.Delete(movie.ID)

	return nil
}

// UpdateMovie updates an existing movie.
func (s *service) UpdateMovie(ctx context.Context, movie *Movie) error {
	if movie == nil {
		return fmt.Errorf("movies: movie required")
	}
	if movie.ID == "" {
		return fmt.Errorf("movies: movie ID required")
	}

	if err := s.repo.UpdateMovie(ctx, movie); err != nil {
		return fmt.Errorf("movies: update movie: %w", err)
	}

	// Invalidate cache
	s.cache.Delete(movie.ID)

	return nil
}

// DeleteMovie removes a movie from the library.
func (s *service) DeleteMovie(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: movie ID required")
	}

	if err := s.repo.DeleteMovie(ctx, id); err != nil {
		return fmt.Errorf("movies: delete movie: %w", err)
	}

	// Invalidate cache
	s.cache.Delete(id)

	return nil
}

// SetMonitoringStatus updates the monitoring status of a movie.
func (s *service) SetMonitoringStatus(ctx context.Context, movieID string, status MonitoringStatus) error {
	if movieID == "" {
		return fmt.Errorf("movies: movie ID required")
	}

	// Validate status
	validStatuses := map[MonitoringStatus]bool{
		MonitoringStatusMonitored:   true,
		MonitoringStatusUnmonitored: true,
		MonitoringStatusDeleted:     true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("movies: invalid monitoring status: %s", status)
	}

	// Get existing movie
	movie, err := s.repo.GetMovie(ctx, movieID)
	if err != nil {
		return fmt.Errorf("movies: set monitoring status: %w", err)
	}
	if movie == nil {
		return fmt.Errorf("movies: movie not found")
	}

	oldStatus := movie.MonitoringStatus
	movie.MonitoringStatus = status

	if err := s.repo.UpdateMovie(ctx, movie); err != nil {
		return fmt.Errorf("movies: set monitoring status: %w", err)
	}

	// Invalidate cache
	s.cache.Delete(movieID)

	// Emit event (deferred until eventbus is available)
	_ = oldStatus // quiet unused warning; event would be emitted here

	return nil
}

// GetRootFolder retrieves a root folder by ID.
func (s *service) GetRootFolder(ctx context.Context, id string) (*RootFolder, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: root folder ID required")
	}

	return s.repo.GetRootFolder(ctx, id)
}

// AddRootFolder adds a new root folder.
func (s *service) AddRootFolder(ctx context.Context, path string) (*RootFolder, error) {
	if path == "" {
		return nil, fmt.Errorf("movies: root folder path required")
	}

	// Check if path already exists
	existing, err := s.repo.ListRootFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("movies: add root folder: %w", err)
	}

	for _, f := range existing {
		if f.Path == path {
			return nil, fmt.Errorf("movies: root folder already exists")
		}
	}

	rf := &RootFolder{
		ID:        uuid.New().String(),
		Path:      path,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.AddRootFolder(ctx, rf); err != nil {
		return nil, fmt.Errorf("movies: add root folder: %w", err)
	}

	return rf, nil
}

// ListRootFolders retrieves all root folders.
func (s *service) ListRootFolders(ctx context.Context) ([]*RootFolder, error) {
	return s.repo.ListRootFolders(ctx)
}

// DeleteRootFolder removes a root folder.
func (s *service) DeleteRootFolder(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: root folder ID required")
	}

	return s.repo.DeleteRootFolder(ctx, id)
}

// ListMovieFiles retrieves all files for a movie.
func (s *service) ListMovieFiles(ctx context.Context, movieID string) ([]*MovieFile, error) {
	if movieID == "" {
		return nil, fmt.Errorf("movies: movie ID required")
	}

	return s.repo.ListMovieFilesByMovie(ctx, movieID)
}

// AddQualityDefinition adds a new quality definition.
func (s *service) AddQualityDefinition(ctx context.Context, qd *QualityDefinition) error {
	if qd == nil {
		return fmt.Errorf("movies: quality definition required")
	}
	if qd.Name == "" {
		return fmt.Errorf("movies: quality definition name required")
	}
	if qd.Source == "" {
		return fmt.Errorf("movies: quality source required")
	}
	if qd.Resolution == "" {
		return fmt.Errorf("movies: quality resolution required")
	}

	qd.CreatedAt = time.Now()
	qd.UpdatedAt = time.Now()

	return s.repo.AddQualityDefinition(ctx, qd)
}

// GetQualityDefinition retrieves a quality definition by ID.
func (s *service) GetQualityDefinition(ctx context.Context, id string) (*QualityDefinition, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: quality definition ID required")
	}
	return s.repo.GetQualityDefinition(ctx, id)
}

// UpdateQualityDefinition updates an existing quality definition.
func (s *service) UpdateQualityDefinition(ctx context.Context, qd *QualityDefinition) error {
	if qd == nil {
		return fmt.Errorf("movies: quality definition required")
	}
	if qd.ID == "" {
		return fmt.Errorf("movies: quality definition ID required")
	}

	qd.UpdatedAt = time.Now()
	return s.repo.UpdateQualityDefinition(ctx, qd)
}

// DeleteQualityDefinition removes a quality definition.
func (s *service) DeleteQualityDefinition(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: quality definition ID required")
	}
	return s.repo.DeleteQualityDefinition(ctx, id)
}

// ListQualityDefinitions retrieves all quality definitions.
func (s *service) ListQualityDefinitions(ctx context.Context) ([]*QualityDefinition, error) {
	return s.repo.ListQualityDefinitions(ctx)
}

// AddQualityProfile adds a new quality profile.
func (s *service) AddQualityProfile(ctx context.Context, qp *QualityProfile) error {
	if qp == nil {
		return fmt.Errorf("movies: quality profile required")
	}
	if qp.Name == "" {
		return fmt.Errorf("movies: quality profile name required")
	}

	// Validate profile
	if err := s.validateQualityProfile(qp); err != nil {
		return err
	}

	qp.CreatedAt = time.Now()
	qp.UpdatedAt = time.Now()

	return s.repo.AddQualityProfile(ctx, qp)
}

// GetQualityProfile retrieves a quality profile by ID.
func (s *service) GetQualityProfile(ctx context.Context, id string) (*QualityProfile, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: quality profile ID required")
	}
	return s.repo.GetQualityProfile(ctx, id)
}

// UpdateQualityProfile updates an existing quality profile.
func (s *service) UpdateQualityProfile(ctx context.Context, qp *QualityProfile) error {
	if qp == nil {
		return fmt.Errorf("movies: quality profile required")
	}
	if qp.ID == "" {
		return fmt.Errorf("movies: quality profile ID required")
	}

	// Validate profile
	if err := s.validateQualityProfile(qp); err != nil {
		return err
	}

	qp.UpdatedAt = time.Now()
	return s.repo.UpdateQualityProfile(ctx, qp)
}

// DeleteQualityProfile removes a quality profile.
func (s *service) DeleteQualityProfile(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: quality profile ID required")
	}
	return s.repo.DeleteQualityProfile(ctx, id)
}

// ListQualityProfiles retrieves all quality profiles.
func (s *service) ListQualityProfiles(ctx context.Context) ([]*QualityProfile, error) {
	return s.repo.ListQualityProfiles(ctx)
}

// validateQualityProfile validates a quality profile for consistency.
func (s *service) validateQualityProfile(qp *QualityProfile) error {
	if qp.Name == "" {
		return fmt.Errorf("movies: quality profile name required")
	}
	if qp.Cutoff == "" {
		return fmt.Errorf("movies: quality profile cutoff required")
	}
	if len(qp.Items) == 0 {
		return fmt.Errorf("movies: quality profile must have at least one quality item")
	}

	// Verify cutoff is in items
	cutoffFound := false
	for _, item := range qp.Items {
		if item.ID == qp.Cutoff {
			cutoffFound = true
			if !item.Allowed {
				return fmt.Errorf("movies: cutoff quality %q must be in allowed items", qp.Cutoff)
			}
			break
		}
	}
	if !cutoffFound {
		return fmt.Errorf("movies: cutoff quality %q not found in profile items", qp.Cutoff)
	}

	return nil
}

// AddCustomFormat adds a new custom format using the custom format service.
func (s *service) AddCustomFormat(ctx context.Context, cf *CustomFormat) error {
cfService := NewCustomFormatService(s.repo)
return cfService.AddCustomFormat(ctx, cf)
}

// GetCustomFormat retrieves a custom format using the custom format service.
func (s *service) GetCustomFormat(ctx context.Context, id string) (*CustomFormat, error) {
cfService := NewCustomFormatService(s.repo)
return cfService.GetCustomFormat(ctx, id)
}

// UpdateCustomFormat updates a custom format using the custom format service.
func (s *service) UpdateCustomFormat(ctx context.Context, cf *CustomFormat) error {
cfService := NewCustomFormatService(s.repo)
return cfService.UpdateCustomFormat(ctx, cf)
}

// DeleteCustomFormat deletes a custom format using the custom format service.
func (s *service) DeleteCustomFormat(ctx context.Context, id string) error {
cfService := NewCustomFormatService(s.repo)
return cfService.DeleteCustomFormat(ctx, id)
}

// ListCustomFormats lists all custom formats using the custom format service.
func (s *service) ListCustomFormats(ctx context.Context) ([]*CustomFormat, error) {
cfService := NewCustomFormatService(s.repo)
return cfService.ListCustomFormats(ctx)
}
