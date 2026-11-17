//go:build linux || darwin

package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/updates"
)

type fakePublisher struct {
	candidate updates.Candidate
	archive   []byte
	fail      bool
	ahead     bool
}

func (p *fakePublisher) Discover(context.Context, string) (updates.Candidate, error) {
	return p.candidate, nil
}
func (p *fakePublisher) Download(_ context.Context, _ updates.Candidate, _ string, out io.Writer) error {
	if p.fail {
		return errors.New("download verification failed")
	}
	_, err := out.Write(p.archive)
	return err
}
func (p *fakePublisher) Ahead(context.Context, string, string) bool { return p.ahead }

type fakeControl struct {
	paths                                         layout
	current                                       Identity
	target                                        Identity
	running                                       bool
	failHealth, failStart, failSnapshot, failStop bool
	starts, stops                                 int
	cancel                                        context.CancelFunc
	beforeSnapshot                                func()
}

func (s *fakeControl) validate(context.Context) error { return nil }
func (s *fakeControl) runtime(context.Context) (Runtime, error) {
	return Runtime{Identity: s.current, Native: true, Ready: true, Engine: "sqlite", Settings: updates.Settings{Channel: "stable", AutoInstall: true}}, nil
}
func (s *fakeControl) quiescent(context.Context) error {
	if s.running {
		return errors.New("service still active")
	}
	return nil
}
func (s *fakeControl) stop(context.Context) error {
	s.stops++
	if s.failStop {
		return errors.New("stop failed")
	}
	s.running = false
	return nil
}
func (s *fakeControl) start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.starts++
	s.running = true
	raw, err := os.ReadFile(filepath.Join(s.paths.bin, "loom"))
	if err != nil {
		return err
	}
	if string(raw) == "new" {
		s.current = s.target
		db, err := sql.Open("sqlite", filepath.Join(s.paths.data, "loom.db"))
		if err != nil {
			return err
		}
		_, err = db.Exec(`UPDATE payload SET value='migrated'; CREATE TABLE migrated (id INTEGER)`)
		_ = db.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(s.paths.config, "loom.json"), []byte("new-config"), 0600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(s.paths.config, "added-by-migration"), []byte("extra"), 0600); err != nil {
			return err
		}
		if s.cancel != nil {
			s.cancel()
		}
		if s.failStart {
			return errors.New("migration/start failure")
		}
	} else {
		s.current = oldIdentity()
	}
	return nil
}
func (s *fakeControl) healthy(ctx context.Context, want Identity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.failHealth && want.SHA == s.target.SHA {
		return errors.New("wrong health/version")
	}
	if !s.running || !sameIdentity(s.current, want) {
		return errors.New("not healthy")
	}
	return nil
}
func (s *fakeControl) snapshotDB(ctx context.Context, dest string) error {
	if s.beforeSnapshot != nil {
		s.beforeSnapshot()
	}
	if s.failSnapshot {
		return errors.New("snapshot disk full")
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(s.paths.data, "loom.db")+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	_, err = db.ExecContext(ctx, "VACUUM INTO '"+strings.ReplaceAll(dest, "'", "''")+"'")
	return err
}
func oldIdentity() Identity {
	return Identity{Version: "1.2.2", SHA: strings.Repeat("a", 40), Ref: "refs/tags/v1.2.2", Channel: "stable"}
}
func newIdentity() Identity {
	return Identity{Version: "1.2.3", SHA: strings.Repeat("b", 40), Ref: "refs/tags/v1.2.3", Channel: "stable"}
}

func testEngine(t *testing.T) (*engine, *fakeControl, *fakePublisher) {
	t.Helper()
	root := t.TempDir()
	p := layout{filepath.Join(root, "bin"), filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "root-state")}
	for _, dir := range []string{p.bin, p.config, p.data, p.state} {
		if err := os.Mkdir(dir, 0750); err != nil {
			t.Fatal(err)
		}
	}
	for path, value := range map[string]string{
		filepath.Join(p.bin, "loom"):         "old",
		filepath.Join(p.config, "loom.yaml"): fmt.Sprintf("config_dir: %q\ndata_dir: %q\nhttp:\n  addr: 127.0.0.1:1925\nstorage:\n  engine: sqlite\n  sqlite:\n    path: %q\n", ConfigDir, DataDir, DBPath),
		filepath.Join(p.config, "loom.json"): "old-config",
	} {
		if err := os.WriteFile(path, []byte(value), 0640); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(p.bin, "loom"), 0755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(p.data, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`PRAGMA journal_mode=WAL;
		CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO schema_meta VALUES ('updates.settings','{"channel":"stable","autoInstall":true}');
		CREATE TABLE payload (value TEXT);
		INSERT INTO payload VALUES ('before');`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(p.state, "state.json", state{Installed: oldIdentity()}); err != nil {
		t.Fatal(err)
	}
	service := &fakeControl{paths: p, current: oldIdentity(), target: newIdentity(), running: true}
	pub := &fakePublisher{candidate: updates.Candidate{Manifest: updates.Manifest{
		SchemaVersion: 1, Channel: "stable", Version: "1.2.3", SourceSHA: newIdentity().SHA, SourceRef: newIdentity().Ref,
	}}, archive: archiveFixture(t, []tar.Header{{Name: "loom", Typeflag: tar.TypeReg, Size: 3}}, [][]byte{[]byte("new")})}
	return &engine{paths: p, platform: "linux-amd64", service: service, publisher: pub}, service, pub
}

func archiveFixture(t *testing.T, headers []tar.Header, contents [][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for i, h := range headers {
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg && len(contents) > i && len(contents[i]) > 0 {
			if _, err := tw.Write(contents[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func assertRestored(t *testing.T, e *engine, s *fakeControl) {
	t.Helper()
	for path, want := range map[string]string{filepath.Join(e.paths.bin, "loom"): "old", filepath.Join(e.paths.config, "loom.json"): "old-config"} {
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != want {
			t.Fatalf("%s=%q %v", path, raw, err)
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(e.paths.data, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var value string
	if err := db.QueryRow("SELECT value FROM payload").Scan(&value); err != nil || value != "before" {
		t.Fatalf("restored DB=%q %v", value, err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='migrated'").Scan(&count); err != nil || count != 0 {
		t.Fatal("migrated schema remained", err)
	}
	if _, err := os.Stat(filepath.Join(e.paths.config, "added-by-migration")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new configuration entry survived rollback")
	}
	var st state
	if err := readJSON(filepath.Join(e.paths.state, "state.json"), &st); err != nil || !sameIdentity(st.Installed, oldIdentity()) {
		t.Fatal("baseline not restored", err)
	}
	if !s.running {
		t.Fatal("old service was not restarted")
	}
}

func TestSuccessfulUpdateAndSnapshot(t *testing.T) {
	e, s, _ := testEngine(t)
	before, err := os.Stat(filepath.Join(e.paths.bin, "loom"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sameIdentity(s.current, newIdentity()) || !s.running {
		t.Fatal("target not running")
	}
	info, err := os.Stat(filepath.Join(e.paths.bin, "loom"))
	if err != nil || info.Mode() != before.Mode() {
		t.Fatal("binary modes changed")
	}
	var st state
	if err := readJSON(filepath.Join(e.paths.state, "state.json"), &st); err != nil || !sameIdentity(st.Installed, newIdentity()) {
		t.Fatal("new baseline not committed", err)
	}
	if _, err := os.Stat(filepath.Join(e.paths.state, "journal.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("journal remains after commit")
	}
	raw, err := os.ReadFile(filepath.Join(e.paths.state, "transaction", "config", "loom.json"))
	if err != nil || string(raw) != "old-config" {
		t.Fatal("config snapshot missing")
	}
}

func TestRollbackRestoresBinarySQLiteAndAllConfig(t *testing.T) {
	for _, failure := range []string{"health", "migration", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			e, s, _ := testEngine(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch failure {
			case "health":
				s.failHealth = true
			case "migration":
				s.failStart = true
			case "cancellation":
				s.cancel = cancel
			}
			if err := e.run(ctx); err == nil {
				t.Fatal("failed update reported success")
			}
			assertRestored(t, e, s)
			var st state
			_ = readJSON(filepath.Join(e.paths.state, "state.json"), &st)
			if st.FailedSHA != newIdentity().SHA {
				t.Fatal("failed build not blocked")
			}
			originalStops := s.stops
			if err := e.run(context.Background()); err == nil || s.stops != originalStops {
				t.Fatal("failed build was retried")
			}
		})
	}
}

func TestFailuresBeforeInstallDoNotReplaceBinary(t *testing.T) {
	for _, failure := range []string{"download", "stop", "snapshot", "consent", "postgres", "path", "symlink", "hardlink"} {
		t.Run(failure, func(t *testing.T) {
			e, s, p := testEngine(t)
			switch failure {
			case "download":
				p.fail = true
			case "stop":
				s.failStop = true
			case "snapshot":
				s.failSnapshot = true
			case "consent", "postgres":
				db, _ := sql.Open("sqlite", filepath.Join(e.paths.data, "loom.db"))
				_, _ = db.Exec(`UPDATE schema_meta SET value='{"channel":"stable","autoInstall":false}'`)
				_ = db.Close()
			case "path":
				_ = os.WriteFile(filepath.Join(e.paths.config, "loom.yaml"), []byte("storage:\n  engine: postgres\n"), 0600)
			case "symlink":
				if err := os.Symlink(filepath.Join(e.paths.bin, "loom"), filepath.Join(e.paths.config, "external")); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(filepath.Join(e.paths.bin, "loom"), filepath.Join(e.paths.config, "linked")); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.run(context.Background()); err == nil {
				t.Fatal("failure accepted")
			}
			raw, _ := os.ReadFile(filepath.Join(e.paths.bin, "loom"))
			if string(raw) != "old" {
				t.Fatal("binary changed on failed preparation")
			}
			if !s.running {
				t.Fatal("pre-install failure left old service stopped")
			}
			if _, err := os.Stat(filepath.Join(e.paths.state, "journal.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("published partial snapshot")
			}
		})
	}
}

func TestInterruptedUpdateBootRecoveryAndRetry(t *testing.T) {
	e, s, _ := testEngine(t)
	s.running = false
	work := filepath.Join(e.paths.state, "transaction")
	if err := os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "new-binary"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	j, err := e.snapshot(context.Background(), state{Installed: oldIdentity()}, newIdentity(), work)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicJSON(e.paths.state, "journal.json", j); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(e.paths.bin, "loom", filepath.Join(work, "new-binary"), j.Binary); err != nil {
		t.Fatal(err)
	}
	if err := s.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.running = false
	// A missing backup prevents boot, retains the journal, and is retryable.
	old := filepath.Join(work, "old-binary")
	if err := os.Rename(old, old+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := e.recover(context.Background(), false); err == nil {
		t.Fatal("incomplete recovery allowed boot")
	}
	if _, err := os.Stat(filepath.Join(e.paths.state, "journal.json")); err != nil {
		t.Fatal("recovery journal lost")
	}
	if err := os.Rename(old+".saved", old); err != nil {
		t.Fatal(err)
	}
	starts := s.starts
	if err := e.recover(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if s.starts != starts {
		t.Fatal("recovery oneshot recursively started its dependent unit")
	}
	if err := e.recover(context.Background(), false); err != nil {
		t.Fatal("recovery not idempotent", err)
	}
	if err := s.start(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertRestored(t, e, s)
}

func TestSnapshotIncludesCommittedUncheckpointedWAL(t *testing.T) {
	e, s, _ := testEngine(t)
	db, err := sql.Open("sqlite", filepath.Join(e.paths.data, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; UPDATE payload SET value='in-wal'"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "copy.db")
	if err := s.snapshotDB(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	copy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = copy.Close() }()
	var got string
	if err := copy.QueryRow("SELECT value FROM payload").Scan(&got); err != nil || got != "in-wal" {
		t.Fatalf("WAL snapshot=%q %v", got, err)
	}
}

func TestArchiveDoesNotExtractPathsLinksOrDuplicateBinary(t *testing.T) {
	for _, header := range []tar.Header{
		{Name: "loom", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		{Name: "loom", Typeflag: tar.TypeLink, Linkname: "/etc/passwd"},
		{Name: "../../outside", Typeflag: tar.TypeReg, Size: 3},
	} {
		path := filepath.Join(t.TempDir(), "archive")
		_ = os.WriteFile(path, archiveFixture(t, []tar.Header{header}, [][]byte{[]byte("bad")}), 0600)
		if err := extractBinary(path, path+".binary"); err == nil {
			t.Fatal("unsafe or missing binary accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "archive")
	_ = os.WriteFile(path, archiveFixture(t, []tar.Header{{Name: "loom", Typeflag: tar.TypeReg, Size: 3}, {Name: "./loom", Typeflag: tar.TypeReg, Size: 3}}, [][]byte{[]byte("new"), []byte("new")}), 0600)
	if err := extractBinary(path, path+".binary"); err == nil {
		t.Fatal("duplicate binary accepted")
	}
}

func TestPreleaseSameVersionNeedsFastForwardAndSameBranch(t *testing.T) {
	e, _, p := testEngine(t)
	old := Identity{Version: "0.4.2-prelease", SHA: strings.Repeat("a", 40), Ref: "refs/heads/prelease/0.4.2", Channel: "prelease"}
	next := old
	next.SHA = strings.Repeat("b", 40)
	if e.newer(context.Background(), old, next) {
		t.Fatal("SHA difference treated as upgrade")
	}
	p.ahead = true
	if !e.newer(context.Background(), old, next) {
		t.Fatal("fast-forward build rejected")
	}
	next.Ref = "refs/heads/prerelease/0.4.2"
	if e.newer(context.Background(), old, next) {
		t.Fatal("equal version from different branch accepted")
	}
	next = old
	next.Version = "0.4.1-prelease"
	next.Ref = "refs/heads/prelease/0.4.1"
	next.SHA = strings.Repeat("c", 40)
	if e.newer(context.Background(), old, next) {
		t.Fatal("downgrade accepted")
	}
}

func TestUnitsMatchInstallerAndUnsupportedRuntimeFailsClosed(t *testing.T) {
	for name, want := range map[string]string{"loom.service": mainUnit, "loom-updater.service": updaterUnit, "loom-updater-recover.service": recoverUnit, "loom-updater.timer": timerUnit, "updater.conf": dependencyUnit} {
		got, err := os.ReadFile(filepath.Join("../../packaging/linux", name))
		if err != nil || string(got) != want {
			t.Fatalf("unit %s drifted: %v", name, err)
		}
	}
	if ok, _ := Capability(Binding{Engine: "postgres"}); ok {
		t.Fatal("PostgreSQL eligible")
	}
	if ok, _ := Capability(Binding{Engine: "sqlite"}); ok {
		t.Fatal("unmanaged install eligible")
	}
	for _, b := range []Binding{{}, {ConfigDir: ConfigDir, DataDir: DataDir, SQLitePath: DBPath, Listen: "127.0.0.1:1925", Engine: "sqlite"}} {
		req := httptest.NewRequest("GET", RuntimePath, nil)
		req.Header.Set("X-Loom-Updater-Token", "forged")
		w := httptest.NewRecorder()
		RuntimeHandler(b, func(context.Context) (updates.Settings, error) { return updates.Settings{}, nil }, func(context.Context) error { return nil })(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusUnauthorized {
			t.Fatal("unmanaged runtime exposed", w.Code)
		}
	}
}
