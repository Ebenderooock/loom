// DownloadsPage shows active downloads (from all configured download clients)
// and the import/file-handling queue. This is the real-time operational view;
// completed/failed history lives on the Activity (History) page.

import * as React from "react";
import {
  Download,
  ArrowDown,
  ArrowUp,
  RefreshCw,
  Import,
} from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Progress } from "@/components/ui/progress";
import { ImportManager } from "@/components/imports/import-manager";

// ─── Types ──────────────────────────────────────────────────────────────

interface QueueItem {
  id: string;
  title: string;
  category: string;
  status: string;
  progress: number;
  size_bytes: number;
  downloaded_bytes: number;
  eta_seconds: number;
  download_rate: number;
  upload_rate: number;
  ratio: number;
  message: string;
  save_path: string;
}

// ─── Helpers ────────────────────────────────────────────────────────────

import { formatBytes, formatSpeed, formatEta } from "@/lib/utils";
import { downloadStatusConfig } from "@/lib/status-utils";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";

// ─── Queue Stats Bar ────────────────────────────────────────────────────

function QueueStats({ items }: { items: QueueItem[] }) {
  const downloading = items.filter((i) => i.status === "downloading");
  const seeding = items.filter((i) => i.status === "seeding");
  const queued = items.filter((i) => i.status === "queued");
  const paused = items.filter((i) => i.status === "paused");

  const totalDown = downloading.reduce((s, i) => s + (i.download_rate || 0), 0);
  const totalUp = [...downloading, ...seeding].reduce((s, i) => s + (i.upload_rate || 0), 0);

  return (
    <div className="flex flex-wrap items-center gap-4 text-xs text-zinc-400">
      {downloading.length > 0 && (
        <span className="flex items-center gap-1.5">
          <ArrowDown className="h-3.5 w-3.5 text-blue-400" />
          <span className="font-medium text-zinc-200">{formatSpeed(totalDown)}</span>
          <span className="text-zinc-600">({downloading.length} active)</span>
        </span>
      )}
      {totalUp > 0 && (
        <span className="flex items-center gap-1.5">
          <ArrowUp className="h-3.5 w-3.5 text-green-400" />
          <span className="font-medium text-zinc-200">{formatSpeed(totalUp)}</span>
        </span>
      )}
      {seeding.length > 0 && (
        <span className="text-green-400">{seeding.length} seeding</span>
      )}
      {queued.length > 0 && (
        <span className="text-yellow-400">{queued.length} queued</span>
      )}
      {paused.length > 0 && (
        <span className="text-zinc-500">{paused.length} paused</span>
      )}
      {items.length === 0 && <span>No active downloads</span>}
    </div>
  );
}

// ─── Queue Item Row ─────────────────────────────────────────────────────

function QueueItemRow({ item }: { item: QueueItem }) {
  const sc = downloadStatusConfig(item.status);
  const pct = Math.min(100, (item.progress ?? 0) * 100);
  const isActive = item.status === "downloading";

  return (
    <div className="group flex items-center gap-4 px-4 py-3 border-b border-zinc-800/50 last:border-0 hover:bg-zinc-800/30 transition-colors">
      {/* Title + message */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-zinc-200 truncate">{item.title}</span>
          {item.category && (
            <Badge variant="outline" className="text-[10px] border-zinc-700 text-zinc-500 shrink-0">
              {item.category}
            </Badge>
          )}
        </div>
        {item.message && (
          <p className="text-xs text-zinc-500 truncate mt-0.5">{item.message}</p>
        )}

        {/* Progress bar (only for downloading/queued/paused) */}
        {item.status !== "seeding" && item.status !== "completed" && (
          <div className="flex items-center gap-2 mt-1.5">
            <Progress value={pct} className="h-1.5 flex-1 bg-zinc-800" />
            <span className="text-[11px] text-zinc-500 tabular-nums w-10 text-right">
              {pct.toFixed(0)}%
            </span>
          </div>
        )}
      </div>

      {/* Status badge */}
      <Badge variant={sc.variant} className={`text-[10px] shrink-0 ${sc.className ?? ""}`}>
        {sc.label}
      </Badge>

      {/* Size */}
      <div className="hidden sm:block w-28 text-right text-xs text-zinc-500 tabular-nums">
        {item.downloaded_bytes > 0 && item.size_bytes > 0
          ? `${formatBytes(item.downloaded_bytes)} / ${formatBytes(item.size_bytes)}`
          : item.size_bytes > 0
          ? formatBytes(item.size_bytes)
          : "—"}
      </div>

      {/* Speeds */}
      <div className="hidden md:flex flex-col items-end gap-0.5 w-24">
        {item.download_rate > 0 && (
          <span className="flex items-center gap-1 text-xs text-zinc-400">
            <ArrowDown className="h-3 w-3 text-blue-400" />
            {formatSpeed(item.download_rate)}
          </span>
        )}
        {item.upload_rate > 0 && (
          <span className="flex items-center gap-1 text-xs text-zinc-400">
            <ArrowUp className="h-3 w-3 text-green-400" />
            {formatSpeed(item.upload_rate)}
          </span>
        )}
        {item.download_rate <= 0 && item.upload_rate <= 0 && (
          <span className="text-xs text-zinc-600">—</span>
        )}
      </div>

      {/* ETA */}
      <div className="hidden lg:block w-16 text-right text-xs text-zinc-500 tabular-nums">
        {isActive ? formatEta(item.eta_seconds) : "—"}
      </div>

      {/* Ratio (for seeding) */}
      {item.status === "seeding" && item.ratio > 0 && (
        <div className="hidden lg:block w-14 text-right text-xs text-zinc-500 tabular-nums">
          {item.ratio.toFixed(2)}
        </div>
      )}
    </div>
  );
}

// ─── Active Downloads Tab ───────────────────────────────────────────────

function ActiveDownloads() {
  const [items, setItems] = React.useState<QueueItem[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [refreshing, setRefreshing] = React.useState(false);

  const fetchActivity = React.useCallback(async (manual = false) => {
    if (manual) setRefreshing(true);
    try {
      const res = await fetch("/api/v1/activity", { credentials: "include" });
      if (res.ok) {
        const body = await res.json();
        setItems(body.items ?? []);
      }
    } catch {
      // silently fail on polling
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  React.useEffect(() => {
    fetchActivity();
    const interval = setInterval(() => fetchActivity(), 5000);
    return () => clearInterval(interval);
  }, [fetchActivity]);

  // Sort: downloading first, then queued, seeding, paused, completed, failed
  const statusOrder: Record<string, number> = {
    downloading: 0,
    queued: 1,
    paused: 2,
    seeding: 3,
    completed: 4,
    failed: 5,
  };
  const sorted = [...items].sort(
    (a, b) => (statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9)
  );

  if (loading) {
    return (
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent>
          <LoadingState label="Connecting to download clients…" />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <QueueStats items={items} />
        <Button
          variant="ghost"
          size="sm"
          onClick={() => fetchActivity(true)}
          disabled={refreshing}
          className="text-zinc-400 hover:text-zinc-200 h-8"
        >
          <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${refreshing ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {items.length === 0 ? (
        <Card className="bg-zinc-900/50 border-zinc-800 border-dashed">
          <CardContent>
            <EmptyState
              icon={<Download className="h-10 w-10" />}
              title="No active downloads"
              description="Downloads will appear here when you search and grab releases"
            />
          </CardContent>
        </Card>
      ) : (
        <Card className="bg-zinc-900/50 border-zinc-800 overflow-hidden">
          <div className="divide-y divide-zinc-800/50">
            {sorted.map((item) => (
              <QueueItemRow key={item.id} item={item} />
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}

// ─── Downloads Page ─────────────────────────────────────────────────────

export function DownloadsPage() {
  useSetPageHeader("Downloads");

  return (
    <div className="space-y-6">
      <Tabs defaultValue="active">
        <TabsList>
          <TabsTrigger value="active" className="flex items-center gap-1.5">
            <Download className="h-3.5 w-3.5" />
            Active
          </TabsTrigger>
          <TabsTrigger value="imports" className="flex items-center gap-1.5">
            <Import className="h-3.5 w-3.5" />
            Imports
          </TabsTrigger>
        </TabsList>
        <TabsContent value="active">
          <ActiveDownloads />
        </TabsContent>
        <TabsContent value="imports">
          <ImportManager />
        </TabsContent>
      </Tabs>
    </div>
  );
}
