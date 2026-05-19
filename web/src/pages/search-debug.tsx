import * as React from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useSetPageHeader } from "@/hooks/use-page-header";
import {
  useSearchDebugList,
  useSearchDebugStats,
  useSearchDebugEntry,
  type SearchDebugParams,
  type SearchDebugEntry,
  type TierDetail,
  type IndexerResult,
  type EvalResult,
} from "@/lib/search-debug-api";
import { useQueryClient } from "@tanstack/react-query";
import {
  Search,
  ChevronDown,
  ChevronUp,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
  Filter,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
} from "lucide-react";

const PAGE_SIZE = 50;

const OUTCOMES = [
  { value: "all", label: "All Outcomes" },
  { value: "grabbed", label: "Grabbed" },
  { value: "no_results", label: "No Results" },
  { value: "all_rejected", label: "All Rejected" },
  { value: "grab_failed", label: "Grab Failed" },
  { value: "already_grabbed", label: "Already Grabbed" },
  { value: "profile_load_failed", label: "Profile Load Failed" },
];

const MEDIA_TYPES = [
  { value: "all", label: "All Media" },
  { value: "movie", label: "Movies" },
  { value: "episode", label: "Episodes" },
  { value: "season", label: "Seasons" },
];

// ─── Outcome badge styling ──────────────────────────────────────────────

function outcomeBadge(outcome: string) {
  const styles: Record<string, string> = {
    grabbed: "bg-green-500/10 text-green-500 border-0",
    no_results: "bg-gray-500/10 text-gray-400 border-0",
    all_rejected: "bg-yellow-500/10 text-yellow-500 border-0",
    grab_failed: "bg-red-500/10 text-red-500 border-0",
    already_grabbed: "bg-blue-500/10 text-blue-400 border-0",
    profile_load_failed: "bg-red-500/10 text-red-500 border-0",
    error: "bg-red-500/10 text-red-500 border-0",
  };
  return (
    <Badge
      variant="outline"
      className={`text-[10px] px-1.5 capitalize ${styles[outcome] ?? "bg-gray-500/10 text-gray-400 border-0"}`}
    >
      {outcome.replace(/_/g, " ")}
    </Badge>
  );
}

// ─── Format helpers ─────────────────────────────────────────────────────

function formatSize(bytes: number) {
  return (bytes / 1024 / 1024).toFixed(1) + " MB";
}

function formatTimestamp(ts: string) {
  try {
    const d = new Date(ts);
    return d.toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return ts;
  }
}

function seasonEpisodeLabel(entry: SearchDebugEntry) {
  if (entry.media_type === "movie") return "—";
  const parts: string[] = [];
  if (entry.season > 0) parts.push(`S${String(entry.season).padStart(2, "0")}`);
  if (entry.episode > 0) parts.push(`E${String(entry.episode).padStart(2, "0")}`);
  return parts.length > 0 ? parts.join("") : "—";
}

// ─── Stats summary ─────────────────────────────────────────────────────

function StatsSummary() {
  const { data, isLoading } = useSearchDebugStats();

  if (isLoading || !data) {
    return (
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i}>
            <CardContent className="p-4">
              <div className="h-8 animate-pulse rounded bg-muted" />
            </CardContent>
          </Card>
        ))}
      </div>
    );
  }

  const grabbedCount = data.outcome_counts["grabbed"] ?? 0;
  const grabbedPct =
    data.total_searches > 0
      ? ((grabbedCount / data.total_searches) * 100).toFixed(1)
      : "0";
  const topReject = data.top_reject_reasons?.[0];
  const failedCount =
    (data.outcome_counts["grab_failed"] ?? 0) +
    (data.outcome_counts["profile_load_failed"] ?? 0);

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      <Card>
        <CardHeader className="pb-1 pt-3 px-4">
          <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
            <Search className="h-3.5 w-3.5" /> Total Searches
          </CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-3">
          <span className="text-2xl font-bold tabular-nums">
            {data.total_searches.toLocaleString()}
          </span>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-1 pt-3 px-4">
          <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
            <CheckCircle2 className="h-3.5 w-3.5 text-green-500" /> Grabbed
          </CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-3">
          <span className="text-2xl font-bold tabular-nums text-green-500">
            {grabbedPct}%
          </span>
          <span className="ml-1.5 text-xs text-muted-foreground">
            ({grabbedCount})
          </span>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-1 pt-3 px-4">
          <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
            <XCircle className="h-3.5 w-3.5 text-red-500" /> Failed
          </CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-3">
          <span className="text-2xl font-bold tabular-nums text-red-500">
            {failedCount}
          </span>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-1 pt-3 px-4">
          <CardTitle className="text-xs font-medium text-muted-foreground flex items-center gap-1.5">
            <AlertTriangle className="h-3.5 w-3.5 text-yellow-500" /> Top
            Reject
          </CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-3">
          {topReject ? (
            <>
              <span className="text-sm font-semibold truncate block">
                {topReject.reason}
              </span>
              <span className="text-xs text-muted-foreground">
                {topReject.count} times
              </span>
            </>
          ) : (
            <span className="text-sm text-muted-foreground">None</span>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Tier detail panel ──────────────────────────────────────────────────

function TierPanel({ tiers }: { tiers: TierDetail[] }) {
  if (tiers.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No tier data recorded.</p>
    );
  }

  return (
    <div className="space-y-2">
      <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wide">
        Tiers
      </p>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-[60px]">Tier</TableHead>
            <TableHead>Queries</TableHead>
            <TableHead className="w-[80px] text-right">Results</TableHead>
            <TableHead className="w-[80px] text-right">Accepted</TableHead>
            <TableHead className="w-[80px] text-right">Rejected</TableHead>
            <TableHead className="w-[70px]">Stopped</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {tiers.map((t) => (
            <TableRow key={t.tier_index}>
              <TableCell className="text-xs font-medium tabular-nums">
                {t.tier_index}
              </TableCell>
              <TableCell className="text-xs text-muted-foreground">
                {t.queries
                  .map((q) => q.term || q.mode || q.imdb_id || "query")
                  .join(", ")}
              </TableCell>
              <TableCell className="text-xs text-right tabular-nums">
                {t.result_count}
              </TableCell>
              <TableCell className="text-xs text-right tabular-nums text-green-500">
                {t.accepted_count}
              </TableCell>
              <TableCell className="text-xs text-right tabular-nums text-red-500">
                {t.rejected_count}
              </TableCell>
              <TableCell>
                {t.stopped_here && (
                  <Badge
                    variant="outline"
                    className="text-[9px] bg-blue-500/10 text-blue-400 border-0"
                  >
                    stopped
                  </Badge>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

// ─── Indexer results panel ──────────────────────────────────────────────

function IndexerPanel({ results }: { results: IndexerResult[] }) {
  if (results.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No indexer data recorded.
      </p>
    );
  }

  const maxLatency = Math.max(...results.map((r) => r.latency_ms), 1);

  return (
    <div className="space-y-2">
      <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wide">
        Indexer Results
      </p>
      {results.map((ix) => (
        <div key={ix.indexer_id} className="flex items-center gap-2 text-xs">
          <span className="w-28 truncate font-medium">{ix.indexer_name}</span>
          <div className="flex-1 h-1.5 rounded-full bg-muted">
            <div
              className={`h-1.5 rounded-full ${ix.status === "completed" || ix.status === "ok" ? "bg-blue-500" : "bg-red-500"}`}
              style={{
                width: `${Math.min((ix.latency_ms / maxLatency) * 100, 100)}%`,
              }}
            />
          </div>
          <span className="text-muted-foreground w-14 text-right tabular-nums">
            {ix.latency_ms}ms
          </span>
          <span className="text-muted-foreground w-16 text-right tabular-nums">
            {ix.result_count} results
          </span>
          <Badge
            variant="outline"
            className={`text-[9px] px-1 ${
              ix.status === "completed" || ix.status === "ok"
                ? "bg-green-500/10 text-green-500 border-0"
                : ix.status === "failed"
                  ? "bg-red-500/10 text-red-500 border-0"
                  : "bg-gray-500/10 text-gray-500 border-0"
            }`}
          >
            {ix.status}
          </Badge>
          {ix.error && (
            <span className="text-red-400 truncate max-w-[12rem]">
              {ix.error}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

// ─── Evaluation panel ───────────────────────────────────────────────────

function EvaluationPanel({ evals }: { evals: EvalResult[] }) {
  if (evals.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No evaluation data recorded.
      </p>
    );
  }

  return (
    <div className="space-y-2">
      <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wide">
        Evaluation ({evals.length} results)
      </p>
      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Title</TableHead>
              <TableHead className="w-[70px]">Status</TableHead>
              <TableHead>Reject Reason</TableHead>
              <TableHead className="w-[90px]">Quality</TableHead>
              <TableHead className="w-[50px] text-right">Tier</TableHead>
              <TableHead className="w-[60px] text-right">Fmt</TableHead>
              <TableHead className="w-[60px] text-right">Score</TableHead>
              <TableHead className="w-[70px] text-right">Size</TableHead>
              <TableHead className="w-[60px] text-right">Seeds</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {evals.map((ev, i) => (
              <TableRow key={i}>
                <TableCell
                  className="text-xs max-w-[300px] truncate"
                  title={ev.title}
                >
                  {ev.title}
                </TableCell>
                <TableCell>
                  {ev.rejected ? (
                    <Badge
                      variant="outline"
                      className="text-[9px] bg-red-500/10 text-red-500 border-0"
                    >
                      rejected
                    </Badge>
                  ) : (
                    <Badge
                      variant="outline"
                      className="text-[9px] bg-green-500/10 text-green-500 border-0"
                    >
                      accepted
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground truncate max-w-[200px]">
                  {ev.reject_reason ?? "—"}
                </TableCell>
                <TableCell className="text-xs">
                  {ev.quality_name ?? "—"}
                </TableCell>
                <TableCell className="text-xs text-right tabular-nums">
                  {ev.quality_tier}
                </TableCell>
                <TableCell className="text-xs text-right tabular-nums">
                  {ev.format_score}
                </TableCell>
                <TableCell className="text-xs text-right tabular-nums font-medium">
                  {ev.composite_score}
                </TableCell>
                <TableCell className="text-xs text-right tabular-nums">
                  {formatSize(ev.size)}
                </TableCell>
                <TableCell className="text-xs text-right tabular-nums">
                  {ev.seeders ?? "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

// ─── Detail panel (loads full entry) ────────────────────────────────────

function DetailPanel({ entryId }: { entryId: string }) {
  const { data, isLoading } = useSearchDebugEntry(entryId);

  if (isLoading) {
    return (
      <p className="text-xs text-muted-foreground animate-pulse">
        Loading details…
      </p>
    );
  }

  if (!data) return null;

  return (
    <div className="space-y-4">
      {data.error_message && (
        <div className="flex items-start gap-2 text-xs text-red-400 bg-red-500/5 rounded p-2">
          <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          <span>{data.error_message}</span>
        </div>
      )}

      {data.grabbed_title && (
        <div className="text-xs">
          <span className="text-muted-foreground">Grabbed: </span>
          <span className="font-medium text-green-500">
            {data.grabbed_title}
          </span>
        </div>
      )}

      {data.tiers && <TierPanel tiers={data.tiers} />}
      {data.indexer_results && <IndexerPanel results={data.indexer_results} />}
      {data.evaluation && <EvaluationPanel evals={data.evaluation} />}
    </div>
  );
}

// ─── Search result row ──────────────────────────────────────────────────

function SearchRow({
  entry,
  expanded,
  onToggle,
}: {
  entry: SearchDebugEntry;
  expanded: boolean;
  onToggle: () => void;
}) {
  const Chevron = expanded ? ChevronUp : ChevronDown;

  return (
    <>
      <TableRow
        className="cursor-pointer hover:bg-muted/40"
        onClick={onToggle}
      >
        <TableCell className="text-xs tabular-nums text-muted-foreground">
          {formatTimestamp(entry.created_at)}
        </TableCell>
        <TableCell className="text-sm">
          <span className="flex items-center gap-1">
            {entry.title}
            {entry.year > 0 && (
              <span className="text-xs text-muted-foreground">
                ({entry.year})
              </span>
            )}
            <Chevron className="h-3 w-3 text-muted-foreground shrink-0" />
          </span>
        </TableCell>
        <TableCell className="text-xs capitalize">{entry.media_type}</TableCell>
        <TableCell className="text-xs tabular-nums">
          {seasonEpisodeLabel(entry)}
        </TableCell>
        <TableCell>{outcomeBadge(entry.outcome)}</TableCell>
        <TableCell className="text-xs text-right tabular-nums">
          {entry.total_results}
        </TableCell>
        <TableCell className="text-xs text-right tabular-nums text-red-500">
          {entry.total_rejected}
        </TableCell>
        <TableCell className="text-xs text-right tabular-nums text-muted-foreground">
          <span className="flex items-center justify-end gap-1">
            <Clock className="h-3 w-3" />
            {entry.duration_ms}ms
          </span>
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow className="bg-muted/20 hover:bg-muted/20">
          <TableCell colSpan={8} className="p-3 pl-6">
            <DetailPanel entryId={entry.id} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

// ─── Main page ──────────────────────────────────────────────────────────

export function SearchDebugPage() {
  useSetPageHeader("Search Debug");

  const [outcome, setOutcome] = React.useState("all");
  const [mediaType, setMediaType] = React.useState("all");
  const [offset, setOffset] = React.useState(0);
  const [expandedId, setExpandedId] = React.useState<string | null>(null);
  const qc = useQueryClient();

  const params: SearchDebugParams = {
    limit: PAGE_SIZE,
    offset,
    ...(outcome !== "all" && { outcome }),
    ...(mediaType !== "all" && { media_type: mediaType }),
  };

  const { data, isLoading, isError } = useSearchDebugList(params);

  React.useEffect(() => setOffset(0), [outcome, mediaType]);

  const entries = data?.entries ?? [];
  const total = data?.total ?? 0;
  const hasNext = offset + PAGE_SIZE < total;
  const hasPrev = offset > 0;

  return (
    <div className="space-y-4">
      {/* Stats */}
      <StatsSummary />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <Filter className="h-4 w-4 text-muted-foreground" />

        <Select value={outcome} onValueChange={setOutcome}>
          <SelectTrigger className="w-[160px]">
            <SelectValue placeholder="Outcome" />
          </SelectTrigger>
          <SelectContent>
            {OUTCOMES.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={mediaType} onValueChange={setMediaType}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Media Type" />
          </SelectTrigger>
          <SelectContent>
            {MEDIA_TYPES.map((m) => (
              <SelectItem key={m.value} value={m.value}>
                {m.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button
          variant="outline"
          size="icon"
          onClick={() =>
            qc.invalidateQueries({ queryKey: ["search-debug"] })
          }
          aria-label="Refresh"
        >
          <RefreshCw className="h-4 w-4" />
        </Button>

        <span className="ml-auto text-xs text-muted-foreground">
          {total} result{total !== 1 ? "s" : ""}
        </span>
      </div>

      {/* Table */}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[160px]">Time</TableHead>
              <TableHead>Title</TableHead>
              <TableHead className="w-[80px]">Media</TableHead>
              <TableHead className="w-[70px]">S/E</TableHead>
              <TableHead className="w-[110px]">Outcome</TableHead>
              <TableHead className="w-[70px] text-right">Results</TableHead>
              <TableHead className="w-[70px] text-right">Rejected</TableHead>
              <TableHead className="w-[90px] text-right">Duration</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell colSpan={8}>
                  <LoadingState label="Loading search debug entries…" />
                </TableCell>
              </TableRow>
            )}
            {isError && (
              <TableRow>
                <TableCell
                  colSpan={8}
                  className="text-center text-destructive py-8"
                >
                  Failed to load search debug data.
                </TableCell>
              </TableRow>
            )}
            {!isLoading && !isError && entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={8}>
                  <EmptyState
                    title="No search debug entries"
                    description="Search debug data will appear here as searches are executed."
                  />
                </TableCell>
              </TableRow>
            )}
            {entries.map((e) => (
              <SearchRow
                key={e.id}
                entry={e}
                expanded={expandedId === e.id}
                onToggle={() =>
                  setExpandedId(expandedId === e.id ? null : e.id)
                }
              />
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      {total > PAGE_SIZE && (
        <div className="flex items-center justify-end gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={!hasPrev}
            onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
          >
            <ChevronLeft className="h-4 w-4 mr-1" /> Previous
          </Button>
          <span className="text-xs text-muted-foreground">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasNext}
            onClick={() => setOffset((o) => o + PAGE_SIZE)}
          >
            Next <ChevronRight className="h-4 w-4 ml-1" />
          </Button>
        </div>
      )}
    </div>
  );
}
