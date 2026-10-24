package libraries

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/auth"
)

func healthLibrary(t *testing.T, store *Store) *Library {
	t.Helper()
	lib := &Library{Name: "Health", Path: t.TempDir(), MediaType: "movie", QualityProfileID: "default"}
	if err := store.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}
	return lib
}

func healthFile(t *testing.T, lib *Library, relative string) string {
	t.Helper()
	path := filepath.Join(lib.Path, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("unchanged media fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func healthMovie(t *testing.T, store *Store, movieID, path, quality string) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO movies (id, title) VALUES (?, ?) ON CONFLICT DO NOTHING`, movieID, movieID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO movie_files (id, movie_id, file_path, quality) VALUES (?, ?, ?, ?)`,
		path, movieID, path, quality); err != nil {
		t.Fatal(err)
	}
}

func findHealth(report *HealthReport, kind, path string) *HealthFinding {
	for _, finding := range report.Findings {
		if finding.Kind == kind && finding.Path == path {
			return &finding
		}
	}
	return nil
}

type healthSnapshot struct {
	Contents string
	Size     int64
	Mode     fs.FileMode
	Modified int64
}

func snapshotHealthFiles(t *testing.T, root string) map[string]healthSnapshot {
	t.Helper()
	snapshot := make(map[string]healthSnapshot)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		snapshot[path] = healthSnapshot{string(data), info.Size(), info.Mode(), info.ModTime().UnixNano()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestHealthEvidenceAndUnchangedFilesystem(t *testing.T) {
	store := openLibraryTestStore(t)
	lib := healthLibrary(t, store)
	feature := healthFile(t, lib, "Feature/feature.mkv")
	sample := healthFile(t, lib, "Feature/sample.mkv")
	extra := healthFile(t, lib, "Feature/extras/sample.mkv")
	subtitle := healthFile(t, lib, "Feature/feature.en.srt")
	alpha := healthFile(t, lib, "Duplicate/alpha.mkv")
	beta := healthFile(t, lib, "Duplicate/beta.mkv")
	version1 := healthFile(t, lib, "Versions/feature.1080p.mkv")
	version2 := healthFile(t, lib, "Versions/feature.2160p.mkv")
	healthMovie(t, store, "feature", feature, "HD")
	healthMovie(t, store, "feature", sample, "HD")
	healthMovie(t, store, "duplicate", alpha, "HD")
	healthMovie(t, store, "duplicate", beta, "HD")
	healthMovie(t, store, "versions", version1, "HD")
	healthMovie(t, store, "versions", version2, "UHD")
	// A linked library record whose media owner no longer exists.
	missing := filepath.Join(lib.Path, "missing.mkv")
	invalidOwner := "removed-media"
	if err := store.UpsertFile(context.Background(), &LibraryFile{LibraryID: lib.ID, Path: missing, MediaID: &invalidOwner}); err != nil {
		t.Fatal(err)
	}
	before := snapshotHealthFiles(t, lib.Path)
	report, err := store.checkHealth(context.Background(), lib, healthEntryLimit)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" {
		t.Fatalf("status %s: %v", report.Status, report.Warnings)
	}
	for _, expected := range []struct{ kind, path string }{
		{"sample", sample}, {"duplicate", beta}, {"protected", extra},
		{"protected", subtitle}, {"protected", version2},
		{"orphan_record", missing}, {"missing_file", missing},
	} {
		if findHealth(report, expected.kind, expected.path) == nil {
			t.Errorf("missing %s finding for %s: %+v", expected.kind, expected.path, report.Findings)
		}
	}
	for _, path := range []string{feature, extra, version1, version2} {
		if findHealth(report, "sample", path) != nil {
			t.Errorf("incorrect sample classification: %s", path)
		}
	}
	if findHealth(report, "duplicate", version2) != nil {
		t.Fatal("intentional quality versions classified as unwanted duplicates")
	}
	for _, finding := range report.Findings {
		if len(finding.Evidence) == 0 || finding.Confidence == "" || finding.Preview.Executable {
			t.Fatalf("unsafe or evidence-free finding: %+v", finding)
		}
	}
	if !reflect.DeepEqual(before, snapshotHealthFiles(t, lib.Path)) {
		t.Fatal("report changed filesystem contents or metadata")
	}
	var movieStatus string
	if err := store.db.QueryRow(`SELECT monitoring_status FROM movies WHERE id = 'feature'`).Scan(&movieStatus); err != nil || movieStatus != "monitored" {
		t.Fatalf("monitoring changed: %q, %v", movieStatus, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM movie_files`).Scan(&count); err != nil || count != 6 {
		t.Fatalf("domain records changed: %d, %v", count, err)
	}
}

func TestHealthMountSafeguards(t *testing.T) {
	for _, mode := range []string{"missing", "empty", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			store := openLibraryTestStore(t)
			lib := healthLibrary(t, store)
			if mode == "missing" {
				lib.Path = filepath.Join(lib.Path, "unmounted")
			}
			if mode == "symlink" {
				target := t.TempDir()
				path := filepath.Join(lib.Path, "linked")
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				lib.Path = path
			}
			if err := store.UpsertFile(context.Background(), &LibraryFile{
				LibraryID: lib.ID, Path: filepath.Join(lib.Path, "feature.mkv"),
			}); err != nil {
				t.Fatal(err)
			}
			report, err := store.checkHealth(context.Background(), lib, healthEntryLimit)
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != "unavailable" || len(report.Findings) != 0 {
				t.Fatalf("unavailable mount reported as deleted files: %+v", report)
			}
			count, err := store.FileCount(context.Background(), lib.ID)
			if err != nil || count != 1 {
				t.Fatalf("report reconciled records: %d, %v", count, err)
			}
		})
	}
}

func TestHealthLimitsCancellationAndSkippedSubtrees(t *testing.T) {
	store := openLibraryTestStore(t)
	lib := healthLibrary(t, store)
	healthFile(t, lib, "a.mkv")
	healthFile(t, lib, "b.mkv")
	report, err := store.checkHealth(context.Background(), lib, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "incomplete" || report.ScannedEntries != 1 {
		t.Fatalf("entry limit not enforced: %+v", report)
	}
	records, truncated, err := store.healthRecords(context.Background(), lib, 1)
	if err != nil || truncated || len(records) != 0 {
		t.Fatalf("unexpected empty records: %v, %v, %v", records, truncated, err)
	}
	for _, relative := range []string{"a.mkv", "b.mkv"} {
		if err := store.UpsertFile(context.Background(), &LibraryFile{LibraryID: lib.ID, Path: filepath.Join(lib.Path, relative)}); err != nil {
			t.Fatal(err)
		}
	}
	records, truncated, err = store.healthRecords(context.Background(), lib, 1)
	if err != nil || !truncated || len(records) != 1 {
		t.Fatalf("record bound not enforced: %v, %v, %v", records, truncated, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	check := newHealthCheck(lib, 10)
	if err := check.walk(ctx, lib.Path, 0); !errors.Is(err, context.Canceled) || check.report.ScannedEntries != 0 {
		t.Fatalf("walk did not honor cancellation: %v, %+v", err, check.report)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sample.mkv"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(lib.Path, "nested-mount")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(link, "sample.mkv")
	if err := store.UpsertFile(context.Background(), &LibraryFile{LibraryID: lib.ID, Path: missing}); err != nil {
		t.Fatal(err)
	}
	report, err = store.checkHealth(context.Background(), lib, healthEntryLimit)
	if err != nil {
		t.Fatal(err)
	}
	finding := findHealth(report, "missing_file", missing)
	if finding == nil || !strings.Contains(strings.Join(finding.Evidence, " "), "skipped") || finding.Confidence != "low" {
		t.Fatalf("skipped subtree lacks uncertainty: %+v", report)
	}
	if findHealth(report, "sample", missing) != nil {
		t.Fatal("followed symlink target")
	}
}

func TestHealthHardlinkAndEditionProtection(t *testing.T) {
	store := openLibraryTestStore(t)
	lib := healthLibrary(t, store)
	original := healthFile(t, lib, "linked/original.mkv")
	link := filepath.Join(lib.Path, "linked/copy.mkv")
	if err := os.Link(original, link); err != nil {
		t.Fatal(err)
	}
	theatrical := healthFile(t, lib, "editions/feature.mkv")
	extended := healthFile(t, lib, "editions/feature.extended.mkv")
	for _, path := range []string{original, link} {
		healthMovie(t, store, "linked", path, "HD")
	}
	for _, path := range []string{theatrical, extended} {
		healthMovie(t, store, "edition", path, "HD")
	}
	report, err := store.checkHealth(context.Background(), lib, healthEntryLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range report.Findings {
		if finding.Kind == "duplicate" {
			t.Fatalf("protected file classified as duplicate: %+v", finding)
		}
	}
	if findHealth(report, "protected", original) == nil || findHealth(report, "protected", theatrical) == nil {
		t.Fatalf("missing hardlink/edition evidence: %+v", report)
	}
}

func TestHealthRoutesPersistenceDismissalAndScanGuard(t *testing.T) {
	store := openLibraryTestStore(t)
	lib := healthLibrary(t, store)
	path := healthFile(t, lib, "sample.mkv")
	scanner := NewScanner(store, testLogger())
	router := Router(store, scanner, testLogger())
	url := "/" + lib.ID + "/health"
	request := func(method, url, body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
		return rec
	}
	if rec := request(http.MethodGet, url, ""); rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("no report should be null, not healthy: %d %s", rec.Code, rec.Body.String())
	}
	scanner.scanMu.Lock()
	rec := request(http.MethodPost, url, "")
	scanner.scanMu.Unlock()
	if rec.Code != http.StatusConflict {
		t.Fatalf("active scan admitted health request: %d", rec.Code)
	}
	rec = request(http.MethodPost, url, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("health run: %d %s", rec.Code, rec.Body.String())
	}
	var report HealthReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	finding := findHealth(&report, "sample", path)
	if finding == nil {
		t.Fatalf("sample not reported: %+v", report)
	}
	body := fmtHealthDismiss(finding.ID, true)
	if rec := request(http.MethodPost, url+"/dismiss", body); rec.Code != http.StatusOK {
		t.Fatalf("dismiss: %d %s", rec.Code, rec.Body.String())
	}
	// New router represents a restart; latest snapshot and dismissal survive.
	router = Router(store, scanner, testLogger())
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := request(method, url, "")
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		current := findHealth(&report, "sample", path)
		if rec.Code != http.StatusOK || current == nil || !current.Dismissed {
			t.Fatalf("dismissal lost on %s: %d %+v", method, rec.Code, report)
		}
	}
	if rec := request(http.MethodPost, url+"/dismiss", fmtHealthDismiss(finding.ID, false)); rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body.String())
	}
	if rec := request(http.MethodPost, url+"/dismiss", `{"finding_id":"unknown","dismissed":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown finding: %d", rec.Code)
	}
	if rec := request(http.MethodPost, url+"/dismiss", `{"finding_id":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid dismissal: %d", rec.Code)
	}
	// Replacement/changed content should not inherit a previous dismissal.
	if rec := request(http.MethodPost, url+"/dismiss", body); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	if err := os.WriteFile(path, []byte("replacement file has changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec = request(http.MethodPost, url, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	current := findHealth(&report, "sample", path)
	if current == nil || current.Dismissed || current.ID == finding.ID {
		t.Fatalf("changed evidence inherited dismissal: %+v", current)
	}
}

func fmtHealthDismiss(id string, dismissed bool) string {
	body, _ := json.Marshal(map[string]any{"finding_id": id, "dismissed": dismissed})
	return string(body)
}

func TestHealthAdmissionBound(t *testing.T) {
	h := &healthHandlers{active: make(map[string]bool), slots: make(chan struct{}, 2)}
	if !h.acquire("one") || h.acquire("one") || !h.acquire("two") || h.acquire("three") {
		t.Fatal("concurrency bound not enforced")
	}
	h.release("one")
	if !h.acquire("three") {
		t.Fatal("slot not released")
	}
	h.release("two")
	h.release("three")
}

func TestHealthRequiresAdminWhenIdentityPresent(t *testing.T) {
	store := openLibraryTestStore(t)
	lib := healthLibrary(t, store)
	_ = healthFile(t, lib, "sample.mkv")
	router := Router(store, NewScanner(store, testLogger()), testLogger())
	tests := []struct {
		name   string
		ctx    context.Context
		status int
	}{
		{"anonymous", context.Background(), http.StatusOK},
		{"admin", auth.WithIdentity(context.Background(), &auth.Identity{Roles: []string{"admin"}}), http.StatusOK},
		{"non-admin", auth.WithIdentity(context.Background(), &auth.Identity{Roles: []string{"user"}}), http.StatusForbidden},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/"+lib.ID+"/health", nil).WithContext(testCase.ctx)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, request)
			if rec.Code != testCase.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
