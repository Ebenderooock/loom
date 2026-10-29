package libraries

import "time"

// HealthReport is a bounded observation, never authorization to modify files.
type HealthReport struct {
	LibraryID        string          `json:"library_id"`
	Root             string          `json:"root"`
	Status           string          `json:"status"`
	CheckedAt        time.Time       `json:"checked_at"`
	ScannedEntries   int             `json:"scanned_entries"`
	InspectedRecords int             `json:"inspected_records"`
	EntryLimit       int             `json:"entry_limit"`
	Warnings         []string        `json:"warnings"`
	Findings         []HealthFinding `json:"findings"`
}

// HealthFinding retains the evidence and uncertainty behind each suggestion.
type HealthFinding struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Path         string        `json:"path"`
	RelatedPaths []string      `json:"related_paths"`
	Evidence     []string      `json:"evidence"`
	Confidence   string        `json:"confidence"`
	Dismissed    bool          `json:"dismissed"`
	Preview      HealthPreview `json:"preview"`
}

// HealthPreview has no execution counterpart in this release.
type HealthPreview struct {
	Action     string `json:"action"`
	Outcome    string `json:"outcome"`
	Executable bool   `json:"executable"`
}
