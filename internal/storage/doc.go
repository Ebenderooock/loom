// Package storage owns Loom's database layer: engine selection,
// connection lifecycle, embedded schema migrations (via goose), and the
// narrow seam consumed by the rest of the application.
//
// Two engines are supported behind one DB interface: SQLite
// (modernc.org/sqlite, pure Go) for the single-binary deployment and
// Postgres (jackc/pgx via database/sql) for HA / shared installs.
// Migrations live under migrations/<engine>/ and are embedded into the
// binary; sqlc-generated query packages live under db/sqlite and
// db/postgres.
//
// See docs/storage.md and ADR-0002.
package storage
