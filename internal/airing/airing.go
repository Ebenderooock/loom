// Package airing keeps actual airing instants distinct from date-only metadata.
package airing

import "time"

// SQLAired is an episodes-table predicate. A known instant always takes
// precedence, even on the original air_date. Date-only fallback remains UTC.
const SQLAired = `(CASE WHEN e.air_date_utc != '' THEN julianday(e.air_date_utc) <= julianday('now')
 ELSE e.air_date != '' AND e.air_date <= date('now') END)`

// HasAired uses an instant when known; it never invents a time for a date.
func HasAired(date, instant string, now time.Time) bool {
	if instant != "" {
		t, err := time.Parse(time.RFC3339, instant)
		return err == nil && !now.Before(t)
	}
	return date != "" && date <= now.UTC().Format("2006-01-02")
}
