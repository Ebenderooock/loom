// Package server hosts Loom's HTTP listener and wires the route tree:
// native /api/v1/*, the /metrics Prometheus endpoint, /healthz /livez
// /readyz probes, optional /debug/pprof/*, the wire-compat surfaces
// (Radarr v3 / Sonarr v3+v4 / Prowlarr v1 — landing in Phase 7), and
// the embedded React UI when built with the embed tag.
//
// Routing is built on go-chi/chi/v5 with a standard middleware chain:
// request-ID, structured access log, panic recovery, gzip, ETag for the
// system status endpoint, and CORS when configured.
//
// See docs/api.md and api/openapi/loom.yaml for the canonical route
// reference, and ADR-0003 for the API design decision.
package server
