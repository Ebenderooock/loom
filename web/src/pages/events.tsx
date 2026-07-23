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
import { ConfirmActionButton } from "@/components/ui/confirm-action";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useAuth } from "@/hooks/use-auth";
import {
  useAuditLog,
  useClearAuditLog,
  type AuditLogParams,
  type AuditLogEntry,
} from "@/lib/audit-log-api";
import { levelVariant } from "@/lib/status-utils";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";
import {
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  ChevronUp,
  RefreshCw,
} from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";
import { toast } from "sonner";

const PAGE_SIZE = 50;

const CATEGORIES = [
  { value: "all", label: "All Categories" },
  { value: "indexer", label: "Indexer" },
  { value: "search", label: "Search" },
  { value: "download", label: "Download" },
  { value: "import", label: "Import" },
  { value: "safety", label: "Safety Review" },
  { value: "system", label: "System" },
  { value: "auth", label: "Auth" },
];

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

function tryParseJSON(s?: string): Record<string, unknown> | null {
  if (!s) return null;
  try {
    return JSON.parse(s);
  } catch {
    return null;
  }
}

// ─── Search diagnostics inline (fetches per-indexer breakdown) ───────

interface IndexerQueryEntry {
  id: string;
  indexer_id: string;
  indexer_name: string;
  latency_ms: number;
  result_count: number;
  error?: string;
  status: string;
}

interface QueryLogDetail {
  id: string;
  query: string;
  query_type: string;
  total_results: number;
  status: string;
  indexers?: IndexerQueryEntry[];
}

function useSearchLogDetail(id: string | null) {
  return useQuery({
    queryKey: ["search-log", id],
    queryFn: async () => {
      const res = await apiFetch(`/api/v1/search/log/${id}`);
      if (!res.ok) throw new Error(`search log: ${res.status}`);
      return (await res.json()) as QueryLogDetail;
    },
    enabled: !!id,
  });
}

function SearchBreakdown({ queryLogId }: { queryLogId: string }) {
  const { data, isLoading } = useSearchLogDetail(queryLogId);

  if (isLoading) {
    return (
      <p className="text-muted-foreground animate-pulse text-xs">
        Loading search diagnostics…
      </p>
    );
  }

  const indexers = data?.indexers ?? [];
  if (indexers.length === 0) {
    return (
      <p className="text-muted-foreground text-xs">
        No per-indexer data recorded.
      </p>
    );
  }

  const maxLatency = Math.max(
    ...indexers.map((indexer: IndexerQueryEntry) => indexer.latency_ms),
    1,
  );

  return (
    <div className="mt-2 space-y-1.5">
      <p className="text-muted-foreground text-[10px] font-medium tracking-wide uppercase">
        Per-Indexer Breakdown
      </p>
      {indexers.map((ix: IndexerQueryEntry) => (
        <div key={ix.id} className="flex items-center gap-2 text-xs">
          <span className="w-28 truncate font-medium">{ix.indexer_name}</span>
          <div className="bg-muted h-1.5 flex-1 rounded-full">
            <div
              className={`h-1.5 rounded-full ${ix.status === "completed" ? "bg-blue-500" : "bg-red-500"}`}
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
            className={`px-1 text-[9px] ${
              ix.status === "completed"
                ? "border-0 bg-green-500/10 text-green-500"
                : ix.status === "failed"
                  ? "border-0 bg-red-500/10 text-red-500"
                  : "border-0 bg-gray-500/10 text-gray-500"
            }`}
          >
            {ix.status}
          </Badge>
          {ix.error && (
            <span className="max-w-[12rem] truncate text-red-400">
              {ix.error}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

function DetailPanel({ entry }: { entry: AuditLogEntry }) {
  const detail = tryParseJSON(entry.detail);
  if (!detail) return null;

  const queryLogId =
    typeof detail.query_log_id === "string" ? detail.query_log_id : null;

  return (
    <div className="bg-muted/50 max-w-2xl space-y-1 rounded p-3 text-xs">
      {Object.entries(detail).map(([k, v]) => (
        <div key={k} className="flex gap-2">
          <span className="text-muted-foreground min-w-[120px] font-medium">
            {k}:
          </span>
          <span className="text-foreground break-all">
            {typeof v === "object" ? JSON.stringify(v) : String(v ?? "—")}
          </span>
        </div>
      ))}
      {entry.entity_type && (
        <div className="flex gap-2">
          <span className="text-muted-foreground min-w-[120px] font-medium">
            entity_type:
          </span>
          <span className="text-foreground">{entry.entity_type}</span>
        </div>
      )}
      {entry.source && (
        <div className="flex gap-2">
          <span className="text-muted-foreground min-w-[120px] font-medium">
            source:
          </span>
          <span className="text-foreground">{entry.source}</span>
        </div>
      )}
      {queryLogId && <SearchBreakdown queryLogId={queryLogId} />}
    </div>
  );
}

function EventRow({
  entry,
  expanded,
  onToggle,
}: {
  entry: AuditLogEntry;
  expanded: boolean;
  onToggle: () => void;
}) {
  const hasDetail = !!entry.detail;
  const Chevron = expanded ? ChevronUp : ChevronDown;

  return (
    <>
      <TableRow
        className={hasDetail ? "hover:bg-muted/40 cursor-pointer" : undefined}
        onClick={hasDetail ? onToggle : undefined}
      >
        <TableCell className="text-muted-foreground text-xs tabular-nums">
          {formatTimestamp(entry.occurred_at || entry.timestamp)}
        </TableCell>
        <TableCell>
          <Badge
            variant={levelVariant(entry.level)}
            className="text-[10px] capitalize"
          >
            {entry.level}
          </Badge>
        </TableCell>
        <TableCell className="text-xs capitalize">{entry.category}</TableCell>
        <TableCell className="text-muted-foreground text-xs">
          {entry.event_type}
        </TableCell>
        <TableCell className="text-sm">
          <span className="flex items-center gap-1">
            {entry.message}
            {hasDetail && (
              <Chevron className="text-muted-foreground h-3 w-3 shrink-0" />
            )}
          </span>
        </TableCell>
        <TableCell className="text-muted-foreground max-w-[140px] truncate text-xs">
          {entry.entity_name ?? "—"}
        </TableCell>
      </TableRow>
      {expanded && (
        <TableRow className="bg-muted/20 hover:bg-muted/20">
          <TableCell colSpan={6} className="p-2 pl-6">
            <DetailPanel entry={entry} />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

export function EventsPage() {
  useSetPageHeader(
    "Events",
    "Centralized audit log — all system activity in one place",
  );

  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const [category, setCategory] = React.useState<string>("all");
  const [level, setLevel] = React.useState<string>("all");
  const [offset, setOffset] = React.useState(0);
  const [expandedId, setExpandedId] = React.useState<string | null>(null);
  const qc = useQueryClient();
  const clearAudit = useClearAuditLog();

  const params: AuditLogParams = {
    limit: PAGE_SIZE,
    offset,
    ...(category !== "all" && { category }),
    ...(level !== "all" && { level }),
  };

  const { data, isLoading, isError } = useAuditLog(params);

  React.useEffect(() => setOffset(0), [category, level]);

  const entries = data?.entries ?? [];
  const total = data?.total ?? 0;
  const hasNext = offset + PAGE_SIZE < total;
  const hasPrev = offset > 0;

  return (
    <div className="space-y-4">
      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <Select value={category} onValueChange={setCategory}>
          <SelectTrigger className="w-[160px]">
            <SelectValue placeholder="Category" />
          </SelectTrigger>
          <SelectContent>
            {CATEGORIES.map((c) => (
              <SelectItem key={c.value} value={c.value}>
                {c.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={level} onValueChange={setLevel}>
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Level" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Levels</SelectItem>
            <SelectItem value="info">Info</SelectItem>
            <SelectItem value="warn">Warning</SelectItem>
            <SelectItem value="error">Error</SelectItem>
          </SelectContent>
        </Select>

        <Button
          variant="outline"
          size="icon"
          onClick={() =>
            qc.invalidateQueries({ queryKey: ["system", "audit-log"] })
          }
          aria-label="Refresh"
        >
          <RefreshCw className="h-4 w-4" />
        </Button>

        {isAdmin && (
          <ConfirmActionButton
            actionLabel="Clear History"
            title="Clear event history?"
            description="Remove the stored audit log entries from the events view."
            confirmLabel="Clear events"
            pending={clearAudit.isPending}
            icon={<RefreshCw className="mr-1.5 h-3.5 w-3.5" />}
            onConfirm={async () => {
              try {
                await clearAudit.mutateAsync();
                setExpandedId(null);
                setOffset(0);
                toast.success("Event history cleared");
              } catch {
                toast.error("Failed to clear event history");
                throw new Error("clear audit history failed");
              }
            }}
          />
        )}

        <span className="text-muted-foreground ml-auto text-xs">
          {total} event{total !== 1 ? "s" : ""}
        </span>
      </div>

      {/* Table */}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[160px]">Time</TableHead>
              <TableHead className="w-[70px]">Level</TableHead>
              <TableHead className="w-[100px]">Category</TableHead>
              <TableHead className="w-[140px]">Event Type</TableHead>
              <TableHead>Message</TableHead>
              <TableHead className="w-[140px]">Entity</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell colSpan={6}>
                  <LoadingState label="Loading events…" />
                </TableCell>
              </TableRow>
            )}
            {isError && (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="text-destructive py-8 text-center"
                >
                  Failed to load audit log.
                </TableCell>
              </TableRow>
            )}
            {!isLoading && !isError && entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={6}>
                  <EmptyState
                    title="No events found"
                    description="Events will appear here as the system operates."
                  />
                </TableCell>
              </TableRow>
            )}
            {entries.map((e) => (
              <EventRow
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
            <ChevronLeft className="mr-1 h-4 w-4" /> Previous
          </Button>
          <span className="text-muted-foreground text-xs">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasNext}
            onClick={() => setOffset((o) => o + PAGE_SIZE)}
          >
            Next <ChevronRight className="ml-1 h-4 w-4" />
          </Button>
        </div>
      )}
    </div>
  );
}
