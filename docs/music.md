# Music

Loom's Music capability manages artists, albums, editions, tracks and audio
files using MusicBrainz metadata. Enable the `music` feature flag to expose the
Music API and UI.

## Artist catalogue

The artist catalogue includes studio albums, EPs and singles with at least one
MusicBrainz release explicitly marked **Official**. Different editions of the
same release group appear as one entry, not as separate albums.

Live releases, compilations, remixes, broadcasts, demos and other secondary
release types are excluded for now, even when officially released. Bootleg,
promotional, pseudo-release and unknown-status editions do not qualify a group
for inclusion.

For artists already added, use **Refresh** on the Music page to apply the
catalogue rules, then reload after the background refresh finishes.
Monitored artists are also refreshed periodically.
Excluded entries no longer appear in the artist catalogue, its counts, automatic
searches or the calendar. Their album, track and file records are retained;
refreshing does not delete audio files. A failed metadata fetch leaves the
existing catalogue unchanged.

## Profiles

Audio quality profiles control which audio tiers are eligible for search, the
upgrade cutoff, and custom-format scores. List the built-in tier definitions
with `GET /api/v1/music/audio-quality-definitions`; create, update and list
profiles under `/api/v1/music/audio-quality-profiles`.

For example, a profile can allow MP3 320 kbps and lossless FLAC, use FLAC as its
cutoff and permit upgrades:

```json
{
  "name": "Lossless preferred",
  "items": [
    { "definition_id": "aq_mp3_320", "allowed": true },
    { "definition_id": "aq_flac", "allowed": true }
  ],
  "cutoff": "aq_flac",
  "upgrade_allowed": true
}
```

The `format_items` list associates custom-format IDs with scores, and
`min_format_score` sets the minimum score a release must reach. Updates change
only supplied fields. A profile cannot be deleted while assigned to an artist.

Metadata profiles filter which MusicBrainz primary types, secondary types and
release statuses are monitored for an artist. Create and manage them under
`/api/v1/music/metadata-profiles`, then assign one when adding or updating an
artist. Profiles control monitoring within the supported catalogue; they do not
reintroduce excluded release types. An empty primary-type list permits all
supported primary types; when an album has
secondary types, each must be included in the profile's secondary-type list.
An empty release-status list uses MusicBrainz's `official` status preference
when choosing an edition.

## Album editions

An album is a MusicBrainz release group and may have multiple concrete
editions. The album response includes its known `releases` and
`selected_release_id`. Select an edition by sending its release ID:

```http
PUT /api/v1/albums/{album_id}/release
Content-Type: application/json
```

```json
{ "release_id": "musicbrainz-release-id" }
```

The selected edition must belong to the album. Choosing another edition
refreshes the album's track listing.

## Imports and upgrades

Music downloads are imported by matching audio tags to a monitored artist,
album and track. Files are organized under the artist's configured music
library. When a track already has a file, an incoming file replaces it only
when its detected quality is higher. The previous file is sent to the recycle
bin before replacement; upgrades are refused if the recycle bin is unavailable
or disabled, so the existing track is not silently discarded.

Tracks are matched by disc and track number first, then by title. If the
titles differ only by a trailing qualifier, such as `Heavy (feat. Kiiara)`
against `Heavy` or `Heavy (album version)`, the file still matches when exactly
one track fits. An album's track list is fetched from MusicBrainz the first
time it is needed. If MusicBrainz is unavailable, the import fails with
`album track list unavailable` rather than a track-match error, and a later
retry or library scan fetches the list again.

## Metadata source

Loom reads artist, album and track data from MusicBrainz. Admins choose the
source under **Settings → Music Profiles → Metadata Source**:

- **Public MusicBrainz** (default) uses `https://musicbrainz.org/ws/2`. The
  public service allows about one request per second per IP address, so adding
  a large artist or refreshing many albums takes minutes. Loom always waits at
  least one second between requests to `musicbrainz.org`, including retries.
- **Custom server** uses any MusicBrainz-compatible web service: your own
  mirror or a hosted instance. Enter the base URL ending in `/ws/2`, the
  minimum gap between requests in milliseconds (`0` for no limit), and an
  optional authentication header such as `Authorization: Bearer …` for hosted
  services. The header value is write-only: the API reports only whether one
  is saved, and leaving it blank keeps the saved value.

Use **Test connection** before saving. It looks up a known artist and, when
your server also handles search, runs an artist search. Saved changes apply
immediately without a restart.

Artist search uses MusicBrainz's search indexes. A mirror built without them
can still serve lookups but not search, so turn on **Search artists on public
MusicBrainz** to send only searches to the public service.

### Running your own mirror

The [musicbrainz-docker](https://github.com/metabrainz/musicbrainz-docker)
project runs a replicated MusicBrainz server. Its published requirements are:

| Setup | Disk | Memory | CPU |
| --- | --- | --- | --- |
| Database only (lookups; use public search) | 100 GB | 4 GB | 2 threads |
| With search indexes | 350 GB | 16 GB | 16 threads |

Point Loom at the mirror's web service, for example
`http://musicbrainz.lan:5000/ws/2`, with a rate limit of `0`. MusicBrainz core
data is CC0, but replication access and commercial use are governed by the
[MetaBrainz terms](https://metabrainz.org/supporters); check them before
offering a hosted instance to others.

See the [API reference](api.md) and
[`api/openapi/loom.yaml`](../api/openapi/loom.yaml) for request and response
schemas for Music routes.
