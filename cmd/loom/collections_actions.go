package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ebenderooock/loom/internal/airing"
	"github.com/ebenderooock/loom/internal/autosearch"
	"github.com/ebenderooock/loom/internal/collections"
	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/series"
)

// collectionActions adapts the existing media services and automatic search
// engine for collection bulk operations; it adds no acquisition logic.
type collectionActions struct {
	db       *sql.DB
	movies   movies.Service
	series   series.Service
	libs     *libraries.Store
	profiles *qualityprofiles.Store
	engine   *autosearch.Engine
	logger   *slog.Logger
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (a *collectionActions) QualityProfiles(ctx context.Context) ([]collections.Option, error) {
	items, err := a.profiles.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]collections.Option, 0, len(items))
	for _, p := range items {
		out = append(out, collections.Option{ID: p.ID, Name: p.Name})
	}
	return out, nil
}

func (a *collectionActions) Libraries(ctx context.Context, mediaType string) ([]collections.Option, error) {
	items, err := a.libs.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []collections.Option{}
	for _, l := range items {
		if l.MediaType == mediaType {
			out = append(out, collections.Option{ID: l.ID, Name: l.Name, QualityProfileID: l.QualityProfileID})
		}
	}
	return out, nil
}

func (a *collectionActions) SetQualityProfile(ctx context.Context, mediaType, mediaID, profileID string) error {
	if _, err := a.profiles.Get(ctx, profileID); err != nil {
		return fmt.Errorf("quality profile not found")
	}
	switch mediaType {
	case "movie":
		m, err := a.movies.GetMovie(ctx, mediaID)
		if err != nil || m == nil {
			return fmt.Errorf("movie not found")
		}
		m.QualityProfileID = profileID
		return a.movies.UpdateMovie(ctx, m)
	case "series":
		sr, err := a.series.GetSeries(ctx, mediaID)
		if err != nil || sr == nil {
			return fmt.Errorf("show not found")
		}
		sr.QualityProfileID = profileID
		return a.series.UpdateSeries(ctx, sr)
	}
	return fmt.Errorf("unsupported media type")
}

func (a *collectionActions) Add(ctx context.Context, req collections.AddRequest) (string, error) {
	if _, err := a.profiles.Get(ctx, req.QualityProfileID); err != nil {
		return "", fmt.Errorf("quality profile not found")
	}
	switch req.MediaType {
	case "movie":
		m, err := a.movies.AddMovieByTMDBID(ctx, req.TMDBID, req.QualityProfileID, req.LibraryID, req.Monitored)
		if err != nil {
			return "", err
		}
		if req.Search && req.Monitored {
			searchCtx := context.WithoutCancel(ctx)
			go func() {
				if err := a.SearchMissing(searchCtx, "movie", m.ID); err != nil {
					a.logger.Warn("collections: search on add failed", "movie", m.ID, "error", err)
				}
			}()
		}
		return m.ID, nil
	case "series":
		shows, err := a.series.ListSeries(ctx)
		if err != nil {
			return "", err
		}
		for _, sr := range shows {
			if derefString(sr.TMDBID) == req.TMDBID {
				return sr.ID, nil
			}
		}
		status := string(series.MonitoringAll)
		if !req.Monitored {
			status = string(series.MonitoringNone)
		}
		sr, err := a.series.AddSeries(ctx, &series.AddSeriesRequest{TMDBID: req.TMDBID, QualityProfileID: req.QualityProfileID, LibraryID: req.LibraryID, MonitoringStatus: status, Search: req.Search && req.Monitored})
		if err != nil {
			return "", err
		}
		return sr.ID, nil
	}
	return "", fmt.Errorf("unsupported media type")
}

// SearchMissing searches a monitored missing movie, or each season with
// monitored aired missing episodes (single-episode seasons search the episode).
func (a *collectionActions) SearchMissing(ctx context.Context, mediaType, mediaID string) error {
	if a.engine == nil {
		return collections.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	switch mediaType {
	case "movie":
		m, err := a.movies.GetMovie(ctx, mediaID)
		if err != nil || m == nil {
			return fmt.Errorf("movie not found")
		}
		if m.MonitoringStatus != movies.MonitoringStatusMonitored {
			return fmt.Errorf("movie is not monitored")
		}
		_, err = a.engine.SearchAndGrab(ctx, autosearch.SearchRequest{MediaType: "movie", MediaID: m.ID, Title: m.Title, Year: m.Year, QualityProfileID: m.QualityProfileID, IMDBID: derefString(m.IMDBID), TMDBID: derefString(m.TMDBID), TVDBID: derefString(m.TVDBID)})
		return err
	case "series":
		sr, err := a.series.GetSeries(ctx, mediaID)
		if err != nil || sr == nil {
			return fmt.Errorf("show not found")
		}
		//nolint:gosec // airing.SQLAired is a constant SQL fragment; the series ID is bound.
		rows, err := a.db.QueryContext(ctx, `SELECT sn.season_number, e.episode_number FROM episodes e
			JOIN seasons sn ON sn.id = e.season_id AND sn.series_id = e.series_id
			WHERE e.series_id = ? AND e.has_file = 0 AND e.monitored = 1 AND sn.monitored = 1
			  AND sn.season_number > 0 AND `+airing.SQLAired+`
			ORDER BY sn.season_number, e.episode_number`, mediaID)
		if err != nil {
			return err
		}
		seasons, order := map[int][]int{}, []int{}
		for rows.Next() {
			var season, episode int
			if err := rows.Scan(&season, &episode); err != nil {
				_ = rows.Close()
				return err
			}
			if _, ok := seasons[season]; !ok {
				order = append(order, season)
			}
			seasons[season] = append(seasons[season], episode)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(order) == 0 {
			return fmt.Errorf("no monitored aired episodes are missing")
		}
		var failures []error
		for _, season := range order {
			req := autosearch.SearchRequest{MediaType: "series", MediaID: sr.ID, Title: sr.Title, Year: sr.Year, QualityProfileID: sr.QualityProfileID, IMDBID: derefString(sr.IMDBID), TMDBID: derefString(sr.TMDBID), TVDBID: derefString(sr.TVDBID), Season: season}
			if eps := seasons[season]; len(eps) == 1 {
				req.Episode = eps[0]
			}
			if _, err := a.engine.SearchAndGrab(ctx, req); err != nil {
				failures = append(failures, fmt.Errorf("season %d: %w", season, err))
			}
		}
		return errors.Join(failures...)
	}
	return fmt.Errorf("unsupported media type")
}
