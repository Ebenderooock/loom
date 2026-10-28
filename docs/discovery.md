# Grow your library and Collections

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

Select **Add to library** to use the normal movie or series add dialog. Choose
the target library, quality profile and monitoring settings, then confirm.
**Search after adding is off by default for suggestions**; enable it explicitly
if you want an immediate search. Browsing, viewing collections and dismissing
suggestions never add, monitor, search for or download a title.

You need a matching movie/series library and a quality profile before adding.
Existing list-based discovery remains available under **Discover > Import
Lists** and is independent of these suggestions.

## Collections

**Discover > Collections** automatically identifies TMDB movie sets from
movies already added to Loom. Open a collection to see its members in release
order, gaps, and upcoming entries, then add individual missing movies through
the same explicit add dialog.

The detail view distinguishes:

- **Added to Loom**: a movie catalog record exists, with monitoring shown
  separately.
- **Imported file recorded** or **No imported file recorded**: whether Loom
  records a movie file. A catalog entry alone does not mean a playable file is
  available; a recorded file is not a fresh filesystem accessibility check.
- **Missing** and **Upcoming**: entries not yet added are split into
  released/undated gaps and future releases. Both can be explicitly added.

Membership comes from TMDB, not title matching or studio names. A TMDB film
series such as Saw is not the same as the entire Disney catalog or every
movie in the Marvel Cinematic Universe. This version does not create custom
collections, automatically follow future releases, or write collection
membership to Plex/Jellyfin. It does not group TV series into movie sets.

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
| `GET` | `/collections` | Movie collections represented in the existing catalog. |
| `GET` | `/collections/{id}` | One TMDB collection with current Loom membership, file and monitoring state. |

Recommendation and collection responses include `warnings`, `generated_at` and
`stale`. Arrays are present even when empty. Invalid media types, booleans and
IDs are rejected rather than silently coerced. The pre-existing
`GET /people/{id}` filmography endpoint is unchanged.

Provider failures return `502`, missing configuration returns `503`, and
deadline/cancellation returns `504` when no response can be served. Internal
storage failures return `500`; an actual absent collection returns `404`.

See the [OpenAPI specification](../api/openapi/loom.yaml) for response schemas.
