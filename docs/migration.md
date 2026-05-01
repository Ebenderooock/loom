# Migration from Radarr / Sonarr / Prowlarr

> **Stub.** Migration tooling lands in **Phase 8**. This page will
> document the `loom migrate import` workflow once it ships.

Planned scope (per the project plan, Phase 8):

- Indexers
- Download clients
- Quality profiles
- Custom formats
- History
- Library (movies / series)
- Lists
- Tags
- Notifications
- Blocklist
- Root folders

The CLI shape (preview, subject to change):

```bash
loom migrate import --from radarr   --db /path/to/radarr.db   --dry-run
loom migrate import --from sonarr   --db /path/to/sonarr.db   --dry-run
loom migrate import --from prowlarr --db /path/to/prowlarr.db --dry-run
```

Migration is **idempotent**: re-running with the same input produces no
extra rows. A side-by-side mode will let Loom run alongside a live arr
instance for verification before cutover.
