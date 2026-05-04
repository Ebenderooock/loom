package movies

import (
	"context"
	"fmt"
	"time"
)

// QualityRepository defines database operations for quality definitions and profiles.
type QualityRepository interface {
	// Quality definitions
	AddQualityDefinition(ctx context.Context, qd *QualityDefinition) error
	GetQualityDefinition(ctx context.Context, id string) (*QualityDefinition, error)
	UpdateQualityDefinition(ctx context.Context, qd *QualityDefinition) error
	DeleteQualityDefinition(ctx context.Context, id string) error
	ListQualityDefinitions(ctx context.Context) ([]*QualityDefinition, error)
	GetQualityDefinitionByName(ctx context.Context, name string) (*QualityDefinition, error)

	// Quality profiles
	AddQualityProfile(ctx context.Context, qp *QualityProfile) error
	GetQualityProfile(ctx context.Context, id string) (*QualityProfile, error)
	UpdateQualityProfile(ctx context.Context, qp *QualityProfile) error
	DeleteQualityProfile(ctx context.Context, id string) error
	ListQualityProfiles(ctx context.Context) ([]*QualityProfile, error)
	GetQualityProfileByName(ctx context.Context, name string) (*QualityProfile, error)
}

// QualityService defines business logic for quality management.
type QualityService interface {
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

	// Validation
	ValidateQualityProfile(qp *QualityProfile) error
}

// qualityService implements QualityService.
type qualityService struct {
	repo QualityRepository
}

// NewQualityService creates a new QualityService instance.
func NewQualityService(repo QualityRepository) QualityService {
	return &qualityService{repo: repo}
}

// AddQualityDefinition adds a new quality definition.
func (qs *qualityService) AddQualityDefinition(ctx context.Context, qd *QualityDefinition) error {
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

	// Check for duplicate name
	existing, err := qs.repo.GetQualityDefinitionByName(ctx, qd.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("movies: quality definition %q already exists", qd.Name)
	}

	qd.CreatedAt = time.Now()
	qd.UpdatedAt = time.Now()

	return qs.repo.AddQualityDefinition(ctx, qd)
}

// GetQualityDefinition retrieves a quality definition by ID.
func (qs *qualityService) GetQualityDefinition(ctx context.Context, id string) (*QualityDefinition, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: quality definition ID required")
	}
	return qs.repo.GetQualityDefinition(ctx, id)
}

// UpdateQualityDefinition updates an existing quality definition.
func (qs *qualityService) UpdateQualityDefinition(ctx context.Context, qd *QualityDefinition) error {
	if qd == nil {
		return fmt.Errorf("movies: quality definition required")
	}
	if qd.ID == "" {
		return fmt.Errorf("movies: quality definition ID required")
	}

	// Verify it exists
	existing, err := qs.repo.GetQualityDefinition(ctx, qd.ID)
	if err != nil {
		return fmt.Errorf("movies: get quality definition: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("movies: quality definition not found")
	}

	qd.UpdatedAt = time.Now()
	return qs.repo.UpdateQualityDefinition(ctx, qd)
}

// DeleteQualityDefinition removes a quality definition.
func (qs *qualityService) DeleteQualityDefinition(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: quality definition ID required")
	}
	return qs.repo.DeleteQualityDefinition(ctx, id)
}

// ListQualityDefinitions retrieves all quality definitions.
func (qs *qualityService) ListQualityDefinitions(ctx context.Context) ([]*QualityDefinition, error) {
	return qs.repo.ListQualityDefinitions(ctx)
}

// AddQualityProfile adds a new quality profile.
func (qs *qualityService) AddQualityProfile(ctx context.Context, qp *QualityProfile) error {
	if qp == nil {
		return fmt.Errorf("movies: quality profile required")
	}
	if qp.Name == "" {
		return fmt.Errorf("movies: quality profile name required")
	}

	// Validate profile
	if err := qs.ValidateQualityProfile(qp); err != nil {
		return err
	}

	// Check for duplicate name
	existing, err := qs.repo.GetQualityProfileByName(ctx, qp.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("movies: quality profile %q already exists", qp.Name)
	}

	qp.CreatedAt = time.Now()
	qp.UpdatedAt = time.Now()

	return qs.repo.AddQualityProfile(ctx, qp)
}

// GetQualityProfile retrieves a quality profile by ID.
func (qs *qualityService) GetQualityProfile(ctx context.Context, id string) (*QualityProfile, error) {
	if id == "" {
		return nil, fmt.Errorf("movies: quality profile ID required")
	}
	return qs.repo.GetQualityProfile(ctx, id)
}

// UpdateQualityProfile updates an existing quality profile.
func (qs *qualityService) UpdateQualityProfile(ctx context.Context, qp *QualityProfile) error {
	if qp == nil {
		return fmt.Errorf("movies: quality profile required")
	}
	if qp.ID == "" {
		return fmt.Errorf("movies: quality profile ID required")
	}

	// Validate profile
	if err := qs.ValidateQualityProfile(qp); err != nil {
		return err
	}

	// Verify it exists
	existing, err := qs.repo.GetQualityProfile(ctx, qp.ID)
	if err != nil {
		return fmt.Errorf("movies: get quality profile: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("movies: quality profile not found")
	}

	qp.UpdatedAt = time.Now()
	return qs.repo.UpdateQualityProfile(ctx, qp)
}

// DeleteQualityProfile removes a quality profile.
func (qs *qualityService) DeleteQualityProfile(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("movies: quality profile ID required")
	}
	return qs.repo.DeleteQualityProfile(ctx, id)
}

// ListQualityProfiles retrieves all quality profiles.
func (qs *qualityService) ListQualityProfiles(ctx context.Context) ([]*QualityProfile, error) {
	return qs.repo.ListQualityProfiles(ctx)
}

// ValidateQualityProfile validates a quality profile for consistency.
func (qs *qualityService) ValidateQualityProfile(qp *QualityProfile) error {
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
