package bots

import (
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/requests"
)

// encodeRequest builds the callback payload for a search-result request button.
func encodeRequest(mt requests.MediaType, tmdbID string) string {
	return "req|" + string(mt) + "|" + tmdbID
}

// decodeCallback splits a callback payload into up to three fields.
func decodeCallback(data string) (action, a1, a2 string) {
	parts := strings.SplitN(data, "|", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], ""
	case 1:
		return parts[0], "", ""
	default:
		return "", "", ""
	}
}

// interleave merges movie, series, and artist results, alternating types, up to
// max.
func interleave(movies, series, artists []MediaResult, max int) []MediaResult {
	out := make([]MediaResult, 0, max)
	i, j, k := 0, 0, 0
	for len(out) < max && (i < len(movies) || j < len(series) || k < len(artists)) {
		if i < len(movies) {
			out = append(out, movies[i])
			i++
		}
		if len(out) >= max {
			break
		}
		if j < len(series) {
			out = append(out, series[j])
			j++
		}
		if len(out) >= max {
			break
		}
		if k < len(artists) {
			out = append(out, artists[k])
			k++
		}
	}
	return out
}

// resultLabel renders a concise button label for a result.
func resultLabel(r MediaResult) string {
	icon := "🎬"
	switch r.MediaType {
	case requests.MediaSeries:
		icon = "📺"
	case requests.MediaArtist:
		icon = "🎵"
	}
	label := icon + " " + r.Title
	if r.Year > 0 {
		label += " (" + strconv.Itoa(r.Year) + ")"
	}
	return truncate(label, 90)
}

// statusEmoji maps a request status to a glyph for status listings.
func statusEmoji(st requests.Status) string {
	switch st {
	case requests.StatusPending:
		return "⏳"
	case requests.StatusApproving, requests.StatusApproved:
		return "✅"
	case requests.StatusAvailable:
		return "🟢"
	case requests.StatusRejected:
		return "❌"
	case requests.StatusFailed:
		return "⚠️"
	default:
		return "•"
	}
}

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
