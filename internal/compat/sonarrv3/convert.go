package sonarrv3

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/series"
)

// seriesToSonarr converts a Loom Series into a Sonarr v3 series
// JSON response, using ids to map string UUIDs to integers and libs
// to resolve the root folder path.
func seriesToSonarr(s *series.Series, ids *idCache, libs []libraries.Library) sonarrSeries {
	numID := ids.toInt(s.ID)

	// Resolve external IDs.
	tvdbID := 0
	if s.TVDBID != nil {
		if n, err := strconv.Atoi(*s.TVDBID); err == nil {
			tvdbID = n
		}
	}
	imdbID := ""
	if s.IMDBID != nil {
		imdbID = *s.IMDBID
	}

	// Resolve root-folder path from library.
	rootFolder := ""
	for _, lib := range libs {
		if lib.ID == s.LibraryID {
			rootFolder = lib.Path
			break
		}
	}
	seriesPath := rootFolder
	if rootFolder != "" && s.Title != "" {
		seriesPath = filepath.Join(rootFolder, s.Title)
	}

	// Map monitoring status → Sonarr "monitored" bool.
	monitored := s.MonitoringStatus != series.MonitoringNone &&
		s.MonitoringStatus != series.MonitoringUnmonitored

	// Map seasons.
	seasons := make([]sonarrSeason, 0, len(s.Seasons))
	for _, sn := range s.Seasons {
		seasons = append(seasons, sonarrSeason{
			SeasonNumber: sn.SeasonNumber,
			Monitored:    sn.Monitored,
		})
	}

	// Build images.
	images := make([]sonarrImage, 0, 2)
	if s.PosterPath != "" {
		images = append(images, sonarrImage{CoverType: "poster", URL: s.PosterPath})
	}
	if s.BackdropPath != "" {
		images = append(images, sonarrImage{CoverType: "fanart", URL: s.BackdropPath})
	}

	// Genres.
	genres := make([]string, 0)
	if s.Genres != nil {
		genres = []string(s.Genres)
	}

	// Statistics.
	stats := sonarrStatistics{SeasonCount: len(s.Seasons)}
	if s.Episodes != nil {
		stats.TotalEpisodeCount = len(s.Episodes)
		stats.EpisodeCount = len(s.Episodes)
		for _, ep := range s.Episodes {
			if ep.HasFile {
				stats.EpisodeFileCount++
			}
		}
		if stats.EpisodeCount > 0 {
			stats.PercentOfEpisodes = float64(stats.EpisodeFileCount) / float64(stats.EpisodeCount) * 100
		}
	}

	// Quality profile ID.
	qpID := ids.toInt(s.QualityProfileID)

	return sonarrSeries{
		ID:                numID,
		Title:             s.Title,
		SortTitle:         strings.ToLower(s.Title),
		Year:              s.Year,
		TvdbID:            tvdbID,
		ImdbID:            imdbID,
		TvMazeID:          0,
		Overview:          s.Overview,
		Network:           s.Network,
		Runtime:           s.Runtime,
		Monitored:         monitored,
		QualityProfileID:  qpID,
		LanguageProfileID: 1,
		RootFolderPath:    rootFolder,
		Path:              seriesPath,
		Added:             s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Status:            string(s.Status),
		SeriesType:        string(s.SeriesType),
		SeasonFolder:      s.SeasonFolder,
		Genres:            genres,
		Tags:              []int{},
		Seasons:           seasons,
		Images:            images,
		Ratings:           sonarrRatings{Votes: 0, Value: s.Rating},
		Statistics:        stats,
	}
}

// episodeToSonarr converts a Loom Episode into a Sonarr v3 episode
// response. seasonNum must be resolved by the caller since the Episode
// struct stores SeasonID, not SeasonNumber.
func episodeToSonarr(ep *series.Episode, seasonNum int, ids *idCache) sonarrEpisode {
	return sonarrEpisode{
		ID:            ids.toInt(ep.ID),
		SeriesID:      ids.toInt(ep.SeriesID),
		SeasonNumber:  seasonNum,
		EpisodeNumber: ep.EpisodeNumber,
		Title:         ep.Title,
		Overview:      ep.Overview,
		AirDate:       ep.AirDate,
		Monitored:     ep.Monitored,
		HasFile:       ep.HasFile,
	}
}

// rootFolderToSonarr converts a Loom Library to a Sonarr v3 root folder.
func rootFolderToSonarr(lib libraries.Library, ids *idCache) sonarrRootFolder {
	return sonarrRootFolder{
		ID:         ids.toInt(lib.ID),
		Path:       lib.Path,
		Accessible: true,
		FreeSpace:  lib.DiskSpace.FreeBytes,
	}
}

// qualityProfileToSonarr converts a Loom QualityProfile to a Sonarr v3
// quality profile.
func qualityProfileToSonarr(qp qualityprofiles.QualityProfile, ids *idCache) sonarrQualityProfile {
	return sonarrQualityProfile{
		ID:   ids.toInt(qp.ID),
		Name: qp.Name,
	}
}

// tmdbResultToSonarr converts a raw TMDB search result map into a
// Sonarr-formatted series lookup response.
func tmdbResultToSonarr(m map[string]interface{}) map[string]interface{} {
	title, _ := m["name"].(string)
	if title == "" {
		title, _ = m["title"].(string)
	}
	overview, _ := m["overview"].(string)
	year := 0
	if fd, ok := m["first_air_date"].(string); ok && len(fd) >= 4 {
		if n, err := strconv.Atoi(fd[:4]); err == nil {
			year = n
		}
	}
	tvdbID := 0
	tmdbID := 0
	if v, ok := m["id"].(float64); ok {
		tmdbID = int(v)
	}

	images := []sonarrImage{}
	if poster, ok := m["poster_path"].(string); ok && poster != "" {
		images = append(images, sonarrImage{
			CoverType: "poster",
			URL:       fmt.Sprintf("https://image.tmdb.org/t/p/w500%s", poster),
		})
	}

	return map[string]interface{}{
		"title":    title,
		"sortTitle": strings.ToLower(title),
		"year":     year,
		"tvdbId":   tvdbID,
		"tmdbId":   tmdbID,
		"overview": overview,
		"images":   images,
		"seasons":  []sonarrSeason{},
		"ratings":  sonarrRatings{},
		"status":   "continuing",
	}
}
