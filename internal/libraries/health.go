package libraries

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	healthEntryLimit   = 10000
	healthFindingLimit = 1000
	healthTimeout      = 30 * time.Second
)

var errHealthLimit = errors.New("health traversal limit reached")

var sampleToken = regexp.MustCompile(`(?i)(^|[ ._/-])sample([ ._/-]|$)`)
var versionToken = regexp.MustCompile(`(?i)(2160p|1080p|720p|480p|director.?s.?cut|extended|theatrical|remaster|unrated|edition|dubbed|multi.?audio|3d)`)

type healthObservation struct {
	info os.FileInfo
}

type healthCheck struct {
	report  *HealthReport
	files   map[string]healthObservation
	skipped []string
	limit   int
}

func newHealthCheck(lib *Library, limit int) *healthCheck {
	return &healthCheck{
		report: &HealthReport{
			LibraryID: lib.ID, Root: filepath.Clean(lib.Path), Status: "complete",
			CheckedAt: time.Now().UTC(), EntryLimit: limit,
			Warnings: []string{"Heuristic snapshot only: no content hashes or duration probing. Imports and mount changes can race observations; recheck before any manual action."},
			Findings: []HealthFinding{},
		},
		files: make(map[string]healthObservation), limit: limit,
	}
}

func (c *healthCheck) warn(message string) {
	if c.report.Status == "complete" {
		c.report.Status = "incomplete"
	}
	if len(c.report.Warnings) < 100 {
		c.report.Warnings = append(c.report.Warnings, message)
	}
}

func (c *healthCheck) add(kind, path, confidence string, related, evidence []string, action, outcome string) {
	if len(c.report.Findings) >= healthFindingLimit {
		c.warn("Finding limit reached; report is truncated.")
		return
	}
	sorted := append([]string{}, related...)
	sort.Strings(sorted)
	key := kind + "\x00" + path + "\x00" + strings.Join(sorted, "\x00") + "\x00" + strings.Join(evidence, "\x00")
	if observed, exists := c.files[path]; exists {
		key += fmt.Sprintf("\x00%d:%d", observed.info.Size(), observed.info.ModTime().UnixNano())
	}
	sum := sha256.Sum256([]byte(key))
	c.report.Findings = append(c.report.Findings, HealthFinding{
		ID: fmt.Sprintf("%x", sum[:16]), Kind: kind, Path: path,
		RelatedPaths: sorted, Evidence: evidence, Confidence: confidence,
		Preview: HealthPreview{Action: action, Outcome: outcome},
	})
}

// walk reads directory entries in small batches, not WalkDir's unbounded sorted
// ReadDir. It never opens media content, follows symlink entries, or mutates disk.
func (c *healthCheck) walk(ctx context.Context, path string, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 64 {
		c.skipped = append(c.skipped, path)
		c.warn("Directory depth limit reached: " + path)
		return nil
	}
	dir, err := os.Open(path) //nolint:gosec // Configured library root or enumerated descendant directory, not an API-supplied path.
	if err != nil {
		c.skipped = append(c.skipped, path)
		c.warn("Directory unavailable: " + path + ": " + err.Error())
		return nil
	}
	defer func() { _ = dir.Close() }()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if c.report.ScannedEntries == c.limit {
				return errHealthLimit
			}
			c.report.ScannedEntries++
			full := filepath.Join(path, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 {
				c.skipped = append(c.skipped, full)
				c.add("protected", full, "high", nil, []string{"Symbolic link; target deliberately not followed."},
					"retain", "No changes. Symlink ownership and target require independent review.")
				continue
			}
			if entry.IsDir() {
				if err := c.walk(ctx, full, depth+1); err != nil {
					return err
				}
				continue
			}
			info, err := entry.Info()
			if err != nil {
				c.skipped = append(c.skipped, full)
				c.warn("Cannot inspect entry: " + full + ": " + err.Error())
				continue
			}
			if info.Mode().IsRegular() {
				c.files[full] = healthObservation{info: info}
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			c.skipped = append(c.skipped, path)
			c.warn("Incomplete directory read: " + path + ": " + readErr.Error())
			return nil
		}
	}
}

func (c *healthCheck) protected(path string) string {
	rel, _ := filepath.Rel(c.report.Root, path)
	for _, part := range strings.Split(strings.ToLower(rel), string(filepath.Separator)) {
		switch part {
		case "extras", "extra", "featurettes", "behind the scenes", "deleted scenes", "trailers", "interviews", "subs", "subtitles":
			return "Extra, trailer or subtitle directory; not an unwanted duplicate."
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".srt", ".ass", ".ssa", ".sub", ".idx", ".vtt":
		return "Subtitle sidecar; not a duplicate media file."
	}
	if file, ok := c.files[path]; ok && healthLinkCount(file.info) > 1 {
		return "Hardlink detected; link count exceeds one. No independent reclaimable space is assumed."
	}
	return ""
}

func healthMedia(path string) bool {
	if mediaExtensions[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".flac", ".mp3", ".m4a", ".aac", ".ogg", ".opus", ".wav", ".aiff", ".alac":
		return true
	}
	return false
}

func (c *healthCheck) classify(ctx context.Context, records []healthRecord, recordsTruncated bool) {
	byPath := make(map[string][]healthRecord)
	groups := make(map[string]map[string]string)
	for _, record := range records {
		if ctx.Err() != nil {
			return
		}
		path := filepath.Clean(record.path)
		if !healthWithin(c.report.Root, path) {
			c.add("orphan_record", record.path, "medium", nil,
				[]string{"Library record path is relative or outside its configured root; filesystem not inspected."},
				"review_record", "No changes. Verify library assignment and record ownership.")
			continue
		}
		byPath[path] = append(byPath[path], record)
		if record.orphan {
			c.add("orphan_record", path, "medium", nil,
				[]string{"File record references an absent or deleted media owner."},
				"review_record", "No changes. Preview: verify ownership before considering record reconciliation.")
		}
		if record.group != "" && !record.orphan {
			if groups[record.group] == nil {
				groups[record.group] = make(map[string]string)
			}
			groups[record.group][path] = record.version
		}
	}
	paths := make([]string, 0, len(c.files))
	for path := range c.files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if ctx.Err() != nil {
			return
		}
		if reason := c.protected(path); reason != "" {
			c.add("protected", path, "high", nil, []string{reason}, "retain", "No changes. Excluded from sample and unwanted-duplicate suggestions.")
			continue
		}
		if !healthMedia(path) {
			continue
		}
		rel, _ := filepath.Rel(c.report.Root, path)
		if sampleToken.MatchString(filepath.ToSlash(rel)) {
			evidence := []string{
				"Sample token in path; filename evidence only, not proof of duration or content.",
				fmt.Sprintf("Observed size: %d bytes. Size alone is never used to choose the feature.", c.files[path].info.Size()),
			}
			related := []string{}
			for _, record := range byPath[path] {
				for peer := range groups[record.group] {
					if peer != path && len(related) < 20 {
						related = append(related, peer)
					}
				}
			}
			c.add("sample", path, "low", related, evidence, "review_selection",
				"No changes. Preview: compare playback/duration with related feature files; do not replace or delete automatically.")
		}
		if len(byPath[path]) == 0 && !recordsTruncated {
			c.add("untracked_file", path, "low", nil,
				[]string{"No matching library, movie, episode or track file record in this bounded snapshot. An import may be in progress."},
				"review_import", "No changes. Preview: verify whether this belongs in the library; never assume an untracked file is disposable.")
		}
	}
	recordPaths := make([]string, 0, len(byPath))
	for path := range byPath {
		recordPaths = append(recordPaths, path)
	}
	sort.Strings(recordPaths)
	for _, path := range recordPaths {
		if ctx.Err() != nil {
			return
		}
		if _, exists := c.files[path]; exists {
			continue
		}
		evidence := []string{"Tracked path not observed in this traversal; this is not confirmed deletion.", "Mount availability (including nested mounts) cannot be proved from directory existence."}
		for _, skipped := range c.skipped {
			if path == skipped || healthWithin(skipped, path) {
				evidence = append(evidence, "Path is in a deliberately skipped or inaccessible subtree.")
				break
			}
		}
		if c.report.Status != "complete" {
			evidence = append(evidence, "Traversal is incomplete; absence is inconclusive.")
		}
		c.add("missing_file", path, "low", nil, evidence, "recheck_availability",
			"No changes. Preview: restore/check mounts and finish a fresh health check before manual reconciliation; monitoring state is unchanged.")
	}
	groupIDs := make([]string, 0, len(groups))
	for id := range groups {
		groupIDs = append(groupIDs, id)
	}
	sort.Strings(groupIDs)
	for _, id := range groupIDs {
		if ctx.Err() != nil {
			return
		}
		c.classifyGroup(ctx, id, groups[id])
	}
}

func (c *healthCheck) classifyGroup(ctx context.Context, id string, group map[string]string) {
	paths := make([]string, 0, len(group))
	for path := range group {
		if _, exists := c.files[path]; exists {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	if len(paths) < 2 {
		return
	}
	first := paths[0]
	for _, path := range paths[1:] {
		if ctx.Err() != nil {
			return
		}
		reason := c.protected(path)
		if reason == "" {
			reason = c.protected(first)
		}
		switch {
		case os.SameFile(c.files[first].info, c.files[path].info):
			reason = "Paths share file identity (hardlinks); no duplicate disk allocation is assumed."
		case group[path] != group[first]:
			reason = "Different quality/format metadata; independent versions are preserved."
		case versionToken.MatchString(filepath.Base(first)) || versionToken.MatchString(filepath.Base(path)):
			reason = "Version/edition markers in filenames; intentional versions are preserved."
		}
		if reason != "" {
			c.add("protected", path, "high", []string{first}, []string{reason, "Shared media owner: " + id},
				"retain_versions", "No changes. Both paths are retained; no preferred version or reclaimable space is claimed.")
			continue
		}
		c.add("duplicate", path, "low", []string{first},
			[]string{"Multiple distinct files share media owner: " + id, "Content equality and version intent are unknown. Metadata and filenames alone do not prove redundancy."},
			"compare_versions", "No changes. Preview: compare content, language, edition and quality manually; neither copy is selected for deletion.")
	}
}

func (s *Store) checkHealth(ctx context.Context, lib *Library, limit int) (*HealthReport, error) {
	c := newHealthCheck(lib, limit)
	if !filepath.IsAbs(lib.Path) {
		c.report.Status = "unavailable"
		c.report.Warnings = append(c.report.Warnings, "Library root must be an absolute path; no filesystem inspection performed.")
		return c.report, nil
	}
	rootInfo, err := os.Lstat(c.report.Root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		c.report.Status = "unavailable"
		c.report.Warnings = append(c.report.Warnings, "Root is unavailable, not a directory or is a symlink. Missing-file findings suppressed; verify the mount.")
		return c.report, nil //nolint:nilerr // Root access failures are explicit report evidence, not an HTTP server failure.
	}
	records, truncated, err := s.healthRecords(ctx, lib, limit)
	if err != nil {
		return nil, fmt.Errorf("read health records: %w", err)
	}
	c.report.InspectedRecords = len(records)
	if truncated {
		c.warn("Record limit reached; untracked-file suggestions suppressed and groups may be incomplete.")
	}
	err = c.walk(ctx, c.report.Root, 0)
	switch {
	case errors.Is(err, errHealthLimit):
		c.warn("Entry limit reached; missing-file observations are inconclusive.")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		c.report.Status = "cancelled"
		c.report.Warnings = append(c.report.Warnings, "Check cancelled or time budget exhausted; no absence conclusions are available.")
		return c.report, nil //nolint:nilerr // Cancellation is recorded in the report.
	case err != nil:
		return nil, err
	}
	endInfo, endErr := os.Lstat(c.report.Root)
	if endErr != nil || !os.SameFile(rootInfo, endInfo) || c.report.ScannedEntries == 0 {
		c.report.Status = "unavailable"
		c.report.Warnings = append(c.report.Warnings, "Root changed, disappeared or was empty. Possible unavailable mount; missing-file findings suppressed.")
		return c.report, nil //nolint:nilerr // Root access failures are explicit report evidence, not an HTTP server failure.
	}
	c.classify(ctx, records, truncated)
	if ctx.Err() != nil {
		c.report.Status = "cancelled"
		c.report.Warnings = append(c.report.Warnings, "Classification cancelled; findings are partial.")
	}
	return c.report, nil
}
