import { useEffect, useState, useCallback, useMemo, useRef } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Download,
  Search,
  Loader2,
  AlertTriangle,
  ChevronDown,
  ExternalLink,
  Filter,
  X,
  Activity,
  CheckCircle2,
  XCircle,
  Clock,
} from "lucide-react";
import { cn, formatBytes } from "@/lib/utils";
import { toast } from "sonner";
import {
  streamSearch,
  type SearchResult,
  type IndexerStreamState,
  type IndexerStatus,
} from "@/lib/indexers-api";
import {
  useDownloads,
  useGrabRelease,
  type Download as DownloadClient,
} from "@/lib/downloads-api";

// ─── Helpers ──────────────────────────────────────────────────────────



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

const CATEGORY_MAP: Record<string, number[]> = {
  movie: [2000],
  series: [5000],
  season: [5000],
  episode: [5000],
};

function qualityBadge(result: SearchResult): string {
  const q = (result.quality || result.title || "").toLowerCase();
  if (q.includes("2160p") || q.includes("4k")) return "2160p";
  if (q.includes("1080p")) return "1080p";
  if (q.includes("720p")) return "720p";
  if (q.includes("480p")) return "480p";
  return "SD";
}

const QUALITY_COLORS: Record<string, string> = {
  "2160p": "bg-purple-500/15 text-purple-700 dark:text-purple-300",
  "1080p": "bg-blue-500/15 text-blue-700 dark:text-blue-300",
  "720p": "bg-green-500/15 text-green-700 dark:text-green-300",
  "480p": "bg-yellow-500/15 text-yellow-700 dark:text-yellow-300",
  SD: "bg-gray-500/15 text-gray-700 dark:text-gray-300",
};

const QUALITY_OPTIONS = ["2160p", "1080p", "720p", "480p", "SD"] as const;

const MB = 1024 * 1024;
const GB = 1024 * 1024 * 1024;

// ─── Filter state ─────────────────────────────────────────────────────

interface SearchFilters {
  indexers: Set<string>;
  qualities: Set<string>;
  minSizeMB: string;
  maxSizeGB: string;
  minSeeders: string;
  titleFilter: string;
  freeleechOnly: boolean;
}

const EMPTY_FILTERS: SearchFilters = {
  indexers: new Set(),
  qualities: new Set(),
  minSizeMB: "",
  maxSizeGB: "",
  minSeeders: "",
  titleFilter: "",
  freeleechOnly: false,
};

function countActiveFilters(f: SearchFilters): number {
  let n = 0;
  if (f.indexers.size > 0) n++;
  if (f.qualities.size > 0) n++;
  if (f.minSizeMB) n++;
  if (f.maxSizeGB) n++;
  if (f.minSeeders) n++;
  if (f.titleFilter) n++;
  if (f.freeleechOnly) n++;
  return n;
}

function applyFilters(results: SearchResult[], f: SearchFilters): SearchResult[] {
  return results.filter((r) => {
    if (f.indexers.size > 0 && !f.indexers.has(r.indexer_id)) return false;
    if (f.qualities.size > 0 && !f.qualities.has(qualityBadge(r))) return false;
    if (f.minSizeMB) {
      const min = parseFloat(f.minSizeMB);
      if (!isNaN(min) && (r.size_bytes ?? 0) < min * MB) return false;
    }
    if (f.maxSizeGB) {
      const max = parseFloat(f.maxSizeGB);
      if (!isNaN(max) && (r.size_bytes ?? 0) > max * GB) return false;
    }
    if (f.minSeeders) {
      const min = parseInt(f.minSeeders, 10);
      if (!isNaN(min) && (r.seeders ?? 0) < min) return false;
    }
    if (f.titleFilter) {
      const needle = f.titleFilter.toLowerCase();
      if (!r.title.toLowerCase().includes(needle)) return false;
    }
    if (f.freeleechOnly && !r.freeleech) return false;
    return true;
  });
}

// ─── Types ────────────────────────────────────────────────────────────

export interface ReleaseSearchProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  query?: string;
  tmdbId?: number;
  tvdbId?: number;
  imdbId?: string;
  season?: number;
  episode?: number;
  mediaType: "movie" | "episode" | "season" | "series";
  /** When true, automatically run the search when the dialog opens. */
  autoSearch?: boolean;
  /** Media context for grab tracking (import pipeline matching). */
  seriesId?: string;
  episodeIds?: string[];
  movieId?: string;
}

// ─── Grab Button ──────────────────────────────────────────────────────

interface MediaContext {
  media_type?: "movie" | "episode";
  series_id?: string;
  episode_ids?: string[];
  movie_id?: string;
}

function GrabButton({
  result,
  clients,
  mediaContext,
}: {
  result: SearchResult;
  clients: DownloadClient[];
  mediaContext?: MediaContext;
}) {
  const grab = useGrabRelease();
  const [grabbing, setGrabbing] = useState(false);

  const doGrab = useCallback(
    async (clientId: string) => {
      setGrabbing(true);
      try {
        await grab.mutateAsync({
          clientId,
          torrent_url: result.link,
          magnet: result.magnet_uri,
          nzb_url: result.nzb_url,
          infohash: result.infohash,
          title: result.title,
          ...mediaContext,
        });
        toast.success(`Grabbed: ${result.title}`);
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : "Grab failed";
        toast.error(msg);
      } finally {
        setGrabbing(false);
      }
    },
    [grab, result, mediaContext],
  );

  if (clients.length === 0) {
    return (
      <Button
        size="icon"
        variant="ghost"
        className="h-7 w-7"
        disabled
        title="No download clients configured"
      >
        <Download className="w-3.5 h-3.5" />
      </Button>
    );
  }

  if (clients.length === 1) {
    return (
      <Button
        size="icon"
        variant="ghost"
        className="h-7 w-7"
        disabled={grabbing}
        title={`Grab via ${clients[0]!.name}`}
        onClick={() => doGrab(clients[0]!.id)}
      >
        {grabbing ? (
          <Loader2 className="w-3.5 h-3.5 animate-spin" />
        ) : (
          <Download className="w-3.5 h-3.5" />
        )}
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="icon"
          variant="ghost"
          className="h-7 w-7"
          disabled={grabbing}
          title="Grab release"
        >
          {grabbing ? (
            <Loader2 className="w-3.5 h-3.5 animate-spin" />
          ) : (
            <Download className="w-3.5 h-3.5" />
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {clients.map((c) => (
          <DropdownMenuItem key={c.id} onClick={() => doGrab(c.id)}>
            {c.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// ─── Search Diagnostics ───────────────────────────────────────────────

function IndexerStatusGrid({ indexers }: { indexers: Map<string, IndexerStreamState> }) {
  if (indexers.size === 0) return null;

  const entries = Array.from(indexers.values());
  const searching = entries.filter((i) => i.status === "searching" || i.status === "pending").length;
  const done = entries.filter((i) => i.status === "done").length;
  const failed = entries.filter((i) => i.status === "error" || i.status === "timeout").length;
  const totalResults = entries.reduce((sum, i) => sum + i.resultCount, 0);

  return (
    <div className="rounded-md border border-border bg-muted/30 text-sm">
      <div className="flex items-center gap-3 px-3 py-1.5 text-xs text-muted-foreground">
        <Activity className="w-3.5 h-3.5 shrink-0" />
        <span>
          {searching > 0 ? (
            <>
              <Loader2 className="w-3 h-3 animate-spin inline mr-1" />
              Searching {searching} indexer{searching !== 1 ? "s" : ""}…
            </>
          ) : (
            `${totalResults} result${totalResults !== 1 ? "s" : ""}`
          )}
          {done > 0 && <span className="text-green-600 dark:text-green-400"> · {done} done</span>}
          {failed > 0 && <span className="text-red-600 dark:text-red-400"> · {failed} failed</span>}
        </span>
      </div>
      <div className="px-3 pb-2 flex flex-wrap gap-1.5">
        {entries.map((ix) => (
          <div
            key={ix.id}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-medium transition-all",
              ix.status === "pending" && "bg-muted text-muted-foreground",
              ix.status === "searching" && "bg-blue-500/15 text-blue-700 dark:text-blue-300",
              ix.status === "done" && "bg-green-500/10 text-green-700 dark:text-green-300",
              ix.status === "error" && "bg-red-500/10 text-red-700 dark:text-red-300",
              ix.status === "timeout" && "bg-yellow-500/10 text-yellow-700 dark:text-yellow-300",
            )}
            title={ix.error ? `${ix.name}: ${ix.error}` : ix.name}
          >
            {ix.status === "pending" && <Clock className="w-3 h-3" />}
            {ix.status === "searching" && <Loader2 className="w-3 h-3 animate-spin" />}
            {ix.status === "done" && <CheckCircle2 className="w-3 h-3" />}
            {ix.status === "error" && <XCircle className="w-3 h-3" />}
            {ix.status === "timeout" && <AlertTriangle className="w-3 h-3" />}
            <span className="truncate max-w-[8rem]">{ix.name}</span>
            {ix.status === "done" && ix.resultCount > 0 && (
              <span className="tabular-nums opacity-70">{ix.resultCount}</span>
            )}
            {(ix.status === "done" || ix.status === "error" || ix.status === "timeout") && ix.elapsedMs > 0 && (
              <span className="tabular-nums opacity-50">{ix.elapsedMs < 1000 ? `${ix.elapsedMs}ms` : `${(ix.elapsedMs / 1000).toFixed(1)}s`}</span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Filter Bar ───────────────────────────────────────────────────────

function FilterBar({
  results,
  filters,
  onChange,
}: {
  results: SearchResult[];
  filters: SearchFilters;
  onChange: (f: SearchFilters) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const activeCount = countActiveFilters(filters);

  const availableIndexers = useMemo(() => {
    const ids = new Set<string>();
    for (const r of results) ids.add(r.indexer_id);
    return Array.from(ids).sort();
  }, [results]);

  const hasFreeleech = useMemo(
    () => results.some((r) => r.freeleech),
    [results],
  );

  const toggleIndexer = (id: string) => {
    const next = new Set(filters.indexers);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    onChange({ ...filters, indexers: next });
  };

  const toggleQuality = (q: string) => {
    const next = new Set(filters.qualities);
    if (next.has(q)) next.delete(q);
    else next.add(q);
    onChange({ ...filters, qualities: next });
  };

  const clearAll = () => onChange({ ...EMPTY_FILTERS });

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="gap-1.5 h-7 text-xs"
          onClick={() => setExpanded((v) => !v)}
        >
          <Filter className="w-3.5 h-3.5" />
          Filters
          {activeCount > 0 && (
            <Badge variant="secondary" className="ml-1 h-4 min-w-4 px-1 text-[10px]">
              {activeCount}
            </Badge>
          )}
          <ChevronDown
            className={cn(
              "w-3 h-3 transition-transform",
              expanded && "rotate-180",
            )}
          />
        </Button>
        {activeCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="gap-1 h-7 text-xs text-muted-foreground"
            onClick={clearAll}
          >
            <X className="w-3 h-3" />
            Clear
          </Button>
        )}
        {/* Inline title filter (always visible) */}
        <Input
          value={filters.titleFilter}
          onChange={(e) => onChange({ ...filters, titleFilter: e.target.value })}
          placeholder="Filter by name…"
          className="h-7 text-xs flex-1 max-w-xs"
        />
      </div>

      {expanded && (
        <div className="rounded-md border border-border bg-muted/30 p-3 space-y-3 text-xs">
          {/* Row 1: Indexers + Quality */}
          <div className="flex flex-wrap gap-4">
            {/* Indexer multi-select */}
            <div className="space-y-1">
              <span className="font-medium text-muted-foreground">Indexer</span>
              <div className="flex flex-wrap gap-1.5">
                {availableIndexers.map((id) => (
                  <label
                    key={id}
                    className="flex items-center gap-1 cursor-pointer select-none"
                  >
                    <Checkbox
                      checked={filters.indexers.has(id)}
                      onCheckedChange={() => toggleIndexer(id)}
                      className="h-3.5 w-3.5"
                    />
                    <span className="truncate max-w-[8rem]">{id}</span>
                  </label>
                ))}
                {availableIndexers.length === 0 && (
                  <span className="text-muted-foreground italic">No indexers</span>
                )}
              </div>
            </div>

            {/* Quality checkboxes */}
            <div className="space-y-1">
              <span className="font-medium text-muted-foreground">Quality</span>
              <div className="flex flex-wrap gap-1.5">
                {QUALITY_OPTIONS.map((q) => (
                  <label
                    key={q}
                    className="flex items-center gap-1 cursor-pointer select-none"
                  >
                    <Checkbox
                      checked={filters.qualities.has(q)}
                      onCheckedChange={() => toggleQuality(q)}
                      className="h-3.5 w-3.5"
                    />
                    <span className={cn("rounded px-1 py-0.5 text-[10px] font-semibold", QUALITY_COLORS[q])}>
                      {q}
                    </span>
                  </label>
                ))}
              </div>
            </div>
          </div>

          {/* Row 2: Size + Seeders + Freeleech */}
          <div className="flex flex-wrap items-end gap-4">
            <div className="space-y-1">
              <span className="font-medium text-muted-foreground">Min Size (MB)</span>
              <Input
                type="number"
                min={0}
                step="any"
                value={filters.minSizeMB}
                onChange={(e) => onChange({ ...filters, minSizeMB: e.target.value })}
                className="h-7 w-24 text-xs"
                placeholder="0"
              />
            </div>
            <div className="space-y-1">
              <span className="font-medium text-muted-foreground">Max Size (GB)</span>
              <Input
                type="number"
                min={0}
                step="any"
                value={filters.maxSizeGB}
                onChange={(e) => onChange({ ...filters, maxSizeGB: e.target.value })}
                className="h-7 w-24 text-xs"
                placeholder="∞"
              />
            </div>
            <div className="space-y-1">
              <span className="font-medium text-muted-foreground">Min Seeders</span>
              <Input
                type="number"
                min={0}
                value={filters.minSeeders}
                onChange={(e) => onChange({ ...filters, minSeeders: e.target.value })}
                className="h-7 w-20 text-xs"
                placeholder="0"
              />
            </div>
            {hasFreeleech && (
              <label className="flex items-center gap-1.5 cursor-pointer select-none pb-1">
                <Checkbox
                  checked={filters.freeleechOnly}
                  onCheckedChange={(v) =>
                    onChange({ ...filters, freeleechOnly: v === true })
                  }
                  className="h-3.5 w-3.5"
                />
                <span className="rounded px-1.5 py-0.5 text-[10px] font-semibold bg-green-500/15 text-green-700 dark:text-green-300">
                  Freeleech only
                </span>
              </label>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Dialog ───────────────────────────────────────────────────────────

export function ReleaseSearchDialog({
  open,
  onOpenChange,
  title,
  query: initialQuery,
  tmdbId,
  tvdbId,
  imdbId,
  season,
  episode,
  mediaType,
  autoSearch = false,
  seriesId,
  episodeIds,
  movieId,
}: ReleaseSearchProps) {
  const [query, setQuery] = useState(initialQuery ?? title);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [indexerStates, setIndexerStates] = useState<Map<string, IndexerStreamState>>(new Map());
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [errorsExpanded, setErrorsExpanded] = useState(false);
  const [filters, setFilters] = useState<SearchFilters>({ ...EMPTY_FILTERS });
  const [didAutoSearch, setDidAutoSearch] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const resultCountRef = useRef(0);

  const { data: clients = [] } = useDownloads({ enabled: open });
  const enabledClients = clients.filter((c) => c.enabled);

  const mediaContext = useMemo((): MediaContext | undefined => {
    if (mediaType === "movie" && movieId) {
      return { media_type: "movie", movie_id: movieId };
    }
    if ((mediaType === "episode" || mediaType === "season") && seriesId) {
      return {
        media_type: "episode",
        series_id: seriesId,
        ...(episodeIds?.length ? { episode_ids: episodeIds } : {}),
      };
    }
    return undefined;
  }, [mediaType, movieId, seriesId, episodeIds]);

  const filteredResults = useMemo(
    () => applyFilters(results, filters),
    [results, filters],
  );

  // Reset state when dialog opens with new context
  useEffect(() => {
    if (open) {
      setQuery(initialQuery ?? title);
      setResults([]);
      setErrors({});
      setIndexerStates(new Map());
      setSearched(false);
      setErrorsExpanded(false);
      setFilters({ ...EMPTY_FILTERS });
      setDidAutoSearch(false);
    } else {
      // Cancel any in-flight search when dialog closes
      abortRef.current?.abort();
      abortRef.current = null;
    }
  }, [open, title, initialQuery]);

  const runSearch = useCallback((e?: React.FormEvent) => {
    e?.preventDefault();
    const q = query.trim();
    if (!q) return;

    // Abort any previous search
    abortRef.current?.abort();

    setLoading(true);
    setSearched(true);
    setResults([]);
    setErrors({});
    setIndexerStates(new Map());
    setFilters({ ...EMPTY_FILTERS });
    resultCountRef.current = 0;

    const controller = streamSearch(
      {
        q,
        categories: CATEGORY_MAP[mediaType],
        imdb_id: imdbId,
        tvdb_id: tvdbId != null ? String(tvdbId) : undefined,
        tmdb_id: tmdbId != null ? String(tmdbId) : undefined,
        season,
        episode,
        timeout_ms: 120_000,
      },
      {
        onSearchStart: (indexers) => {
          setIndexerStates((prev) => {
            const next = new Map(prev);
            for (const ix of indexers) {
              next.set(ix.id, {
                id: ix.id,
                name: ix.name,
                status: "pending",
                resultCount: 0,
                elapsedMs: 0,
              });
            }
            return next;
          });
        },
        onIndexerStart: (id, name) => {
          setIndexerStates((prev) => {
            const next = new Map(prev);
            const existing = next.get(id);
            next.set(id, {
              id,
              name: existing?.name ?? name,
              status: "searching",
              resultCount: 0,
              elapsedMs: 0,
            });
            return next;
          });
        },
        onIndexerResult: (id, name, newResults, count, elapsedMs) => {
          setResults((prev) => [...prev, ...newResults]);
          resultCountRef.current += newResults.length;
          setIndexerStates((prev) => {
            const next = new Map(prev);
            next.set(id, {
              id,
              name: next.get(id)?.name ?? name,
              status: "done",
              resultCount: count,
              elapsedMs,
            });
            return next;
          });
        },
        onIndexerError: (id, name, error, status, elapsedMs) => {
          setErrors((prev) => ({ ...prev, [name]: error }));
          setIndexerStates((prev) => {
            const next = new Map(prev);
            next.set(id, {
              id,
              name: next.get(id)?.name ?? name,
              status: (status === "timeout" ? "timeout" : "error") as IndexerStatus,
              resultCount: 0,
              elapsedMs,
              error,
            });
            return next;
          });
        },
        onDone: () => {
          setLoading(false);
          const total = resultCountRef.current;
          if (total > 0) {
            toast.success(
              `Search complete: ${total} result${total !== 1 ? "s" : ""} found`,
            );
          } else {
            toast.warning("Search complete: no results found across any indexer.");
          }
        },
        onError: (err) => {
          toast.error(err.message);
          setLoading(false);
        },
      },
    );

    abortRef.current = controller;
  }, [query, mediaType]);

  // Auto-run search when dialog opens with autoSearch enabled
  useEffect(() => {
    if (open && autoSearch && !didAutoSearch && query.trim()) {
      setDidAutoSearch(true);
      runSearch();
    }
  }, [open, autoSearch, didAutoSearch, query, runSearch]);

  const errorEntries = Object.entries(errors);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Search className="w-4 h-4" />
            Search: {title}
          </DialogTitle>
          <DialogDescription>
            Search indexers for releases and grab them to a download client.
          </DialogDescription>
        </DialogHeader>

        {/* Search form */}
        <form
          onSubmit={runSearch}
          className="flex items-center gap-2"
        >
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search query…"
            className="flex-1"
            autoFocus
          />
          <Button type="submit" disabled={loading} className="gap-1.5">
            {loading ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Search className="w-4 h-4" />
            )}
            Search
          </Button>
        </form>

        {/* Live indexer status */}
        {searched && indexerStates.size > 0 && <IndexerStatusGrid indexers={indexerStates} />}

        {/* Indexer errors */}
        {errorEntries.length > 0 && (
          <div className="rounded-md border border-amber-500/40 bg-amber-500/10 text-sm">
            <button
              onClick={() => setErrorsExpanded((v) => !v)}
              className="flex items-center gap-2 w-full px-3 py-2 text-amber-700 dark:text-amber-300"
            >
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>
                {errorEntries.length} indexer{errorEntries.length > 1 ? "s" : ""} reported errors
              </span>
              <ChevronDown
                className={cn(
                  "w-4 h-4 ml-auto transition-transform",
                  errorsExpanded && "rotate-180",
                )}
              />
            </button>
            {errorsExpanded && (
              <div className="px-3 pb-2 space-y-1">
                {errorEntries.map(([id, msg]) => (
                  <p key={id} className="text-xs text-amber-600 dark:text-amber-400">
                    <span className="font-medium">{id}:</span> {msg}
                  </p>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Filters */}
        {searched && results.length > 0 && (
          <FilterBar results={results} filters={filters} onChange={setFilters} />
        )}

        {/* Results table */}
        <div className="flex-1 overflow-auto rounded-md border border-border">
          <table className="w-full text-sm">
            <caption className="sr-only">Search results</caption>
            <thead className="bg-muted/50 text-left sticky top-0 z-10">
              <tr>
                <th scope="col" className="px-3 py-2">
                  Title
                </th>
                <th scope="col" className="px-3 py-2 w-16">
                  Quality
                </th>
                <th scope="col" className="px-3 py-2 w-14">
                  Score
                </th>
                <th scope="col" className="px-3 py-2 w-20">
                  Size
                </th>
                <th scope="col" className="px-3 py-2 w-14">
                  Age
                </th>
                <th scope="col" className="px-3 py-2 w-16">
                  S/L
                </th>
                <th scope="col" className="px-3 py-2 w-24">
                  Indexer
                </th>
                <th scope="col" className="px-3 py-2 w-16">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {!searched && !loading && (
                <tr>
                  <td
                    colSpan={8}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    Enter a query and click Search to find releases.
                  </td>
                </tr>
              )}
              {searched && !loading && filteredResults.length === 0 && results.length === 0 && (
                <tr>
                  <td
                    colSpan={8}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    No results found.
                  </td>
                </tr>
              )}
              {searched && !loading && filteredResults.length === 0 && results.length > 0 && (
                <tr>
                  <td
                    colSpan={8}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    No results match the current filters.
                  </td>
                </tr>
              )}
              {loading && results.length === 0 && (
                <tr>
                  <td
                    colSpan={8}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    <Loader2 className="w-5 h-5 animate-spin inline-block mr-2" />
                    Searching indexers…
                  </td>
                </tr>
              )}
              {filteredResults.map((r, idx) => {
                const qb = qualityBadge(r);
                return (
                <tr
                  key={`${r.indexer_id}-${r.link}-${idx}`}
                  className="border-t border-border hover:bg-accent/5 transition-colors"
                >
                  <td className="px-3 py-2">
                    <div className="font-medium text-xs leading-snug line-clamp-2">
                      {r.title}
                    </div>
                    {/* Tracker flags */}
                    {(r.freeleech || r.internal || r.scene) && (
                      <div className="flex gap-1 mt-0.5">
                        {r.freeleech && (
                          <span className="rounded px-1 py-0.5 text-[9px] font-semibold bg-green-500/15 text-green-700 dark:text-green-300">
                            FL
                          </span>
                        )}
                        {r.internal && (
                          <span className="rounded px-1 py-0.5 text-[9px] font-semibold bg-blue-500/15 text-blue-700 dark:text-blue-300">
                            Internal
                          </span>
                        )}
                        {r.scene && (
                          <span className="rounded px-1 py-0.5 text-[9px] font-semibold bg-orange-500/15 text-orange-700 dark:text-orange-300">
                            Scene
                          </span>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap">
                    <span className={cn("inline-block rounded px-1.5 py-0.5 text-[10px] font-semibold", QUALITY_COLORS[qb] ?? QUALITY_COLORS.SD)}>
                      {qb}
                    </span>
                  </td>
                  <td className="px-3 py-2 tabular-nums text-xs text-muted-foreground whitespace-nowrap">
                    {typeof r.score === "number" ? Math.round(r.score) : "—"}
                  </td>
                  <td className="px-3 py-2 tabular-nums text-xs text-muted-foreground whitespace-nowrap">
                    {formatBytes(r.size_bytes)}
                  </td>
                  <td className="px-3 py-2 tabular-nums text-xs text-muted-foreground whitespace-nowrap">
                    {formatAge(r.publish_date)}
                  </td>
                  <td className="px-3 py-2 tabular-nums text-xs text-muted-foreground whitespace-nowrap">
                    {typeof r.seeders === "number" ||
                    typeof r.leechers === "number"
                      ? `${r.seeders ?? 0}/${r.leechers ?? 0}`
                      : "—"}
                  </td>
                  <td className="px-3 py-2 text-xs text-muted-foreground truncate max-w-[6rem]">
                    {r.indexer_id}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex items-center gap-1">
                      <GrabButton
                        result={r}
                        clients={enabledClients}
                        mediaContext={mediaContext}
                      />
                      {r.info_url && (
                        <a
                          href={r.info_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center justify-center h-7 w-7 rounded-md hover:bg-accent/10 text-muted-foreground hover:text-foreground transition-colors"
                          title="View details"
                        >
                          <ExternalLink className="w-3.5 h-3.5" />
                        </a>
                      )}
                    </div>
                  </td>
                </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        {/* Result count */}
        {searched && !loading && results.length > 0 && (
          <p className="text-xs text-muted-foreground text-right">
            {filteredResults.length === results.length
              ? `${results.length} result${results.length !== 1 ? "s" : ""}`
              : `${filteredResults.length} of ${results.length} results (filtered)`}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
