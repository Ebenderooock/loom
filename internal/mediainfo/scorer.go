package mediainfo

import "strings"

// ScoreRelease scores a parsed MediaInfo against user preferences.
// Higher is better. Negative scores indicate hard fails (e.g. missing
// required subtitles).
func ScoreRelease(info MediaInfo, prefs MediaPreferences) int {
	score := 0

	// Audio codec preference scoring: first in list = highest score.
	if info.AudioCodec != "" && len(prefs.PreferredAudioCodecs) > 0 {
		for i, codec := range prefs.PreferredAudioCodecs {
			if strings.EqualFold(info.AudioCodec, codec) {
				score += (len(prefs.PreferredAudioCodecs) - i) * 10
				break
			}
		}
	}

	// Atmos bonus
	if prefs.PreferAtmos && containsCI([]string{info.AudioCodec}, "Atmos") {
		score += 50
	}
	if prefs.PreferAtmos && strings.EqualFold(info.AudioCodec, "TrueHD Atmos") {
		score += 50
	}

	// HDR preference
	if prefs.PreferHDR && info.HDR != "" && info.HDR != "SDR" {
		score += 30
		if info.HDR == "Dolby Vision" {
			score += 20
		}
	}

	// Subtitle language matching
	if len(prefs.PreferredSubLanguages) > 0 {
		matched := 0
		for _, wanted := range prefs.PreferredSubLanguages {
			if containsCI(info.SubLanguages, wanted) {
				matched++
			}
		}
		score += matched * 15
	}

	// Require subtitles penalty
	if prefs.RequireSubtitles && info.SubType == "none" && len(info.SubLanguages) == 0 {
		score -= 100
	}

	return score
}
