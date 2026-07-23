import { useState, useRef, useEffect } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ConfirmActionButton } from "@/components/ui/confirm-action";
import { useAuth } from "@/hooks/use-auth";
import {
  useSystemLogs,
  useLogStream,
  useLogConfig,
  useUpdateLogConfig,
  useClearSystemLogs,
  type LogEntry,
  type LogListParams,
} from "@/lib/system-logs-api";
import { toast } from "sonner";
import {
  ArrowDown,
  ChevronDown,
  ChevronRight,
  Pause,
  Play,
  Search,
  Settings2,
  Trash2,
  Wifi,
  WifiOff,
} from "lucide-react";

const LEVEL_STYLES: Record<string, { bg: string; text: string }> = {
  debug: { bg: "bg-zinc-700", text: "text-zinc-300" },
  info: { bg: "bg-blue-900/60", text: "text-blue-300" },
  warn: { bg: "bg-yellow-900/60", text: "text-yellow-300" },
  error: { bg: "bg-red-900/60", text: "text-red-300" },
};

interface LogViewerProps {
  workflowId?: string;
  showConfig?: boolean;
  showStreamToggle?: boolean;
}

export function LogViewer({
  workflowId,
  showConfig = true,
  showStreamToggle = true,
}: LogViewerProps) {
  const [mode, setMode] = useState<"stream" | "history">("stream");
  const [levelFilter, setLevelFilter] = useState("");
  const [searchText, setSearchText] = useState("");
  const [page, setPage] = useState(0);
  const [paused, setPaused] = useState(false);

  // Stream mode
  const stream = useLogStream({
    workflowId,
    enabled: mode === "stream" && !paused,
  });

  // History mode
  const historyParams: LogListParams = {
    level: levelFilter || undefined,
    search: searchText || undefined,
    workflow_id: workflowId,
    limit: 100,
    offset: page * 100,
  };
  const history = useSystemLogs(
    mode === "history" ? historyParams : { limit: 0 },
  );

  const entries =
    mode === "stream" ? stream.entries : (history.data?.items ?? []);
  const total =
    mode === "history" ? (history.data?.total ?? 0) : stream.entries.length;

  return (
    <div className="space-y-3">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2">
        {showStreamToggle && (
          <div className="flex overflow-hidden rounded-md border border-border">
            <button
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${mode === "stream" ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-accent"}`}
              onClick={() => setMode("stream")}
            >
              Live
            </button>
            <button
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${mode === "history" ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground hover:bg-accent"}`}
              onClick={() => {
                setMode("history");
                setPage(0);
              }}
            >
              History
            </button>
          </div>
        )}

        {mode === "stream" && (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setPaused((p) => !p)}
            >
              {paused ? (
                <Play className="mr-1 h-3.5 w-3.5" />
              ) : (
                <Pause className="mr-1 h-3.5 w-3.5" />
              )}
              {paused ? "Resume" : "Pause"}
            </Button>
            <span className="flex items-center gap-1 text-xs text-muted-foreground">
              {stream.connected ? (
                <Wifi className="h-3 w-3 text-green-400" />
              ) : (
                <WifiOff className="h-3 w-3 text-red-400" />
              )}
              {stream.connected ? "Connected" : "Disconnected"}
            </span>
          </>
        )}

        {mode === "history" && (
          <>
            <select
              className="h-8 rounded-md border border-border bg-background px-2 text-xs"
              value={levelFilter}
              onChange={(e) => {
                setLevelFilter(e.target.value);
                setPage(0);
              }}
            >
              <option value="">All levels</option>
              <option value="debug">Debug</option>
              <option value="info">Info</option>
              <option value="warn">Warn</option>
              <option value="error">Error</option>
            </select>
            <div className="relative">
              <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                type="text"
                placeholder="Search messages…"
                className="h-8 w-48 rounded-md border border-border bg-background pl-7 pr-2 text-xs"
                value={searchText}
                onChange={(e) => {
                  setSearchText(e.target.value);
                  setPage(0);
                }}
              />
            </div>
          </>
        )}

        <span className="ml-auto text-xs text-muted-foreground">
          {total} entries
        </span>
      </div>

      {/* Log entries */}
      <LogTable entries={entries} />

      {/* Pagination for history mode */}
      {mode === "history" && total > 100 && (
        <div className="flex items-center justify-between">
          <Button
            variant="outline"
            size="sm"
            disabled={page === 0}
            onClick={() => setPage((p) => p - 1)}
          >
            Previous
          </Button>
          <span className="text-xs text-muted-foreground">
            Page {page + 1} of {Math.ceil(total / 100)}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={(page + 1) * 100 >= total}
            onClick={() => setPage((p) => p + 1)}
          >
            Next
          </Button>
        </div>
      )}

      {/* Config section */}
      {showConfig && <LogConfigSection />}
    </div>
  );
}

function LogTable({ entries }: { entries: LogEntry[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [entries.length, autoScroll]);

  const handleScroll = () => {
    if (!containerRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 50);
  };

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  if (entries.length === 0) {
    return (
      <div className="rounded-md border border-border bg-muted/30 p-8 text-center text-sm text-muted-foreground">
        No log entries to display
      </div>
    );
  }

  return (
    <div className="relative">
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="max-h-[500px] overflow-y-auto rounded-md border border-border bg-zinc-950 font-mono text-xs"
      >
        {entries.map((entry) => {
          const style = (LEVEL_STYLES[entry.level] ?? LEVEL_STYLES.info)!;
          const expanded = expandedIds.has(entry.id);
          const hasAttrs = entry.attrs && entry.attrs !== "{}";
          const ts = new Date(entry.timestamp).toLocaleTimeString();

          return (
            <div
              key={entry.id}
              className="border-b border-border/30 px-3 py-1.5 hover:bg-zinc-900/50"
            >
              <div
                className="flex cursor-pointer items-start gap-2"
                role="button"
                tabIndex={hasAttrs ? 0 : -1}
                onClick={() => hasAttrs && toggleExpand(entry.id)}
                onKeyDown={(e) => {
                  if (hasAttrs && (e.key === "Enter" || e.key === " ")) {
                    e.preventDefault();
                    toggleExpand(entry.id);
                  }
                }}
              >
                {hasAttrs &&
                  (expanded ? (
                    <ChevronDown className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
                  ))}
                <span className="shrink-0 text-muted-foreground">{ts}</span>
                <Badge
                  variant="outline"
                  className={`${style.bg} ${style.text} shrink-0 px-1.5 py-0 text-[10px] uppercase`}
                >
                  {entry.level}
                </Badge>
                <span className="flex-1 break-all text-zinc-200">
                  {entry.message}
                </span>
                {entry.source && (
                  <span className="shrink-0 text-[10px] text-muted-foreground">
                    {entry.source.split("/").slice(-2).join("/")}
                  </span>
                )}
              </div>
              {expanded && hasAttrs && (
                <pre className="ml-5 mt-1 overflow-x-auto rounded bg-zinc-900 p-2 text-[10px] text-zinc-400">
                  {JSON.stringify(JSON.parse(entry.attrs!), null, 2)}
                </pre>
              )}
            </div>
          );
        })}
      </div>

      {!autoScroll && (
        <button
          className="absolute bottom-2 right-2 rounded-full bg-primary p-1.5 text-primary-foreground shadow-lg hover:bg-primary/90"
          onClick={() => {
            setAutoScroll(true);
            containerRef.current?.scrollTo({
              top: containerRef.current.scrollHeight,
              behavior: "smooth",
            });
          }}
        >
          <ArrowDown className="h-3.5 w-3.5" />
        </button>
      )}
    </div>
  );
}

function LogConfigSection() {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const { data: config } = useLogConfig();
  const updateMut = useUpdateLogConfig();
  const clearMut = useClearSystemLogs();
  const [captureLevel, setCaptureLevel] = useState("");

  useEffect(() => {
    if (config) setCaptureLevel(config.capture_level);
  }, [config]);

  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-4 py-3">
        <div className="flex items-center gap-2">
          <Settings2 className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm font-medium">Capture Level</span>
          <select
            className="h-8 rounded-md border border-border bg-background px-2 text-xs"
            value={captureLevel}
            onChange={(e) => setCaptureLevel(e.target.value)}
          >
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>
          <Button
            variant="outline"
            size="sm"
            disabled={
              !isAdmin ||
              captureLevel === config?.capture_level ||
              updateMut.isPending
            }
            onClick={() => updateMut.mutate({ capture_level: captureLevel })}
          >
            Save
          </Button>
        </div>
        <div className="ml-auto">
          {isAdmin && (
            <ConfirmActionButton
              actionLabel="Clear Logs"
              title="Clear system logs?"
              description="Remove the stored system log history shown in this viewer."
              confirmLabel="Clear logs"
              pending={clearMut.isPending}
              icon={<Trash2 className="mr-1.5 h-3.5 w-3.5" />}
              onConfirm={async () => {
                try {
                  await clearMut.mutateAsync();
                  toast.success("System logs cleared");
                } catch {
                  toast.error("Failed to clear system logs");
                  throw new Error("clear system logs failed");
                }
              }}
            />
          )}
        </div>
      </CardContent>
    </Card>
  );
}
