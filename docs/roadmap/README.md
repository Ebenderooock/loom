# Loom — Implementation Roadmap

These are detailed design proposals, not a current capability inventory.
Some proposed functionality has shipped with different schemas or APIs.
The [reviewed root roadmap](../../ROADMAP.md) records current implementation
status and the distinction between published `v0.4.1` and later `main` work.
The phase numbers in this directory are a separate proposal sequence, not
the root roadmap's phase numbers.

Authenticated movie, series, and artist requests, request quotas, admin approval,
and Discord/Telegram request bots already exist. The music module includes
artist/album/release/track management and acquisition, with important import and
profile fixes after `v0.4.1`. Read-only library health, basic media analytics and
admin-defined JavaScript event plugins are also implemented. Guest requests,
auto-approval policies, proactive requestor decision notifications, external
media-server request availability and a script marketplace remain separate work.

**Usenet/NNTP has not been tested at all.** External SABnzbd/NZBGet and
Newznab/NZB implementations exist but are unverified. The built-in NNTP
downloader remains a proposal, not an available client.

## User-Feedback Intake (2026-10-23; status refreshed 2026-11-17)

This intake is based on a bounded review of public Arr ecosystem reports, not
a statistical popularity ranking or proof of equivalent defects in Loom.
Each issue includes source links, acceptance criteria and exclusions. Historical
Overseerr/Readarr reports are needs evidence, not claims about current Seerr
behavior or commitments from retired projects. Reddit search-index leads were
not verified against original post bodies and do not drive implementation priority.

| Work | Current implementation status |
|------|-------------------------------|
| [Durable acquisition failures](https://github.com/Ebenderooock/loom/issues/174) | Persistent acquisition failure/retry history and recovery exist; continue real-world failure testing. |
| [Explainable matching and policy](https://github.com/Ebenderooock/loom/issues/171) | Actual-engine decision evidence and read-only policy previews exist. Ambiguous multi-season packs remain rejected; general mapping previews are not implied. |
| [Request history versus availability](https://github.com/Ebenderooock/loom/issues/172) | Separate fulfillment/presence, missing-media re-requests and storage-aware reconciliation exist; further reconciliation refinements postdate `v0.4.1`. |
| [Independent version tracks](https://github.com/Ebenderooock/loom/issues/169) | [Proposed ADR-0027](../adr/0027-independent-version-tracks.md); not implemented. |
| [Provider-aware API budgets](https://github.com/Ebenderooock/loom/issues/173) | Persistent allowances, shared-account groups and reset signals are implemented. |
| [Read-only library health](https://github.com/Ebenderooock/loom/issues/170) | Read-only untracked/missing/possible-duplicate findings and disk evidence are implemented; no general automatic cleanup. |

These are capability observations, not changes to issue state or assertions
that every original acceptance criterion is complete. Open tracking issues
can include remaining validation even when their core workflow exists.

### Independent version-track implementation slices

[ADR-0027](../adr/0027-independent-version-tracks.md) is proposed, not an
implemented capability. Maintainer acceptance precedes coding; each slice uses a
separate branch/PR. Additional tracks remain gated until the whole chain passes.

| Slice | Issue | Prerequisites |
|-------|-------|---------------|
| Ownership and migration | [#176](https://github.com/Ebenderooock/loom/issues/176) | ADR acceptance |
| API and compatibility | [#178](https://github.com/Ebenderooock/loom/issues/178) | #176 |
| Acquisition isolation | [#180](https://github.com/Ebenderooock/loom/issues/180) | #176, #178 |
| Import, naming and packs | [#179](https://github.com/Ebenderooock/loom/issues/179) | #176, #178 |
| Deletion and reconciliation | [#181](https://github.com/Ebenderooock/loom/issues/181) | #179 |
| UI and activation gate | [#182](https://github.com/Ebenderooock/loom/issues/182) | #178, #180, #179, #181 |

These are child implementation slices of #169, not replacements for the other
five intake items. Language/edition activation, shared files, temporary-copy
automation and general retention remain outside the resolution-track MVP.

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
| 3 | [Library Maintenance](./phase-3-library-maintenance.md) | Read-only health and orphan-download cleanup implemented; general maintenance proposed |
| 4 | [Media Server Analytics](./phase-4-media-server-analytics.md) | Basic session/history/reporting implemented; richer analytics proposed |
| 5 | [Script Engine](./phase-5-script-engine.md) | Admin JavaScript event plugins implemented; broader engine/SDK proposed |
| 6 | [Script Marketplace](./phase-6-script-marketplace.md) | Proposed, not implemented |

---

## Original Proposal Order

This diagram records the original design sequence, not today's remaining
queue. Core requests, health reporting, analytics and event scripts already
exist; use the root roadmap for what remains.

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
