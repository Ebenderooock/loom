package migrate

import (
	"fmt"
	"strings"
	"time"
)

// ImportResult tracks statistics from a single import run.
type ImportResult struct {
	Source        string
	MoviesAdded   int
	SeriesAdded   int
	EpisodesAdded int
	IndexersAdded int
	ProfilesAdded int
	LibrariesAdded int
	Skipped       int
	Errors        []string
	Duration      time.Duration
}

// Summary returns a human-readable summary of the import result.
func (r *ImportResult) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Import from %s completed in %s\n", r.Source, r.Duration.Round(time.Millisecond))
	if r.MoviesAdded > 0 {
		fmt.Fprintf(&b, "  Movies added:    %d\n", r.MoviesAdded)
	}
	if r.SeriesAdded > 0 {
		fmt.Fprintf(&b, "  Series added:    %d\n", r.SeriesAdded)
	}
	if r.EpisodesAdded > 0 {
		fmt.Fprintf(&b, "  Episodes added:  %d\n", r.EpisodesAdded)
	}
	if r.IndexersAdded > 0 {
		fmt.Fprintf(&b, "  Indexers added:  %d\n", r.IndexersAdded)
	}
	if r.ProfilesAdded > 0 {
		fmt.Fprintf(&b, "  Profiles added:  %d\n", r.ProfilesAdded)
	}
	if r.LibrariesAdded > 0 {
		fmt.Fprintf(&b, "  Libraries added: %d\n", r.LibrariesAdded)
	}
	if r.Skipped > 0 {
		fmt.Fprintf(&b, "  Skipped (dupes): %d\n", r.Skipped)
	}
	if len(r.Errors) > 0 {
		fmt.Fprintf(&b, "  Errors: %d\n", len(r.Errors))
		for _, e := range r.Errors {
			fmt.Fprintf(&b, "    - %s\n", e)
		}
	}
	return b.String()
}
