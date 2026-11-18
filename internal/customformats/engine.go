package customformats

import (
	"regexp"
	"strconv"
	"strings"
)

// Engine evaluates releases against a set of loaded custom formats.
type Engine struct {
	formats []CustomFormat
}

// NewEngine creates an Engine pre-loaded with the given formats.
func NewEngine(formats []CustomFormat) *Engine {
	return &Engine{formats: formats}
}

// SetFormats replaces the loaded formats.
func (e *Engine) SetFormats(formats []CustomFormat) {
	e.formats = formats
}

// ScoreRelease evaluates all loaded custom formats against the release
// and returns every match with its score. Total score is the sum of all
// returned FormatMatch.Score values.
func (e *Engine) ScoreRelease(release ReleaseInfo) []FormatMatch {
	return e.score(release, nil)
}

func (e *Engine) score(release ReleaseInfo, file *FileMetadata) []FormatMatch {
	var matches []FormatMatch
	for _, cf := range e.formats {
		if matchesFormat(cf, release, file) {
			matches = append(matches, FormatMatch{
				CustomFormatID:   cf.ID,
				CustomFormatName: cf.Name,
			})
		}
	}
	return matches
}

// TotalScore is a convenience that sums all match scores.
func TotalScore(matches []FormatMatch) int {
	total := 0
	for _, m := range matches {
		total += m.Score
	}
	return total
}

// ApplyScores assigns the selected profile's points to each matched format.
// Unassigned formats contribute zero; negative scores are penalties.
func ApplyScores(matches []FormatMatch, scores map[string]int) int {
	for i := range matches {
		matches[i].Score = scores[matches[i].CustomFormatID]
	}
	return TotalScore(matches)
}

// matchesFormat uses the same semantics as Radarr/Sonarr:
//
//  1. Specs are GROUPED by Implementation type.
//  2. Within a group: OR logic — at least one spec must match.
//  3. Required=true acts as within-group AND — if a required spec
//     in the group fails, the entire group fails regardless of others.
//  4. Across groups: AND logic — every group must pass.
//
// This means two ReleaseTitleSpecification specs = "title matches A OR B".
// To require both: set each Required=true.
func matchesFormat(cf CustomFormat, ri ReleaseInfo, file *FileMetadata) bool {
	if len(cf.Specifications) == 0 {
		return false
	}

	// Group specs by Implementation type (same as *arr grouping by C# type).
	groups := make(map[string][]Specification)
	for _, spec := range cf.Specifications {
		groups[spec.Implementation] = append(groups[spec.Implementation], spec)
	}

	// Every group must pass.
	for _, specs := range groups {
		if !groupMatches(specs, ri, file) {
			return false
		}
	}
	return true
}

// groupMatches returns true when:
//   - No required spec in the group fails, AND
//   - At least one spec in the group matches (not all false).
func groupMatches(specs []Specification, ri ReleaseInfo, file *FileMetadata) bool {
	allFalse := true
	for _, spec := range specs {
		result := evalSpec(spec, ri, file)
		if spec.Required && !result {
			return false // required spec failed → group fails
		}
		if result {
			allFalse = false
		}
	}
	return !allFalse // at least one must match
}

// evalSpec evaluates a single specification, honouring the Negate flag.
func evalSpec(spec Specification, ri ReleaseInfo, file *FileMetadata) bool {
	if spec.Implementation == ImplFileInfo {
		if file == nil {
			return true // Release-only evaluation has no file evidence yet.
		}
		result := matchFileInfo(spec.Fields, *file)
		if spec.Negate {
			return !result
		}
		return result
	}
	raw := specMatch(spec, ri)
	if spec.Negate {
		return !raw
	}
	return raw
}

func specMatch(spec Specification, ri ReleaseInfo) bool {
	switch spec.Implementation {
	case ImplReleaseTitle:
		return matchReleaseTitle(spec.Fields, ri)
	case ImplQuality:
		return matchStringField(spec.Fields, "value", ri.Quality)
	case ImplSource:
		return matchNormalisedField(spec.Fields, ri.Source, normaliseSource)
	case ImplResolution:
		return matchStringField(spec.Fields, "value", ri.Resolution)
	case ImplCodec:
		return matchNormalisedField(spec.Fields, ri.Codec, normaliseCodec)
	case ImplAudio:
		return matchNormalisedField(spec.Fields, ri.Audio, normaliseAudio)
	case ImplReleaseGroup:
		return matchStringField(spec.Fields, "value", ri.Group)
	case ImplLanguage:
		return matchLanguage(spec.Fields, ri)
	case ImplSize:
		return matchSize(spec.Fields, ri)
	case ImplIndexerFlag:
		return matchIndexerFlag(spec.Fields, ri)
	default:
		return false
	}
}

func matchReleaseTitle(fields map[string]any, ri ReleaseInfo) bool {
	pattern, _ := fields["value"].(string)
	if pattern == "" {
		return false
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return false
	}
	return re.MatchString(ri.Title)
}

func matchStringField(fields map[string]any, key, actual string) bool {
	want, _ := fields[key].(string)
	if want == "" {
		return false
	}
	return strings.EqualFold(want, actual)
}

func matchNormalisedField(fields map[string]any, actual string, normalise func(string) string) bool {
	want, _ := fields["value"].(string)
	return want != "" && strings.EqualFold(normalise(want), normalise(actual))
}

func matchLanguage(fields map[string]any, ri ReleaseInfo) bool {
	want, _ := fields["value"].(string)
	if want == "" {
		return false
	}
	for _, lang := range ri.Languages {
		if strings.EqualFold(lang, want) {
			return true
		}
	}
	return false
}

func matchSize(fields map[string]any, ri ReleaseInfo) bool {
	minF, _ := toFloat(fields["min"])
	maxF, _ := toFloat(fields["max"])
	sizeGB := float64(ri.Size) / (1024 * 1024 * 1024)
	if minF > 0 && sizeGB < minF {
		return false
	}
	if maxF > 0 && sizeGB > maxF {
		return false
	}
	return true
}

func matchIndexerFlag(fields map[string]any, ri ReleaseInfo) bool {
	want, _ := fields["value"].(string)
	if want == "" {
		return false
	}
	for _, f := range ri.IndexerFlags {
		if strings.EqualFold(f, want) {
			return true
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	default:
		return 0, false
	}
}
