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
import { useAuditLog, type AuditLogParams } from "@/lib/audit-log-api";
import { ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";

const PAGE_SIZE = 50;

function levelVariant(level: string) {
  switch (level) {
    case "error":
      return "destructive" as const;
    case "warn":
      return "secondary" as const;
    default:
      return "outline" as const;
  }
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

export function EventsPage() {
  useSetPageHeader("Events", "System audit log");

  const [category, setCategory] = React.useState<string>("all");
  const [level, setLevel] = React.useState<string>("all");
  const [offset, setOffset] = React.useState(0);
  const qc = useQueryClient();

  const params: AuditLogParams = {
    limit: PAGE_SIZE,
    offset,
    ...(category !== "all" && { category }),
    ...(level !== "all" && { level }),
  };

  const { data, isLoading, isError } = useAuditLog(params);

  // Reset offset when filters change
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
            <SelectItem value="all">All Categories</SelectItem>
            <SelectItem value="indexer">Indexer</SelectItem>
            <SelectItem value="download">Download</SelectItem>
            <SelectItem value="import">Import</SelectItem>
            <SelectItem value="system">System</SelectItem>
            <SelectItem value="auth">Auth</SelectItem>
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
              <TableHead className="w-[80px]">Level</TableHead>
              <TableHead className="w-[100px]">Category</TableHead>
              <TableHead>Message</TableHead>
              <TableHead className="w-[140px]">Entity</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                  Loading…
                </TableCell>
              </TableRow>
            )}
            {isError && (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-destructive py-8">
                  Failed to load audit log.
                </TableCell>
              </TableRow>
            )}
            {!isLoading && !isError && entries.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                  No events found.
                </TableCell>
              </TableRow>
            )}
            {entries.map((e) => (
              <TableRow key={e.id}>
                <TableCell className="text-xs tabular-nums text-muted-foreground">
                  {formatTimestamp(e.timestamp)}
                </TableCell>
                <TableCell>
                  <Badge variant={levelVariant(e.level)} className="capitalize text-[10px]">
                    {e.level}
                  </Badge>
                </TableCell>
                <TableCell className="text-xs capitalize">{e.category}</TableCell>
                <TableCell className="text-sm">{e.message}</TableCell>
                <TableCell className="text-xs text-muted-foreground truncate max-w-[140px]">
                  {e.entity_name ?? "—"}
                </TableCell>
              </TableRow>
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
