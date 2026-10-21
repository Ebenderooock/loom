package airing

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestAvailabilityAtExactInstant(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	instant := "2026-10-22T00:00:00Z" // Oct 21, 8pm in New York; Oct 22, 1pm NZDT.
	for _, tc := range []struct {
		name, date, stamp, now string
		want                   bool
	}{
		{"before", "2026-10-21", instant, "2026-10-21T23:59:59Z", false},
		{"exact", "2026-10-21", instant, instant, true},
		{"after", "2026-10-21", instant, "2026-10-22T00:00:01Z", true},
		{"offset", "2026-10-21", "2026-10-21T20:00:00-04:00", instant, true},
		{"date only", "2026-10-21", "", "2026-10-21T00:00:00Z", true},
		{"future date", "2026-10-22", "", "2026-10-21T23:59:59Z", false},
		{"unknown", "", "", instant, false},
		{"invalid timestamp", "2026-10-21", "invalid", instant, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			nz, err := time.LoadLocation("Pacific/Auckland")
			if err != nil {
				t.Fatal(err)
			}
			for _, localNow := range []time.Time{now, now.In(nz)} {
				if got := HasAired(tc.date, tc.stamp, localNow); got != tc.want {
					t.Errorf("HasAired = %v, want %v", got, tc.want)
				}
			}
			// Exercise the very same SQL predicate used by all stats and
			// scheduler queries, substituting only its clock deterministically.
			query := `SELECT COALESCE(` + strings.ReplaceAll(SQLAired, "'now'", "?") +
				`, 0) FROM (SELECT ? AS air_date, ? AS air_date_utc) e`
			var got bool
			if err := db.QueryRow(query, tc.now, tc.now, tc.date, tc.stamp).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("SQLAired = %v, want %v", got, tc.want)
			}
		})
	}
}
