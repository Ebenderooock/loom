# Explain release matching

Loom shows the decision made by its acquisition engine, not a browser-side
approximation. In a movie or series manual search, expand **Explain matching /
preview policy** after results arrive. In Search Log, expand **decision evidence**
on a recorded result. Older log entries retain their original rejection code but
cannot recover evidence that was not recorded.

## Choose a release

Manual search keeps the query and compact tools above a vertically scrolling
results area. Release metadata wraps beneath each title; **Download** stays in
the right-hand column on desktop and beneath each release on phones. Expand
**Indexer details** for individual indexer progress or the warning banner for
reported errors. Filters and **Explain matching / preview policy** remain
optional and do not change the search query or start a download.

Click **Download** to send that release to the configured client, or choose a
client when several are enabled. The action shows **Sending…** while pending and
**Sent** after the client accepts it; **Sent** does not mean the media download
has completed. Failed submissions show an inline error and can be retried.
Repeated clicks cannot submit the same pending or successful row again.

Blocklisted releases remain hidden by default. **Ignore blocklist** reveals
them with a **Blocklisted** badge and explicitly permits a manual blocklist
override. Policy previews remain read-only and do not override grab safeguards.

## Read a decision

Identity evidence shows the requested title and alternate titles, embedded IDs,
year and requested numbering next to what the parser found. An ID-filtered query
alone is not proof of identity. A matching normalized title or embedded ID must
corroborate the result; a conflicting embedded IMDb/TMDB ID rejects it even when
the title matches. Movie years retain the existing one-year tolerance.

The parser interpretation includes season, first episode, multi-episode numbers,
absolute anime number, daily date and the season-pack marker. `-1` means numbering
was not parsed. An absolute number such as `Naruto - 017` does not silently become
`S02E05`. Supported relative numbering such as `Naruto.S02E05` remains usable.
Names with insufficient season evidence stay rejected.

The summary distinguishes incorrect identity, ambiguous identity, unsupported
naming, configuration mismatch and availability. Evidence rows are observations,
not independent vetoes: an absent embedded ID can be corroborated by the title.
The first rejection stops evaluation; later quality or format checks are absent
rather than guessed.

Quality position is **zero-based, lower is better**. Accepted results are ordered
by quality tier, then custom-format points (higher is better), then the engine's
seeders/age/size/freeleech tiebreaker. The ranking explanation names the first key
that differs from the adjacent accepted result and reports both values. Exact
ties keep input order. The composite score and the raw indexer search score are
display values, not an alternate ordering rule. An accepted result can rank first
and still fail to download; the log's outcome records grab failures separately.

For example, a tier-0 BluRay result beats a tier-1 WEB result even if WEB has
50,000 more custom-format points. At the same tier, a preferred-group match worth
100 points beats a match worth 50. Contributions use this profile's scores, not
generic custom-format defaults. A matched format absent from the profile adds 0.

Size checks display expected and actual **bytes** after applying the quality
definition's absolute or per-minute limit. Runtime is in minutes; unknown runtime
uses the existing per-minute fallback. A size of 0 means unknown and is not
size-filtered. The minimum format comparison displays expected and actual
**points**.

## Preview without saving

The manual-search panel starts with the media item's assigned profile. Select
another saved profile or type an unsaved minimum format score to preview that
policy against the same results. For example, raising the minimum from 0 to 100
rejects a release with 50 points. **Reset to assigned policy** discards the preview.

Previews call the same evaluation method as production searches, including
existing-file upgrade/cutoff checks. They do not save a profile, search additional
indexers, start a download, or change identity rules. Pending and failed
evaluations are shown explicitly rather than retaining a previous policy's
answer. The original manual-grab table remains separate from this read-only panel.

The API can submit a full unsaved profile to
`POST /api/v1/autosearch/evaluate` as `policy_preview`. Its `items` field is a
JSON-encoded array, just like the native quality-profile API. Only quality-profile
policy is replaced: quality definitions and custom-format matching rules remain
the currently loaded production definitions. Invalid quality arrays/duplicate
IDs return HTTP 400. See [API](api.md#release-evaluation-and-policy-previews).

## Catalog-aware pack mapping: discovery boundary

No correction or catalog-coverage claim is applied by this release. The existing
`packs.Detect` parser recognizes season ranges and complete-series labels, while
`packs.EpisodesFromPack` infers episodes from season totals rather than inspecting
the archive. That is not sufficient evidence that a download contains the
requested season. Multi-season and complete-series results therefore remain
ambiguous and are rejected by the acquisition engine; a positively parsed single
season pack still follows the existing checks.

The catalog has season/episode entries, anime absolute-to-relative mappings and
alternate episode-order mappings (`internal/anime`, `internal/episodeorder`).
These are separate catalog models, not proof of a pack's contents. A future
catalog-aware preview would need to display source ordering, catalog version,
explicit per-episode targets and all conflicting/missing candidates. Any proposed
correction must be opt-in, reversible and separately persisted with its source;
it must not be a title/season override or authorize a grab of an ambiguous pack.
Sports naming and automatic cross-catalog corrections are outside this slice.
