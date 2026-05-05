import { useEffect, useState, useCallback } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
} from "lucide-react";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import {
  searchIndexers,
  type SearchResult,
  ApiError as IndexerApiError,
} from "@/lib/indexers-api";
import {
  useDownloads,
  useGrabRelease,
  type Download as DownloadClient,
} from "@/lib/downloads-api";

// ─── Helpers ──────────────────────────────────────────────────────────

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

const CATEGORY_MAP: Record<string, number[]> = {
  movie: [2000],
  series: [5000],
  season: [5000],
  episode: [5000],
};

function sortResults(results: SearchResult[]): SearchResult[] {
  return [...results].sort((a, b) => {
    // seeders desc
    const sa = a.seeders ?? -1;
    const sb = b.seeders ?? -1;
    if (sb !== sa) return sb - sa;
    // age asc (newer first)
    const da = a.publish_date ? Date.parse(a.publish_date) : 0;
    const db = b.publish_date ? Date.parse(b.publish_date) : 0;
    return db - da;
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
}

// ─── Grab Button ──────────────────────────────────────────────────────

function GrabButton({
  result,
  clients,
}: {
  result: SearchResult;
  clients: DownloadClient[];
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
          title: result.title,
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
    [grab, result],
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
        title={`Grab via ${clients[0].name}`}
        onClick={() => doGrab(clients[0].id)}
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
}: ReleaseSearchProps) {
  const [query, setQuery] = useState(initialQuery ?? title);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);
  const [errorsExpanded, setErrorsExpanded] = useState(false);

  const { data: clients = [] } = useDownloads({ enabled: open });
  const enabledClients = clients.filter((c) => c.enabled);

  // Reset state when dialog opens with new context
  useEffect(() => {
    if (open) {
      setQuery(initialQuery ?? title);
      setResults([]);
      setErrors({});
      setSearched(false);
      setErrorsExpanded(false);
    }
  }, [open, title, initialQuery]);

  const runSearch = async (e?: React.FormEvent) => {
    e?.preventDefault();
    const q = query.trim();
    if (!q) return;

    setLoading(true);
    setSearched(true);
    try {
      const res = await searchIndexers({
        q,
        categories: CATEGORY_MAP[mediaType],
        timeout_ms: 30000,
      });
      setResults(sortResults(res.results ?? []));
      setErrors(res.errors ?? {});
    } catch (err) {
      const msg =
        err instanceof IndexerApiError
          ? `Search failed (HTTP ${err.status}): ${err.message}`
          : err instanceof Error
            ? err.message
            : "Search failed";
      toast.error(msg);
      setResults([]);
      setErrors({});
    } finally {
      setLoading(false);
    }
  };

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

        {/* Results table */}
        <div className="flex-1 overflow-auto rounded-md border border-border">
          <table className="w-full text-sm">
            <caption className="sr-only">Search results</caption>
            <thead className="bg-muted/50 text-left sticky top-0 z-10">
              <tr>
                <th scope="col" className="px-3 py-2">
                  Title
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
                    colSpan={6}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    Enter a query and click Search to find releases.
                  </td>
                </tr>
              )}
              {searched && !loading && results.length === 0 && (
                <tr>
                  <td
                    colSpan={6}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    No results found.
                  </td>
                </tr>
              )}
              {loading && (
                <tr>
                  <td
                    colSpan={6}
                    className="px-3 py-10 text-center text-muted-foreground"
                  >
                    <Loader2 className="w-5 h-5 animate-spin inline-block mr-2" />
                    Searching indexers…
                  </td>
                </tr>
              )}
              {results.map((r, idx) => (
                <tr
                  key={`${r.indexer_id}-${r.link}-${idx}`}
                  className="border-t border-border hover:bg-accent/5 transition-colors"
                >
                  <td className="px-3 py-2">
                    <div className="font-medium text-xs leading-snug line-clamp-2">
                      {r.title}
                    </div>
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
              ))}
            </tbody>
          </table>
        </div>

        {/* Result count */}
        {searched && !loading && results.length > 0 && (
          <p className="text-xs text-muted-foreground text-right">
            {results.length} result{results.length !== 1 ? "s" : ""}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
