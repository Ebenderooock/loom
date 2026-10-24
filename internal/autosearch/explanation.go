package autosearch

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/packs"
	"github.com/ebenderooock/loom/internal/parser"
	"github.com/ebenderooock/loom/internal/qualityprofiles"
	"github.com/ebenderooock/loom/internal/searchdebug"
)

func validatePolicyPreview(profile *qualityprofiles.QualityProfile) error {
	var items []profileItem
	if err := json.Unmarshal([]byte(profile.Items), &items); err != nil || items == nil {
		return fmt.Errorf("%w: items must be a JSON-encoded quality array", ErrInvalidPolicyPreview)
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.ID == "" || seen[item.ID] {
			return fmt.Errorf("%w: quality IDs must be non-empty and unique", ErrInvalidPolicyPreview)
		}
		seen[item.ID] = true
	}
	return nil
}

func addCheck(sr *ScoredRelease, field, expected, actual, unit string, passed bool) {
	sr.Explanation.Checks = append(sr.Explanation.Checks, searchdebug.Evidence{
		Field: field, Expected: expected, Actual: actual, Unit: unit, Passed: passed,
	})
}

func explainIdentity(req SearchRequest, sr *ScoredRelease, idBased bool) {
	p := sr.Parsed
	x := &searchdebug.Explanation{}
	sr.Explanation = x
	x.Numbering = searchdebug.Numbering{
		Season: p.Season, Episode: p.Episode, Episodes: p.Episodes,
		AbsoluteEpisode: p.AbsoluteEpisode, SeasonPack: p.IsSeasonPack, DailyDate: p.DailyDate,
	}
	titles := append([]string{req.Title}, req.AlternateTitles...)
	x.Identity = append(x.Identity, searchdebug.Evidence{
		Field: "normalized title / alternate titles", Expected: strings.Join(titles, " | "),
		Actual: p.Title, Unit: "title", Passed: matchesRequestedTitle(req, parser.CleanSeriesTitle(p.Title)),
	})
	for _, id := range []struct{ field, want, got string }{
		{"IMDb", req.IMDBID, p.ImdbID}, {"TMDB", req.TMDBID, p.TmdbID},
	} {
		if id.want != "" {
			x.Identity = append(x.Identity, searchdebug.Evidence{
				Field: id.field + " embedded ID", Expected: id.want, Actual: id.got,
				Unit: "ID (absence is not corroboration)", Passed: strings.EqualFold(id.want, id.got),
			})
		}
	}
	if idBased {
		x.Identity = append(x.Identity, searchdebug.Evidence{
			Field: "ID-filtered query", Expected: "independent title or embedded-ID corroboration",
			Actual: "indexer filtering alone is not trusted", Unit: "identity",
			Passed: matchesRequestedTitle(req, parser.CleanSeriesTitle(p.Title)) || matchesRequestedID(req, p),
		})
	}
	if req.Year > 0 {
		year := p.Year
		if year == 0 {
			year, _ = strconv.Atoi(yearRe.FindString(p.Name))
		}
		x.Identity = append(x.Identity, searchdebug.Evidence{
			Field: "release year", Expected: fmt.Sprintf("%d +/- 1", req.Year),
			Actual: strconv.Itoa(year), Unit: "year (0 = not parsed)",
			Passed: year > 0 && year >= req.Year-1 && year <= req.Year+1,
		})
	}
	if req.Season > 0 {
		x.Identity = append(x.Identity, searchdebug.Evidence{
			Field: "season", Expected: strconv.Itoa(req.Season), Actual: strconv.Itoa(p.Season),
			Unit: "season (-1 = not parsed)", Passed: p.Season == req.Season,
		})
	}
	if req.Episode > 0 {
		x.Identity = append(x.Identity, searchdebug.Evidence{
			Field: "episode target", Expected: strconv.Itoa(req.Episode),
			Actual: fmt.Sprintf("first=%d; multi=%v; absolute=%d", p.Episode, p.Episodes, p.AbsoluteEpisode),
			Unit:   "episode (absolute numbers are not mapped to seasons)", Passed: p.Episode == req.Episode || containsEpisode(p.Episodes, req.Episode),
		})
	}
	if req.DailyDate != "" {
		x.Identity = append(x.Identity, searchdebug.Evidence{
			Field: "air date", Expected: req.DailyDate, Actual: p.DailyDate,
			Unit: "YYYY-MM-DD", Passed: req.DailyDate == p.DailyDate,
		})
	}
	pack := packs.Detect(p.Name)
	if p.IsSeasonPack || pack.IsPack || (req.Season > 0 && req.Episode == 0) {
		x.Numbering.PackMapping = "Discovery only: parsed numbering is not catalog coverage. No pack correction is applied; ambiguous seasons remain rejected."
	}
}

func containsEpisode(episodes []int, target int) bool {
	for _, ep := range episodes {
		if ep == target {
			return true
		}
	}
	return false
}

func finishExplanation(sr *ScoredRelease) {
	x := sr.Explanation
	x.Category, x.Summary = "accepted", "Accepted by the evaluated identity and policy checks."
	if !sr.Rejected {
		return
	}
	x.Category = "configuration_mismatch"
	switch sr.RejectReason {
	case "wrong_embedded_id", "wrong_year", "wrong_season", "wrong_episode", "wrong_air_date", "title_mismatch":
		x.Category = "incorrect_identity"
	case "identity_unverified", "unverifiable_title", "unverifiable_season", "ambiguous_pack":
		x.Category = "ambiguous_identity"
	case "unverifiable_episode", "missing_air_date", "not_a_season_pack", "season_pack_for_episode_search":
		x.Category = "unsupported_naming"
	case "zero_seeders":
		x.Category = "availability"
	}
	if sr.RejectReason == "unverifiable_season" && sr.Parsed.AbsoluteEpisode > 0 {
		x.Category = "unsupported_naming"
	}
	messages := map[string]string{
		"wrong_embedded_id":              "An embedded external ID conflicts with the requested media. Changing policy cannot override identity.",
		"wrong_year":                     "The release year is outside the requested year +/- 1.",
		"title_mismatch":                 "The normalized title does not match the primary or alternate titles.",
		"identity_unverified":            "The indexer ID filter was not corroborated by the release title or an embedded ID.",
		"unverifiable_title":             "The title could not be verified. Add a known alternate title only after checking the identity.",
		"unverifiable_season":            "No unambiguous season was parsed. Absolute numbering and unscoped packs are not silently mapped.",
		"ambiguous_pack":                 "A multi-season or complete-series pack cannot establish unambiguous requested-season coverage. No catalog correction is applied.",
		"wrong_season":                   "The parsed season differs from the requested season.",
		"wrong_episode":                  "The requested episode is not in the parsed episode numbers.",
		"unverifiable_episode":           "No supported episode numbering was parsed. Absolute numbering is not converted to relative numbering.",
		"not_a_season_pack":              "A season search requires a pack or multi-episode release, not a single episode.",
		"season_pack_for_episode_search": "A season pack is not selected for a single-episode search.",
		"missing_air_date":               "The dated search requires a parsed air date.",
		"wrong_air_date":                 "The parsed air date differs from the requested date.",
		"unknown_quality":                "No quality definition matches the parsed source and resolution.",
		"quality_not_allowed":            "The matched quality is disabled or absent in this profile.",
		"quality_cutoff_met":             "The existing file has already reached the configured quality cutoff.",
		"upgrade_not_allowed":            "The profile disables upgrading an existing file.",
		"not_an_upgrade":                 "This quality is not a strict upgrade (same-tier Proper/Repack is allowed).",
		"existing_quality_unknown":       "The existing file quality is unknown and the profile disables upgrades.",
		"zero_seeders":                   "The torrent reports zero seeders.",
		"below_min_size":                 "The release is below the effective minimum size. Check bytes and runtime below.",
		"above_max_size":                 "The release exceeds the effective maximum size. Check bytes and runtime below.",
		"below_min_format_score":         "Matched custom formats do not reach the profile minimum score.",
	}
	x.Summary = messages[sr.RejectReason]
	if x.Summary == "" {
		x.Summary = "Rejected: " + sr.RejectReason
	}
}

func rankReleases(scored []ScoredRelease) {
	sort.SliceStable(scored, func(i, j int) bool { return betterScoredRelease(scored[i], scored[j]) })
	for i := range scored {
		x := scored[i].Explanation
		x.Rank = i + 1
		x.RankingReason = "Only accepted candidate."
		if len(scored) == 1 {
			continue
		}
		other, relation := i-1, "Ranks below the previous accepted candidate by "
		if i == 0 {
			other, relation = 1, "Ranks above the next accepted candidate by "
		}
		order, key := compareScoredRelease(scored[i], scored[other])
		if order == 0 {
			relation = "Tied with adjacent candidate: "
		}
		x.RankingReason = relation + key + fmt.Sprintf(
			". Actual: tier %d, format %d points, tiebreaker %.3f points; adjacent: tier %d, format %d points, tiebreaker %.3f points. Composite score is display-only.",
			scored[i].QualityTier, scored[i].FormatScore, scored[i].TiebreakerScore,
			scored[other].QualityTier, scored[other].FormatScore, scored[other].TiebreakerScore)
	}
}
