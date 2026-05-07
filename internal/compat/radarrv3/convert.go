package radarrv3

import (
	"fmt"
	"strconv"
	"time"

	"github.com/ebenderooock/loom/internal/libraries"
	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
)

// movieToRadarr converts a Loom Movie into a Radarr v3 movie response.
func (h *Handler) movieToRadarr(m *movies.Movie, hasFile bool) radarrMovie {
	rm := radarrMovie{
		ID:        h.movies.toInt(m.ID),
		Title:     m.Title,
		SortTitle: m.Title,
		Year:      m.Year,
		Overview:  m.Overview,
		Monitored: m.MonitoringStatus == movies.MonitoringStatusMonitored,
		HasFile:   hasFile,
		Added:     m.CreatedAt,
		Status:    loomStatusToRadarr(m.Status),
		Runtime:   m.Runtime,
		Genres:    m.Genres,
		Ratings:   radarrRatings{Value: m.Rating},
		TitleSlug: fmt.Sprintf("%s-%d", m.Title, m.Year),
	}

	if m.TMDBID != nil {
		rm.TmdbID, _ = strconv.Atoi(*m.TMDBID)
	}
	if m.IMDBID != nil {
		rm.ImdbID = *m.IMDBID
	}

	if m.QualityProfileID != "" {
		rm.QualityProfileID = h.qpIDs.toInt(m.QualityProfileID)
	}

	if m.LibraryID != "" {
		rm.RootFolderPath = h.resolveLibraryPath(m.LibraryID)
		rm.Path = fmt.Sprintf("%s/%s (%d)", rm.RootFolderPath, m.Title, m.Year)
		rm.FolderName = fmt.Sprintf("%s (%d)", m.Title, m.Year)
	}

	rm.IsAvailable = hasFile || m.Status == movies.MovieStatusAvailableRightQuality ||
		m.Status == movies.MovieStatusAvailableHigherQuality

	// Images
	if m.PosterPath != "" {
		rm.Images = append(rm.Images, radarrImage{CoverType: "poster", RemoteURL: m.PosterPath})
	}
	if m.BackdropPath != "" {
		rm.Images = append(rm.Images, radarrImage{CoverType: "fanart", RemoteURL: m.BackdropPath})
	}
	if rm.Images == nil {
		rm.Images = []radarrImage{}
	}
	if rm.Genres == nil {
		rm.Genres = []string{}
	}

	return rm
}

// metadataToRadarr converts search metadata into a Radarr v3 movie stub.
func (h *Handler) metadataToRadarr(md *metadata.MovieMetadata) radarrMovie {
	rm := radarrMovie{
		Title:     md.Title,
		Year:      md.Year,
		Overview:  md.Overview,
		Monitored: false,
		HasFile:   false,
		Status:    "released",
		Runtime:   md.Runtime,
		Genres:    md.Genres,
		Ratings:   radarrRatings{Value: md.Rating},
		Images:    []radarrImage{},
		Added:     time.Time{},
		TitleSlug: fmt.Sprintf("%s-%d", md.Title, md.Year),
	}
	if md.TMDBID != nil {
		rm.TmdbID, _ = strconv.Atoi(*md.TMDBID)
		rm.ID = rm.TmdbID
	}
	if md.IMDBID != nil {
		rm.ImdbID = *md.IMDBID
	}
	if md.PosterPath != "" {
		rm.Images = append(rm.Images, radarrImage{CoverType: "poster", RemoteURL: md.PosterPath})
	}
	if rm.Genres == nil {
		rm.Genres = []string{}
	}
	return rm
}

// radarrToLoomMovie converts an incoming Radarr add-movie request to a Loom Movie.
func (h *Handler) radarrToLoomMovie(req radarrAddMovieRequest) *movies.Movie {
	m := &movies.Movie{
		Title:    req.Title,
		Year:     req.Year,
		Overview: req.Overview,
	}
	if req.TmdbID != 0 {
		s := strconv.Itoa(req.TmdbID)
		m.TMDBID = &s
	}
	if req.ImdbID != "" {
		m.IMDBID = &req.ImdbID
	}
	if req.Monitored {
		m.MonitoringStatus = movies.MonitoringStatusMonitored
	} else {
		m.MonitoringStatus = movies.MonitoringStatusUnmonitored
	}
	if req.QualityProfileID != 0 {
		if id, ok := h.qpIDs.toString(req.QualityProfileID); ok {
			m.QualityProfileID = id
		}
	}
	if req.RootFolderPath != "" {
		m.LibraryID = h.resolveLibraryID(req.RootFolderPath)
	}
	m.Status = movies.MovieStatusMissing
	return m
}

// libraryToRootFolder converts a Loom Library to a Radarr root folder.
func (h *Handler) libraryToRootFolder(lib libraries.Library) radarrRootFolder {
	return radarrRootFolder{
		ID:              h.libIDs.toInt(lib.ID),
		Path:            lib.Path,
		Accessible:      lib.Accessible,
		FreeSpace:       lib.DiskSpace.FreeBytes,
		UnmappedFolders: []radarrUnmappedFolder{},
	}
}

// qpToRadarr converts a Loom QualityProfile to a Radarr quality profile.
func (h *Handler) qpToRadarr(qp qualityprofiles.QualityProfile) radarrQualityProfile {
	cutoff, _ := strconv.Atoi(qp.Cutoff)
	return radarrQualityProfile{
		ID:             h.qpIDs.toInt(qp.ID),
		Name:           qp.Name,
		UpgradeAllowed: qp.UpgradeAllowed,
		Cutoff:         cutoff,
		Items:          []any{},
	}
}

func loomStatusToRadarr(s movies.MovieStatus) string {
	switch s {
	case movies.MovieStatusUnreleased:
		return "announced"
	case movies.MovieStatusMissing:
		return "released"
	case movies.MovieStatusDownloading, movies.MovieStatusStoring:
		return "released"
	case movies.MovieStatusAvailableRightQuality, movies.MovieStatusAvailableHigherQuality,
		movies.MovieStatusAvailableWrongQuality:
		return "released"
	default:
		return "released"
	}
}

// resolveLibraryPath looks up a cached library path by ID.
func (h *Handler) resolveLibraryPath(libID string) string {
	h.libCacheMu.RLock()
	defer h.libCacheMu.RUnlock()
	if p, ok := h.libPathCache[libID]; ok {
		return p
	}
	return ""
}

// resolveLibraryID finds a library ID by its root folder path.
func (h *Handler) resolveLibraryID(path string) string {
	h.libCacheMu.RLock()
	defer h.libCacheMu.RUnlock()
	for id, p := range h.libPathCache {
		if p == path {
			return id
		}
	}
	return ""
}
