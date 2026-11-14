# ADR-0033: Persistent file-level import queue

- Status: Proposed
- Date: 2026-11-11
- Deciders: @loom-maintainers

## Context

A download can contain an entire season with both successful and failed
episode imports. A two-slot concurrency semaphore rejected extra manual
submissions rather than queueing them, and download-level outcomes could not
explain or recover partial imports. Restarting Loom also lost queued manual
work.

## Decision

Persist manual import jobs and individual file outcomes in SQLite. Separate
admission from bounded worker concurrency, retry only unsuccessful files, and
recover interrupted jobs on startup. Match season-pack files independently.
Certify import skipping and source deletion using the current source and
destination content, not a download ID or path alone. Read-only scans use
saved metadata to display explicitly historical hints without hashing payloads.
Keep client-blocked jobs waiting without spending
their retry budget; safety-held jobs require review approval before retry.
Retain the workflow orchestrator as the owner of download-triggered imports.

For bare `Episode N` filenames, resolve the series from exact cleaned folder
context within the download root, or its title when the root is the download-ID
directory. Never override an unidentified named pack with a requested title. Strip
complete-series descriptors and broadcast date ranges, not sequel names.
Explicit season folders use local episode numbers; an unqualified single
regular season uses its unique episode numbers without requiring correctly
spelled descriptive titles. Multi-season absolute numbering requires complete,
contiguous regular-season metadata and matching episode titles. Exclude specials
from inferred absolute order. Unknown subfolders and duplicate series identities
require manual matching; grab targets never override these contextual decisions.
Deterministic no-library-match failures stop automatic workflow retries, while
metadata service failures remain errors eligible for retry.

Quality decisions retain missing source/resolution tags from a download release
only when it identifies the matched series/season/episode or movie. Explicit
filename evidence takes precedence, and the resolved quality is registered with
the imported file so later upgrades can compare it. Unknown quality remains
unknown: allow it without a known existing quality only when the profile permits
every seeded quality tier (or explicitly permits unknown). A profile named Any
with an excluded tier does not bypass validation, and unknown incoming quality
cannot replace a known-quality file.

## Consequences

### Positive

- Users can submit entire scans and inspect per-file progress and errors.
- Successful files survive partial failures and restarts without duplicate work.
- Torrent removal or temporary client outages do not discard recoverable imports.

### Negative / trade-offs

- Copy-mode import skipping and source deletion require fresh full-content
  verification; hardlinks use filesystem identity. Scans only compare saved
  metadata and label copies as Previously imported, not content-verified.
- Migration 0101 adds queue tables and source fingerprints to import history.
- Migration 0102 adds destination metadata snapshots; legacy copies without
  those snapshots are not automatically hidden by scans.
- Manual matching imports individual media files and their subtitles/extras,
  rather than moving unrelated loose NFO and image files with a folder.

### Neutral

- Finished jobs expire after 30 days; pending safety reviews remain available.
- Existing import submission routes remain asynchronous and return a job ID.

## Alternatives considered

- Increase the semaphore capacity: still rejects work and loses it on restart.
- Retry whole downloads: obscures partial failures and repeats successful work.
- Treat source paths or equal file sizes as success: can hide replaced sources
  and missing or corrupt destinations.
