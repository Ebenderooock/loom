# Collections

Collections live at **Collections** (`/collections`), outside Discover and
Libraries. A Loom-owned collection can combine movies and TV shows from any
library and titles not yet added to a library. A title can belong to any number
of collections. Membership does not imply monitoring, imported files, or
availability on a media server.

## Creating and maintaining collections

Administrators can:

- **New collection**: create a local grouping or import an external source.
- Collections are found **automatically in the background** from TMDB movie
  sets and official TVDB lists declared by library titles. **Rescan library** is
  optional: it queues another scan without waiting for provider requests.
  Discovery does not claim complete franchise coverage. Import known source IDs
  directly when a provider does not declare the grouping.
- **Add titles**: search your library or TMDB and select one or more real
  movies/shows. Library titles attach by their Loom ID; provider titles attach
  by exact TMDB ID with canonical artwork. Titles already in the collection are
  marked and never duplicated. Membership alone does not acquire or monitor.
- **Add to collection** from movie/show detail drawers or the library bulk
  selection bar: choose an existing collection or create one by name, then
  attach the selected titles. Admin only.
- **Find match**: unresolved source rows (no usable TMDB ID) stay visible and
  open the picker prefilled with their title. Loom never invents media.
- **Manage** (one side sheet): name, description and quality for new
  additions; sources and refresh; merge; delete.
- **Quality for new additions** is a default only. It preselects the profile in
  single and bulk add. **Apply to existing members** previews each affected
  library title with its current and new profile and changes only those
  titles after confirmation; other collections' defaults are untouched.
- **Add missing / Search** work on selected titles or the whole collection.
  Both open a preview with eligible/skipped counts and reasons (in library,
  unresolved, no library, unreleased, not monitored, already has file).
  Nothing changes until confirmed. Add uses the shared add settings
  (library, profile, monitor, search after adding); Search only searches
  monitored library titles that are missing files (series respect episode
  monitoring) through the existing automatic search, and never adds titles.
  Confirmed runs execute as background jobs with per-title results and retry.
- **Delete collection**: after confirmation, removes only the collection, its
  sources and membership. Library titles and files are never touched. Its
  exact source references are suppressed so background discovery does not
  recreate it; importing a source again explicitly lifts that suppression.
- **Exclude locally**: hide a source member without deleting library media.
  Use the **Locally excluded** status filter to restore it.
- **Refresh sources**: reconcile current provider membership. Local additions,
  local title overrides, and exclusions survive. Successful sources update
  independently; a failed source retains its previous members and displays an
  actionable error and its last successful synchronization time. Refresh is
  explicit/on-demand, not a new periodic import-list acquisition job.
- **Merge collections**: choose another collection, review the destination,
  and explicitly confirm. Sources and members are combined in one transaction.
  Exclusion wins if either collection excludes a member; destination local
  metadata wins over incoming metadata. The removed collection ID continues to
  resolve to the retained collection, including after subsequent merges.

Similar names or overlapping titles never imply collection identity. An exact
`(provider, source kind, external source ID)` belongs to one Loom collection.
If a reference is already attached elsewhere, open that collection or manually
merge it. Collections can have multiple references from the same provider.
TMDB movie-collection IDs and TMDB list IDs are distinct namespaces.

Member identity is scoped by movie versus show and exact external IDs. Provider
enrichment can bridge IMDb/TVDB/TMDB IDs; it never uses title-name search.
Contradictory mappings are reported, not silently joined. Refresh provenance
tracks current source membership separately from historical contributing sources.
Unmatched titles remain external members.

## Sources and access

| Source | Accepted ID | Access / limitations |
|---|---|---|
| TMDB movie collection | positive numeric collection ID | Existing `LOOM_TMDB_API_KEY` / bundled metadata key. TMDB collections cover movie sequels, not shows or entire universes. |
| TMDB mixed list | positive numeric v4 list ID | `LOOM_TMDB_READ_TOKEN` must be a TMDB v4 API read-access token. All pages are fetched through the shared import-list adapter. Existing v3 import lists remain compatible. |
| TVDB list | positive numeric list ID | Existing `LOOM_METADATA_TVDB_APIKEY` / bundled key and optional `LOOM_METADATA_TVDB_PIN`; uses the metadata client's login/token refresh and rate-limit handling. Movie and series IDs are read from list entities and extended title records. |
| IMDb public list | `ls…` | Reuses Loom's existing public IMDb import-list adapter. Lists must be accessible; upstream access restrictions/errors are surfaced. IMDb free datasets do not provide franchise membership, and no new scraping endpoint or completeness claim is introduced. |

TMDB's external-ID lookup supports IMDb movies/shows and TVDB shows, **not TVDB
movie IDs**. A TVDB movie can still be a collection member; a TVDB-only movie
needs an IMDb/TMDB mapping before the shared external details/add flow can load
it. The API returns an explicit 422 for an unsupported TVDB-only movie lookup.
Unresolvable identities or unavailable metadata show errors rather than adding
a title guessed by name.

Source inputs accept IDs only, not arbitrary URLs or per-source credentials.
Credentials stay in server metadata configuration, not collection records or
responses. Refreshes are bounded to 90 seconds; source payloads are bounded to
5,000 movie/show members and collection aggregates to 8 MiB. Oversized or
incomplete sources fail visibly instead of committing partial membership.

## Library details and filtering

Click any member's poster/title. Added titles open the normal movie/series
management sheet using the **actual local media ID**, including settings,
monitoring, files and search controls. External members open shared title
details and the same explicit add form as other title entry points. Adding,
editing, refreshing, or deleting a library title refreshes collection statistics
while retaining collection context.

The optional library filter matches members actually added to that library;
external/missing members are visible with **All libraries & external titles**.
Media and status filters can be combined. “Has imported files” is a detail-level
filter because file status is checked on detail reads, not guessed on summary
cards. Collections reads follow existing authenticated library access;
collection management and source imports require an administrator.

## API and persistence

See `api/openapi/loom.yaml` for native `/api/v1/collections` endpoints.
Collection IDs are Loom UUIDs, never provider numeric IDs. List responses and
detail responses use the same persistent collection shape; detail reads also
compute current file status. Merged stale IDs return the canonical collection's
`id`, and the UI offers its retained link.
Writes through a merged stale ID return 409, even if an old revision number
happens to match the retained collection; reload the canonical collection first.

Bulk add/search jobs run in memory (deduplicated per title, last 50 per
process). They are not persisted across restarts; rerun the action after a
restart. Migration `0095_collection_suppressions.sql` stores deleted source
references and adds the collection `quality_profile_id` default.

SQLite and PostgreSQL migration `0093_collections.sql` stores collection
aggregates (members, sources, provenance and local intent), relational unique
source ownership, and merge redirects. Revisions and transactions prevent stale
edits/refreshes from overwriting concurrent work. API edits and merges require
the current revisions and return 409 on stale state or conflicting identities.

### Background discovery

The existing scheduler runs `collections.discovery` every 15 seconds, including
after startup. Each run reconciles the complete library (movies are paginated
in 1,000-row pages), then processes up to 16 due titles with a 90-second batch
budget and 15-second per-title budget. Movies and shows are interleaved so a
large movie library cannot starve shows. New titles and changed external IDs
join the queue automatically; removed titles leave it.

Migration `0094_collection_scan.sql` stores per-title checkpoints, retry errors
and a cross-process lease. Restarts resume unfinished titles instead of
repeating the beginning of the library. Successful titles are checked again
after 24 hours; failures retry after five minutes. Requests are paced in
addition to provider-client throttling, and shutdown cancels in-flight work.
The Collections page polls `/api/v1/collections/scan` for checked/total/pending
counts and actionable per-title errors. Scheduler failures are also recorded by
the normal scheduler job history. Discovery requires the scheduler to be
enabled; `Rescan library` only queues work, never runs provider calls in an HTTP
request.

This queue governs finding collections, not refreshing existing collection
members. **Refresh sources** still explicitly reconciles full attached sources,
with its existing payload bounds and preservation of local intent. Titles
without a supported exact lookup remain visible as discovery issues rather
than being silently skipped or matched by name.

The old `/api/v1/discover/collections` endpoints remain read-only legacy TMDB
movie-set views for compatibility. The standalone UI and recommendation links
use the new persistent API.
