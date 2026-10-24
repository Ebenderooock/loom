package searchdebug

// Explanation records evidence from the decision path, not a second evaluation.
// Missing checks mean evaluation stopped before that stage.
type Explanation struct {
	Category      string               `json:"category"`
	Summary       string               `json:"summary"`
	Identity      []Evidence           `json:"identity,omitempty"`
	Numbering     Numbering            `json:"numbering"`
	Checks        []Evidence           `json:"checks,omitempty"`
	Contributions []FormatContribution `json:"contributions,omitempty"`
	Rank          int                  `json:"rank,omitempty"`
	RankingReason string               `json:"ranking_reason,omitempty"`
}

// Evidence labels comparisons with their domain units and expected/actual values.
type Evidence struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Unit     string `json:"unit"`
	Passed   bool   `json:"passed"`
}

// Numbering preserves the parser's interpretation, including unknown (-1) values.
type Numbering struct {
	Season          int    `json:"season"`
	Episode         int    `json:"episode"`
	Episodes        []int  `json:"episodes,omitempty"`
	AbsoluteEpisode int    `json:"absolute_episode"`
	SeasonPack      bool   `json:"season_pack"`
	DailyDate       string `json:"daily_date,omitempty"`
	PackMapping     string `json:"pack_mapping,omitempty"`
}

// FormatContribution is the profile-specific score actually added by the engine.
type FormatContribution struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Score int    `json:"score"`
}
