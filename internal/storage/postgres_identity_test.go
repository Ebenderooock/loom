package storage

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestPostgresConnectionIdentityUsesEffectiveConfig(t *testing.T) {
	t.Setenv("PGHOST", "env.example")
	t.Setenv("PGPORT", "5444")
	t.Setenv("PGDATABASE", "envdb")
	t.Setenv("PGUSER", "envuser")

	for _, tc := range []struct {
		dsn  string
		want PostgresIdentity
		ok   bool
	}{
		{"sslmode=disable password=secret", PostgresIdentity{"env.example", 5444, "envdb", "envuser"}, true},
		{"host=dsn.example port=5555 dbname=dsndb user=dsnuser sslmode=disable", PostgresIdentity{"dsn.example", 5555, "dsndb", "dsnuser"}, true},
		{"host=one,two sslmode=disable", PostgresIdentity{}, false},
	} {
		c, err := pgx.ParseConfig(tc.dsn)
		if err != nil {
			t.Fatal("could not parse test configuration")
		}
		got, ok := postgresConnectionIdentity(c)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("identity = %#v, %v; want %#v, %v", got, ok, tc.want, tc.ok)
		}
	}
}
