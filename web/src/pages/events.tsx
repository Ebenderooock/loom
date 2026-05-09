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
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useAuditLog, type AuditLogParams, type AuditLogEntry } from "@/lib/audit-log-api";
import { levelVariant } from "@/lib/status-utils";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";
import { ChevronLeft, ChevronRight, ChevronDown, ChevronUp, RefreshCw } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";

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

function DetailPanel({ entry }: { entry: AuditLogEntry }) {
  const detail = tryParseJSON(entry.detail);
  if (!detail) return null;

  return (
    <div className="bg-muted/50 rounded p-3 text-xs space-y-1 max-w-2xl">
      {Object.entries(detail).map(([k, v]) => (
        <div key={k} className="flex gap-2">
          <span className="text-muted-foreground font-medium min-w-[120px]">{k}:</span>
          <span className="text-foreground break-all">
            {typeof v === "object" ? JSON.stringify(v) : String(v ?? "—")}
          </span>
        </div>
      ))}
      {entry.entity_type && (
        <div className="flex gap-2">
          <span className="text-muted-foreground font-medium min-w-[120px]">entity_type:</span>
          <span className="text-foreground">{entry.entity_type}</span>
        </div>
      )}
      {entry.source && (
        <div className="flex gap-2">
          <span className="text-muted-foreground font-medium min-w-[120px]">source:</span>
          <span className="text-foreground">{entry.source}</span>
        </div>
      )}
    </div>
  );
}

function EventRow({ entry, expanded, onToggle }: { entry: AuditLogEntry; expanded: boolean; onToggle: () => void }) {
  const hasDetail = !!entry.detail;
  const Chevron = expanded ? ChevronUp : ChevronDown;

  return (
    <>
      <TableRow
        className={hasDetail ? "cursor-pointer hover:bg-muted/40" : undefined}
        onClick={hasDetail ? onToggle : undefined}
      >
        <TableCell className="text-xs tabular-nums text-muted-foreground">
          {formatTimestamp(entry.occurred_at || entry.timestamp)}
        </TableCell>
        <TableCell>
          <Badge variant={levelVariant(entry.level)} className="capitalize text-[10px]">
            {entry.level}
          </Badge>
        </TableCell>
        <TableCell className="text-xs capitalize">{entry.category}</TableCell>
        <TableCell className="text-xs text-muted-foreground">{entry.event_type}</TableCell>
        <TableCell className="text-sm">
          <span className="flex items-center gap-1">
            {entry.message}
            {hasDetail && <Chevron className="h-3 w-3 text-muted-foreground shrink-0" />}
          </span>
        </TableCell>
        <TableCell className="text-xs text-muted-foreground truncate max-w-[140px]">
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
  useSetPageHeader("Events", "Centralized audit log — all system activity in one place");

  const [category, setCategory] = React.useState<string>("all");
  const [level, setLevel] = React.useState<string>("all");
  const [offset, setOffset] = React.useState(0);
  const [expandedId, setExpandedId] = React.useState<string | null>(null);
  const qc = useQueryClient();

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
              <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>
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

        <span className="ml-auto text-xs text-muted-foreground">
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
                <TableCell colSpan={6} className="text-center text-destructive py-8">
                  Failed to load audit log.
                </TableCell>
              </TableRow>
            )}
            {!isLoading && !isError && entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={6}>
                  <EmptyState title="No events found" description="Events will appear here as the system operates." />
                </TableCell>
              </TableRow>
            )}
            {entries.map((e) => (
              <EventRow
                key={e.id}
                entry={e}
                expanded={expandedId === e.id}
                onToggle={() => setExpandedId(expandedId === e.id ? null : e.id)}
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
