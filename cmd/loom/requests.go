package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ebenderooock/loom/internal/autosearch"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/requests"
	"github.com/ebenderooock/loom/internal/series"
	"github.com/ebenderooock/loom/internal/storage"
)

// buildRequestsService assembles the media-requests Service, wiring it to the
// movies/series add flows, the autosearch grab engine, and the quality
// profile / library stores for target validation. All request fulfillment
// re-fetches metadata server-side; caller-supplied fields are never trusted.
func buildRequestsService(
	db storage.DB,
	moviesSvc movies.Service,
	seriesSvc series.Service,
	engine *autosearch.Engine,
	libStore *libraries.Store,
	qpStore *qualityprofiles.Store,
	logger *slog.Logger,
) *requests.Service {
	f := &requestsFulfiller{
		movies: moviesSvc,
		series: seriesSvc,
		engine: engine,
		logger: logger,
	}
	v := &requestsValidator{
		libStore: libStore,
		qpStore:  qpStore,
	}
	return requests.NewService(requests.Options{
		Store:     requests.NewStore(db.DB()),
		Fulfiller: f,
		Validator: v,
		Logger:    logger,
	})
}

// requestsFulfiller implements requests.Fulfiller against the real add-media
// and search-and-grab flows.
type requestsFulfiller struct {
	movies movies.Service
	series series.Service
	engine *autosearch.Engine
	logger *slog.Logger
}

func (f *requestsFulfiller) MovieExists(ctx context.Context, tmdbID string) (string, error) {
	m, err := f.movies.GetMovieByTMDBID(ctx, tmdbID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", nil
	}
	return m.ID, nil
}

func (f *requestsFulfiller) SeriesExists(ctx context.Context, tmdbID string) (string, error) {
	list, err := f.series.ListSeries(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range list {
		if s.TMDBID != nil && *s.TMDBID == tmdbID {
			return s.ID, nil
		}
	}
	return "", nil
}

func (f *requestsFulfiller) FulfillMovie(ctx context.Context, tmdbID, qualityProfileID, libraryID string) (string, error) {
	m, err := f.movies.AddMovieByTMDBID(ctx, tmdbID, qualityProfileID, libraryID, true)
	if err != nil {
		return "", err
	}
	req := autosearch.SearchRequest{
		MediaType:        "movie",
		MediaID:          m.ID,
		Title:            m.Title,
		Year:             m.Year,
		QualityProfileID: qualityProfileID,
	}
	if m.IMDBID != nil {
		req.IMDBID = *m.IMDBID
	}
	if m.TMDBID != nil {
		req.TMDBID = *m.TMDBID
	}
	f.grab(req)
	return m.ID, nil
}

func (f *requestsFulfiller) FulfillSeries(ctx context.Context, tmdbID, qualityProfileID, libraryID string) (string, error) {
	sr, err := f.series.AddSeries(ctx, &series.AddSeriesRequest{
		TMDBID:           tmdbID,
		QualityProfileID: qualityProfileID,
		LibraryID:        libraryID,
		MonitoringStatus: string(series.MonitoringAll),
	})
	if err != nil {
		return "", err
	}
	req := autosearch.SearchRequest{
		MediaType:        "series",
		MediaID:          sr.ID,
		Title:            sr.Title,
		Year:             sr.Year,
		QualityProfileID: qualityProfileID,
	}
	if sr.IMDBID != nil {
		req.IMDBID = *sr.IMDBID
	}
	if sr.TMDBID != nil {
		req.TMDBID = *sr.TMDBID
	}
	if sr.TVDBID != nil {
		req.TVDBID = *sr.TVDBID
	}
	f.grab(req)
	return sr.ID, nil
}

// grab launches a detached search-and-grab; failures are logged, never block
// the approval response.
func (f *requestsFulfiller) grab(req autosearch.SearchRequest) {
	if f.engine == nil {
		return
	}
	go func() {
		ctx := context.WithoutCancel(context.Background())
		if _, err := f.engine.SearchAndGrab(ctx, req); err != nil {
			f.logger.Warn("requests: search-and-grab failed",
				"media_type", req.MediaType, "media_id", req.MediaID, "err", err)
		}
	}()
}

// requestsValidator implements requests.LibraryValidator using the quality
// profile and library stores.
type requestsValidator struct {
	libStore *libraries.Store
	qpStore  *qualityprofiles.Store
}

func (v *requestsValidator) ValidateTarget(ctx context.Context, mediaType requests.MediaType, qualityProfileID, libraryID string) error {
	if _, err := v.qpStore.Get(ctx, qualityProfileID); err != nil {
		return fmt.Errorf("quality profile %q not found", qualityProfileID)
	}
	lib, err := v.libStore.Get(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("library %q not found", libraryID)
	}
	want := "movie"
	if mediaType == requests.MediaSeries {
		want = "series"
	}
	if lib.MediaType != want {
		return fmt.Errorf("library %q is a %s library, not %s", libraryID, lib.MediaType, want)
	}
	return nil
}
