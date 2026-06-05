// DownloadsPage shows active downloads (from all configured download clients)
// and the import/file-handling queue. This is the real-time operational view;
// completed/failed history lives on the Activity (History) page.

import * as React from "react";
import {
  Download,
  ArrowDown,
  ArrowUp,
  RefreshCw,
  Loader2,
  Import,
  Square,
  Pause,
  Play,
  FolderInput,
  Users,
  FileText,
  Radio,
  Info,
  Gauge,
  Trash2,
  EyeOff,
} from "lucide-react";
import { apiFetch } from "@/lib/fetch";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Progress } from "@/components/ui/progress";
import { ImportManager } from "@/components/imports/import-manager";
import {
  useDownloads,
  useTorrentStatus,
  useSetTorrentSpeedLimits,
  useTorrentPauseAll,
  useTorrentResumeAll,
} from "@/lib/downloads-api";
import {
  useOrphans,
  useCleanupSettings,
  useScanCleanup,
  useSaveCleanupSettings,
  useApproveOrphan,
  useIgnoreOrphan,
  type Orphan,
} from "@/lib/cleanup-api";
import { toast } from "sonner";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// ─── Types ──────────────────────────────────────────────────────────────

interface QueueItem {
  id: string;
  client_id: string;
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

interface PeerInfo {
  ip: string;
  port: number;
  client: string;
  flags: string;
  progress: number;
  down_rate: number;
  up_rate: number;
}

interface FileInfo {
  path: string;
  size: number;
  progress: number;
  priority: string;
}

interface TrackerInfo {
  url: string;
  tier: number;
  status: string;
  peers: number;
}

interface TorrentDetail {
  Hash: string;
  Title: string;
  Category: string;
  SavePath: string;
  Status: string;
  Progress: number;
  SizeBytes: number;
  Downloaded: number;
  Uploaded: number;
  DownloadRate: number;
  UploadRate: number;
  Ratio: number;
  Paused: boolean;
  peers: PeerInfo[];
  files: FileInfo[];
  trackers: TrackerInfo[];
  total_peers: number;
  total_seeds: number;
  added_at: string;
  comment: string;
  created_by: string;
  info_hash: string;
}

// ─── Helpers ────────────────────────────────────────────────────────────

import { formatBytes, formatSpeed, formatEta } from "@/lib/utils";
import { downloadStatusConfig } from "@/lib/status-utils";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";

// ─── Actions ────────────────────────────────────────────────────────────

async function activityAction(
  endpoint: string,
  clientId: string,
  ids: string[],
  extra?: Record<string, unknown>,
) {
  const res = await apiFetch(`/api/v1/activity/${endpoint}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ client_id: clientId, ids, ...extra }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Action failed (${res.status})`);
  }
}

async function forceImport(path: string) {
  const res = await apiFetch("/api/v1/imports/manual", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Import failed (${res.status})`);
  }
}

// ─── Queue Stats Bar ────────────────────────────────────────────────────

function QueueStats({ items }: { items: QueueItem[] }) {
  const downloading = items.filter((i) => i.status === "downloading");
  const seeding = items.filter((i) => i.status === "seeding");
  const queued = items.filter((i) => i.status === "queued");
  const paused = items.filter((i) => i.status === "paused");

  const totalDown = downloading.reduce((s, i) => s + (i.download_rate || 0), 0);
  const totalUp = [...downloading, ...seeding].reduce(
    (s, i) => s + (i.upload_rate || 0),
    0,
  );

  return (
    <div className="flex flex-wrap items-center gap-4 text-xs text-zinc-400">
      {downloading.length > 0 && (
        <span className="flex items-center gap-1.5">
          <ArrowDown className="h-3.5 w-3.5 text-blue-400" />
          <span className="font-medium text-zinc-200">
            {formatSpeed(totalDown)}
          </span>
          <span className="text-zinc-600">({downloading.length} active)</span>
        </span>
      )}
      {totalUp > 0 && (
        <span className="flex items-center gap-1.5">
          <ArrowUp className="h-3.5 w-3.5 text-green-400" />
          <span className="font-medium text-zinc-200">
            {formatSpeed(totalUp)}
          </span>
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

function QueueItemRow({
  item,
  onRefresh,
  onSelect,
}: {
  item: QueueItem;
  onRefresh: () => void;
  onSelect: (item: QueueItem) => void;
}) {
  const sc = downloadStatusConfig(item.status);
  const pct = Math.min(100, (item.progress ?? 0) * 100);
  const isActive = item.status === "downloading";
  const isPaused = item.status === "paused";
  const isCompleted = item.status === "completed";
  const canPauseResume = isActive || isPaused || item.status === "queued";
  const [busy, setBusy] = React.useState(false);

  const runAction = async (label: string, fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
      toast.success(`${label}: ${item.title}`);
      onRefresh();
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : `${label} failed`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="group flex cursor-pointer items-center gap-4 border-b border-zinc-800/50 px-4 py-3 transition-colors last:border-0 hover:bg-zinc-800/30"
      role="button"
      tabIndex={0}
      onClick={() => onSelect(item)}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(item);
        }
      }}
    >
      {/* Title + message */}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium text-zinc-200">
            {item.title}
          </span>
          {item.category && (
            <Badge
              variant="outline"
              className="shrink-0 border-zinc-700 text-[10px] text-zinc-500"
            >
              {item.category}
            </Badge>
          )}
        </div>
        {item.message && (
          <p className="mt-0.5 truncate text-xs text-zinc-500">
            {item.message}
          </p>
        )}

        {/* Progress bar (only for downloading/queued/paused) */}
        {item.status !== "seeding" && !isCompleted && (
          <div className="mt-1.5 flex items-center gap-2">
            <Progress value={pct} className="h-1.5 flex-1 bg-zinc-800" />
            <span className="w-10 text-right text-[11px] tabular-nums text-zinc-500">
              {pct.toFixed(0)}%
            </span>
          </div>
        )}
      </div>

      {/* Status badge */}
      <Badge
        variant={sc.variant}
        className={`shrink-0 text-[10px] ${sc.className ?? ""}`}
      >
        {sc.label}
      </Badge>

      {/* Size */}
      <div className="hidden w-28 text-right text-xs tabular-nums text-zinc-500 sm:block">
        {item.downloaded_bytes > 0 && item.size_bytes > 0
          ? `${formatBytes(item.downloaded_bytes)} / ${formatBytes(item.size_bytes)}`
          : item.size_bytes > 0
            ? formatBytes(item.size_bytes)
            : "—"}
      </div>

      {/* Speeds */}
      <div className="hidden w-24 flex-col items-end gap-0.5 md:flex">
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
      <div className="hidden w-16 text-right text-xs tabular-nums text-zinc-500 lg:block">
        {isActive ? formatEta(item.eta_seconds) : "—"}
      </div>

      {/* Ratio (for seeding) */}
      {item.status === "seeding" && item.ratio > 0 && (
        <div className="hidden w-14 text-right text-xs tabular-nums text-zinc-500 lg:block">
          {item.ratio.toFixed(2)}
        </div>
      )}

      {/* Actions */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions, jsx-a11y/click-events-have-key-events -- non-interactive wrapper; only stops row activation so its child buttons work */}
      <div
        className="flex items-center gap-1"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Pause / Resume */}
        {canPauseResume && (
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-zinc-200"
            disabled={busy}
            title={isPaused ? "Resume" : "Pause"}
            onClick={() =>
              runAction(isPaused ? "Resumed" : "Paused", () =>
                activityAction(isPaused ? "resume" : "pause", item.client_id, [
                  item.id,
                ]),
              )
            }
          >
            {isPaused ? (
              <Play className="h-3.5 w-3.5" />
            ) : (
              <Pause className="h-3.5 w-3.5" />
            )}
          </Button>
        )}

        {/* Force Import (completed items only) */}
        {isCompleted && item.save_path && (
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-emerald-400"
            disabled={busy}
            title="Force Import"
            onClick={() =>
              runAction("Import started", () => forceImport(item.save_path))
            }
          >
            <FolderInput className="h-3.5 w-3.5" />
          </Button>
        )}

        {/* Stop / Remove */}
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 text-zinc-400 hover:text-red-400"
          disabled={busy}
          title="Stop &amp; remove (delete files)"
          onClick={() =>
            runAction("Stopped", () =>
              activityAction("remove", item.client_id, [item.id], {
                delete_files: true,
              }),
            )
          }
        >
          <Square className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

// ─── Torrent Detail Panel ────────────────────────────────────────────────

function TorrentDetailPanel({ item }: { item: QueueItem }) {
  const [detail, setDetail] = React.useState<TorrentDetail | null>(null);
  const [loading, setLoading] = React.useState(true);
  const isTorrent = React.useRef(false);

  React.useEffect(() => {
    let active = true;

    const fetchDetail = async () => {
      try {
        const res = await apiFetch(
          `/api/v1/activity/detail?client_id=${encodeURIComponent(item.client_id)}&item_id=${encodeURIComponent(item.id)}`,
        );
        if (!res.ok || !active) return;
        const body = await res.json();
        if (!active) return;
        // Detect if this is a torrent detail (has peers array) vs basic item
        if (body.peers !== undefined) {
          isTorrent.current = true;
          setDetail(body);
        } else {
          isTorrent.current = false;
          setDetail(null);
        }
      } catch {
        // ignore polling errors
      } finally {
        if (active) setLoading(false);
      }
    };

    fetchDetail();
    const interval = setInterval(fetchDetail, 3000);
    return () => {
      active = false;
      clearInterval(interval);
    };
  }, [item.client_id, item.id]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-zinc-500">
        Loading…
      </div>
    );
  }

  // Non-torrent clients: show overview only
  if (!isTorrent.current || !detail) {
    return <OverviewTab item={item} detail={null} />;
  }

  return (
    <Tabs defaultValue="overview" className="mt-4">
      <TabsList className="grid w-full grid-cols-4">
        <TabsTrigger value="overview" className="gap-1 text-xs">
          <Info className="h-3 w-3" />
          Overview
        </TabsTrigger>
        <TabsTrigger value="peers" className="gap-1 text-xs">
          <Users className="h-3 w-3" />
          Peers
        </TabsTrigger>
        <TabsTrigger value="files" className="gap-1 text-xs">
          <FileText className="h-3 w-3" />
          Files
        </TabsTrigger>
        <TabsTrigger value="trackers" className="gap-1 text-xs">
          <Radio className="h-3 w-3" />
          Trackers
        </TabsTrigger>
      </TabsList>

      <TabsContent value="overview">
        <OverviewTab item={item} detail={detail} />
      </TabsContent>
      <TabsContent value="peers">
        <PeersTab detail={detail} />
      </TabsContent>
      <TabsContent value="files">
        <FilesTab detail={detail} />
      </TabsContent>
      <TabsContent value="trackers">
        <TrackersTab detail={detail} />
      </TabsContent>
    </Tabs>
  );
}

function DetailRow({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex justify-between border-b border-zinc-800/50 py-1.5 last:border-0">
      <span className="text-xs text-zinc-500">{label}</span>
      <span className="max-w-[60%] truncate text-right text-xs text-zinc-200">
        {value}
      </span>
    </div>
  );
}

function OverviewTab({
  item,
  detail,
}: {
  item: QueueItem;
  detail: TorrentDetail | null;
}) {
  const sc = downloadStatusConfig(item.status);
  const pct = Math.min(100, (item.progress ?? 0) * 100);

  return (
    <div className="mt-3 space-y-3">
      <div className="flex items-center gap-2">
        <Badge
          variant={sc.variant}
          className={`text-[10px] ${sc.className ?? ""}`}
        >
          {sc.label}
        </Badge>
        <span className="text-xs tabular-nums text-zinc-500">
          {pct.toFixed(1)}%
        </span>
      </div>
      <Progress value={pct} className="h-2 bg-zinc-800" />

      <div className="space-y-0">
        <DetailRow
          label="Size"
          value={item.size_bytes > 0 ? formatBytes(item.size_bytes) : "—"}
        />
        <DetailRow
          label="Downloaded"
          value={
            item.downloaded_bytes > 0 ? formatBytes(item.downloaded_bytes) : "—"
          }
        />
        {detail && (
          <DetailRow
            label="Uploaded"
            value={detail.Uploaded > 0 ? formatBytes(detail.Uploaded) : "—"}
          />
        )}
        <DetailRow
          label="Ratio"
          value={item.ratio > 0 ? item.ratio.toFixed(3) : "—"}
        />
        <DetailRow
          label="Download Speed"
          value={item.download_rate > 0 ? formatSpeed(item.download_rate) : "—"}
        />
        <DetailRow
          label="Upload Speed"
          value={item.upload_rate > 0 ? formatSpeed(item.upload_rate) : "—"}
        />
        <DetailRow
          label="ETA"
          value={
            item.status === "downloading" ? formatEta(item.eta_seconds) : "—"
          }
        />
        {detail && (
          <>
            <DetailRow
              label="Peers"
              value={`${detail.total_seeds} seeds / ${detail.total_peers} peers`}
            />
            <DetailRow
              label="Save Path"
              value={detail.SavePath || item.save_path || "—"}
            />
            <DetailRow
              label="Added"
              value={
                detail.added_at
                  ? new Date(detail.added_at).toLocaleString()
                  : "—"
              }
            />
            {detail.info_hash && (
              <DetailRow label="Info Hash" value={detail.info_hash} />
            )}
            {detail.comment && (
              <DetailRow label="Comment" value={detail.comment} />
            )}
            {detail.created_by && (
              <DetailRow label="Created By" value={detail.created_by} />
            )}
          </>
        )}
        {!detail && (
          <DetailRow label="Save Path" value={item.save_path || "—"} />
        )}
      </div>
    </div>
  );
}

function PeersTab({ detail }: { detail: TorrentDetail }) {
  if (!detail.peers || detail.peers.length === 0) {
    return (
      <p className="py-4 text-center text-xs text-zinc-500">
        No connected peers
      </p>
    );
  }
  return (
    <div className="mt-2 overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="border-zinc-800">
            <TableHead className="text-[10px] text-zinc-500">IP</TableHead>
            <TableHead className="text-[10px] text-zinc-500">Client</TableHead>
            <TableHead className="text-right text-[10px] text-zinc-500">
              Progress
            </TableHead>
            <TableHead className="text-right text-[10px] text-zinc-500">
              DL
            </TableHead>
            <TableHead className="text-right text-[10px] text-zinc-500">
              UL
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {detail.peers.map((peer, i) => (
            <TableRow
              key={`${peer.ip}-${peer.port}-${i}`}
              className="border-zinc-800/50"
            >
              <TableCell className="py-1.5 font-mono text-[11px] text-zinc-300">
                {peer.ip}
              </TableCell>
              <TableCell className="max-w-[120px] truncate py-1.5 text-[11px] text-zinc-400">
                {peer.client || "—"}
              </TableCell>
              <TableCell className="py-1.5 text-right text-[11px] tabular-nums text-zinc-400">
                {(peer.progress * 100).toFixed(0)}%
              </TableCell>
              <TableCell className="py-1.5 text-right text-[11px] tabular-nums text-zinc-400">
                {peer.down_rate > 0 ? formatBytes(peer.down_rate) : "—"}
              </TableCell>
              <TableCell className="py-1.5 text-right text-[11px] tabular-nums text-zinc-400">
                {peer.up_rate > 0 ? formatBytes(peer.up_rate) : "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function FilesTab({ detail }: { detail: TorrentDetail }) {
  if (!detail.files || detail.files.length === 0) {
    return (
      <p className="py-4 text-center text-xs text-zinc-500">
        No file info available
      </p>
    );
  }
  return (
    <div className="mt-2 space-y-1.5">
      {detail.files.map((file, i) => {
        const pct = Math.min(100, (file.progress ?? 0) * 100);
        const name = file.path.split("/").pop() || file.path;
        return (
          <div key={i} className="space-y-1 rounded bg-zinc-800/40 p-2">
            <div className="flex items-center justify-between">
              <span
                className="mr-2 flex-1 truncate text-[11px] text-zinc-300"
                title={file.path}
              >
                {name}
              </span>
              <span className="shrink-0 text-[10px] tabular-nums text-zinc-500">
                {formatBytes(file.size)}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Progress value={pct} className="h-1 flex-1 bg-zinc-800" />
              <span className="w-9 text-right text-[10px] tabular-nums text-zinc-500">
                {pct.toFixed(0)}%
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function TrackersTab({ detail }: { detail: TorrentDetail }) {
  if (!detail.trackers || detail.trackers.length === 0) {
    return (
      <p className="py-4 text-center text-xs text-zinc-500">No trackers</p>
    );
  }
  return (
    <div className="mt-2 overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="border-zinc-800">
            <TableHead className="text-[10px] text-zinc-500">URL</TableHead>
            <TableHead className="text-right text-[10px] text-zinc-500">
              Tier
            </TableHead>
            <TableHead className="text-right text-[10px] text-zinc-500">
              Status
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {detail.trackers.map((tracker, i) => (
            <TableRow
              key={`${tracker.url}-${i}`}
              className="border-zinc-800/50"
            >
              <TableCell className="max-w-[280px] truncate py-1.5 font-mono text-[11px] text-zinc-300">
                {tracker.url}
              </TableCell>
              <TableCell className="py-1.5 text-right text-[11px] tabular-nums text-zinc-400">
                {tracker.tier}
              </TableCell>
              <TableCell className="py-1.5 text-right text-[11px]">
                <Badge
                  variant="outline"
                  className={`text-[9px] ${
                    tracker.status === "working"
                      ? "border-green-800 text-green-400"
                      : tracker.status === "error"
                        ? "border-red-800 text-red-400"
                        : "border-zinc-700 text-zinc-500"
                  }`}
                >
                  {tracker.status}
                </Badge>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

// ─── Active Downloads Tab ───────────────────────────────────────────────

function ActiveDownloads() {
  const [items, setItems] = React.useState<QueueItem[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [refreshing, setRefreshing] = React.useState(false);
  const [selectedItem, setSelectedItem] = React.useState<QueueItem | null>(
    null,
  );

  const fetchActivity = React.useCallback(async (manual = false) => {
    if (manual) setRefreshing(true);
    try {
      const res = await apiFetch("/api/v1/activity");
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
    const interval = setInterval(() => fetchActivity(), 2000);
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
    (a, b) => (statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9),
  );

  if (loading) {
    return (
      <Card className="border-zinc-800 bg-zinc-900/50">
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
          className="h-8 text-zinc-400 hover:text-zinc-200"
        >
          {refreshing ? (
            <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
          ) : (
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
          )}
          Refresh
        </Button>
      </div>

      {items.length === 0 ? (
        <Card className="border-dashed border-zinc-800 bg-zinc-900/50">
          <CardContent>
            <EmptyState
              icon={<Download className="h-10 w-10" />}
              title="No active downloads"
              description="Downloads will appear here when you search and grab releases"
            />
          </CardContent>
        </Card>
      ) : (
        <Card className="overflow-hidden border-zinc-800 bg-zinc-900/50">
          <div className="divide-y divide-zinc-800/50">
            {sorted.map((item) => (
              <QueueItemRow
                key={item.id}
                item={item}
                onRefresh={() => fetchActivity()}
                onSelect={setSelectedItem}
              />
            ))}
          </div>
        </Card>
      )}

      <Sheet
        open={!!selectedItem}
        onOpenChange={(open) => !open && setSelectedItem(null)}
      >
        <SheetContent
          side="right"
          className="w-[500px] overflow-y-auto sm:max-w-[500px]"
        >
          <SheetHeader>
            <SheetTitle className="truncate pr-8 text-sm font-medium">
              {selectedItem?.title}
            </SheetTitle>
          </SheetHeader>
          {selectedItem && <TorrentDetailPanel item={selectedItem} />}
        </SheetContent>
      </Sheet>
    </div>
  );
}

// ─── Built-in Torrent Engine Panel ──────────────────────────────────────

const MB = 1024 * 1024;

function bytesToMbInput(bytes: number): string {
  if (!bytes || bytes <= 0) return "0";
  // Show up to 2 decimals, trimming trailing zeros.
  return String(Math.round((bytes / MB) * 100) / 100);
}

function StatPill({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col">
      <span className="text-[10px] uppercase tracking-wide text-zinc-500">
        {label}
      </span>
      <span className="text-sm font-medium tabular-nums text-zinc-100">
        {value}
      </span>
    </div>
  );
}

function TorrentEnginePanel() {
  const { data: clients } = useDownloads();
  const torrentClient = React.useMemo(
    () => clients?.find((c) => c.kind === "builtin/torrent" && c.enabled),
    [clients],
  );
  const clientId = torrentClient?.id;

  const { data: summary } = useTorrentStatus(clientId, {
    // Keep showing the last value while the engine briefly errors.
    retry: false,
  });
  const setLimits = useSetTorrentSpeedLimits();
  const pauseAll = useTorrentPauseAll();
  const resumeAll = useTorrentResumeAll();

  const [downInput, setDownInput] = React.useState("0");
  const [upInput, setUpInput] = React.useState("0");
  const [dirty, setDirty] = React.useState(false);

  // Sync editable inputs from the server value until the user edits them.
  React.useEffect(() => {
    if (!summary || dirty) return;
    setDownInput(bytesToMbInput(summary.download_limit));
    setUpInput(bytesToMbInput(summary.upload_limit));
  }, [summary, dirty]);

  if (!clientId) return null;

  const applyLimits = () => {
    const down =
      Math.max(0, Math.round(parseFloat(downInput || "0") * MB)) || 0;
    const up = Math.max(0, Math.round(parseFloat(upInput || "0") * MB)) || 0;
    setLimits.mutate(
      { clientId, download_limit: down, upload_limit: up },
      {
        onSuccess: (data) => {
          // Reflect the server's authoritative values, then unfreeze polling.
          setDownInput(bytesToMbInput(data.download_limit));
          setUpInput(bytesToMbInput(data.upload_limit));
          setDirty(false);
          toast.success("Speed limits updated");
        },
        onError: (e) =>
          toast.error(
            e instanceof Error ? e.message : "Failed to update limits",
          ),
      },
    );
  };

  return (
    <Card className="border-zinc-800 bg-zinc-900/50">
      <CardContent className="space-y-4 pt-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Gauge className="h-4 w-4 text-zinc-400" />
            <span className="text-sm font-medium text-zinc-100">
              Built-in Torrent Engine
            </span>
            <Badge
              variant="outline"
              className="border-zinc-700 text-[10px] text-zinc-400"
            >
              {torrentClient?.name}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="h-8"
              disabled={pauseAll.isPending}
              onClick={() =>
                pauseAll.mutate(clientId, {
                  onSuccess: () => toast.success("All torrents paused"),
                  onError: (e) =>
                    toast.error(
                      e instanceof Error ? e.message : "Pause failed",
                    ),
                })
              }
            >
              <Pause className="mr-1.5 h-3.5 w-3.5" />
              Pause all
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="h-8"
              disabled={resumeAll.isPending}
              onClick={() =>
                resumeAll.mutate(clientId, {
                  onSuccess: () => toast.success("All torrents resumed"),
                  onError: (e) =>
                    toast.error(
                      e instanceof Error ? e.message : "Resume failed",
                    ),
                })
              }
            >
              <Play className="mr-1.5 h-3.5 w-3.5" />
              Resume all
            </Button>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-4 lg:grid-cols-6">
          <StatPill label="Torrents" value={summary?.total_torrents ?? "—"} />
          <StatPill label="Downloading" value={summary?.downloading ?? "—"} />
          <StatPill label="Seeding" value={summary?.seeding ?? "—"} />
          <StatPill label="Paused" value={summary?.paused ?? "—"} />
          <StatPill
            label="Down rate"
            value={
              <span className="flex items-center gap-1 text-blue-400">
                <ArrowDown className="h-3 w-3" />
                {formatSpeed(summary?.download_rate ?? 0)}
              </span>
            }
          />
          <StatPill
            label="Up rate"
            value={
              <span className="flex items-center gap-1 text-green-400">
                <ArrowUp className="h-3 w-3" />
                {formatSpeed(summary?.upload_rate ?? 0)}
              </span>
            }
          />
        </div>

        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-zinc-500">
          <span>
            Port{" "}
            <span className="tabular-nums text-zinc-300">
              {summary?.listen_port ?? "—"}
            </span>
          </span>
          <span className={summary?.dht ? "text-zinc-300" : "text-zinc-600"}>
            DHT
          </span>
          <span className={summary?.pex ? "text-zinc-300" : "text-zinc-600"}>
            PEX
          </span>
          <span className={summary?.upnp ? "text-zinc-300" : "text-zinc-600"}>
            UPnP
          </span>
          {summary?.save_path && (
            <span className="truncate">
              Save path{" "}
              <span className="font-mono text-zinc-300">
                {summary.save_path}
              </span>
            </span>
          )}
        </div>

        <div className="flex flex-wrap items-end gap-4 border-t border-zinc-800 pt-4">
          <div className="space-y-1">
            <Label
              htmlFor="torrent-down-limit"
              className="text-[11px] text-zinc-400"
            >
              Download limit (MB/s, 0 = unlimited)
            </Label>
            <Input
              id="torrent-down-limit"
              type="number"
              min={0}
              step="0.1"
              value={downInput}
              onChange={(e) => {
                setDownInput(e.target.value);
                setDirty(true);
              }}
              className="h-8 w-40"
            />
          </div>
          <div className="space-y-1">
            <Label
              htmlFor="torrent-up-limit"
              className="text-[11px] text-zinc-400"
            >
              Upload limit (MB/s, 0 = unlimited)
            </Label>
            <Input
              id="torrent-up-limit"
              type="number"
              min={0}
              step="0.1"
              value={upInput}
              onChange={(e) => {
                setUpInput(e.target.value);
                setDirty(true);
              }}
              className="h-8 w-40"
            />
          </div>
          <Button
            size="sm"
            className="h-8"
            disabled={!dirty || setLimits.isPending}
            onClick={applyLimits}
          >
            {setLimits.isPending ? (
              <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
            ) : null}
            Apply limits
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// ─── Cleanup Tab ────────────────────────────────────────────────────────

function relativeAge(iso: string): string {
  const then = new Date(iso).getTime();
  if (!then) return "—";
  const days = Math.floor((Date.now() - then) / 86_400_000);
  if (days <= 0) return "today";
  if (days === 1) return "1 day ago";
  return `${days} days ago`;
}

function OrphanRow({
  orphan,
  onApprove,
  onIgnore,
  busy,
}: {
  orphan: Orphan;
  onApprove: (id: string) => void;
  onIgnore: (id: string) => void;
  busy: boolean;
}) {
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3">
      <div className="min-w-0">
        <div className="truncate font-mono text-xs text-zinc-200">
          {orphan.path}
        </div>
        <div className="mt-0.5 flex items-center gap-3 text-[11px] text-zinc-500">
          <span className="tabular-nums">{formatBytes(orphan.size_bytes)}</span>
          <span>first seen {relativeAge(orphan.first_seen_at)}</span>
          {orphan.client_name && <span>{orphan.client_name}</span>}
          {orphan.status === "delete_failed" && (
            <span className="text-red-400">
              delete failed{orphan.error ? `: ${orphan.error}` : ""}
            </span>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="h-8"
          disabled={busy}
          onClick={() => onIgnore(orphan.id)}
        >
          <EyeOff className="mr-1.5 h-3.5 w-3.5" />
          Keep
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="h-8 text-red-400 hover:text-red-300"
          disabled={busy}
          onClick={() => onApprove(orphan.id)}
        >
          <Trash2 className="mr-1.5 h-3.5 w-3.5" />
          Delete
        </Button>
      </div>
    </div>
  );
}

function CleanupTab() {
  const { data: orphans, isLoading } = useOrphans("pending");
  const { data: settings } = useCleanupSettings();
  const scan = useScanCleanup();
  const saveSettings = useSaveCleanupSettings();
  const approve = useApproveOrphan();
  const ignore = useIgnoreOrphan();

  const [retention, setRetention] = React.useState("7");
  const [autoDelete, setAutoDelete] = React.useState(true);
  const [dirty, setDirty] = React.useState(false);

  React.useEffect(() => {
    if (!settings || dirty) return;
    setRetention(String(settings.retention_days));
    setAutoDelete(settings.auto_delete_enabled);
  }, [settings, dirty]);

  const busy = approve.isPending || ignore.isPending;

  const onApprove = (id: string) =>
    approve.mutate(id, {
      onSuccess: () => toast.success("Orphan deleted"),
      onError: (e) =>
        toast.error(e instanceof Error ? e.message : "Delete failed"),
    });
  const onIgnore = (id: string) =>
    ignore.mutate(id, {
      onSuccess: () => toast.success("Orphan kept"),
      onError: (e) => toast.error(e instanceof Error ? e.message : "Failed"),
    });

  const saveCfg = () => {
    const days = Math.max(1, parseInt(retention || "7", 10) || 7);
    saveSettings.mutate(
      { auto_delete_enabled: autoDelete, retention_days: days },
      {
        onSuccess: () => {
          setDirty(false);
          toast.success("Cleanup settings saved");
        },
        onError: (e) =>
          toast.error(e instanceof Error ? e.message : "Save failed"),
      },
    );
  };

  return (
    <div className="space-y-4">
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 pt-5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Trash2 className="h-4 w-4 text-zinc-400" />
              <span className="text-sm font-medium text-zinc-100">
                Cleanup settings
              </span>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 text-zinc-400 hover:text-zinc-200"
              disabled={scan.isPending}
              onClick={() =>
                scan.mutate(undefined, {
                  onSuccess: (r) =>
                    toast.success(`Scan complete — ${r.found} orphan(s)`),
                  onError: (e) =>
                    toast.error(e instanceof Error ? e.message : "Scan failed"),
                })
              }
            >
              {scan.isPending ? (
                <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              )}
              Scan now
            </Button>
          </div>

          <p className="text-xs text-zinc-500">
            Files in your download folders that are no longer tied to any active
            download or import are listed below for review. Media libraries are
            never touched.
          </p>

          <div className="flex flex-wrap items-end gap-6 border-t border-zinc-800 pt-4">
            <div className="flex items-center gap-2">
              <Switch
                id="cleanup-auto-delete"
                checked={autoDelete}
                onCheckedChange={(v) => {
                  setAutoDelete(v);
                  setDirty(true);
                }}
              />
              <Label
                htmlFor="cleanup-auto-delete"
                className="text-xs text-zinc-300"
              >
                Auto-delete orphans
              </Label>
            </div>
            <div className="space-y-1">
              <Label
                htmlFor="cleanup-retention"
                className="text-[11px] text-zinc-400"
              >
                Retention (days)
              </Label>
              <Input
                id="cleanup-retention"
                type="number"
                min={1}
                value={retention}
                onChange={(e) => {
                  setRetention(e.target.value);
                  setDirty(true);
                }}
                className="h-8 w-28"
              />
            </div>
            <Button
              size="sm"
              className="h-8"
              disabled={!dirty || saveSettings.isPending}
              onClick={saveCfg}
            >
              {saveSettings.isPending ? (
                <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              ) : null}
              Save
            </Button>
          </div>
        </CardContent>
      </Card>

      {isLoading ? (
        <Card className="border-zinc-800 bg-zinc-900/50">
          <CardContent>
            <LoadingState label="Scanning download folders…" />
          </CardContent>
        </Card>
      ) : !orphans || orphans.length === 0 ? (
        <Card className="border-dashed border-zinc-800 bg-zinc-900/50">
          <CardContent>
            <EmptyState
              icon={<Trash2 className="h-10 w-10" />}
              title="No orphans found"
              description="Everything in your download folders is accounted for. Run a scan to check again."
            />
          </CardContent>
        </Card>
      ) : (
        <Card className="overflow-hidden border-zinc-800 bg-zinc-900/50">
          <div className="divide-y divide-zinc-800/50">
            {orphans.map((o) => (
              <OrphanRow
                key={o.id}
                orphan={o}
                onApprove={onApprove}
                onIgnore={onIgnore}
                busy={busy}
              />
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
          <TabsTrigger value="cleanup" className="flex items-center gap-1.5">
            <Trash2 className="h-3.5 w-3.5" />
            Cleanup
          </TabsTrigger>
        </TabsList>
        <TabsContent value="active" className="space-y-4">
          <TorrentEnginePanel />
          <ActiveDownloads />
        </TabsContent>
        <TabsContent value="imports">
          <ImportManager />
        </TabsContent>
        <TabsContent value="cleanup">
          <CleanupTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
