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

See the [API reference](api.md) and
[`api/openapi/loom.yaml`](../api/openapi/loom.yaml) for request and response
schemas for Music routes.
