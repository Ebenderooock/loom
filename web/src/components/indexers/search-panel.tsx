// SearchPanel runs a manual search against a single indexer (via the
// fan-out endpoint scoped with `indexer_ids`) and renders the
// aggregated results in a sortable-ish table.

import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  searchIndexers,
  type Indexer,
  type SearchResult,
  ApiError,
} from "@/lib/indexers-api";

function formatBytes(n?: number): string {
  if (typeof n !== "number" || !Number.isFinite(n) || n < 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 10 || i === 0 ? 0 : 1)} ${units[i]}`;
}

function formatAge(iso?: string): string {
  if (!iso) return "—";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "—";
  const diff = Date.now() - t;
  const sec = Math.max(1, Math.floor(diff / 1000));
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  const d = Math.floor(hr / 24);
  if (d < 30) return `${d}d`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return `${mo}mo`;
  return `${Math.floor(mo / 12)}y`;
}

export function SearchPanel({
  indexer,
  onClose,
}: {
  indexer: Indexer;
  onClose?: () => void;
}) {
  const [q, setQ] = React.useState("");
  const [categories, setCategories] = React.useState<string>(
    (indexer.categories ?? []).join(", "),
  );
  const [results, setResults] = React.useState<SearchResult[]>([]);
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  const [loading, setLoading] = React.useState(false);
  const [submitError, setSubmitError] = React.useState<string | undefined>();

  async function runSearch(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSubmitError(undefined);
    if (!q.trim()) {
      setSubmitError("Enter a query before searching.");
      return;
    }
    setLoading(true);
    try {
      const cats = categories
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean)
        .map(Number)
        .filter(Number.isFinite);
      const res = await searchIndexers({
        q: q.trim(),
        indexer_ids: [indexer.id],
        categories: cats.length > 0 ? cats : undefined,
      });
      setResults(res.results ?? []);
      setErrors(res.errors ?? {});
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? `Search failed (HTTP ${err.status}): ${err.message}`
          : err instanceof Error
            ? err.message
            : "Search failed.";
      setSubmitError(msg);
      setResults([]);
      setErrors({});
    } finally {
      setLoading(false);
    }
  }

  const indexerError = errors[indexer.id];

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">
            Search “{indexer.name}”
          </h2>
          <p className="text-sm text-muted-foreground">
            Sends a fan-out search restricted to this indexer. Results are not
            persisted; download links open directly from the upstream feed.
          </p>
        </div>
        {onClose ? (
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
        ) : null}
      </header>

      <form
        onSubmit={runSearch}
        className="grid gap-3 sm:grid-cols-[1fr_18rem_auto]"
        aria-label="Manual indexer search"
      >
        <div className="grid gap-1">
          <Label htmlFor="search-q">Query</Label>
          <Input
            id="search-q"
            placeholder="ubuntu 24.04"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor="search-cats">Categories</Label>
          <Input
            id="search-cats"
            placeholder="2000, 5000"
            value={categories}
            onChange={(e) => setCategories(e.target.value)}
          />
        </div>
        <div className="flex items-end">
          <Button type="submit" disabled={loading}>
            {loading ? "Searching…" : "Search"}
          </Button>
        </div>
      </form>

      {submitError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {submitError}
        </div>
      ) : null}

      {indexerError ? (
        <div
          role="alert"
          className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300"
        >
          The indexer reported an error: {indexerError}
        </div>
      ) : null}

      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full text-sm">
          <caption className="sr-only">Search results</caption>
          <thead className="bg-muted/50 text-left">
            <tr>
              <th scope="col" className="px-3 py-2">
                Title
              </th>
              <th scope="col" className="px-3 py-2">
                Size
              </th>
              <th scope="col" className="px-3 py-2">
                Categories
              </th>
              <th scope="col" className="px-3 py-2">
                Age
              </th>
              <th scope="col" className="px-3 py-2">
                S/L
              </th>
              <th scope="col" className="px-3 py-2">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {results.length === 0 && !loading ? (
              <tr>
                <td
                  colSpan={6}
                  className="px-3 py-6 text-center text-muted-foreground"
                >
                  No results yet. Run a search to populate this table.
                </td>
              </tr>
            ) : null}
            {results.map((r, idx) => (
              <tr
                key={`${r.indexer_id}-${r.link}-${idx}`}
                className="border-t border-border"
              >
                <td className="px-3 py-2">
                  <div className="font-medium">{r.title}</div>
                  <div className="text-xs text-muted-foreground">
                    via {r.indexer_id}
                  </div>
                </td>
                <td className="px-3 py-2 tabular-nums">
                  {formatBytes(r.size_bytes)}
                </td>
                <td className="px-3 py-2 text-xs text-muted-foreground">
                  {(r.categories ?? []).join(", ") || "—"}
                </td>
                <td className="px-3 py-2 tabular-nums">
                  {formatAge(r.publish_date)}
                </td>
                <td className="px-3 py-2 tabular-nums">
                  {typeof r.seeders === "number" ||
                  typeof r.leechers === "number"
                    ? `${r.seeders ?? 0}/${r.leechers ?? 0}`
                    : "—"}
                </td>
                <td className="px-3 py-2">
                  <div className="flex gap-2">
                    <a
                      href={r.link}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary underline-offset-4 hover:underline"
                    >
                      Download
                    </a>
                    {r.info_url ? (
                      <a
                        href={r.info_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-muted-foreground underline-offset-4 hover:underline"
                      >
                        Details
                      </a>
                    ) : null}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
