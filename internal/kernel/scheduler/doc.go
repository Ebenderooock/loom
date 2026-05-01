// Package scheduler runs Loom's recurring and one-shot work: RSS sync,
// indexer health checks, library refresh, and other periodic tasks.
//
// The Phase-1 implementation is fully in-memory. Persistence — so that
// pause/resume state and last-run timestamps survive restarts — is
// scheduled to land alongside the database integration. See
// docs/architecture.md and the Phase 1 entry in ROADMAP.md.
package scheduler
