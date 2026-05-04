// Package autosearch provides automatic media discovery and search orchestration
// for Loom's RSS monitoring system.
//
// # Overview
//
// AutoSearch monitors RSS feeds for media releases and automatically matches them
// against the library's wanted movies. When a match is found, the orchestrator
// triggers a search in the media discovery subsystem (Phase 5e-c).
//
// # Components
//
// [Matcher] uses fuzzy title matching (Levenshtein distance) with optional year
// verification to identify potential matches between RSS items and library wants.
// Matches are confidence-scored (0.0-1.0) with a 70% threshold for triggering searches.
//
// [Orchestrator] coordinates RSS feed monitoring with search operations:
//   - ScanAndMatch: Process recent RSS items against library wants
//   - TriggerSearches: Create searchable events for matched items
//   - GetSearchHistory: Retrieve past search attempts for a movie
//   - UpdateSearchStatus: Track search progress (pending/found/error)
//   - GetStats: Gather orchestration metrics
//
// # Matching Algorithm
//
// Titles are normalized before comparison:
//   - Remove leading articles (the, a, an)
//   - Replace punctuation with spaces (., -, _)
//   - Case-insensitive comparison
//
// Confidence score is computed as:
//   1. Levenshtein distance between normalized titles → similarity %
//   2. Year bonus (+15%) if item year matches want year exactly
//   3. Year bonus (+5%) if within 1 year
//   4. Year penalty (-20%) if year mismatch > 1 year
//   5. Final confidence = min(1.0, similarity + bonuses - penalties)
//
// Example matches:
//   - "Inception.2010.1080p" vs "Inception (2010)" → 1.0 confidence (exact)
//   - "Matrix.1999.1080p" vs "The Matrix (1999)" → 0.95+ confidence (minor difference)
//   - "Incpetion.2010.1080p" vs "Inception (2010)" → 0.80+ confidence (typo tolerance)
//   - "Inception.2009.1080p" vs "Inception (2010)" → 0.70+ confidence (year mismatch)
//
// # Integration Points
//
// AutoSearch depends on:
//   - [rss.SyncManager]: Provides recent RSS items from all sources
//   - [movies.Library]: Provides monitored movies for matching (future integration)
//   - Database: Stores search_history table for auditing
//
// AutoSearch feeds into:
//   - Phase 5e-c (Custom Sources): User-defined RSS/scraper sources
//   - Phase 5e-d (Automatic Downloads): Integration with download clients
//   - Event Bus: Potential real-time UI updates via search events
//
// # Schema
//
// search_history table:
//   - id: Composite key (rss_item_id + "_" + movie_id)
//   - rss_item_id: Foreign key to rss_items.id
//   - movie_id: Foreign key to movies.id
//   - status: pending, searching, found, not_found, error
//   - error_msg: Reason for error status (if applicable)
//   - created_at: Timestamp of match trigger
//
// # Limitations & Future Work
//
//   - Year extraction only handles 1900-2100; won't match rare old films
//   - No regex patterns for sequel/spin-off detection (may cause false positives)
//   - Search triggering is async; no guarantee of execution or result feedback
//   - No deduplication of search attempts (same match triggers multiple times = multiple searches)
//   - No support for quality/source filtering before search trigger
//
// # Examples
//
// Basic matching:
//
//	matcher := autosearch.NewMatcher([]autosearch.WantedMovie{
//		{ID: "m1", Title: "Inception", Year: 2010},
//	})
//	item := &rss.Item{GUID: "i1", SourceID: "s1", Title: "Inception.2010.1080p.BluRay"}
//	matches := matcher.FindMatches(item)
//	for _, match := range matches {
//		fmt.Printf("Match: %s (%.1f%% confidence)\n", match.Title, match.Confidence*100)
//	}
//
// Orchestration workflow:
//
//	orch := autosearch.NewOrchestrator(db, syncManager, matcher)
//	matches, _ := orch.ScanAndMatch(ctx)
//	count, _ := orch.TriggerSearches(ctx, matches)
//	stats, _ := orch.GetStats(ctx)
//	fmt.Printf("Triggered %d searches; %d pending, %d completed\n",
//		count, stats.PendingSearches, stats.CompletedSearhes)
package autosearch
