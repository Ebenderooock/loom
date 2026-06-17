package requests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Fulfiller adds requested media to the library and triggers a grab. It is the
// boundary to the movies/series/autosearch services. All metadata is fetched
// server-side by the implementation; the request's caller-supplied fields are
// never used for fulfillment.
type Fulfiller interface {
	// MovieExists reports an existing library movie's media id for the TMDB id,
	// or "" if none exists.
	MovieExists(ctx context.Context, tmdbID string) (string, error)
	// SeriesExists reports an existing library series' media id for the TMDB id,
	// or "" if none exists.
	SeriesExists(ctx context.Context, tmdbID string) (string, error)
	// FulfillMovie adds the movie (monitored) and starts a search-and-grab.
	FulfillMovie(ctx context.Context, tmdbID, qualityProfileID, libraryID string) (mediaID string, err error)
	// FulfillSeries adds the series (monitored) and starts a search-and-grab.
	FulfillSeries(ctx context.Context, tmdbID, qualityProfileID, libraryID string) (mediaID string, err error)
	// ArtistExists reports an existing library artist's media id for the
	// MusicBrainz id, or "" if none exists.
	ArtistExists(ctx context.Context, mbid string) (string, error)
	// FulfillArtist adds the artist (monitored) and starts album searches.
	FulfillArtist(ctx context.Context, mbid, qualityProfileID, libraryID string) (mediaID string, err error)
}

// LibraryValidator validates that an admin-supplied quality profile and library
// exist and that the library's media type matches the request.
type LibraryValidator interface {
	// ValidateTarget returns nil when the quality profile and library exist and
	// the library accepts the given media type.
	ValidateTarget(ctx context.Context, mediaType MediaType, qualityProfileID, libraryID string) error
}

// Notifier sends a best-effort notification when a request is decided.
type Notifier func(ctx context.Context, r Request, decision string)

// Service coordinates request creation, approval, and rejection.
type Service struct {
	store     *Store
	fulfiller Fulfiller
	validator LibraryValidator
	notify    Notifier
	logger    *slog.Logger
}

// Options configures a request Service.
type Options struct {
	Store     *Store
	Fulfiller Fulfiller
	Validator LibraryValidator
	Notify    Notifier
	Logger    *slog.Logger
}

// NewService constructs a request Service.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:     opts.Store,
		fulfiller: opts.Fulfiller,
		validator: opts.Validator,
		notify:    opts.Notify,
		logger:    logger,
	}
}

// Store exposes the underlying store for read handlers.
func (s *Service) Store() *Store { return s.store }

// Get returns a single request by id.
func (s *Service) Get(ctx context.Context, id string) (Request, error) {
	return s.store.Get(ctx, id)
}

// ErrAlreadyAvailable indicates the requested media already exists in the library.
var ErrAlreadyAvailable = errors.New("requests: media already available")

// Create validates and stores a new request on behalf of the given user. It
// rejects unsupported media types, missing TMDB ids, already-available media,
// and duplicate open requests. Non-admin users are subject to the configured
// per-user quota; admins (isAdmin) bypass it.
func (s *Service) Create(ctx context.Context, userID, username string, isAdmin bool, in CreateInput) (Request, error) {
	if !validMediaType(in.MediaType) {
		return Request{}, fmt.Errorf("requests: invalid media type %q", in.MediaType)
	}
	in.TMDBID = strings.TrimSpace(in.TMDBID)
	if in.TMDBID == "" {
		return Request{}, errors.New("requests: tmdb_id required")
	}

	// Short-circuit if the media is already in the library. A lookup error is
	// surfaced rather than swallowed so we never create a request on the false
	// assumption that the media is absent.
	if s.fulfiller != nil {
		mediaID, err := s.mediaExists(ctx, in.MediaType, in.TMDBID)
		if err != nil {
			return Request{}, fmt.Errorf("requests: checking existing media: %w", err)
		}
		if mediaID != "" {
			return Request{}, ErrAlreadyAvailable
		}
	}

	req := Request{
		UserID:     userID,
		Username:   username,
		MediaType:  in.MediaType,
		TMDBID:     in.TMDBID,
		Title:      in.Title,
		Year:       in.Year,
		PosterPath: in.PosterPath,
		Overview:   in.Overview,
	}

	// Enforce the per-user quota for non-admins when a limit is configured.
	if !isAdmin {
		cfg, err := s.store.GetQuotaConfig(ctx)
		if err != nil {
			return Request{}, fmt.Errorf("requests: loading quota: %w", err)
		}
		if limit := quotaLimitFor(cfg, in.MediaType); limit > 0 {
			return s.store.CreateWithinQuota(ctx, req, limit, quotaSince(cfg))
		}
	}

	return s.store.Create(ctx, req)
}

// quotaLimitFor returns the configured limit for a media type (0 = unlimited).
func quotaLimitFor(cfg QuotaConfig, mt MediaType) int {
	switch mt {
	case MediaMovie:
		return cfg.MovieLimit
	case MediaArtist:
		return cfg.MusicLimit
	default:
		return cfg.SeriesLimit
	}
}

// quotaSince returns the start of the rolling quota window (zero = all time).
func quotaSince(cfg QuotaConfig) time.Time {
	if cfg.WindowDays <= 0 {
		return time.Time{}
	}
	return time.Now().UTC().AddDate(0, 0, -cfg.WindowDays)
}

func (s *Service) mediaExists(ctx context.Context, mt MediaType, externalID string) (string, error) {
	switch mt {
	case MediaMovie:
		return s.fulfiller.MovieExists(ctx, externalID)
	case MediaArtist:
		return s.fulfiller.ArtistExists(ctx, externalID)
	default:
		return s.fulfiller.SeriesExists(ctx, externalID)
	}
}

// ListAll returns requests filtered by status (empty = all).
func (s *Service) ListAll(ctx context.Context, status Status) ([]Request, error) {
	return s.store.List(ctx, status)
}

// ListMine returns the given user's requests.
func (s *Service) ListMine(ctx context.Context, userID string) ([]Request, error) {
	return s.store.ListByUser(ctx, userID)
}

// Clear removes all persisted request history.
func (s *Service) Clear(ctx context.Context) error {
	return s.store.Clear(ctx)
}

// GetQuotaConfig returns the global per-user request quota.
func (s *Service) GetQuotaConfig(ctx context.Context) (QuotaConfig, error) {
	return s.store.GetQuotaConfig(ctx)
}

// ErrInvalidQuota indicates a quota config failed validation.
var ErrInvalidQuota = errors.New("requests: invalid quota configuration")

// SetQuotaConfig validates and persists the global per-user request quota.
// Limits must be non-negative; the window is clamped to [1, MaxWindowDays] and
// defaults to DefaultWindowDays when limits are set without an explicit window.
func (s *Service) SetQuotaConfig(ctx context.Context, c QuotaConfig) (QuotaConfig, error) {
	if c.MovieLimit < 0 || c.SeriesLimit < 0 {
		return QuotaConfig{}, fmt.Errorf("%w: limits must be non-negative", ErrInvalidQuota)
	}
	if c.WindowDays <= 0 {
		c.WindowDays = DefaultWindowDays
	}
	if c.WindowDays > MaxWindowDays {
		return QuotaConfig{}, fmt.Errorf("%w: window_days must be <= %d", ErrInvalidQuota, MaxWindowDays)
	}
	if err := s.store.SetQuotaConfig(ctx, c); err != nil {
		return QuotaConfig{}, err
	}
	return c, nil
}

// QuotaStatus reports a user's current quota usage. Admins (isAdmin) are
// exempt and reported as unlimited, though their usage is still counted for
// informational display.
func (s *Service) QuotaStatus(ctx context.Context, userID string, isAdmin bool) (QuotaStatus, error) {
	cfg, err := s.store.GetQuotaConfig(ctx)
	if err != nil {
		return QuotaStatus{}, err
	}
	since := quotaSince(cfg)
	movieUsed, err := s.store.CountUserRequests(ctx, userID, MediaMovie, since)
	if err != nil {
		return QuotaStatus{}, err
	}
	seriesUsed, err := s.store.CountUserRequests(ctx, userID, MediaSeries, since)
	if err != nil {
		return QuotaStatus{}, err
	}
	musicUsed, err := s.store.CountUserRequests(ctx, userID, MediaArtist, since)
	if err != nil {
		return QuotaStatus{}, err
	}
	return QuotaStatus{
		WindowDays: cfg.WindowDays,
		Movie:      mediaQuota(cfg.MovieLimit, movieUsed, isAdmin),
		Series:     mediaQuota(cfg.SeriesLimit, seriesUsed, isAdmin),
		Music:      mediaQuota(cfg.MusicLimit, musicUsed, isAdmin),
	}, nil
}

func mediaQuota(limit, used int, isAdmin bool) MediaQuota {
	if isAdmin || limit <= 0 {
		return MediaQuota{Limit: limit, Used: used, Remaining: -1, Unlimited: true}
	}
	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	return MediaQuota{Limit: limit, Used: used, Remaining: remaining, Unlimited: false}
}

// Approve fulfills a pending request: it validates the admin-chosen target,
// atomically claims the request, adds the media, and triggers a grab. On
// fulfillment failure the request is marked failed (re-requestable). If the
// media already exists the request is marked available without re-adding.
func (s *Service) Approve(ctx context.Context, id, qualityProfileID, libraryID, decidedBy string) (Request, error) {
	req, err := s.store.Get(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if req.Status != StatusPending {
		return Request{}, fmt.Errorf("requests: cannot approve a %s request", req.Status)
	}
	if s.fulfiller == nil {
		return Request{}, errors.New("requests: fulfillment not configured")
	}

	// Validate admin input before mutating state so bad input is a clean 400.
	if s.validator != nil {
		if err := s.validator.ValidateTarget(ctx, req.MediaType, qualityProfileID, libraryID); err != nil {
			return Request{}, fmt.Errorf("requests: invalid target: %w", err)
		}
	}

	// Already in the library? Short-circuit to available.
	mediaID, err := s.mediaExists(ctx, req.MediaType, req.TMDBID)
	if err != nil {
		return Request{}, fmt.Errorf("requests: checking existing media: %w", err)
	}
	if mediaID != "" {
		if err := s.store.MarkAvailable(ctx, id, mediaID); err != nil {
			return Request{}, err
		}
		return s.store.Get(ctx, id)
	}

	// Atomically claim so the request is fulfilled at most once.
	won, err := s.store.ClaimForApproval(ctx, id, decidedBy)
	if err != nil {
		return Request{}, err
	}
	if !won {
		return Request{}, errors.New("requests: request was already being processed")
	}

	var fulfilledID string
	switch req.MediaType {
	case MediaMovie:
		fulfilledID, err = s.fulfiller.FulfillMovie(ctx, req.TMDBID, qualityProfileID, libraryID)
	case MediaArtist:
		fulfilledID, err = s.fulfiller.FulfillArtist(ctx, req.TMDBID, qualityProfileID, libraryID)
	default:
		fulfilledID, err = s.fulfiller.FulfillSeries(ctx, req.TMDBID, qualityProfileID, libraryID)
	}
	if err != nil {
		s.logger.Warn("requests: fulfillment failed", "id", id, "tmdb", req.TMDBID, "err", err)
		_ = s.store.MarkFailed(ctx, id, err.Error())
		return Request{}, fmt.Errorf("requests: fulfillment failed: %w", err)
	}

	if err := s.store.MarkApproved(ctx, id, fulfilledID); err != nil {
		return Request{}, err
	}
	out, err := s.store.Get(ctx, id)
	if err == nil && s.notify != nil {
		go s.notify(context.WithoutCancel(ctx), out, "approved")
	}
	return out, err
}

// Reject declines a pending or failed request with a reason. Requests that are
// already approved, available, or being fulfilled (approving) cannot be
// rejected.
func (s *Service) Reject(ctx context.Context, id, reason, decidedBy string) (Request, error) {
	req, err := s.store.Get(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if req.Status != StatusPending && req.Status != StatusFailed {
		return Request{}, fmt.Errorf("requests: cannot reject a %s request", req.Status)
	}
	if err := s.store.MarkRejected(ctx, id, reason, decidedBy); err != nil {
		return Request{}, err
	}
	out, err := s.store.Get(ctx, id)
	if err == nil && s.notify != nil {
		go s.notify(context.WithoutCancel(ctx), out, "rejected")
	}
	return out, err
}
