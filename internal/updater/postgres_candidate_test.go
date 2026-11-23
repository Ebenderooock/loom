//go:build loom_postgres_candidate

package updater

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

type pgFakeRow struct {
	values []any
	err    error
}

func (r pgFakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("wrong number of values")
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}

type pgFakeDB struct {
	rows    []pgFakeRow
	queries []string
	args    [][]any
}

func (d *pgFakeDB) QueryRowContext(_ context.Context, query string, args ...any) postgresRow {
	d.queries = append(d.queries, query)
	d.args = append(d.args, args)
	if len(d.rows) == 0 {
		return pgFakeRow{err: errors.New("unexpected query")}
	}
	r := d.rows[0]
	d.rows = d.rows[1:]
	return r
}

type pgFakeTools struct {
	major    int
	database string
	fail     string
	calls    []string
}

func (t *pgFakeTools) Validate(_ context.Context, major int, database string) error {
	t.major = major
	t.database = database
	if t.fail == "version" {
		return errors.New("missing binary")
	}
	return nil
}

func (t *pgFakeTools) Run(_ context.Context, name string, args []string, in io.Reader, out io.Writer) error {
	t.calls = append(t.calls, name+" "+strings.Join(args, " "))
	if t.fail == name {
		return errors.New("failed")
	}
	if name == "pg_dump" {
		_, err := io.WriteString(out, "fake custom archive")
		return err
	}
	raw, err := io.ReadAll(in)
	if err != nil || string(raw) != "fake custom archive" {
		return errors.New("archive not rewound")
	}
	return nil
}

type pgFakeFence bool

func (f pgFakeFence) VerifyUpdaterOnly(context.Context) error {
	if !f {
		return errors.New("unavailable")
	}
	return nil
}

func pgCandidateFixture() (postgresCandidate, *pgFakeDB, *pgFakeTools) {
	db := &pgFakeDB{rows: []pgFakeRow{
		{values: []any{150002}}, {values: []any{true, true}}, {values: []any{int64(0)}},
	}}
	tools := &pgFakeTools{}
	return postgresCandidate{db: db, tools: tools, fence: pgFakeFence(true)}, db, tools
}

func TestPostgresIdentityAndAcknowledgement(t *testing.T) {
	t.Setenv("PGHOST", "env.example")
	t.Setenv("PGPORT", "5444")
	t.Setenv("PGUSER", "envuser")
	t.Setenv("PGDATABASE", "envdb")
	id, err := effectivePostgresIdentity("sslmode=disable password=secret")
	want := postgresIdentity{Host: "env.example", Port: 5444, User: "envuser", Database: "envdb"}
	if err != nil || id != want {
		t.Fatalf("effective identity = %#v, %v", id, err)
	}
	if postgresAcknowledged(id, nil) || !postgresAcknowledged(id, &want) {
		t.Fatal("acknowledgement was not bound to identity")
	}
	want.Port++
	if postgresAcknowledged(id, &want) {
		t.Fatal("changed endpoint accepted")
	}
	_, err = effectivePostgresIdentity("host=one,two sslmode=disable password=secret")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("multi-host endpoint accepted or credentials exposed")
	}
}

func TestPostgresCandidateSnapshotAndFailureGates(t *testing.T) {
	for _, failure := range []string{"", "old-server", "privilege", "tools", "fence", "sessions", "pg_dump", "pg_restore"} {
		t.Run(failure, func(t *testing.T) {
			p, db, tools := pgCandidateFixture()
			switch failure {
			case "old-server":
				db.rows[0].values[0] = 120000
			case "privilege":
				db.rows[1].values[0] = false
			case "tools":
				tools.fail = "version"
			case "fence":
				p.fence = pgFakeFence(false)
			case "sessions":
				db.rows[2].values[0] = int64(1)
			case "pg_dump", "pg_restore":
				tools.fail = failure
			}
			archive, err := os.CreateTemp(t.TempDir(), "archive")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = archive.Close() }()
			err = p.snapshot(context.Background(), "loom", archive)
			if (err != nil) != (failure != "") {
				t.Fatalf("snapshot: %v", err)
			}
			if failure == "" {
				if tools.major != 15 || tools.database != "loom" || !reflect.DeepEqual(tools.calls, []string{"pg_dump --format=custom", "pg_restore --list"}) {
					t.Fatalf("wrong tools: %#v", tools)
				}
				if !reflect.DeepEqual(db.args[2], []any{"loom"}) {
					t.Fatal("database was not bound as a parameter")
				}
			} else if failure != "pg_dump" && failure != "pg_restore" && len(tools.calls) != 0 {
				t.Fatal("started dump despite failed safety check")
			}
		})
	}
}
