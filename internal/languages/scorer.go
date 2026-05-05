package languages

// ScoreRelease computes a LanguageScore for a parsed release against a
// LanguageProfile. Higher scores indicate better matches.
//
// Scoring rules:
//   - Each matched language adds (1000 - priority) so higher-priority
//     languages produce larger scores.
//   - MULTi releases get a 500-point bonus when the profile has 2+
//     allowed languages.
//   - Dual-audio releases get a 250-point bonus when the profile has 2+
//     allowed languages.
//   - MeetsMinimum is true when at least one allowed language from the
//     profile was detected.
func ScoreRelease(profile *LanguageProfile, result ParseResult) LanguageScore {
	if profile == nil {
		return LanguageScore{}
	}

	// Build lookup: language code → LanguagePriority
	allowed := map[string]LanguagePriority{}
	allowedCount := 0
	for _, lp := range profile.Languages {
		if lp.Allowed {
			allowed[lp.Language.Code] = lp
			allowedCount++
		}
	}

	score := 0
	var matched []Language

	for _, lang := range result.Languages {
		if lp, ok := allowed[lang.Code]; ok {
			score += 1000 - lp.Priority
			matched = append(matched, lang)
		}
	}

	// MULTi bonus: release claims multiple languages and the profile wants ≥2.
	if result.IsMulti && allowedCount >= 2 {
		score += 500
	}

	// Dual-audio bonus.
	if result.IsDualAudio && allowedCount >= 2 {
		score += 250
	}

	return LanguageScore{
		Score:            score,
		MatchedLanguages: matched,
		MeetsMinimum:     len(matched) > 0 || result.IsMulti,
	}
}
