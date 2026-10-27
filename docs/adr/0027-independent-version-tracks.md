# ADR-0027: Independent media version tracks

- Status: Proposed (revision 2)
- Date: 2026-10-23; revised 2026-10-27
- Deciders: @loom-maintainers

## Context

[Issue #169](https://github.com/Ebenderooock/loom/issues/169) requests permanent,
concurrent versions of a title without separate Loom instances. 1080p and 4K is
the most common request, but it is not the only separation users need: 4K HDR/DV
beside 4K SDR, a remux beside a compact encode, original audio beside a dubbed
language, or a theatrical cut beside a director's cut. Multiple file records are
not independent acquisition policies.

Revision 2 (maintainer review) replaces three revision-1 choices:

1. Tracks are no longer limited to disjoint *resolution* membership; they are
   defined by a general, validated **version selector**.
2. File identity no longer depends on a reserved `[vt-<uuid>]` filename suffix.
   Other tools (media servers, renamers, manual reorganisation) can rename files,
   so ownership is anchored to a reproducible **content fingerprint** instead.
3. Choosing versions is specified explicitly: quality profiles stay single-select
   and a title gets one or more tracks, usually from a reusable **version set**. This ADR proposes the contract
and implementation sequence; it does not deliver the feature or approve a
production migration. Maintainers accept the decision before implementation.

### Implementation evidence

These observations refer to baseline `2627f5b`
([PR #175](https://github.com/Ebenderooock/loom/pull/175)), not the older
architecture page's aspirational package boundaries:

| Surface | Current implementation and constraint |
|---|---|
| Catalog | `internal/movies/types.go` has one movie quality profile and status; `internal/series/types.go` has one series profile, episode monitoring and `HasFile`. Movie and episode file rows have no version-track owner. |
| Storage | Media migrations live under `internal/storage/migrations/{sqlite,postgres}`. Libraries have a profile and one root path (`internal/libraries/types.go`); `library_files` indexes physical paths, not version policies. |
| Workflows | `internal/workflows/store.go` initializes its own tables in `migrate()`. Items identify only media type/ID, with primary key `(workflow_id, media_id)`. Active-download uniqueness is separate from media claim lookup. `types.go` includes post-download and cleanup states; older workflow prose is not the full state machine. |
| Acquisition | `internal/autosearch/engine.go` checks media-wide active workflows and existing quality. Episode quality lookup falls back to `HasFile`. `internal/scheduler/store.go` selects missing movies/episodes and keys `search_state` by media type/ID. |
| Import | `internal/imports/pipeline.go` matches via workflow or filename, builds movie `Title (Year).ext` and episode `Title - S01E02.ext`, then updates file records. `enrichCandidateQuality` uses the first movie file and lacks episode file quality lookup. Manual preview has separate destination/comparison logic. |
| Rename | `internal/organizer/config.go` separately renders quality-based movie names. `{Edition Tags}` currently renders empty despite edition parsing in `internal/parser`. Organizer naming is not the automatic import naming contract. |
| Lifecycle | `internal/libraries/store.go` reconciles removed paths; library settings include unmonitor-on-delete and watched archival. `internal/cleanup` handles orphan downloads, not a general version-retention service. |
| Consumers | Request availability uses media-level file predicates in `internal/requests/availability.go`. Radarr/Sonarr adapters expose scalar profiles and availability. React movie/series detail sheets and `release-search-dialog.tsx` assume media-level operations. |

Baseline import safety rejects unsafe destinations and preserves files after a
library-update failure. Version support must retain those guarantees, not
reintroduce overwrite behavior through a new naming or replacement path.

## Decision

Introduce an immutable version-track ID owned by a movie or series and make
every acquisition target `(media_type, media_id, version_track_id)`. Keep one
backfilled default track for existing clients. Opt-in additional tracks represent
permanent copies distinguished by a version selector (resolution, dynamic range,
source, codec, audio language, edition). They do not share files, replacement
candidates, monitoring or completion. Owned files are identified by content
fingerprint, not by path or name. Build and verify
the ownership foundation before enabling additional tracks.

### Identity and scope

A **version track** is an acquisition and retention policy, not a quality name,
file, download, language profile or music track. A movie owns its tracks. A
series owns tracks inherited by its episodes; seasons are selection scopes,
not new track owners. Metadata IDs, episode ordering and catalog identity stay
unchanged. A version track cannot be moved to another owner.

Use an opaque UUID as identity, a mutable display label (`1080p`, `4K HDR`) and
a mutable naming suffix. The UUID never appears in filenames; files are linked to
tracks by content identity. Renaming a label or editing a profile never creates
a new identity or renames files implicitly. Exactly one
default track exists per owner. The default cannot be removed or switched in
the MVP.

A track's **acceptance** is its quality profile's allowed qualities intersected
with its version selector. Quality profile ordering, cutoff, custom formats and
size constraints rank releases *inside* that acceptance set. A release outside a
track's acceptance is never a candidate for it: a 4K release is not an upgrade for
a permanent 1080p track, and a dubbed release is not an upgrade for an
original-language track.

A **version selector** is a typed set of dimension constraints. Each dimension is
either unconstrained (`any`) or an explicit include list:

| Dimension | Values (normalised) | Evidence source |
|---|---|---|
| Resolution | 480p, 576p, 720p, 1080p, 2160p | Parsed release / quality; file probe when available |
| Dynamic range | SDR, HDR10, HDR10+, Dolby Vision, HLG | `internal/mediainfo` parsing; file probe when available |
| Source | Remux, Blu-ray encode, WEB-DL, WEBRip, HDTV, DVD | Quality source / parser |
| Video codec | AVC, HEVC, AV1, other | Parser; file probe when available |
| Audio language | ISO 639 codes, with `original` meaning the title's original language | Parser language detection; file probe when available |
| Edition | Theatrical, Director's Cut, Extended, Unrated, IMAX, Remastered, custom normalised names | `internal/parser` edition extraction |

Selectors are evaluated by one shared function used by search, RSS, import,
manual import, scanner assignment and preview, so every surface agrees on which
track a release or file belongs to. Dimensions whose evidence is unknown never
satisfy an include list; the item goes to review instead of guessing.

Enabled tracks of one owner must be **mutually exclusive**: no release may be
accepted by two tracks. Validation checks this statically: two tracks are
disjoint when at least one dimension has non-overlapping include lists (for
example resolution `1080p` versus `2160p`, or dynamic range `SDR` versus
`HDR10, HDR10+, Dolby Vision`). If no dimension separates them, the policy is
rejected with the overlapping combination shown; it is not resolved by implicit
priority. The default track keeps unrestricted legacy membership until another
track is added; adding a track requires constraining the default in the same
revision-checked transaction, so intermediate overlapping policies are never
enabled. A retained default file outside its new acceptance stays owned and
playable (`available`, `below_cutoff`, reason `policy_mismatch`); it is never
reassigned or deleted automatically. The preview requires acknowledgment of this
state before activation.

Multi-language releases (`MULTi`, dual-audio) are evaluated per language: a
release matching more than one track's language list is ambiguous and rejected
for automatic acquisition rather than counted twice. Shared files across tracks
remain out of scope (see Alternatives). Music and books are out of scope.

Initial selector rollout order: resolution and dynamic range first (needed for
1080p/4K and 4K HDR/SDR), then source/codec, then language and edition once the
file-level evidence and ambiguity fixtures pass. Unsupported dimensions are
rejected by the API rather than stored as inert fields.

A **temporary copy** is a lower-quality fallback within one eventual target,
eligible for same-track replacement when its desired quality arrives. It is not
a second permanent track and never satisfies another track. No new temporary-copy
policy, expiry, transcoding or cross-track garbage collection ships in this MVP;
existing single-track upgrade behavior remains.

### Choosing versions: profiles, tracks and version sets

Quality profiles do **not** become multi-select. A profile already means "accept
these qualities and upgrade toward the cutoff"; selecting several profiles on one
title would be ambiguous (keep all? keep the best?). Instead, a title has one or
more tracks, and **each track picks exactly one quality profile** plus its
selector and label:

```text
Dune: Part Two (2024)
  Track "1080p"      profile: HD-1080p        selector: resolution=1080p          monitored
  Track "4K HDR"     profile: Ultra-HD        selector: resolution=2160p,
                                                         range=HDR10|HDR10+|DV    monitored
  Track "4K SDR"     profile: Ultra-HD        selector: resolution=2160p,
                                                         range=SDR               not monitored
```

To avoid configuring tracks per title, introduce reusable **version sets**: a
named, ordered list of track templates (label, profile, selector, monitored,
naming suffix). Examples: `Standard` (one unrestricted track, today's
behaviour), `HD + 4K`, `4K HDR + 4K SDR`, `Original + English dub`.

- Libraries choose a default version set; adding a title creates its tracks from
  it. The existing "quality profile" picker on add/edit becomes a "Quality" picker
  offering either a single profile (one default track, unchanged behaviour) or a
  version set.
- Titles can deviate: add, remove or edit a track locally. Once edited, the title
  stops following the set's future changes for that track.
- Changing a version set previews affected titles and applies per-title,
  revision-checked; it never deletes files or silently retargets existing tracks.
- Track templates are matched to existing tracks by template ID, never by label.

### Proposed persistence contract

The following are proposed logical records/fields, **not shipped SQL**:

| Record | Required contract |
|---|---|
| Version track | UUID; typed owner movie/series; label; naming suffix; default flag; monitored flag; quality-profile reference; version selector; optional version-set/template origin; policy revision; timestamps. Enforce owner existence and exactly one default transactionally, with database uniqueness for at-most-one default. |
| Target state | Movie/episode ID plus version-track ID; per-episode track monitor override; selected file ID; availability evidence/time; policy revision. A selected file must belong to that target; no cross-owner references. |
| Version set | Named ordered track templates (label, profile, selector, monitored, naming suffix); library default reference. Edits are revisioned and previewed; they never cascade deletes. |
| File identity | Per owned file: size, fast fingerprint, full content hash (nullable until computed), hash algorithm/version, device+inode and mtime observed at hashing, verified-at time. Indexed by fingerprint and by full hash. |
| File ownership | Add version-track ownership to existing movie/episode file records. Multiple historical files remain possible; select at most one current file per target. Do not add uniqueness on media ID or discard duplicates. |
| Workflow items/claims | Include version-track ID in item identity, indexes and active target claims. Acquire one durable unique claim per target transactionally before dispatching a grab. Claims release on completion/cancellation/failure; retry reacquires and revalidates policy. Preserve unique active client/download ownership. |
| Search state | Extend cooldown/search count identity to include version-track ID. Keep indexer quota accounting at actual admitted provider queries, not per track. |
| Evidence/history | Track ID and policy revision on search decisions, import records, replacement/recycle manifests, workflow events and notifications; legacy historical entries may remain explicitly unknown. |

Use typed owner relations or equivalent database constraints rather than relying
on an unchecked polymorphic ID. A series track can target only that series'
episodes. A current file belongs to one target; path uniqueness and same-path
idempotency must validate ownership, not treat any uniqueness error as success.
`library_files` remains a physical inventory: connect its path to the owned file
record without treating an inventory row as a second media copy.

Profiles remain reusable `internal/qualityprofiles` resources. A track references
one profile; do not duplicate the older movie profile implementation. Resolve
initial defaults from the current effective media/library configuration. After
backfill, the default track is authoritative and legacy media profile fields are
transactional compatibility projections. Library profile changes retain their
existing defaulting semantics, not silently rewrite all tracks.

### Migration and rollout

1. Add nullable ownership and supporting records with the multi-track capability
   disabled. Use paired goose migrations for SQLite/Postgres; explicitly migrate
   workflow tables currently initialized by `Store.migrate()`. Fresh installation
   and upgrade must converge; do not assume goose already owns these tables.
2. Backfill one default track per movie/series; attach existing file rows,
   episode targets and search state without moving, renaming or deleting files.
   Preserve media IDs, profiles, monitoring and historical search timestamps.
   For multiple files choose a deterministic current candidate from verified
   live files (existing quality ranking, then ascending file ID); retain every row.
   Unknown quality/presence is reported, not guessed into cutoff completion.
3. Drain active imports before switching writers. Backfill legacy workflow items
   to the default target and validate claims before resuming; conflicting active
   work stays paused for review. Legacy history may stay unassigned. Make
   backfill transactional/resumable and verify row counts and owner integrity.
4. Switch all writers/readers to ownership-aware services and enforce non-null
   ownership for new media file/target writes. Old binaries are not compatible
   writers after this step. Do not enable extra tracks while a legacy scanner,
   bulk handler, compatibility adapter or job can mutate media-wide state.
5. Enable opt-in additional tracks only after the release matrix below passes.
   Single-track installations keep current paths, API shapes and default behavior.

Back up database and filesystem manifests before conversion. Rollback before
multi-track activation can restore the backup with no file movement. After extra
tracks exist, disable new acquisition and use a read-only recovery/export path;
do not flatten tracks through a down migration. Restoring the original snapshot
also requires reconciling subsequently imported files. No destructive downgrade
or implicit duplicate cleanup.

### API and backward compatibility

Add native version-track collection operations under movie/series resources and
target-state reads for movie/episode resources. Exact routes and schemas belong
in the API implementation issue and must update `api/openapi/loom.yaml` and
`docs/api.md`; no route described here exists yet.

Search/evaluate/grab, manual import/match/reimport/preview, rename, workflow and
file operations gain an explicit `version_track_id` (existing camelCase workflow
DTOs use `versionTrackId`). Validate owner, authorization, enabled capability,
policy revision and monitoring before work is admitted. Missing IDs on legacy
requests resolve **only to the default**, never the first matching track. Invalid,
foreign or disabled IDs return explicit errors; stale policy/preview returns
conflict rather than silently switching tracks.

Retain existing scalar profile/status/`has_file` fields as default-track
projections, including Radarr/Sonarr adapters. Existing legacy monitor/profile
writes affect only the default; whole-title delete still explicitly means the
whole title and all tracks, with confirmation in the native UI. Add separate
native summary fields rather than redefining old booleans to mean all tracks.
Legacy file lists may retain all records with additive ownership; any mutation
checks the concrete file owner and cannot use a media-wide replacement shortcut.
Arr file/availability projections and commands select only the default.

Request creation without a track ID and current availability reconciliation
remain default-track scoped. Track-selectable requests and preservation of
historical fulfillment belong to
[issue #172](https://github.com/Ebenderooock/loom/issues/172), not this feature.
The tracks implementation only adapts that issue's current contract to avoid
cross-track fulfillment; it does not create a second request-history project.

### Monitoring, search and upgrade isolation

Effective acquisition monitoring is owner lifecycle eligibility AND track
monitoring AND the target's episode selection. Owner archival/deletion and an
explicit native whole-title stop are master stops. Episode selection starts from
season/episode monitoring; a nullable per-target override inherits when null,
includes when true and excludes when false. Inclusion cannot bypass owner/track
stops or airing eligibility. Existing series monitor modes initialize selection
as today. Legacy monitor writes update only the default track/target selection
after migration, not a sibling's master stop; scalar monitor reads project that
default. Native whole-title controls explicitly say they stop all tracks.
Unmonitor preserves files.

Manual and automatic searches resolve identity from the catalog, then select the
track policy. Rolling missing/cutoff-unmet candidates, cooldown, RSS matching,
season/series fallback, pack wanted sets, active-grab badges and retry eligibility
all use target identity. Do not use `HasFile`, first movie file or media-wide
active-workflow lookup as a multi-track decision. Reuse the current lexicographic
quality-tier/custom-format/tiebreaker ranking; do not replace it with display score.

Reserve target claims before client dispatch to prevent simultaneous duplicate
grabs. Two different tracks may acquire concurrently. One client download has
one owning workflow; if deduplication returns an existing download for another
target, report conflict rather than stealing ownership. A workflow's items may
cover many episodes of **one** series track. Retry restores that same target and
policy revision, re-evaluating changed policy instead of resetting another track.
Durable failure taxonomy/backoff work remains
[issue #174](https://github.com/Ebenderooock/loom/issues/174); provider budgets remain
[issue #173](https://github.com/Ebenderooock/loom/issues/173).

### Import, naming, packs and hardlinks

Resolve catalog identity and track ownership separately. Workflow-linked files
inherit the claimed track, but still pass identity/year and track membership
checks. Unlinked manual imports/scans on a multi-track title require explicit
track selection or an unambiguous validated assignment; unknown/ambiguous
resolution goes to review. Never infer ownership solely from a mutable label.
Single-track legacy calls continue selecting the default.

Share one destination plan between automatic import, manual preview/execution,
reimport, scanner registration and organizer rename. Keep existing default-track
paths unchanged.

**Filenames are presentation, not identity.** Ownership is never inferred from a
path, label or suffix, so a rename by a media server or another tool cannot move
a file to another track or orphan it (see *File identity* below). Non-default
tracks append their human-readable naming suffix in the multi-version form that
Plex and Jellyfin already group as versions of one title:

```text
Movie (2024)/Movie (2024).mkv                    default track (unchanged)
Movie (2024)/Movie (2024) - 2160p HDR.mkv        track "4K HDR"
Show/Season 01/Show - S01E02 - 2160p HDR.mkv     track "4K HDR"
```

Suffixes are sanitised and must be unique per owner (case-insensitively). They
may come from the label or from naming tokens (`{Quality}`, `{HDR}`,
`{Edition}`, `{Audio Language}`). Preview fails if any two tracks would produce
colliding paths after templating, sanitising and truncation; truncation never
removes the suffix. Changing a label or suffix renames only through an explicit
organizer preview. Subtitles and extras follow the same basename. Previews list
existing paths, ownership, replacement candidate and policy revision; execute
revalidates that plan.

### File identity and rename tolerance

Every owned file has a reproducible content identity so Loom can find it again
after an external rename or move:

- **Fast fingerprint**, computed at import/registration: SHA-256 over file size
  plus fixed-size samples (first, middle and last 4 MiB). Cost is bounded
  regardless of file size, so scans of large libraries stay cheap.
- **Full content hash** (SHA-256 of the whole file), computed when the bytes are
  already being read (copy imports) or later by a throttled background verifier
  for move/hardlink imports and backfilled files. Until it exists, the fast
  fingerprint plus size is the lookup key; matches that rely only on the
  fingerprint are confirmed against the full hash before any destructive action.
- Device, inode and mtime recorded at hashing time are a cheap change detector:
  an unchanged inode/mtime/size is not re-hashed on every scan.

Scanner and import reconciliation uses identity in this order:

1. Known path with unchanged size/inode/mtime: same file, no hashing.
2. Known path whose content changed (size/mtime differ and fingerprint differs):
   the owned file was replaced or edited externally; mark the target `unknown`
   and send the file to review. Never treat it as the same version silently.
3. Unknown path whose fingerprint (and, when available, full hash) matches an
   owned file whose recorded path is missing: treat it as a rename/move inside
   the owner's root, update the path and inventory, keep the track, and log it.
4. Unknown path matching nothing: an unowned file. On single-track titles it is
   registered to the default as today; on multi-track titles it is assigned only
   when the shared selector puts it in exactly one track, otherwise review.
5. One hash at two paths (hardlinks or copies) never proves two tracks are
   satisfied; the second path is reported as a duplicate of the owning track.

Missing or unmounted roots keep identity records untouched and yield `unknown`.
Hash mismatch after an in-place metadata edit (for example `mkvpropedit` tag
changes) is expected; it is surfaced for one-click re-confirmation rather than
reassigned automatically. Identity records are deleted only with the owned file
record. Optional extended attributes may cache the track ID for speed but are
never authoritative, because copies and some filesystems drop them.

### Import safety

An import may replace/recycle only the selected same-track file after verification
of the new file and durable ownership update. Filesystem and database writes
cannot be one transaction: record an operation manifest, stage a uniquely named
new file, verify it, commit ownership, then recycle the old owned path. Recovery
must preserve at least one usable copy and resume idempotently after every crash
boundary. Never overwrite or recycle an unowned/foreign destination. Preserve
baseline exclusive-create, symlink rejection and failed-move recovery. Rename
must update ownership/inventory references without changing the track.

The import slice defines a durable manifest with `planned`, `staged`, `verified`,
`ownership_committed`, `old_recycled`, `complete` and explicit `needs_review`
states. It records target/track/revision, old and new file IDs/paths and the old and new
content fingerprints before side effects, so recovery can locate files by
identity even if they were renamed between crash and restart. The import service's startup recovery owns
resumption before accepting imports: verify staged content, reconcile committed
ownership, then recycle only a still-owned old path. Pre-commit recovery keeps
the old selection; post-commit recovery keeps the new selection. An ambiguous
identity or inaccessible root pauses that operation in review without deleting
either copy. One target claim covers execution and recovery.

Respect existing move/copy/hardlink/hardlink-only modes. Hardlink upgrades create
new directory entries; never edit a shared inode in place or truncate a seeding
source. Cross-device `hardlink_only` is an explicit failure; retain existing
fallback semantics for `hardlink`. Do not treat two paths to one inode as proof
that two tracks are satisfied; cross-track shared-file ownership is deferred.

Packs compute wanted episodes for the selected series track and use per-file
catalog mapping, never apply a single item's identity to every file. Persist
per-target import outcomes, report partial completion and retry only unresolved
targets. Mixed-resolution, unmapped or conflicting pack members stay untouched
for review; no other track becomes completed. Keep a pack's source until all
owned members are safely imported and download-client seeding rules permit
cleanup. Catalog-aware mapping explanations remain
[issue #171](https://github.com/Ebenderooock/loom/issues/171).

### Retention, deletion and reconciliation

Permanent tracks are not duplicates for automatic cleanup. A higher-quality
track never authorizes deletion of a lower-resolution permanent track.
Retention settings inherit current library behavior for the default and expose
explicit per-track opt-in changes later; no new automatic retention policy is
part of this MVP.

File delete, recycle restore, scan reconciliation, watched archival and
unmonitor-on-delete operate on the owning target. Removing its file makes only
that target missing and follows its monitor policy; it does not clear sibling
claims or files. Missing/unmounted roots yield unknown availability and inhibit
destructive reconciliation. Removing a non-default track defaults to stopping
acquisition and preserving files; explicit purge previews list only owned paths.
Keep archived ownership records while retained files/history refer to them.
Whole-title/library operations must enumerate all affected tracks rather than
accidentally cascade through one selected track.

Orphan download cleanup must check all workflow/file owners before deleting a
source. General duplicate/health reports must group by track, not label permanent
copies as disposable duplicates. Read-only health/report UX remains
[issue #170](https://github.com/Ebenderooock/loom/issues/170); this design supplies
ownership predicates, not a new cleanup engine.

### State, UI and operational monitoring

Return independent fields per target: `monitored`, availability
(`missing`, `available`, `unknown`), policy satisfaction
(`below_cutoff`, `at_cutoff`, `unknown`), acquisition phase and last error.
An available file can remain playable while an upgrade downloads or fails.
Unknown metadata never means at-cutoff; unknown filesystem accessibility never
means confirmed missing.

For series, completeness uses aired, effectively monitored episode targets with
the existing `internal/airing` rules. Return numerator/denominator for each track.
No eligible aired episodes is not a completed series. A pack is partial until
every claimed target has verified import success; a generic workflow success
cannot substitute for those results.

Native movie/series detail sheets expose track rows/tabs with label, profile,
monitoring, missing/available/upgradable state, progress, files and errors.
Series episode tables show a selected track and explicit partial counts;
library cards summarize `1/2 monitored tracks available` rather than a misleading
green downloaded badge. Summary means all eligible targets have live files;
cutoff completion is separate. Search dialog, Activity, manual import/preview,
rename and delete confirmation retain a visible track context through submission.
Adding a second track requires a policy/collision preview and explicit opt-in.
Keyboard/screen-reader labels include the track, not only a color or icon.

Logs/events and search/import decisions include track ID, target ID, policy
revision and outcome. Link them to workflow/download IDs for diagnosis. Extend
existing diagnostics rather than duplicate #171. Expose bounded aggregate counts
for missing/unknown/available targets and import conflicts; avoid UUID/filename
Prometheus labels. Track IDs belong in logs/traces. Failed backfill or ownership
checks stop activation with actionable counts, not success-shaped fallbacks.

### Required implementation verification

These are acceptance fixtures for the child issues, **not tests run by this
documentation PR**. Test the real engine, repository and filesystem paths.

| Scenario | Required observation |
|---|---|
| Upgrade existing installations | SQLite/Postgres fresh and upgrade fixtures converge; all rows/paths and default behavior survive, including multiple files, unknown quality, interrupted backfill and active workflows. |
| Concurrent targets | Two tracks (for example 1080p and 4K, or 4K HDR and 4K SDR) can search/grab concurrently; simultaneous same-target admission creates one claim; client deduplication and restart cannot steal it. |
| Cross-track quality | A 2160p result cannot satisfy/replace 1080p; only selected-track file quality participates in cutoff and import ranking. |
| Partial series/pack | One track/episode succeeds and another fails; only successful owned targets become available, cooldown/retry and counts remain independent. |
| Naming and manual work | Automatic/manual/preview/reimport/rename agree; per-track suffixes survive custom templates and truncation; existing/foreign files, case collisions and symlinks fail without data loss. |
| External rename | A file renamed or moved within the owner's root by another tool keeps its track and target after rescan; an externally replaced or edited file goes to review; copies/hardlinks are not counted as a second track. |
| Selectors | Resolution, dynamic range, source, codec, language and edition fixtures assign each release/file to at most one track; overlapping policies are rejected; MULTi/dual-audio and unknown evidence go to review. |
| Version sets | Adding a title from a set creates its tracks; editing a set previews and applies per title without deleting files or retargeting by label. |
| Hardlinks and crash recovery | Same-device links preserve source inode contents; cross-device hardlink-only fails; injected failures at staging/DB/recycle boundaries preserve recoverable files and operation ownership. |
| Deletion and reconciliation | Delete/archive/unmonitor/rescan/recycle/cleanup touch only selected ownership; inaccessible roots remain unknown; permanent siblings survive. |
| Compatibility and UI | Legacy API/Arr commands and requests use default only; foreign IDs fail; stale previews conflict; UI shows track-specific partial/cutoff/upgrade state with accessible labels. |
| Gate disabled | No extra-track creation or dispatch; normal single-track import, scheduler, requests and UI keep their current behavior and paths. |

### Delivery plan and approval gates

Implementation issues are linked below. Each is a separate
reviewable branch/PR; this design PR never implements them. Dependencies are
contract prerequisites, not priority ordering. #169 closes the design scope,
not delivery of concurrent tracks.

| Slice | Issue | Blocks on |
|---|---|---|
| Ownership migration: schema/backfill/repository/default projections, file identity records and fingerprint backfill | [#176](https://github.com/Ebenderooock/loom/issues/176) | ADR acceptance |
| API and compatibility: selector/disjointness validation, version sets, DTOs, default-only adapters | [#178](https://github.com/Ebenderooock/loom/issues/178) | #176 |
| Acquisition: shared selector evaluation, claims, search/RSS/rolling/upgrade/retry isolation | [#180](https://github.com/Ebenderooock/loom/issues/180) | #176, #178 |
| Import and naming: owned plans, per-track suffixes, fingerprint at import, hardlink safety, pack outcomes | [#179](https://github.com/Ebenderooock/loom/issues/179) | #176, #178 |
| Lifecycle: identity-based rename detection, scanner/delete/recycle/archive/cleanup boundaries (including the #195 file manager) | [#181](https://github.com/Ebenderooock/loom/issues/181) | #179 |
| UI and release gate: per-track UX, version-set picker and activation matrix | [#182](https://github.com/Ebenderooock/loom/issues/182) | #178, #180, #179, #181 |

Ownership precedes API. Acquisition and import each require ownership and API.
Lifecycle requires import ownership. UI/release integration requires API,
acquisition, import and lifecycle. No sibling intake issue is duplicated or made
an artificial blocker; integrate their merged contracts when implementation
begins. Do not activate the capability until all six slices pass.

### Decisions requiring maintainer acceptance

The defaults above let implementation planning proceed without guessing. Review
must explicitly accept or revise these substantive choices before coding:

- Tracks are defined by a version selector over resolution, dynamic range,
  source, codec, audio language and edition; enabled tracks of a title must be
  provably disjoint, and shared files across tracks are deferred.
- Quality profiles stay single-select; a title has one or more tracks, each with
  one profile, normally created from a library's default version set.
- File ownership is anchored to a content fingerprint/hash, not filename;
  externally renamed files are re-identified, externally modified files go to
  review.
- Additional tracks stay under the owner's existing library root and use
  Plex/Jellyfin-style `Title - <suffix>` names, rather than separate roots.
- Legacy/Arr scalar state and requests project the immutable default track, not
  any-track or all-track availability.
- Post-activation rollback is recovery/export or snapshot restore, not flattening;
  retained files keep archived track ownership.

## Consequences

### Positive

- Permanent version ownership (resolution, HDR, source, language, edition) is
  explicit throughout acquisition and deletion.
- Ownership survives renames by media servers or other tools.
- Existing installations keep one default policy and unchanged paths.
- Partial completion and unknown evidence become observable, not hidden by flags.

### Negative / trade-offs

- More targets mean more storage and searches; existing quota admission still
  applies. A claim reservation and crash-safe file manifest add persistence work.
- Hashing costs I/O: bounded for fingerprints, but full hashes of large
  libraries need a throttled background job and storage for identity records.
- Selector evidence parsed from release names can be wrong or missing; unknown
  dimensions send items to review, which is safer but adds manual work until
  file probing is available.
- Every legacy media-wide mutation must be adapted before multi-track activation.

### Neutral

- This is a proposed clean-room design grounded in Loom, not copied upstream code.
- No new schema, route, feature flag, retention engine or UI is implemented here.

## Alternatives considered

- **Separate instances:** isolates policies but duplicates configuration and
  prevents unified status; remains a workaround, not the proposed model.
- **More allowed qualities or file rows:** cannot express independent ownership,
  monitoring, workflow claims or retention; still permits destructive replacement.
- **Clone catalog titles per quality:** duplicates metadata/episode relationships
  and breaks external-ID compatibility instead of isolating policy.
- **One physical file satisfies several tracks:** useful for future overlapping
  language policies, but requires reference-counted deletion and shared-content
  evidence. Deliberately deferred rather than hidden inside MVP.
- **Reserved `[vt-<uuid>]` filename suffix (revision 1):** deterministic but
  lost when another tool renames the file, ugly in media servers, and still
  needs a recovery path. Rejected in favour of content identity.
- **Full-file hash only:** strongest identity, but hashing every multi-GB file on
  each scan is too slow. Kept as the verification layer behind a fast
  fingerprint.
- **Extended attributes or sidecar files as identity:** cheap, but lost on copy,
  on some filesystems and network shares, and editable by other tools. Allowed
  only as a non-authoritative cache.
- **Multi-select quality profiles:** conflates "accept several qualities and
  upgrade" with "keep several versions". Rejected; tracks with one profile each
  express the intent directly.
- **Track-specific roots immediately:** useful for media-server libraries, but
  introduces root migration and mount policy. Defer pending actual demand;
  per-track suffixes plus content identity solve collisions without moving files.
