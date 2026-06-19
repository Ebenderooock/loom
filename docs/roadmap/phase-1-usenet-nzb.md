# Phase 1: Usenet / NZB Built-in Client

### Goal
Add a built-in NZB download engine so users don't need an external SABnzbd/NZBGet instance.

### Approach
The download client abstraction already supports Usenet — `ProtocolUsenet`, `NZBURL`, `RawBytes` are defined in `types.go`. SABnzbd and NZBGet external clients already exist as reference implementations.

### Backend Tasks
1. **NZB parser** — Parse `.nzb` XML files into article/segment lists (`internal/downloads/nzb/parser.go`)
2. **NNTP connection pool** — Manage connections to Usenet servers with TLS, authentication, and connection limits (`internal/downloads/nzb/nntp.go`)
3. **Article assembler** — Download article segments in parallel, reassemble, yEnc decode (`internal/downloads/nzb/assembler.go`)
4. **PAR2 repair** — Integrate PAR2 verification and repair for incomplete downloads (`internal/downloads/nzb/par2.go`)
5. **RAR extraction** — Unpack multi-part RAR archives post-download (`internal/downloads/nzb/unpack.go`)
6. **NZB engine** — Orchestrate the download pipeline: parse → download → verify → repair → extract (`internal/downloads/nzb/engine.go`)
7. **Download client implementation** — Implement `DownloadClient` interface wrapping the engine (`internal/downloads/nzb/client.go`)
8. **Kind registration** — Register `KindBuiltinNZB` via `RegisterKind` factory pattern (`internal/downloads/nzb/kind.go`)
9. **Usenet server management** — CRUD for Usenet server configs (host, port, TLS, connections, username/password) — stored alongside download client definitions
10. **API routes** — REST endpoints for NZB client status, server management, queue operations
11. **Priority/fill server support** — Multiple Usenet servers with priority ordering and fill-server fallback

### Frontend Tasks
12. **Download client setup UI** — Add "Built-in NZB" option to download client creation with server configuration form
13. **NZB queue view** — Show NZB downloads in the downloads page with segment progress, repair status, extraction status
14. **Usenet server health** — Health indicators for configured Usenet servers (connection count, speed, failures)

### Dependencies
- None — can start immediately

### Risks
- PAR2 repair is complex; consider using an external Go library or shelling out to `par2cmdline`
- NNTP protocol handling edge cases (article retention, DMCA takedowns, incomplete articles)
- Memory management for large NZB downloads with many segments
