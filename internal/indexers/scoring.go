package indexers

import (
	"math"
	"sort"
	"strings"
	"time"
)

// ScoreResults assigns a ranking score to each result and sorts
// results by score descending. The scoring considers:
//   - Quality tier (2160p > 1080p > 720p > 480p > SD)
//   - Seeder count (logarithmic, torrent only)
//   - Age penalty (newer = better, exponential decay)
//   - Size preference (prefer mid-range, penalize very small/huge)
func ScoreResults(results []Result) {
	for i := range results {
		results[i].Score = computeScore(&results[i])
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}

func computeScore(r *Result) float64 {
	var score float64

	// 1. Quality tier (0-40 points)
	score += qualityScore(r.Quality, r.Title)

	// 2. Seeders (0-30 points, log scale)
	if r.Seeders != nil && *r.Seeders > 0 {
		score += math.Min(30, math.Log2(float64(*r.Seeders+1))*5)
	}

	// 3. Age (0-20 points, newer is better)
	if !r.PubDate.IsZero() {
		age := time.Since(r.PubDate)
		days := age.Hours() / 24
		switch {
		case days < 1:
			score += 20
		case days < 7:
			score += 18
		case days < 30:
			score += 15
		case days < 90:
			score += 10
		case days < 365:
			score += 5
		}
	}

	// 4. Size (0-10 points, prefer reasonable sizes)
	if r.Size > 0 {
		gb := float64(r.Size) / (1024 * 1024 * 1024)
		switch {
		case gb >= 1 && gb <= 15:
			score += 10 // sweet spot
		case gb > 0.3 && gb < 50:
			score += 5
		}
	}

	// 5. Freeleech bonus (0-15 points)
	if r.Freeleech {
		score += 15
	}

	return score
}

func qualityScore(quality, title string) float64 {
	q := strings.ToLower(quality)
	if q == "" {
		q = strings.ToLower(title)
	}

	switch {
	case strings.Contains(q, "2160p") || strings.Contains(q, "4k") || strings.Contains(q, "uhd"):
		return 40
	case strings.Contains(q, "1080p"):
		return 35
	case strings.Contains(q, "720p"):
		return 25
	case strings.Contains(q, "480p"):
		return 15
	case strings.Contains(q, "dvd") || strings.Contains(q, "sd"):
		return 10
	default:
		return 20 // unknown quality gets middle score
	}
}
