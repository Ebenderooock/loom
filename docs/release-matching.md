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

## Clearing inactive import entries

In **Downloads > Imports**, use **Dismiss** on one inactive queue entry, select
entries and choose **Dismiss selected**, or choose **Clear inactive** to hide
all inactive entries, including older entries outside the current page. This
works even when the source download has already been deleted.

Dismissal keeps the job's outcomes, import history, safety reviews, downloads,
and media files. Queued, importing, and retry-scheduled jobs are not dismissed.
Selected dismissal fails without changing any entries if one selected job is
active or no longer exists. Explicitly retrying a dismissed job restores it to
the queue; re-enqueuing the same review-held request restores its original review
hold, not a bypass around approval.

## Custom-format points and imports

Custom formats are matching rules, not new base-quality definitions. A release
such as `the.body.2026.s01e07.1080p.web.h264-cakes[EZTVx.to].mkv` has base quality
`webdl-1080p`. Matching a custom format does not rename that quality or override
the profile's allowed-quality check. Source and codec aliases such as
`WebDL`/`WEB-DL` and `h264`/`x264` match consistently in testing, search and import.

### One quality decision from search to import

Search decides a release's quality once, when it grabs the release, and stores
that decision on the download's workflow. Import reuses it instead of reading
the quality from the file name again. When a file name contradicts the grabbed
release, such as a different resolution inside a pack, import records the
file's own quality using the same detector as search.

Import never rejects a download Loom grabbed because of its quality: search
already checked it against the profile, the existing file, and the custom-format
minimum. If the file still falls outside the profile, it is imported and the
workflow timeline shows a **quality warning**.

A release you download yourself from the search dialog overrides the quality
profile. It is imported even when the profile disallows its quality, and the
workflow timeline warns when it is outside the profile or below the cutoff.

Only downloads Loom did not grab (added directly in the download client or
imported from a folder) are checked against the profile at import.

A release name that states a resolution but no source, such as
`Chicago PD S14E06 1080p HEVC x265-MeGusta`, is **assumed** to be WEB-DL at that
resolution (remuxes are assumed BluRay). The assumption shows as
"(assumed)" in search results and decisions and in the workflow's grab event.
The stored file quality is the assumed one, for example `webdl-1080p`. A profile
with custom tiers first tries an enabled **Unknown** tier or a matching custom
tier, then the assumed tier. If the assumed tier is disabled, an allowed Unknown
tier still accepts a first import with no existing file; it never authorizes
replacing an existing file. For downloads Loom did not grab, known-source
exclusions and custom-format minimums still apply.

## Custom quality tiers

In **Quality Profiles > Manage custom tiers**, create a tier with matching rules.
For example, add Resolution `1080p`, Codec `x265`, and Source `unknown` to
match an HEVC release with no parsed source. Add the tier to the profile, enable
it, and use **Upgrade order** to position it relative to other tiers (best
first). The cutoff can target any enabled tier. This does not enable Unknown
generally or change the parsed source to WEB.

Rules use the custom-format matcher: different rule types must all match; rules
of the same type are alternatives unless marked required. Negation is supported.
An explicitly disabled known-source quality cannot be bypassed by a custom
matcher. If multiple allowed tiers match, the first in profile order wins.
Custom formats still supply points independently, and the minimum score remains
an additional acceptance check. The format tester shows the selected profile
tier as well as detected source and score; search evaluation and import previews
use the same tier rules.

Loom stores a stable tier ID for an imported file, so renaming or editing its
matcher does not silently reclassify that existing file. Reordering the profile
changes future comparisons and cutoff evaluation. A tier referenced by a
profile or imported file cannot be deleted; remove its profile references and
file registrations first. If an existing custom tier is absent from the current
profile, replacement is rejected rather than guessed. Tiers have no size limit
of their own; when a real known-source definition matches, its size limits still
apply. File-only and indexer-only matcher rules are not supported for tiers
because that evidence is not consistently available at both search and import.

Assign points to a format in the quality profile used by the movie or series.
For example, a matching WEB format worth 100 points and a matching codec penalty
worth -15 total **85 points**. A profile minimum of 90 rejects that result; a
minimum of 85 accepts its format score. Formats absent from the profile add zero.
Imports and previews check the assigned profile's current minimum in addition
to base-quality and downgrade checks. Rule and score edits apply to later
imports without restarting Loom. File-info conditions also check downloaded
file metadata; a title-only test cannot prove those conditions.

In **Custom formats > Test formats**, select a saved quality profile to see its
actual points and minimum outcome. Without a profile, the tester shows matches
only. In the profile editor, quality selection and format scores remain in one
scrollable body with Save/Cancel accessible below it. Negative points are valid.
If an earlier failed save left scores without format IDs, the editor warns you
to re-enter the affected scores. Saving removes those invalid entries while
preserving valid assignments. Profile settings and scores now save together;
a rejected or failed save leaves the stored profile unchanged.

The minimum is an acceptance threshold, not an upgrade target or a substitute
for quality order. Format-score cutoff storage is not currently used to trigger
same-tier automatic upgrades; the editor does not expose it as a working cutoff.

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
