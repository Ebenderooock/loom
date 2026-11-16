# Migration from Radarr / Sonarr / Prowlarr

Loom includes a limited importer for existing Radarr, Sonarr and Prowlarr
SQLite databases. It is not a full settings migration or a guaranteed
drop-in cutover. Test against a separate Loom SQLite database first.

## Implemented scope

| Source | Records the importer attempts to copy |
| ------ | ------------------------------------ |
| Radarr | Movie catalog, root folders, basic quality-profile records |
| Sonarr | Series, seasons/episodes, root folders, basic quality-profile records |
| Prowlarr | Indexer configuration records |

Quality-profile records do not reproduce the original quality rules. Download
clients, custom formats, history, lists, tags, notifications and blocklists are
not imported. Indexer records need manual verification and connection testing;
copying a record does not prove its configuration works in Loom.

## Run the importer

Stop Loom and back up its state first. Take consistent copies of the source
databases using their apps' backup procedure. Point the config at your test
SQLite database, then run one source at a time:

```sh
loom migrate --config /path/to/test/loom.yaml --from radarr --db /path/to/radarr.db
loom migrate --config /path/to/test/loom.yaml --from sonarr --db /path/to/sonarr.db
loom migrate --config /path/to/test/loom.yaml --from prowlarr --db /path/to/prowlarr.db
```

There is **no `import` subcommand or `--dry-run` flag**. The command applies
Loom schema migrations and writes imported records to the configured database.
It opens source databases read-only.

Inspect the printed summary and logs for errors: individual tables or rows can
fail while others are imported. Existing IDs may be skipped; do not assume
re-running performs a complete synchronization. Verify root paths, monitoring,
profiles, metadata and indexer connectivity in the UI before allowing searches
or imports. Keep the original databases and media backups until validation is
complete, and do not let both applications modify the same library during testing.

See the [roadmap](../ROADMAP.md) for the
remaining compatibility and migration work.
