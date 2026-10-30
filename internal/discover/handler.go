// Package discover provides read-only title metadata and person filmographies.
package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/metadata"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

// PersonDetail holds basic info about a person.
type PersonDetail struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Biography   string `json:"biography,omitempty"`
	ProfilePath string `json:"profile_path,omitempty"`
	Birthday    string `json:"birthday,omitempty"`
	Deathday    string `json:"deathday,omitempty"`
	KnownFor    string `json:"known_for,omitempty"`
}

// CreditItem is a single movie or TV entry from a person's filmography.
type CreditItem struct {
	TMDBID      int     `json:"tmdb_id"`
	MediaType   string  `json:"media_type"` // "movie" or "tv"
	Title       string  `json:"title"`
	Year        int     `json:"year,omitempty"`
	PosterPath  string  `json:"poster_path,omitempty"`
	Overview    string  `json:"overview,omitempty"`
	Rating      float64 `json:"rating"`
	Popularity  float64 `json:"popularity"`
	CreditType  string  `json:"credit_type"` // "cast" or "crew"
	Character   string  `json:"character,omitempty"`
	Job         string  `json:"job,omitempty"`
	Department  string  `json:"department,omitempty"`
	ReleaseDate string  `json:"release_date,omitempty"`
}

// PersonFilmography is the response for the person discover endpoint.
type PersonFilmography struct {
	Person  PersonDetail `json:"person"`
	Credits []CreditItem `json:"credits"`
}

const (
	posterCDN      = "https://image.tmdb.org/t/p/w342"
	profileCDN     = "https://image.tmdb.org/t/p/w185"
	maxBioLen      = 1000
	maxOverviewLen = 500
)

type MusicDetails struct {
	Provider metadata.MusicMetadataProvider
	Enabled  func() bool
}

// Router returns read-only provider metadata and optional library-growth routes.
func Router(client *tmdb.Client, services ...*Service) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}", getPersonFilmography(client))
	r.Get("/movie/{id}", getVideo(client, false))
	r.Get("/series/{id}", getVideo(client, true))
	if len(services) > 0 {
		mountGrowth(r, services[0])
	}
	return r
}

// RouterWithMusic wires the MusicBrainz routes alongside video details and
// library growth, while keeping Router's optional service argument strongly typed.
func RouterWithMusic(client *tmdb.Client, service *Service, music MusicDetails) chi.Router {
	r := Router(client, service)
	if music.Provider == nil {
		return r
	}
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if music.Enabled != nil && !music.Enabled() {
					http.NotFound(w, req)
					return
				}
				next.ServeHTTP(w, req)
			})
		})
		r.Get("/artist/{id}", getArtist(music.Provider))
		r.Get("/album/{id}", getAlbum(music.Provider))
		r.Get("/release/{id}", getRelease(music.Provider))
	})
	return r
}

func getAlbum(provider metadata.MusicMetadataProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "invalid MusicBrainz ID", http.StatusBadRequest)
			return
		}
		album, releases, err := provider.GetAlbum(r.Context(), id.String())
		if err != nil {
			http.Error(w, "Unable to load album metadata", http.StatusBadGateway)
			return
		}
		if album == nil || album.MBID != id.String() {
			http.Error(w, "Album not found for this identity", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"album": album, "releases": releases})
	}
}

func getRelease(provider metadata.MusicMetadataProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "invalid MusicBrainz ID", http.StatusBadRequest)
			return
		}
		release, err := provider.GetAlbumRelease(r.Context(), id.String())
		if err != nil {
			http.Error(w, "Unable to load track listing", http.StatusBadGateway)
			return
		}
		if release == nil || release.MBID != id.String() {
			http.Error(w, "Edition not found for this identity", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}
}

// These routes are read-only: provider identity is always scoped by media type,
// never interpreted as a local library ID or resolved by a title search.
func getVideo(client *tmdb.Client, series bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawID := chi.URLParam(r, "id")
		provider := r.URL.Query().Get("provider")
		if provider == "" {
			provider = "tmdb"
		}
		if !validVideoIdentity(provider, rawID) {
			http.Error(w, "invalid TMDB ID", http.StatusBadRequest)
			return
		}
		id, _ := strconv.Atoi(rawID)
		var err error
		if provider != "tmdb" {
			media := "movie"
			if series {
				media = "tv"
			}
			id, err = client.ResolveExternalID(r.Context(), media, provider, rawID)
			if err != nil {
				http.Error(w, "No unambiguous title found for this provider ID", http.StatusNotFound)
				return
			}
		}
		var title any
		var credits *metadata.Credits
		if series {
			title, err = client.GetTV(r.Context(), id)
		} else {
			title, err = client.GetMovie(r.Context(), id)
		}
		if err != nil {
			http.Error(w, "Unable to load title metadata", http.StatusBadGateway)
			return
		}
		var actualID *string
		switch result := title.(type) {
		case *metadata.MovieMetadata:
			if result != nil {
				actualID = result.TMDBID
			}
		case *metadata.SeriesMetadata:
			if result != nil {
				actualID = result.TMDBID
			}
		}
		if actualID == nil || *actualID != strconv.Itoa(id) {
			http.Error(w, "Provider returned a conflicting title identity", http.StatusBadGateway)
			return
		}
		if series {
			credits, err = client.GetTVCredits(r.Context(), id)
		} else {
			credits, err = client.GetMovieCredits(r.Context(), id)
		}
		creditsError := ""
		if err != nil {
			creditsError = "Cast and crew could not be loaded."
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"title": title, "credits": credits, "credits_error": creditsError})
	}
}

var imdbIdentity = regexp.MustCompile(`^tt[0-9]{1,12}$`)

func validVideoIdentity(provider, id string) bool {
	if provider == "imdb" {
		return imdbIdentity.MatchString(id)
	}
	if provider != "tmdb" && provider != "tvdb" {
		return false
	}
	n, err := strconv.Atoi(id)
	return err == nil && n > 0
}

func getArtist(provider metadata.MusicMetadataProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "invalid MusicBrainz ID", http.StatusBadRequest)
			return
		}
		artist, err := provider.GetArtist(r.Context(), id.String())
		if err != nil {
			http.Error(w, "Unable to load artist metadata", http.StatusBadGateway)
			return
		}
		if artist == nil || artist.MBID != id.String() {
			http.Error(w, "Artist not found", http.StatusNotFound)
			return
		}
		albums, err := provider.GetArtistAlbums(r.Context(), id.String())
		if err != nil {
			http.Error(w, "Unable to load artist albums", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"title": artist, "albums": albums})
	}
}

func getPersonFilmography(client *tmdb.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		personID, err := strconv.Atoi(idStr)
		if err != nil || personID <= 0 {
			http.Error(w, "invalid person ID", http.StatusBadRequest)
			return
		}

		if !client.Configured() {
			writeDiscoveryError(w, ErrNotConfigured)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Fetch person details and credits in parallel
		type personResult struct {
			person *tmdb.PersonResponse
			err    error
		}
		type creditsResult struct {
			credits *tmdb.CombinedCreditsResponse
			err     error
		}

		pCh := make(chan personResult, 1)
		cCh := make(chan creditsResult, 1)

		go func() {
			p, err := client.GetPerson(ctx, personID)
			pCh <- personResult{p, err}
		}()
		go func() {
			c, err := client.GetPersonCredits(ctx, personID)
			cCh <- creditsResult{c, err}
		}()

		pRes := <-pCh
		cRes := <-cCh

		if pRes.err != nil {
			writeDiscoveryError(w, pRes.err)
			return
		}
		if cRes.err != nil {
			writeDiscoveryError(w, cRes.err)
			return
		}

		person := pRes.person
		credits := cRes.credits

		bio := person.Biography
		if len(bio) > maxBioLen {
			bio = bio[:maxBioLen]
		}

		resp := PersonFilmography{
			Credits: []CreditItem{},
			Person: PersonDetail{
				ID:          person.ID,
				Name:        person.Name,
				Biography:   bio,
				ProfilePath: buildImageURL(profileCDN, person.ProfilePath),
				Birthday:    person.Birthday,
				Deathday:    person.Deathday,
				KnownFor:    person.KnownFor,
			},
		}

		// Deduplicate credits by TMDB ID + media type (keep highest-priority credit)
		seen := make(map[string]bool)
		for _, c := range credits.Cast {
			key := c.MediaType + ":" + strconv.Itoa(c.ID)
			if seen[key] {
				continue
			}
			seen[key] = true
			resp.Credits = append(resp.Credits, mapCreditEntry(c, "cast"))
		}
		for _, c := range credits.Crew {
			key := c.MediaType + ":" + strconv.Itoa(c.ID)
			if seen[key] {
				continue
			}
			seen[key] = true
			resp.Credits = append(resp.Credits, mapCreditEntry(c, "crew"))
		}

		// Sort by popularity descending
		sort.Slice(resp.Credits, func(i, j int) bool {
			return resp.Credits[i].Popularity > resp.Credits[j].Popularity
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func mapCreditEntry(c tmdb.CombinedCreditEntry, creditType string) CreditItem {
	title := c.Title
	if title == "" {
		title = c.Name
	}

	releaseDate := c.ReleaseDate
	if releaseDate == "" {
		releaseDate = c.FirstAirDate
	}

	var year int
	if len(releaseDate) >= 4 {
		year, _ = strconv.Atoi(releaseDate[:4])
	}

	overview := c.Overview
	if len(overview) > maxOverviewLen {
		overview = overview[:maxOverviewLen]
	}

	return CreditItem{
		TMDBID:      c.ID,
		MediaType:   c.MediaType,
		Title:       title,
		Year:        year,
		PosterPath:  buildImageURL(posterCDN, c.PosterPath),
		Overview:    overview,
		Rating:      c.VoteAverage,
		Popularity:  c.Popularity,
		CreditType:  creditType,
		Character:   c.Character,
		Job:         c.Job,
		Department:  c.Department,
		ReleaseDate: releaseDate,
	}
}

func buildImageURL(cdn, path string) string {
	if path == "" {
		return ""
	}
	if len(path) > 4 && path[:4] == "http" {
		return path
	}
	return cdn + path
}
