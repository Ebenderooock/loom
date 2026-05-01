# ADR-0001: Use Go for the backend

- Status: Accepted
- Date: 2025-05-01
- Deciders: Loom maintainers

## Context

Loom replaces Radarr/Sonarr/Prowlarr (.NET/C#) for a self-hosting audience
that runs on everything from a Synology NAS or Raspberry Pi to a multi-node
Kubernetes cluster. The runtime must:

- Ship as a small, multi-arch container (amd64 / arm64 / armv7).
- Idle in well under 100 MB RAM on small ARM boards.
- Cross-compile for Linux, macOS, Windows, FreeBSD without a heavy SDK.
- Have first-class HTTP, filesystem, concurrency, and observability libs.
- Attract a broad contributor pool.

## Decision

Use **Go 1.23+** for the backend.

## Consequences

### Positive
- Single static binary — trivially containerized in `gcr.io/distroless/static`.
- `GOOS`/`GOARCH` cross-compile is built into the toolchain; one CI matrix
  produces every artifact we need.
- Idle memory is dramatically lower than .NET; matters on Pi/NAS.
- Stdlib covers HTTP, TLS, filesystem, crypto without third-party deps.
- Excellent ecosystem for self-hosted ops: Prometheus client, OpenTelemetry,
  pprof, slog, NATS, Cobra, Viper.
- Larger contributor pool than Rust; lower barrier to entry than .NET for
  the typical homelab tinkerer.

### Negative / trade-offs
- We re-implement *arr behavior; we do not port C# code (clean-room).
- Generics are good but not as expressive as Rust's; some parser code will
  be more verbose.
- GC pauses theoretically matter; in practice Go's pacer is fine for our
  workload (mostly I/O-bound).

### Neutral
- We adopt `gofmt`, `go vet`, `golangci-lint`, `govulncheck` as the lint set.

## Alternatives considered

- **Rust** — best raw performance and memory characteristics, but smaller
  contributor pool and slower iteration. Reserve for performance-critical
  plugin SDK pieces if needed.
- **C# / .NET 9** — match upstream but fight the same image-size and ARM
  memory issues we want to solve.
- **TypeScript / Node** — fine for the API but weak for long-running
  filesystem-heavy daemons; memory profile is worse than Go on ARM.
- **Python** — easy to write but startup, packaging, and idle memory are
  all worse than Go for this profile.
