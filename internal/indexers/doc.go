// Package indexers is the search-source subsystem of Loom.
//
// An Indexer is one place Loom can ask "do you have anything matching
// this query?" — examples include public torrent sites driven by
// Cardigann definitions, Newznab/Torznab compatible Usenet trackers,
// and (for testing) the built-in null indexer that returns nothing.
//
// # Shape
//
// The package is intentionally small and orthogonal:
//
//   - The Indexer interface is the contract every kind implements.
//   - Registry is an in-memory, concurrency-safe map keyed by ID. It
//     fans search out across enabled indexers with per-source timeouts
//     and returns partial results plus per-source errors.
//   - Repository is the persistence seam over storage.DB. It handles
//     row-level CRUD and JSON column round-trip; it does not know
//     anything about kinds.
//   - kindRegistry is the catalogue of factory functions used to
//     instantiate concrete Indexers from the `kind` + `config_json`
//     columns when the Service hydrates a row at startup.
//   - HealthChecker is a scheduler job that calls Test on each enabled
//     Indexer on a fixed cadence and writes the outcome to
//     indexer_health, with a bounded worker pool so a slow source can
//     never starve the others.
//   - Service ties Registry, Repository, and HealthChecker together
//     and exposes the operations the HTTP handlers need.
//
// # Threading
//
// Registry and Repository are safe for concurrent use. Each Indexer
// implementation is responsible for its own internal synchronisation;
// the interface contract is that Search and Test must be safe to call
// concurrently on a single Indexer value.
//
// # Adding a new kind
//
// 1. Implement the Indexer interface.
//
// 2. Register a Factory under a stable kind string (e.g. "newznab")
// during package init or via Service.RegisterKind.
//
// 3. Document the expected shape of config_json in your kind's package
// godoc; the indexers package treats it as opaque bytes.
package indexers
