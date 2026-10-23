# Loom — Implementation Roadmap

This is the detailed implementation roadmap for Loom's upcoming features. Phase documents are design proposals, not a current capability inventory; some proposed functionality has already shipped with different schemas or APIs. See [platform documentation](../platform-documentation.md) and the implementation before treating a task as outstanding.

Authenticated movie, series, and artist requests, request quotas, admin approval,
and Discord/Telegram request bots already exist. The music module includes
artist/album/release/track management and acquisition. Guest requests, broader
auto-approval rules, and external media-server request availability checks remain
separate roadmap work.

## User-Feedback Backlog (2026-10-23)

This intake is based on a bounded review of public Arr ecosystem reports, not
a statistical popularity ranking or proof of equivalent defects in Loom.
Each issue includes source links, acceptance criteria and exclusions. Historical
Overseerr/Readarr reports are needs evidence, not claims about current Seerr
behavior or commitments from retired projects. Reddit search-index leads were
not verified against original post bodies and do not drive implementation priority.

| Order | Priority | Work | Scope |
|-------|----------|------|-------|
| 1 | P1 | [Durable acquisition failures](https://github.com/Ebenderooock/loom/issues/174) | Extend history/blocklist mechanisms; bounded retries and restart recovery, including pre-workflow failures. |
| 2 | P1 | [Explainable matching and policy](https://github.com/Ebenderooock/loom/issues/171) | Actual-engine identity/ranking explanations, configuration guidance and regression fixtures; investigate safe pack-mapping previews. |
| 3 | P1 | [Request history versus availability](https://github.com/Ebenderooock/loom/issues/172) | Preserve fulfillment history, support re-requesting missing media and protect against disconnected mounts. |
| Design alongside 1-3 | P1 | [Independent version tracks](https://github.com/Ebenderooock/loom/issues/169) | Design 1080p/4K isolation and migration before splitting implementation; single-track defaults unchanged. |
| Later | P2 | [Provider-aware API budgets](https://github.com/Ebenderooock/loom/issues/173) | Persistent allowances, explicit shared-account groups and provider reset signals. |
| Later | P2 | [Read-only library health](https://github.com/Ebenderooock/loom/issues/170) | Safe first slice of Phase 3, with evidence and previews; no automatic deletion. |

The ordering above is a delivery recommendation, not a claim that all items
depend on one another. These issues are not marked implemented. The current
uncommitted review-fix batch covers quota admission/retry eligibility,
file-derived request fulfillment and import collision safety, not these complete
follow-on capabilities.

### Discovery and Deferred Work

- **Anime pack mapping:** investigate catalog-aware, reversible previews within
  matching work; ambiguous identities must remain unresolved. Do not relax
  matching globally.
- **Metadata resilience:** assess cached reads, outage behavior, provider
  provenance and identity migration before expanding
  [books support](https://github.com/Ebenderooock/loom/issues/17).
- **Storage fairness and destructive retention:** defer until shared-content
  ownership, availability and monitoring/reacquisition semantics are explicit.
- **SSO:** already supported; treat authorization coverage, recovery and logout
  as audits of existing behavior, not a new missing authentication feature.
- **Sports naming and bulk folder migration:** discovery only. Sports may need a
  different catalog model; migration needs collision previews, extras/subtitle
  preservation and a reversible operation manifest. Search snippets alone do
  not justify prioritizing either above reliability work.
- **Music backlog:** [the existing music issue](https://github.com/Ebenderooock/loom/issues/16)
  tracks remaining coverage validation rather than describing the implemented
  module as entirely absent.

## Architecture Context

Loom is a Go + React media automation platform. Key architectural facts relevant to planning:

- **Download clients** use a `DownloadClient` interface with `RegisterKind` factory pattern. The built-in torrent client shares a single engine across definitions. SABnzbd and NZBGet external Usenet clients already exist. The `AddRequest` type already has `NZBURL`, `RawBytes`, and `ProtocolUsenet` fields.
- **Indexer/search pipeline** fans out across indexers in parallel, scores results (quality/seeders/age/size/freeleech), and feeds into workflows. Kinds register via `RegisterKind`.
- **Workflow engine** manages the full lifecycle: searching → grabbed → downloading → post_download → importing → completed/failed. Uses an orchestrator with command channels and event logging.
- **Notification system** has an event bus with pub/sub, parallel fan-out to senders (Discord, Slack, webhook, etc.), and template support.
- **Libraries** have scanning, file tracking, and disk space monitoring.
- **Config** is layered (defaults < YAML < env `LOOM_*` < flags) with hot-reload support.
- **API** uses `go-chi/chi/v5` with `Set*` dependency injection and `RouteExtensions`.
- **Frontend** is React 19 + TanStack Router/Query + shadcn/ui + Tailwind.

---

## Phase Documents

| Phase | Document | Status |
|-------|----------|--------|
| 1 | [Usenet / NZB Built-in Client](./phase-1-usenet-nzb.md) | High-level plan (deferred) |
| 2 | [Media Requests](./phase-2-media-requests.md) | Core implemented; expanded features remain proposed |
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
