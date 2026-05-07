import * as React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ChevronDown,
  ChevronUp,
  Trash2,
  Search,
  RefreshCw,
} from "lucide-react";
import { useApiClient } from "@/lib/api-client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

// ---------- Types ----------

interface IndexerQueryEntry {
  id: string;
  indexer_id: string;
  indexer_name: string;
  started_at: string;
  finished_at?: string;
  latency_ms: number;
  result_count: number;
  error?: string;
  status: string;
}

interface QueryLogEntry {
  id: string;
  query: string;
  query_type: string;
  media_type: string;
  media_id: string;
  started_at: string;
  finished_at?: string;
  total_results: number;
  status: string;
  indexers?: IndexerQueryEntry[];
}

// ---------- Hooks ----------

function useSearchLog(limit: number, offset: number) {
  const api = useApiClient();
  return useQuery({
    queryKey: ["search-log", limit, offset],
    queryFn: () =>
      api.get<{ data: QueryLogEntry[] }>(
        `/search/log?limit=${limit}&offset=${offset}`
      ),
    refetchInterval: 10_000,
  });
}

function useSearchLogDetail(id: string | null) {
  const api = useApiClient();
  return useQuery({
    queryKey: ["search-log", id],
    queryFn: () => api.get<QueryLogEntry>(`/search/log/${id}`),
    enabled: !!id,
  });
}

function usePruneSearchLog() {
  const api = useApiClient();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (days: number) =>
      api.delete<{ deleted: number }>(`/search/log?days=${days}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["search-log"] }),
  });
}

// ---------- Helpers ----------

function statusBadgeVariant(status: string) {
  switch (status) {
    case "completed":
      return "bg-green-500/10 text-green-500 border-0";
    case "failed":
      return "bg-red-500/10 text-red-500 border-0";
    case "running":
      return "bg-yellow-500/10 text-yellow-500 border-0";
    default:
      return "bg-gray-500/10 text-gray-500 border-0";
  }
}

function formatTime(iso: string) {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function LatencyBar({ ms, maxMs }: { ms: number; maxMs: number }) {
  const pct = maxMs > 0 ? Math.min((ms / maxMs) * 100, 100) : 0;
  return (
    <div className="flex items-center gap-2">
      <div className="h-2 flex-1 rounded-full bg-muted">
        <div
          className="h-2 rounded-full bg-blue-500"
          style={{ width: `${pct}%` }}
        />
      </div>
      <span className="text-xs text-muted-foreground w-16 text-right">
        {ms} ms
      </span>
    </div>
  );
}

// ---------- Sub-components ----------

function IndexerBreakdown({ queryId }: { queryId: string }) {
  const { data, isLoading } = useSearchLogDetail(queryId);

  if (isLoading) {
    return <Skeleton className="h-24 w-full rounded-md" />;
  }

  const indexers = data?.indexers ?? [];
  if (indexers.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">No indexer data recorded.</p>
    );
  }

  const maxLatency = Math.max(...indexers.map((i) => i.latency_ms), 1);

  return (
    <div className="space-y-2">
      {indexers.map((ix) => (
        <div
          key={ix.id}
          className="rounded-md border border-border p-2 space-y-1"
        >
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium">{ix.indexer_name}</span>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">
                {ix.result_count} results
              </span>
              <Badge
                variant="outline"
                className={statusBadgeVariant(ix.status)}
              >
                {ix.status}
              </Badge>
            </div>
          </div>
          <LatencyBar ms={ix.latency_ms} maxMs={maxLatency} />
          {ix.error && (
            <p className="text-xs text-red-400 break-all">{ix.error}</p>
          )}
        </div>
      ))}
    </div>
  );
}

function QueryRow({ entry }: { entry: QueryLogEntry }) {
  const [expanded, setExpanded] = React.useState(false);

  return (
    <div className="rounded-lg border border-border">
      <button
        type="button"
        className="flex w-full items-center justify-between gap-2 p-3 text-left hover:bg-muted/50"
        onClick={() => setExpanded((v) => !v)}
      >
        <div className="flex items-center gap-2 min-w-0 flex-1">
          <Search className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          <span className="text-sm font-medium truncate">
            {entry.query || "(empty)"}
          </span>
          <Badge variant="outline" className="text-[10px] shrink-0">
            {entry.query_type}
          </Badge>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <span className="text-xs text-muted-foreground">
            {entry.total_results} results
          </span>
          <Badge
            variant="outline"
            className={statusBadgeVariant(entry.status)}
          >
            {entry.status}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {formatTime(entry.started_at)}
          </span>
          {expanded ? (
            <ChevronUp className="h-3.5 w-3.5 text-muted-foreground" />
          ) : (
            <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
          )}
        </div>
      </button>
      {expanded && (
        <div className="border-t border-border p-3">
          <IndexerBreakdown queryId={entry.id} />
        </div>
      )}
    </div>
  );
}

// ---------- Main component ----------

export function SearchDiagnostics() {
  const [typeFilter, setTypeFilter] = React.useState<string>("all");
  const [statusFilter, setStatusFilter] = React.useState<string>("all");
  const { data, isLoading, isError, refetch } = useSearchLog(100, 0);
  const prune = usePruneSearchLog();

  const entries = React.useMemo(() => {
    let items = data?.data ?? [];
    if (typeFilter !== "all") {
      items = items.filter((e) => e.query_type === typeFilter);
    }
    if (statusFilter !== "all") {
      items = items.filter((e) => e.status === statusFilter);
    }
    return items;
  }, [data, typeFilter, statusFilter]);

  const handlePrune = () => {
    if (window.confirm("Delete search log entries older than 30 days?")) {
      prune.mutate(30);
    }
  };

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-3">
        <Select value={typeFilter} onValueChange={setTypeFilter}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Query type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All types</SelectItem>
            <SelectItem value="search">Search</SelectItem>
            <SelectItem value="rss">RSS</SelectItem>
            <SelectItem value="auto">Auto</SelectItem>
            <SelectItem value="rolling">Rolling</SelectItem>
          </SelectContent>
        </Select>

        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="running">Running</SelectItem>
            <SelectItem value="completed">Completed</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
          </SelectContent>
        </Select>

        <div className="flex-1" />

        <Button variant="outline" size="sm" className="gap-2" onClick={() => refetch()}>
          <RefreshCw className="h-3.5 w-3.5" />
          Refresh
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="gap-2 text-destructive"
          onClick={handlePrune}
          disabled={prune.isPending}
        >
          <Trash2 className="h-3.5 w-3.5" />
          Prune
        </Button>
      </div>

      {/* Loading / error states */}
      {isLoading && (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 rounded-lg" />
          ))}
        </div>
      )}

      {isError && (
        <p className="text-sm text-destructive">
          Failed to load search log. Will retry automatically.
        </p>
      )}

      {/* Entries */}
      {!isLoading && !isError && (
        <div className="space-y-2">
          {entries.length === 0 ? (
            <Card>
              <CardContent className="p-6 text-center text-sm text-muted-foreground">
                No search queries recorded yet.
              </CardContent>
            </Card>
          ) : (
            entries.map((entry) => (
              <QueryRow key={entry.id} entry={entry} />
            ))
          )}
        </div>
      )}
    </div>
  );
}
