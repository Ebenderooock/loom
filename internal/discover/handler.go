// Package discover provides person-filmography lookup backed by TMDB.
package discover

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

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

// Router returns a chi.Router for person discovery endpoints.
func Router(client *tmdb.Client) chi.Router {
	r := chi.NewRouter()
	r.Get("/people/{id}", getPersonFilmography(client))
	return r
}

func getPersonFilmography(client *tmdb.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		personID, err := strconv.Atoi(idStr)
		if err != nil || personID <= 0 {
			http.Error(w, "invalid person ID", http.StatusBadRequest)
			return
		}

		ctx := r.Context()

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
			http.Error(w, pRes.err.Error(), http.StatusInternalServerError)
			return
		}
		if cRes.err != nil {
			http.Error(w, cRes.err.Error(), http.StatusInternalServerError)
			return
		}

		person := pRes.person
		credits := cRes.credits

		bio := person.Biography
		if len(bio) > maxBioLen {
			bio = bio[:maxBioLen]
		}

		resp := PersonFilmography{
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
