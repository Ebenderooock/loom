# Grow your library

Open **Discover** to find movies and series to add, rather than to choose
something already in your library to watch. **Import Lists are optional**:
Loom uses your existing catalog to identify interests and queries
[TMDB](https://www.themoviedb.org/) for titles you do not already have.

## Grow your library

Movie suggestions draw on missing collection members, repeated actors and
directors, genres (including romance and comedy together), and related titles.
Series suggestions also use repeated cast/creators, related series and genre
discovery. Each card explains why it was suggested. Candidates rank first by
the number of distinct supporting reasons, then by source (collection,
director, actor, related title, genre), rating, popularity and TMDB ID as a
stable tie-breaker. These are explainable catalog heuristics, not a trained
personal taste model.

Already-added titles are excluded even when they are unmonitored or have no
imported files. Adult and undated entries are excluded. By default,
recommendations also exclude future releases; select **Include upcoming** to
include them.
**Dismiss** persists a title's exclusion for your authenticated user, not for
everyone sharing the instance.

Select **Details & add** to open shared full title details and the normal add form. Choose
the target library, quality profile and monitoring settings, then confirm.
**Search after adding is off by default for suggestions**; enable it explicitly
if you want an immediate search. Browsing, viewing collections and dismissing
suggestions never add, monitor, search for or download a title.

You need a matching movie/series library and a quality profile before adding.
Existing list-based discovery remains available under **Discover > Import
Lists** and is independent of these suggestions.

## Collections

**Collections** is now a standalone destination outside Discover and Libraries.
It supports persistent mixed movie/show groupings, multiple simultaneous source
references, local membership and exclusions, and explicit manual merges.
Recommendation **View collection** links open that destination; an unsaved TMDB
source can be explicitly imported by an administrator. Merely following a link
does not mutate the collection or acquire any title.

See the [Collections guide](collections.md) for source access requirements,
identity rules, library-management details, synchronization and limitations.

## Provider access, freshness and privacy

Discovery uses Loom's existing TMDB client and credentials
(`LOOM_TMDB_API_KEY`, with the application's bundled default when available).
It needs network access to TMDB but does not require an Import List, Trakt
account, or a Plex/Jellyfin connection. If provider access fails, the page
shows an error and retry action instead of presenting the outage as an empty
library.

Candidate generation samples up to 16 library TMDB IDs, with up to 8 movie
sets, 4 person/role sources, 4 related-title lookups and 3 genres (plus a movie
rom-com combination). Repeated people must occur in at least two sampled
titles; actor selection considers the first ten cast members. At most 60
recommendations are returned. This is not exhaustive coverage of a large
library or the whole TMDB catalog.

Each provider lookup has a two-second deadline within a twenty-second total
request/queue deadline. Up to 512 catalog responses are cached in memory for
six hours. Transient failures may reuse cached data less than seven days old;
authentication errors, invalid requests and missing resources never use stale
fallbacks. Current library membership and user dismissals are applied when
serving results. Successful additions refresh the page's discovery queries.
Partial-source failures and stale cached results are surfaced as warnings.

TMDB requests contain catalog IDs, people and genre filters derived from your
library. This is server-side catalog lookup, not an upload of your media files
or a scan of watch history. Dismissals are stored in Loom's database.

This product uses the TMDB API but is not endorsed or certified by TMDB.

## API

All paths below are under `/api/v1/discover` and require a signed-in user or a
user-backed API key. Anonymous access and shared configuration keys without a
user identity are rejected; there is no shared anonymous preference bucket.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/recommendations?media_type=movie&include_upcoming=false` | Movie or series candidates with reasons. |
| `POST` | `/dismissals` | Persist `{ "tmdb_id": 123, "media_type": "movie" }` for the current user; returns `204`. |
| `GET` | `/collections` | Legacy read-only TMDB movie-set view; deprecated for new clients. |
| `GET` | `/collections/{id}` | Legacy TMDB set detail; use the standalone `/api/v1/collections` API for persistent mixed collections. |

Recommendation and collection responses include `warnings`, `generated_at` and
`stale`. Arrays are present even when empty. Invalid media types, booleans and
IDs are rejected rather than silently coerced. The pre-existing
`GET /people/{id}` filmography endpoint is unchanged.

Provider failures return `502`, missing configuration returns `503`, and
deadline/cancellation returns `504` when no response can be served. Internal
storage failures return `500`; an actual absent collection returns `404`.

See the [OpenAPI specification](../api/openapi/loom.yaml) for response schemas.
