// Package cleanup implements the Downloads Cleanup feature: it scans the
// configured download save folders for "orphans" — files or folders no longer
// tracked by any download client or in-progress import — and surfaces them for
// review. When auto-delete is enabled, orphans are recycled after a configurable
// retention period. The cleanup logic operates strictly on download folders and
// never touches media libraries.
package cleanup

import (
	"path/filepath"
	"strings"
	"time"
)

// OrphanStatus is the lifecycle state of a tracked orphan.
type OrphanStatus string

const (
	// StatusPending — discovered, awaiting review or retention expiry.
	StatusPending OrphanStatus = "pending"
	// StatusIgnored — user chose to keep it; never auto-deleted or re-flagged.
	StatusIgnored OrphanStatus = "ignored"
	// StatusDeleted — recycled/removed from disk.
	StatusDeleted OrphanStatus = "deleted"
	// StatusFailed — a removal attempt errored.
	StatusFailed OrphanStatus = "delete_failed"
)

// Orphan is a single untracked entry inside a download folder.
type Orphan struct {
	ID          string       `json:"id"`
	Path        string       `json:"path"`
	ClientID    string       `json:"client_id"`
	ClientName  string       `json:"client_name,omitempty"`
	Root        string       `json:"root"`
	SizeBytes   int64        `json:"size_bytes"`
	Status      OrphanStatus `json:"status"`
	Error       string       `json:"error,omitempty"`
	FirstSeenAt time.Time    `json:"first_seen_at"`
	LastSeenAt  time.Time    `json:"last_seen_at"`
	DeletedAt   *time.Time   `json:"deleted_at,omitempty"`
}

// Settings is the global cleanup configuration.
type Settings struct {
	AutoDeleteEnabled bool `json:"auto_delete_enabled"`
	RetentionDays     int  `json:"retention_days"`
}

// Root is a download folder to scan, tagged with the owning client for display.
type Root struct {
	Path       string
	ClientID   string
	ClientName string
}

// normPath cleans a path for stable comparison. It does not resolve symlinks;
// symlinked entries are handled (skipped) by the scanner via Lstat.
func normPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// isWithin reports whether child is the same path as, or nested under, parent.
// Both inputs must be cleaned. It uses filepath.Rel rather than naive string
// prefixing so that "/downloads/Movie" does not match "/downloads/Movie 2".
func isWithin(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	if filepath.IsAbs(rel) {
		return false
	}
	return true
}

// isProtected reports whether entry must be protected from deletion given the
// set of tracked paths. An entry is protected when it equals, contains, or is
// contained by any tracked path. The "contains" rule is critical: it prevents
// deleting an intermediate/category directory (e.g. /downloads/tv) that holds a
// tracked download somewhere beneath it.
func isProtected(entry string, tracked []string) bool {
	entry = normPath(entry)
	for _, t := range tracked {
		t = normPath(t)
		if t == "" {
			continue
		}
		if entry == t || isWithin(entry, t) || isWithin(t, entry) {
			return true
		}
	}
	return false
}

// underAnyRoot reports whether path sits inside (or equals) one of the roots.
func underAnyRoot(path string, roots []string) bool {
	path = normPath(path)
	for _, r := range roots {
		if isWithin(normPath(r), path) {
			return true
		}
	}
	return false
}
