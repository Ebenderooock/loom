package episodeorder

import "context"

// ResolvedEpisode holds the result of mapping one numbering scheme to another.
type ResolvedEpisode struct {
	SeasonFrom   *int         `json:"seasonFrom,omitempty"`
	EpisodeFrom  *int         `json:"episodeFrom,omitempty"`
	AbsoluteFrom *int         `json:"absoluteFrom,omitempty"`
	SeasonTo     *int         `json:"seasonTo,omitempty"`
	EpisodeTo    *int         `json:"episodeTo,omitempty"`
	AbsoluteTo   *int         `json:"absoluteTo,omitempty"`
	OrderingType OrderingType `json:"orderingType"`
	Found        bool         `json:"found"`
}

// Resolve looks up a mapping for a given series, ordering type, and
// source episode numbers. It returns the mapped episode in the target
// scheme, or Found=false if no mapping exists.
func Resolve(ctx context.Context, store *Store, seriesID string, orderingType OrderingType, season *int, episode *int, absolute *int) (*ResolvedEpisode, error) {
	mappings, err := store.ListMappings(ctx, seriesID, string(orderingType))
	if err != nil {
		return nil, err
	}

	for _, m := range mappings {
		if matchesSource(m, season, episode, absolute) {
			return &ResolvedEpisode{
				SeasonFrom:   m.SeasonFrom,
				EpisodeFrom:  m.EpisodeFrom,
				AbsoluteFrom: m.AbsoluteFrom,
				SeasonTo:     m.SeasonTo,
				EpisodeTo:    m.EpisodeTo,
				AbsoluteTo:   m.AbsoluteTo,
				OrderingType: m.OrderingType,
				Found:        true,
			}, nil
		}
	}

	return &ResolvedEpisode{OrderingType: orderingType, Found: false}, nil
}

func matchesSource(m EpisodeMapping, season *int, episode *int, absolute *int) bool {
	if season != nil && m.SeasonFrom != nil && *season == *m.SeasonFrom &&
		episode != nil && m.EpisodeFrom != nil && *episode == *m.EpisodeFrom {
		return true
	}
	if absolute != nil && m.AbsoluteFrom != nil && *absolute == *m.AbsoluteFrom {
		return true
	}
	return false
}
