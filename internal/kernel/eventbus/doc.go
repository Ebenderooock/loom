// Package eventbus is Loom's typed in-process publish/subscribe bus.
// Modules (indexers, movies, series, downloads, …) emit and consume
// typed Events without taking direct dependencies on each other,
// preserving the modular-monolith boundaries described in ADR-0001
// and docs/architecture.md.
//
// The Bus interface is shaped to match what an embedded NATS backend
// will provide in split-mode (Phase 11), so module code does not change
// when the deployment topology does.
package eventbus
