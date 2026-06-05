package packs

// WantedEpisodes describes the episodes a user wants in a given season.
type WantedEpisodes struct {
	Season        int
	EpisodeNums   []int // episode numbers the user still needs
	TotalInSeason int   // total episodes in the season
}

// DecidePack evaluates whether grabbing a pack is worthwhile compared
// to grabbing individual episodes. The threshold is 50 %: if more than
// half the episodes in the pack are wanted, grab the pack.
//
// packSize is in bytes; individualSize is the estimated per-episode size.
func DecidePack(pack DetectedPack, wanted []WantedEpisodes, packSize int64, individualSize int64) PackDecision {
	if !pack.IsPack {
		return PackDecision{Reason: "not a pack"}
	}

	totalInPack := 0
	wantedInPack := 0

	for _, w := range wanted {
		if !seasonInPack(pack, w.Season) {
			continue
		}
		if pack.Type == PackTypeEpisodeRange {
			for _, ep := range w.EpisodeNums {
				if ep >= pack.EpisodeStart && ep <= pack.EpisodeEnd {
					wantedInPack++
				}
			}
			totalInPack += pack.EpisodeEnd - pack.EpisodeStart + 1
		} else {
			wantedInPack += len(w.EpisodeNums)
			totalInPack += w.TotalInSeason
		}
	}

	if totalInPack == 0 {
		return PackDecision{Reason: "no episodes in pack range"}
	}

	pct := float64(wantedInPack) / float64(totalInPack)

	var packCostPerEp, individualCost float64
	if totalInPack > 0 && packSize > 0 {
		packCostPerEp = float64(packSize) / float64(totalInPack)
	}
	if individualSize > 0 {
		individualCost = float64(individualSize)
	}

	d := PackDecision{
		WantedInPack:     wantedInPack,
		TotalInPack:      totalInPack,
		WantedPercentage: pct * 100,
		PackCostPerEp:    packCostPerEp,
		IndividualCost:   individualCost,
	}

	if pct >= 0.5 {
		d.ShouldGrabPack = true
		d.Reason = "≥50% of episodes wanted"
	} else {
		d.Reason = "<50% of episodes wanted — prefer individual"
	}
	return d
}

// EpisodesFromPack returns the list of episode numbers covered by a pack
// in a given season. If the pack is an episode range, it returns that
// range; otherwise it returns all episodes 1..totalInSeason.
func EpisodesFromPack(pack DetectedPack, season int, totalInSeason int) []int {
	if !seasonInPack(pack, season) {
		return nil
	}
	if pack.Type == PackTypeEpisodeRange && season == pack.SeasonStart {
		eps := make([]int, 0, pack.EpisodeEnd-pack.EpisodeStart+1)
		for e := pack.EpisodeStart; e <= pack.EpisodeEnd; e++ {
			eps = append(eps, e)
		}
		return eps
	}
	eps := make([]int, 0, totalInSeason)
	for e := 1; e <= totalInSeason; e++ {
		eps = append(eps, e)
	}
	return eps
}

func seasonInPack(pack DetectedPack, season int) bool {
	if pack.Type == PackTypeCompleteSeries {
		return true
	}
	return season >= pack.SeasonStart && season <= pack.SeasonEnd
}
