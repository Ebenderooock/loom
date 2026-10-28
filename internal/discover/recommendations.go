package discover

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

func recommendation(item tmdb.SearchResult, mediaType, source, reason string, collectionID int) Recommendation {
	title, date := item.Title, item.ReleaseDate
	if mediaType == "series" {
		title, date = item.Name, item.FirstAirDate
	}
	year := 0
	if len(date) >= 4 {
		year, _ = strconv.Atoi(date[:4])
	}
	return Recommendation{TMDBID: item.ID, MediaType: mediaType, Title: title,
		Year: year, PosterPath: buildImageURL(posterCDN, item.PosterPath),
		Overview: item.Overview, Rating: item.VoteAverage, Popularity: item.Popularity,
		ReleaseDate: date, Source: source, Reasons: []string{reason}, CollectionID: collectionID}
}

func future(date string, now time.Time) bool {
	d, err := time.Parse("2006-01-02", date)
	return err == nil && d.Format("2006-01-02") > now.UTC().Format("2006-01-02")
}

func sourcePriority(source string) int {
	switch source {
	case "collection":
		return 0
	case "director":
		return 1
	case "actor":
		return 2
	case "related":
		return 3
	default:
		return 4
	}
}

// Recommendations suggests titles to add, excluding catalog and dismissed titles.
func (s *Service) Recommendations(parent context.Context, userID int64, mediaType string, includeUpcoming bool) (*RecommendationsResponse, error) {
	if !validMediaType(mediaType) || userID <= 0 {
		return nil, fmt.Errorf("invalid recommendation request")
	}
	ctx, done, err := s.begin(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	library, err := s.library(ctx, mediaType)
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, fmt.Errorf("dismissal storage unavailable")
	}
	dismissed, err := s.store.Dismissed(ctx, userID, mediaType)
	if err != nil {
		return nil, err
	}
	c := s.session()
	now := s.now()
	candidates := make(map[int]*Recommendation)
	add := func(item tmdb.SearchResult, source, reason string, collectionID int) {
		if item.ID <= 0 || item.Adult || dismissed[item.ID] {
			return
		}
		if _, known := library[item.ID]; known {
			return
		}
		rec := recommendation(item, mediaType, source, reason, collectionID)
		if rec.Title == "" {
			return
		}
		if _, err := time.Parse("2006-01-02", rec.ReleaseDate); err != nil {
			return // undated titles are never default growth recommendations
		}
		if !includeUpcoming && future(rec.ReleaseDate, now) {
			return
		}
		if old, ok := candidates[item.ID]; ok {
			for _, r := range old.Reasons {
				if r == reason {
					return
				}
			}
			old.Reasons = append(old.Reasons, reason)
			if sourcePriority(source) < sourcePriority(old.Source) {
				old.Source = source
			}
			if collectionID > 0 && (old.CollectionID == 0 || collectionID < old.CollectionID) {
				old.CollectionID = collectionID
			}
		} else {
			candidates[item.ID] = &rec
		}
	}
	seedDetails := details(c, ctx, mediaType, library)
	if mediaType == "movie" {
		for _, id := range collectionIDs(c, seedDetails) {
			set, err := getCollection(c, ctx, id)
			if err != nil {
				continue
			}
			for _, part := range set.Parts {
				add(part, "collection", "Missing from "+set.Name, set.ID)
			}
		}
	}
	for _, person := range repeatedPeople(seedDetails) {
		credits, err := fetch(c, ctx, fmt.Sprintf("person/%d/combined_credits", person.id), func(ctx context.Context) (*tmdb.CombinedCreditsResponse, error) {
			return s.client.GetPersonCredits(ctx, person.id)
		})
		if err != nil {
			continue
		}
		entries := credits.Cast
		reason := fmt.Sprintf("Featuring %s, in %d library titles", person.name, person.count)
		if person.role == "director" {
			entries = credits.Crew
			reason = fmt.Sprintf("Directed by %s, director of %d library titles", person.name, person.count)
		}
		for _, entry := range entries {
			if person.role == "director" && entry.Job != "Director" {
				continue
			}
			expected := "movie"
			if mediaType == "series" {
				expected = "tv"
			}
			if entry.MediaType != expected {
				continue
			}
			add(tmdb.SearchResult{ID: entry.ID, Title: entry.Title, Name: entry.Name,
				ReleaseDate: entry.ReleaseDate, FirstAirDate: entry.FirstAirDate, Adult: entry.Adult,
				Overview: entry.Overview, PosterPath: entry.PosterPath, VoteAverage: entry.VoteAverage,
				Popularity: entry.Popularity}, person.role, reason, 0)
		}
	}
	for i, seed := range seedDetails {
		if i >= maxRelated {
			break
		}
		related, err := fetch(c, ctx, fmt.Sprintf("%s/%d/recommendations", mediaType, seed.id), func(ctx context.Context) (*tmdb.SearchResponse, error) {
			return s.client.GetRecommendations(ctx, mediaType, seed.id)
		})
		if err == nil {
			for _, item := range related.Results {
				add(item, "related", "TMDB recommendations for "+seed.title, 0)
			}
		}
	}
	for _, genre := range genreSources(seedDetails, mediaType) {
		until := now.UTC().Format("2006-01-02")
		if includeUpcoming {
			until = ""
		}
		key := "discover/" + mediaType + "/" + genre.key + "/" + until
		results, err := fetch(c, ctx, key, func(ctx context.Context) (*tmdb.SearchResponse, error) {
			return s.client.Discover(ctx, mediaType, tmdb.DiscoverOptions{GenreIDs: genre.ids, ReleasedBy: until})
		})
		if err == nil {
			for _, item := range results.Results {
				add(item, "genre", "Based on "+genre.name+" in your library", 0)
			}
		}
	}
	if err := c.resultError(ctx); err != nil {
		return nil, err
	}
	out := &RecommendationsResponse{Items: []Recommendation{}, ResultStatus: c.status()}
	for _, rec := range candidates {
		sort.Strings(rec.Reasons)
		out.Items = append(out.Items, *rec)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if len(a.Reasons) != len(b.Reasons) {
			return len(a.Reasons) > len(b.Reasons)
		}
		if sourcePriority(a.Source) != sourcePriority(b.Source) {
			return sourcePriority(a.Source) < sourcePriority(b.Source)
		}
		if a.Rating != b.Rating {
			return a.Rating > b.Rating
		}
		if a.Popularity != b.Popularity {
			return a.Popularity > b.Popularity
		}
		return a.TMDBID < b.TMDBID
	})
	if len(out.Items) > maxResults {
		out.Items = out.Items[:maxResults]
	}
	return out, nil
}

type personSource struct {
	id    int
	name  string
	role  string
	count int
}

func repeatedPeople(seeds []seedDetail) []personSource {
	people := make(map[string]personSource)
	for _, seed := range seeds {
		seen := make(map[string]bool)
		count := func(id int, name, role string) {
			key := role + "/" + strconv.Itoa(id)
			if id <= 0 || seen[key] {
				return
			}
			seen[key] = true
			p := people[key]
			p.id, p.name, p.role = id, name, role
			p.count++
			people[key] = p
		}
		cast := append([]tmdb.CastEntry{}, seed.credits.Cast...)
		sort.Slice(cast, func(i, j int) bool {
			if cast[i].Order != cast[j].Order {
				return cast[i].Order < cast[j].Order
			}
			return cast[i].ID < cast[j].ID
		})
		for i, member := range cast {
			if i >= 10 {
				break
			}
			count(member.ID, member.Name, "actor")
		}
		for _, member := range seed.credits.Crew {
			if member.Job == "Director" {
				count(member.ID, member.Name, "director")
			}
		}
	}
	out := []personSource{}
	for _, p := range people {
		if p.count >= 2 {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		if out[i].role != out[j].role {
			return out[i].role < out[j].role
		}
		return out[i].id < out[j].id
	})
	if len(out) > maxPeople {
		// Reserve a slot for each qualifying role so a prolific recurring cast
		// cannot crowd every director out of the bounded filmography budget.
		selected := make(map[int]bool)
		roles := make(map[string]bool)
		for i, person := range out {
			if !roles[person.role] {
				selected[i], roles[person.role] = true, true
			}
		}
		for i := range out {
			if len(selected) >= maxPeople {
				break
			}
			selected[i] = true
		}
		bounded := make([]personSource, 0, maxPeople)
		for i, person := range out {
			if selected[i] {
				bounded = append(bounded, person)
			}
		}
		out = bounded
	}
	return out
}

type genreSource struct {
	ids   []int
	key   string
	name  string
	count int
}

func genreSources(seeds []seedDetail, mediaType string) []genreSource {
	genres := make(map[int]genreSource)
	romcom := 0
	for _, seed := range seeds {
		seen := make(map[int]bool)
		for _, g := range seed.genres {
			if g.ID <= 0 || seen[g.ID] {
				continue
			}
			seen[g.ID] = true
			entry := genres[g.ID]
			entry.ids, entry.name, entry.key = []int{g.ID}, g.Name, strconv.Itoa(g.ID)
			entry.count++
			genres[g.ID] = entry
		}
		if seen[35] && seen[10749] {
			romcom++
		}
	}
	out := []genreSource{}
	for _, g := range genres {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].ids[0] < out[j].ids[0]
	})
	if len(out) > 3 {
		out = out[:3]
	}
	if mediaType == "movie" && romcom > 0 {
		out = append(out, genreSource{ids: []int{35, 10749}, key: "35,10749", name: "Comedy + Romance", count: romcom})
	}
	return out
}
