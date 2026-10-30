# Collections

Collections live at **Collections** (`/collections`), outside Discover and
Libraries. A Loom-owned collection can combine movies and TV shows from any
library and titles not yet added to a library. A title can belong to any number
of collections. Membership does not imply monitoring, imported files, or
availability on a media server.

## Creating and maintaining collections

Administrators can:

- **New collection**: create a local grouping or import an external source.
- **Find from library**: discover TMDB movie sets and official TVDB list
  candidates declared by library titles. This bounded action checks up to 32
  titles and reports if additional titles were not checked. It does not claim
  complete franchise coverage. Import other source IDs directly.
- **Edit collection**: edit its name/description, attach another source, or add
  a local movie/show by exact external title ID. Local membership does not
  acquire or monitor the title.
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

SQLite and PostgreSQL migration `0093_collections.sql` stores collection
aggregates (members, sources, provenance and local intent), relational unique
source ownership, and merge redirects. Revisions and transactions prevent stale
edits/refreshes from overwriting concurrent work. API edits and merges require
the current revisions and return 409 on stale state or conflicting identities.

The old `/api/v1/discover/collections` endpoints remain read-only legacy TMDB
movie-set views for compatibility. The standalone UI and recommendation links
use the new persistent API.
