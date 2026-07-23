// SearchPanel runs a manual search against a single indexer (via the
// fan-out endpoint scoped with `indexer_ids`) and renders the
// aggregated results in a sortable-ish table. When a quality profile
// is provided (via props), the user can toggle "Evaluate quality" to
// score each result through the autosearch evaluate endpoint.

import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  searchIndexers,
  evaluateResults,
  type Indexer,
  type SearchResult,
  type EvaluatedResult,
  ApiError,
} from "@/lib/indexers-api";

import { formatBytes } from "@/lib/utils";

function formatAge(iso?: string): string {
  if (!iso) return "\u2014";
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return "\u2014";
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

function QualityBadge({ result }: { result: EvaluatedResult }) {
  if (result.rejected) {
    return (
      <span
        className="inline-flex items-center gap-1 rounded-full bg-red-500/15 px-2 py-0.5 text-xs font-medium text-red-700 dark:text-red-300"
        title={`Rejected: ${result.reject_reason}`}
      >
        ✕ {result.reject_reason?.replace(/_/g, " ")}
      </span>
    );
  }
  return (
    <div className="flex flex-wrap items-center gap-1">
      {result.quality_name ? (
        <span className="inline-flex items-center rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-300">
          {result.quality_name}
        </span>
      ) : null}
      {result.format_score !== 0 ? (
        <span
          className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
            result.format_score > 0
              ? "bg-blue-500/15 text-blue-700 dark:text-blue-300"
              : "bg-amber-500/15 text-amber-700 dark:text-amber-300"
          }`}
        >
          CF {result.format_score > 0 ? "+" : ""}
          {result.format_score}
        </span>
      ) : null}
      <span className="text-xs tabular-nums text-muted-foreground">
        {result.composite_score.toFixed(0)}
      </span>
    </div>
  );
}

export interface SearchPanelProps {
  indexer: Indexer;
  onClose?: () => void;
  qualityProfileId?: string;
  mediaType?: string;
  title?: string;
  year?: number;
  imdbId?: string;
  tmdbId?: string;
  tvdbId?: string;
  season?: number;
  episode?: number;
}

export function SearchPanel({
  indexer,
  onClose,
  qualityProfileId,
  mediaType,
  title: mediaTitle,
  year,
  imdbId,
  tmdbId,
  tvdbId,
  season,
  episode,
}: SearchPanelProps) {
  const [q, setQ] = React.useState(mediaTitle ?? "");
  const [categories, setCategories] = React.useState<string>(
    (indexer.categories ?? []).join(", "),
  );
  const [results, setResults] = React.useState<SearchResult[]>([]);
  const [evaluated, setEvaluated] = React.useState<EvaluatedResult[] | null>(
    null,
  );
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  const [loading, setLoading] = React.useState(false);
  const [evaluating, setEvaluating] = React.useState(false);
  const [submitError, setSubmitError] = React.useState<string | undefined>();
  const [evaluateEnabled, setEvaluateEnabled] =
    React.useState(!!qualityProfileId);

  async function runEvaluate(searchResults: SearchResult[]) {
    if (!qualityProfileId || !evaluateEnabled || searchResults.length === 0)
      return;
    setEvaluating(true);
    try {
      const resp = await evaluateResults({
        quality_profile_id: qualityProfileId,
        media_type: mediaType,
        title: mediaTitle,
        year,
        imdb_id: imdbId,
        tmdb_id: tmdbId,
        tvdb_id: tvdbId,
        season,
        episode,
        results: searchResults,
      });
      setEvaluated(resp.results);
    } catch {
      // Non-fatal: results still show without quality info.
      setEvaluated(null);
    } finally {
      setEvaluating(false);
    }
  }

  async function runSearch(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSubmitError(undefined);
    setEvaluated(null);
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
        imdb_id: imdbId,
        tvdb_id: tvdbId,
        tmdb_id: tmdbId,
        season,
        episode,
      });
      const searchResults = res.results ?? [];
      setResults(searchResults);
      setErrors(res.errors ?? {});
      // Fire evaluate in background.
      runEvaluate(searchResults);
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

  // Build a lookup map from evaluated results (keyed by title+link).
  const evalMap = React.useMemo(() => {
    if (!evaluated) return null;
    const m = new Map<string, EvaluatedResult>();
    for (const er of evaluated) {
      m.set(`${er.title}|${er.link}`, er);
    }
    return m;
  }, [evaluated]);

  const indexerError = errors[indexer.id];
  const showQuality = evaluateEnabled && !!qualityProfileId;

  return (
    <div className="space-y-4">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">
            Search &ldquo;{indexer.name}&rdquo;
          </h2>
          <p className="text-sm text-muted-foreground">
            Sends a fan-out search restricted to this indexer.
            {showQuality
              ? " Results are scored against your quality profile."
              : " Results are not evaluated; download links open directly."}
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
        <div className="flex items-end gap-2">
          <Button type="submit" disabled={loading}>
            {loading ? "Searching\u2026" : "Search"}
          </Button>
        </div>
      </form>

      {qualityProfileId ? (
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={evaluateEnabled}
            onChange={(e) => {
              setEvaluateEnabled(e.target.checked);
              if (e.target.checked && results.length > 0) {
                runEvaluate(results);
              } else {
                setEvaluated(null);
              }
            }}
            className="rounded border-border"
          />
          Evaluate quality
          {evaluating ? (
            <span className="text-xs text-muted-foreground">
              (scoring\u2026)
            </span>
          ) : null}
        </label>
      ) : null}

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
              {showQuality ? (
                <th scope="col" className="px-3 py-2">
                  Quality
                </th>
              ) : null}
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
                  colSpan={showQuality ? 7 : 6}
                  className="px-3 py-6 text-center text-muted-foreground"
                >
                  No results yet. Run a search to populate this table.
                </td>
              </tr>
            ) : null}
            {results.map((r, idx) => {
              const er = evalMap?.get(`${r.title}|${r.link}`);
              return (
                <tr
                  key={`${r.indexer_id}-${r.link}-${idx}`}
                  className={`border-t border-border ${er?.rejected ? "opacity-50" : ""}`}
                >
                  <td className="px-3 py-2">
                    <div className="font-medium">{r.title}</div>
                    <div className="text-xs text-muted-foreground">
                      via {r.indexer_id}
                      {er?.parsed_source ? ` \u00b7 ${er.parsed_source}` : null}
                      {er?.parsed_resolution
                        ? ` \u00b7 ${er.parsed_resolution}p`
                        : null}
                    </div>
                  </td>
                  {showQuality ? (
                    <td className="px-3 py-2">
                      {er ? (
                        <QualityBadge result={er} />
                      ) : evaluating ? (
                        "\u2026"
                      ) : null}
                    </td>
                  ) : null}
                  <td className="px-3 py-2 tabular-nums">
                    {formatBytes(r.size_bytes)}
                  </td>
                  <td className="px-3 py-2 text-xs text-muted-foreground">
                    {(r.categories ?? []).join(", ") || "\u2014"}
                  </td>
                  <td className="px-3 py-2 tabular-nums">
                    {formatAge(r.publish_date)}
                  </td>
                  <td className="px-3 py-2 tabular-nums">
                    {typeof r.seeders === "number" ||
                    typeof r.leechers === "number"
                      ? `${r.seeders ?? 0}/${r.leechers ?? 0}`
                      : "\u2014"}
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
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
