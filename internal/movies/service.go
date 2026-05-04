package movies

import (
	"context"
	"fmt"
	"sync"
	"time"
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
		Path: path,
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
