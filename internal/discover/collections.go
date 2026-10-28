package discover

import (
	"context"
	"fmt"
	"sort"

	"github.com/ebenderooock/loom/internal/metadata/tmdb"
)

// Collections lists movie sets that contain at least one catalog movie.
func (s *Service) Collections(parent context.Context) (*CollectionsResponse, error) {
	ctx, done, err := s.begin(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	library, err := s.library(ctx, "movie")
	if err != nil {
		return nil, err
	}
	c := s.session()
	out := &CollectionsResponse{Items: []Collection{}}
	for _, id := range collectionIDs(c, details(c, ctx, "movie", library)) {
		set, err := getCollection(c, ctx, id)
		if err == nil {
			detail := s.collection(c, ctx, set, library, false)
			out.Items = append(out.Items, detail.Collection)
		}
	}
	if err := c.resultError(ctx); err != nil {
		return nil, err
	}
	out.ResultStatus = c.status()
	return out, nil
}

// Collection returns one movie set with release-ordered member status.
func (s *Service) Collection(parent context.Context, id int) (*CollectionResponse, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid collection ID")
	}
	ctx, done, err := s.begin(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	library, err := s.library(ctx, "movie")
	if err != nil {
		return nil, err
	}
	c := s.session()
	set, err := getCollection(c, ctx, id)
	if err != nil {
		return nil, err
	}
	out := s.collection(c, ctx, set, library, true)
	if err := c.resultError(ctx); err != nil {
		return nil, err
	}
	out.ResultStatus = c.status()
	return out, nil
}

func (s *Service) collection(c *catalogSession, ctx context.Context, set *tmdb.CollectionResponse, library map[int]libraryTitle, files bool) *CollectionResponse {
	out := &CollectionResponse{Collection: Collection{ID: set.ID, Name: set.Name,
		Overview: set.Overview, PosterPath: buildImageURL(posterCDN, set.PosterPath)}, Items: []CollectionItem{}}
	seen := make(map[int]bool)
	for _, part := range set.Parts {
		if part.ID <= 0 || part.Adult || seen[part.ID] {
			continue
		}
		seen[part.ID] = true
		known, inLibrary := library[part.ID]
		item := CollectionItem{Recommendation: recommendation(part, "movie", "collection", "Member of "+set.Name, set.ID),
			InLibrary: inLibrary, MediaID: known.mediaID, Monitored: known.monitored}
		if inLibrary {
			out.Collection.InLibrary++
			if files {
				records, err := s.movies.ListMovieFiles(ctx, known.mediaID)
				if err != nil {
					c.warn("Movie file status unavailable")
				} else {
					hasFile := false
					for _, record := range records {
						if record != nil && record.DeletedAt == nil {
							hasFile = true
							break
						}
					}
					item.HasFile = &hasFile
				}
			}
		} else {
			hasFile := false
			item.HasFile = &hasFile
			if future(part.ReleaseDate, s.now()) {
				out.Collection.Upcoming++
			} else {
				out.Collection.Missing++
			}
		}
		out.Items = append(out.Items, item)
	}
	out.Collection.Total = len(out.Items)
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.ReleaseDate == "" && b.ReleaseDate != "" {
			return false
		}
		if a.ReleaseDate != "" && b.ReleaseDate == "" {
			return true
		}
		if a.ReleaseDate != b.ReleaseDate {
			return a.ReleaseDate < b.ReleaseDate
		}
		return a.TMDBID < b.TMDBID
	})
	return out
}
