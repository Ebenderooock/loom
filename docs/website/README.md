# Loom docs site

This directory will host the public documentation site (Docusaurus 3,
mounting [`../*.md`](..) as the docs sidebar). The site is **not** yet
checked in — it is scheduled for **Phase 11** alongside the 1.0 release.

## Why no install is committed yet

A full Docusaurus install adds ~250 MB of `node_modules`, pulls in a
React-DOM toolchain unrelated to the app frontend in `web/`, and
duplicates lint/typecheck pipelines. We deferred it to keep the
repo small until the docs surface is broad enough to warrant it. The
underlying Markdown lives at `../*.md` and renders fine on GitHub
today.

## Bootstrap when ready

```bash
cd docs/website
pnpm dlx create-docusaurus@latest . classic --typescript
# Edit docusaurus.config.ts so presets.docs.path = '..' to mount the
# existing markdown files at the site root, then:
pnpm install
pnpm start
pnpm build
```

The site should:

- Mount `docs/*.md` directly (no content duplication).
- Generate the API reference from
  [`../../api/openapi/loom.yaml`](../../api/openapi/loom.yaml) using
  `docusaurus-plugin-openapi-docs`.
- Publish to GitHub Pages via a CI workflow (added in Phase 11).

## Why this README instead of a real install

The original task allowed either a working Docusaurus skeleton or this
README; we picked the README for two reasons:

1. **Build-time reliability.** A fresh Docusaurus 3 install in a
   restricted environment often hits `node-gyp`, registry, or
   peer-dependency issues unrelated to the project.
2. **Cost.** ~250 MB of `node_modules` for a site that today would
   render five pages is poor value before Phase 11.

When Phase 11 starts, run the bootstrap above and commit the result.
