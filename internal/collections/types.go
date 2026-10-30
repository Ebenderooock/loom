// Package collections owns persistent movie/TV groupings independently of
// library acquisition, monitoring and files. Provider identity is never inferred
// from a name or from overlapping membership.
package collections

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrInvalid identifies rejected input, not a provider access failure.
	ErrInvalid = errors.New("invalid collection input")
	// ErrConflict identifies stale writes or contradictory exact identities.
	ErrConflict = errors.New("collection identity conflict; reload and retry")
	digits      = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
	imdbTitle   = regexp.MustCompile(`^tt[0-9]{1,12}$`)
	imdbList    = regexp.MustCompile(`^ls[0-9]{1,12}$`)
)

// Source is one explicit provider identity and its last reconciliation outcome.
type Source struct {
	Provider   string `json:"provider"`
	Kind       string `json:"kind"`
	ExternalID string `json:"external_id"`
	LastSync   string `json:"last_sync,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Key is the provenance namespace; a movie set and a list never collide.
func (s Source) Key() string { return s.Provider + ":" + s.Kind + ":" + s.ExternalID }

// Validate rejects unsupported kinds and URL-shaped/unbounded source IDs.
func (s Source) Validate() error {
	valid := (s.Provider == "tmdb" && (s.Kind == "collection" || s.Kind == "list") && digits.MatchString(s.ExternalID)) ||
		(s.Provider == "tvdb" && s.Kind == "list" && digits.MatchString(s.ExternalID)) ||
		(s.Provider == "imdb" && s.Kind == "list" && imdbList.MatchString(s.ExternalID))
	if !valid {
		return fmt.Errorf("%w: expected TMDB collection/list ID, TVDB list ID, or IMDb ls… list ID", ErrInvalid)
	}
	return nil
}

// Member stores exact title identity and local intent, independently of files.
type Member struct {
	ID          string            `json:"id"`
	MediaType   string            `json:"media_type"`
	Title       string            `json:"title"`
	Year        int               `json:"year,omitempty"`
	PosterPath  string            `json:"poster_path,omitempty"`
	Overview    string            `json:"overview,omitempty"`
	ReleaseDate string            `json:"release_date,omitempty"`
	ExternalIDs map[string]string `json:"external_ids"`
	Sources     []string          `json:"sources"`
	Provenance  []string          `json:"provenance"`
	Local       bool              `json:"local"`
	Excluded    bool              `json:"excluded"`
	// Library state is computed on reads, never persisted as membership.
	MediaID   string `json:"media_id,omitempty"`
	LibraryID string `json:"library_id,omitempty"`
	InLibrary bool   `json:"in_library"`
	Monitored bool   `json:"monitored"`
	HasFile   *bool  `json:"has_file,omitempty"`
}

// Validate accepts only bounded movie/show titles and supported external IDs.
func (m Member) Validate() error {
	if (m.MediaType != "movie" && m.MediaType != "series") || strings.TrimSpace(m.Title) == "" || len(m.Title) > 500 || len(m.Overview) > 20000 || len(m.PosterPath) > 2000 || len(m.ReleaseDate) > 32 || m.Year < 0 || m.Year > 9999 || len(m.ExternalIDs) == 0 {
		return fmt.Errorf("%w: a movie/show title and external IDs are required", ErrInvalid)
	}
	for provider, id := range m.ExternalIDs {
		valid := (provider == "imdb" && imdbTitle.MatchString(id)) || ((provider == "tmdb" || provider == "tvdb") && digits.MatchString(id))
		if !valid {
			return fmt.Errorf("%w: invalid external title ID", ErrInvalid)
		}
	}
	return nil
}

// Collection is a Loom-owned aggregate with simultaneous provider references.
type Collection struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Overview  string   `json:"overview"`
	Sources   []Source `json:"sources"`
	Members   []Member `json:"members"`
	Revision  int      `json:"revision"`
	Total     int      `json:"total"`
	InLibrary int      `json:"in_library"`
}

// Validate checks user-controlled collection metadata before persistence.
func (c Collection) Validate() error {
	if strings.TrimSpace(c.Name) == "" || len(c.Name) > 300 || len(c.Overview) > 20000 {
		return fmt.Errorf("%w: name is required (maximum 300 characters)", ErrInvalid)
	}
	return nil
}

func number(s string) int { n, _ := strconv.Atoi(s); return n }

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// coalesce only joins members that share an exact external ID in the same media
// namespace. Conflicting cross-provider mappings are rejected, not guessed.
func coalesce(members []Member, incoming Member) ([]Member, error) {
	for i := 0; i < len(members); i++ {
		candidate := members[i]
		if candidate.MediaType != incoming.MediaType {
			continue
		}
		match := false
		for p, id := range incoming.ExternalIDs {
			if candidate.ExternalIDs[p] == id {
				match = true
			}
		}
		if !match {
			continue
		}
		for p, id := range candidate.ExternalIDs {
			if other := incoming.ExternalIDs[p]; other != "" && other != id {
				return nil, ErrConflict
			}
			incoming.ExternalIDs[p] = id
		}
		for _, source := range candidate.Sources {
			if !contains(incoming.Sources, source) {
				incoming.Sources = append(incoming.Sources, source)
			}
			for _, source := range candidate.Provenance {
				if !contains(incoming.Provenance, source) {
					incoming.Provenance = append(incoming.Provenance, source)
				}
			}
		}
		incoming.ID = candidate.ID
		incoming.Local = incoming.Local || candidate.Local
		incoming.Excluded = incoming.Excluded || candidate.Excluded
		// Local metadata wins over provider refreshes.
		if candidate.Local {
			incoming.Title, incoming.Year = candidate.Title, candidate.Year
			incoming.PosterPath, incoming.Overview = candidate.PosterPath, candidate.Overview
			incoming.ReleaseDate = candidate.ReleaseDate
		}
		members = append(members[:i], members[i+1:]...)
		// New IDs can bridge previously disjoint records; rescan the union.
		i = -1
	}
	if incoming.Sources == nil {
		incoming.Sources = []string{}
	}
	for _, source := range incoming.Sources {
		if !contains(incoming.Provenance, source) {
			incoming.Provenance = append(incoming.Provenance, source)
		}
	}
	if incoming.Provenance == nil {
		incoming.Provenance = []string{}
	}
	return append(members, incoming), nil
}
