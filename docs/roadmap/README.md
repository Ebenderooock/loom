# Loom — Implementation Roadmap

This is the detailed implementation roadmap for Loom's upcoming features. Each phase document contains full technical specifications including SQL schemas, Go interfaces, API routes, frontend tasks, implementation order, dependencies, and risks.

## Architecture Context

Loom is a Go + React media automation platform. Key architectural facts relevant to planning:

- **Download clients** use a `DownloadClient` interface with `RegisterKind` factory pattern. The built-in torrent client shares a single engine across definitions. SABnzbd and NZBGet external Usenet clients already exist. The `AddRequest` type already has `NZBURL`, `RawBytes`, and `ProtocolUsenet` fields.
- **Indexer/search pipeline** fans out across indexers in parallel, scores results (quality/seeders/age/size/freeleech), and feeds into workflows. Kinds register via `RegisterKind`.
- **Workflow engine** manages the full lifecycle: searching → grabbed → downloading → post_download → importing → completed/failed. Uses an orchestrator with command channels and event logging.
- **Notification system** has an event bus with pub/sub, parallel fan-out to senders (Discord, Slack, webhook, etc.), and template support.
- **Libraries** have scanning, file tracking, unmapped folder detection, and disk space monitoring.
- **Config** is layered (defaults < YAML < env `LOOM_*` < flags) with hot-reload support.
- **API** uses `go-chi/chi/v5` with `Set*` dependency injection and `RouteExtensions`.
- **Frontend** is React 18 + TanStack Router/Query + shadcn/ui + Tailwind.

---

## Phase Documents

| Phase | Document | Status |
|-------|----------|--------|
| 1 | [Usenet / NZB Built-in Client](./phase-1-usenet-nzb.md) | High-level plan (deferred) |
| 2 | [Media Requests](./phase-2-media-requests.md) | Fully detailed |
| 3 | [Library Maintenance](./phase-3-library-maintenance.md) | Fully detailed |
| 4 | [Media Server Analytics](./phase-4-media-server-analytics.md) | Fully detailed |
| 5 | [Script Engine](./phase-5-script-engine.md) | Fully detailed |
| 6 | [Script Marketplace](./phase-6-script-marketplace.md) | Fully detailed |

---

## Implementation Order

```
Phase 2: Media Requests ──────────────────────┐
Phase 3: Library Maintenance ─────────────────┤ (independent, can be parallel)
Phase 4: Media Server Analytics ──────────────┘
Phase 5: Script Engine ──────────────────────── (benefits from stable pipelines)
Phase 6: Script Marketplace ─────────────────── (requires Phase 5)
Phase 1: Usenet/NZB ──────────────────────────── (last — massive task, deferred)
```

- **Phases 2–4** are independent and can be built in any order or in parallel.
- **Phase 5** should come after core pipelines are stable.
- **Phase 6** depends on Phase 5.
- **Phase 1** (Usenet/NZB built-in) is deferred to last due to scope (NNTP pool, yEnc, PAR2 port).
