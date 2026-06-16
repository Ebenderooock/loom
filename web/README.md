# Loom Web

React + TypeScript frontend for [Loom](../README.md).

## Stack

- Vite + TypeScript (strict, `noUncheckedIndexedAccess`)
- TanStack Router (code-based) + TanStack Query v5
- shadcn/ui + Tailwind CSS v3 + lucide-react
- react-hook-form + zod
- Vitest + Testing Library, Playwright e2e
- Storybook 8

## Commands

```sh
pnpm install
pnpm dev            # http://localhost:5173 (proxies /api/*, /healthz, /readyz, /livez, /metrics → :1925)
pnpm typecheck
pnpm lint
pnpm test
pnpm build
pnpm storybook
```
