package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ebenderooock/loom/internal/auth"
	"github.com/ebenderooock/loom/internal/bots"
	"github.com/ebenderooock/loom/internal/bots/discord"
	"github.com/ebenderooock/loom/internal/bots/telegram"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/music"
	"github.com/ebenderooock/loom/internal/requests"
	"github.com/ebenderooock/loom/internal/storage"
)

// buildBots assembles the request-bot store, brain service, HTTP router, and
// platform supervisor. The returned router is mounted by the server; the
// supervisor is started/stopped by the caller. metadataSvc is shared with the
// movies service for catalog search; authStore resolves linked Loom users.
func buildBots(
	db storage.DB,
	requestsSvc *requests.Service,
	metadataSvc *metadata.Service,
	musicSvc music.Service,
	authStore auth.Store,
	adminMW func(http.Handler) http.Handler,
	logger *slog.Logger,
) (http.Handler, *bots.Supervisor) {
	store := bots.NewStore(db.DB())
	users := &botUserDirectory{store: authStore}

	brain := bots.NewService(bots.Options{
		Store:    store,
		Requests: requestsSvc,
		Search:   &botSearchAdapter{meta: metadataSvc, music: musicSvc},
		Users:    users,
		Logger:   logger,
	})

	tgFactory := func(token string) bots.Transport {
		return telegram.New(token, brain, logger)
	}
	dcFactory := func(token string) bots.Transport {
		return discord.New(token, brain, logger)
	}
	sup := bots.NewSupervisor(store, tgFactory, dcFactory, logger)

	if adminMW == nil {
		adminMW = func(next http.Handler) http.Handler { return next }
	}
	router := bots.Router(store, sup, users, adminMW)
	return router, sup
}

// botUserDirectory resolves a Loom user's display name and admin status.
type botUserDirectory struct {
	store auth.Store
}

func (d *botUserDirectory) Lookup(ctx context.Context, userID int64) (string, bool, error) {
	u, err := d.store.GetUserByID(ctx, userID)
	if err != nil {
		return "", false, err
	}
	return u.Username, u.Role == "admin", nil
}

// botSearchAdapter adapts the metadata service to the bots SearchService,
// performing trusted by-id re-fetches and multi-result searches.
type botSearchAdapter struct {
	meta  *metadata.Service
	music music.Service
}

func (a *botSearchAdapter) SearchMovies(ctx context.Context, query string) ([]bots.MediaResult, error) {
	res, err := a.meta.FindMovieByQuery(ctx, query, 0)
	if err != nil {
		return nil, err
	}
	out := make([]bots.MediaResult, 0, len(res))
	for _, m := range res {
		if r, ok := movieToResult(m); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (a *botSearchAdapter) SearchSeries(ctx context.Context, query string) ([]bots.MediaResult, error) {
	res, err := a.meta.FindSeriesByQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]bots.MediaResult, 0, len(res))
	for _, s := range res {
		if r, ok := seriesToResult(s); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (a *botSearchAdapter) GetMovie(ctx context.Context, tmdbID string) (*bots.MediaResult, error) {
	m, err := a.meta.FindMovieByTMDBID(ctx, tmdbID)
	if err != nil || m == nil {
		return nil, err
	}
	r, ok := movieToResult(m)
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (a *botSearchAdapter) GetSeries(ctx context.Context, tmdbID string) (*bots.MediaResult, error) {
	s, err := a.meta.FindSeriesByTMDBID(ctx, tmdbID)
	if err != nil || s == nil {
		return nil, err
	}
	r, ok := seriesToResult(s)
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (a *botSearchAdapter) SearchArtists(ctx context.Context, query string) ([]bots.MediaResult, error) {
	if a.music == nil {
		return nil, nil
	}
	res, err := a.music.LookupArtists(ctx, query, 0)
	if err != nil {
		return nil, err
	}
	out := make([]bots.MediaResult, 0, len(res))
	for _, ar := range res {
		if r, ok := artistToResult(ar); ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (a *botSearchAdapter) GetArtist(ctx context.Context, mbid string) (*bots.MediaResult, error) {
	if a.music == nil {
		return nil, nil
	}
	ar, err := a.music.LookupArtistByMBID(ctx, mbid)
	if err != nil || ar == nil {
		return nil, err
	}
	r, ok := artistToResult(ar)
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func artistToResult(a *music.ArtistLookupResult) (bots.MediaResult, bool) {
	if a == nil || a.MBID == "" {
		return bots.MediaResult{}, false
	}
	return bots.MediaResult{
		MediaType:  requests.MediaArtist,
		TMDBID:     a.MBID,
		Title:      a.Name,
		Overview:   a.Disambiguation,
		PosterPath: a.ImageURL,
	}, true
}

func movieToResult(m *metadata.MovieMetadata) (bots.MediaResult, bool) {
	if m == nil || m.TMDBID == nil || *m.TMDBID == "" {
		return bots.MediaResult{}, false
	}
	return bots.MediaResult{
		MediaType:  requests.MediaMovie,
		TMDBID:     *m.TMDBID,
		Title:      m.Title,
		Year:       m.Year,
		Overview:   m.Overview,
		PosterPath: m.PosterPath,
	}, true
}

func seriesToResult(s *metadata.SeriesMetadata) (bots.MediaResult, bool) {
	if s == nil || s.TMDBID == nil || *s.TMDBID == "" {
		return bots.MediaResult{}, false
	}
	return bots.MediaResult{
		MediaType:  requests.MediaSeries,
		TMDBID:     *s.TMDBID,
		Title:      s.Title,
		Year:       yearFromDate(s.FirstAirDate),
		Overview:   s.Overview,
		PosterPath: s.PosterPath,
	}, true
}

// yearFromDate extracts the leading year from an ISO date like "2021-05-03".
func yearFromDate(d string) int {
	if len(d) < 4 {
		return 0
	}
	y, err := strconv.Atoi(d[:4])
	if err != nil {
		return 0
	}
	return y
}
