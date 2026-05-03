# ADR-0010: Indexers & Proxies management UI

- Status: Accepted
- Date: 2025-01-15
- Deciders: @loom-maintainers

## Context

ADR-0009 introduced the backend for indexers and proxies (CRUD, health
probing, fan-out search, OpenAPI contract). Phase 2g calls for a
management UI in `web/` so operators can configure these resources without
hitting the API directly.

The frontend skeleton already chose its tools: React 18 + Vite,
TanStack Router (code-based), TanStack Query v5, Tailwind, and a small set
of shadcn/ui-style primitives over Radix (Dialog, DropdownMenu, Label,
Tabs). `react-hook-form` and `zod` are listed in `package.json` but were
not yet used. Notably, **`@radix-ui/react-select` is not installed** —
the design system relies on native `<select>` elements styled with
Tailwind for compactness and to avoid a portal-based combobox where one
isn't needed.

The UI must round-trip the OpenAPI contract faithfully, including:

- The PATCH null-vs-omit semantics for `proxy_id` (null detaches; omitting
  leaves unchanged).
- The `409 proxy_in_use` error when deleting a proxy attached to indexers.
- Health badges sourced from `health.status` plus a 24 h staleness rule.

The brief mentioned a per-indexer search endpoint
(`POST /api/v1/indexers/{id}/search`) but the OpenAPI spec only defines
the fan-out `POST /api/v1/indexers/search`.

## Decision

Build the Indexers and Proxies pages as small composable React components
with the following choices:

1. **TanStack Query** for all server state (indexers list, proxies list,
   create/update/delete/search). One query key per resource;
   `invalidateQueries` on mutation success.
2. **Plain `useState` for forms**, not `react-hook-form`. The forms are
   small (~10 fields), validation is straightforward, and explicit local
   state keeps the null-vs-omit PATCH logic legible. Keeping
   `react-hook-form` available for larger future forms is fine.
3. **Native `<select>` styled with Tailwind** for kind/proxy pickers. No
   `react-select` dependency; matches the existing skeleton.
4. **Pure helper functions** (`mapHealth`, `validateIndexerForm`,
   `toCreatePayload`, `toPatchPayload`, `valuesToConfig`,
   `maskUrlCredentials`) so the tricky bits — health staleness, PATCH
   diffing, proxy_id null-vs-omit, credential masking — are unit-tested
   without a DOM.
5. **Manual search via the fan-out endpoint scoped to one indexer**:
   send `POST /api/v1/indexers/search` with `indexer_ids: [id]` until and
   unless a per-indexer endpoint lands in the OpenAPI contract.
6. **Typed fetch wrapper** (`indexers-api.ts`) parses the API's
   `{error: {code, message}}` envelope into a custom `ApiError` carrying
   status + machine-readable code so callers can render specific messages
   (e.g. for `proxy_in_use`).
7. **Accessibility baseline** baked into every form: `<Label htmlFor>`,
   `aria-invalid` + `aria-describedby` on invalid inputs, `role="alert"`
   on error messages, `role="status"` on the health badge, focus traps
   from Radix Dialog.

## Consequences

### Positive

- One stack, no new dependencies. Builds, tests and types stay clean
  under `verbatimModuleSyntax` + `noUncheckedIndexedAccess`.
- The PATCH null-vs-omit and credential-masking edge cases are unit-tested
  pure functions, so regressions surface in `pnpm test` not in production.
- TanStack Query's cache + invalidation handles optimistic refresh after
  mutations without per-page bookkeeping.
- Accessibility is consistent across both pages because the same form
  primitives are reused.

### Negative / trade-offs

- Two form code paths exist (`useState` here, `react-hook-form` planned
  later). If we adopt RHF for larger forms we'll need to migrate these
  forms or accept the inconsistency.
- Native `<select>` doesn't support search/typeahead. If the proxy list
  grows beyond ~20 entries we'll want a combobox, which means pulling in
  `@radix-ui/react-select` or building one over `cmdk`.
- The manual-search flow uses the fan-out endpoint, so the wire shape
  is slightly heavier than a per-indexer endpoint would be; if/when the
  API gains `/api/v1/indexers/{id}/search` we should switch.

### Neutral

- Forms remain plain controlled components — easy onboarding for new
  contributors, no third-party form library to learn.

## Alternatives considered

- **`react-hook-form` + `zod` resolver everywhere.** Rejected for these
  forms because they're small and the PATCH null-vs-omit logic is clearer
  with explicit local state. Re-evaluate when forms get larger or share
  many fields.
- **Build a Radix-Select combobox.** Rejected for now; native `<select>`
  is sufficient at current resource counts, keeps the bundle smaller, and
  matches the rest of the skeleton.
- **Server-driven search via a per-indexer endpoint.** Not currently in
  the OpenAPI contract. Using the fan-out endpoint scoped by
  `indexer_ids` is a one-line swap if the contract changes.
- **SWR / RTK Query.** TanStack Query is already a project dependency and
  works well with TanStack Router; no reason to add another data layer.
