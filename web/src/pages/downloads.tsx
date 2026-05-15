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
} from "lucide-react";
import { apiFetch } from "@/lib/fetch";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Progress } from "@/components/ui/progress";
import { ImportManager } from "@/components/imports/import-manager";
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

function QueueItemRow({ item, onRefresh, onSelect }: { item: QueueItem; onRefresh: () => void; onSelect: (item: QueueItem) => void }) {
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
      className="group flex items-center gap-4 px-4 py-3 border-b border-zinc-800/50 last:border-0 hover:bg-zinc-800/30 transition-colors cursor-pointer"
      onClick={() => onSelect(item)}
    >
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
        {item.status !== "seeding" && !isCompleted && (
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

      {/* Actions */}
      <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
        {/* Pause / Resume */}
        {canPauseResume && (
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-zinc-200"
            disabled={busy}
            title={isPaused ? "Resume" : "Pause"}
            onClick={() =>
              runAction(
                isPaused ? "Resumed" : "Paused",
                () => activityAction(isPaused ? "resume" : "pause", item.client_id, [item.id]),
              )
            }
          >
            {isPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
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
            onClick={() => runAction("Import started", () => forceImport(item.save_path))}
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
              activityAction("remove", item.client_id, [item.id], { delete_files: true }),
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
      <div className="flex items-center justify-center py-12 text-zinc-500 text-sm">
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
      <TabsList className="w-full grid grid-cols-4">
        <TabsTrigger value="overview" className="text-xs gap-1">
          <Info className="h-3 w-3" />
          Overview
        </TabsTrigger>
        <TabsTrigger value="peers" className="text-xs gap-1">
          <Users className="h-3 w-3" />
          Peers
        </TabsTrigger>
        <TabsTrigger value="files" className="text-xs gap-1">
          <FileText className="h-3 w-3" />
          Files
        </TabsTrigger>
        <TabsTrigger value="trackers" className="text-xs gap-1">
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

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex justify-between py-1.5 border-b border-zinc-800/50 last:border-0">
      <span className="text-xs text-zinc-500">{label}</span>
      <span className="text-xs text-zinc-200 text-right max-w-[60%] truncate">{value}</span>
    </div>
  );
}

function OverviewTab({ item, detail }: { item: QueueItem; detail: TorrentDetail | null }) {
  const sc = downloadStatusConfig(item.status);
  const pct = Math.min(100, (item.progress ?? 0) * 100);

  return (
    <div className="space-y-3 mt-3">
      <div className="flex items-center gap-2">
        <Badge variant={sc.variant} className={`text-[10px] ${sc.className ?? ""}`}>
          {sc.label}
        </Badge>
        <span className="text-xs text-zinc-500 tabular-nums">{pct.toFixed(1)}%</span>
      </div>
      <Progress value={pct} className="h-2 bg-zinc-800" />

      <div className="space-y-0">
        <DetailRow label="Size" value={item.size_bytes > 0 ? formatBytes(item.size_bytes) : "—"} />
        <DetailRow
          label="Downloaded"
          value={item.downloaded_bytes > 0 ? formatBytes(item.downloaded_bytes) : "—"}
        />
        {detail && (
          <DetailRow
            label="Uploaded"
            value={detail.Uploaded > 0 ? formatBytes(detail.Uploaded) : "—"}
          />
        )}
        <DetailRow label="Ratio" value={item.ratio > 0 ? item.ratio.toFixed(3) : "—"} />
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
          value={item.status === "downloading" ? formatEta(item.eta_seconds) : "—"}
        />
        {detail && (
          <>
            <DetailRow
              label="Peers"
              value={`${detail.total_seeds} seeds / ${detail.total_peers} peers`}
            />
            <DetailRow label="Save Path" value={detail.SavePath || item.save_path || "—"} />
            <DetailRow
              label="Added"
              value={detail.added_at ? new Date(detail.added_at).toLocaleString() : "—"}
            />
            {detail.info_hash && <DetailRow label="Info Hash" value={detail.info_hash} />}
            {detail.comment && <DetailRow label="Comment" value={detail.comment} />}
            {detail.created_by && <DetailRow label="Created By" value={detail.created_by} />}
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
    return <p className="text-xs text-zinc-500 py-4 text-center">No connected peers</p>;
  }
  return (
    <div className="mt-2 overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="border-zinc-800">
            <TableHead className="text-[10px] text-zinc-500">IP</TableHead>
            <TableHead className="text-[10px] text-zinc-500">Client</TableHead>
            <TableHead className="text-[10px] text-zinc-500 text-right">Progress</TableHead>
            <TableHead className="text-[10px] text-zinc-500 text-right">DL</TableHead>
            <TableHead className="text-[10px] text-zinc-500 text-right">UL</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {detail.peers.map((peer, i) => (
            <TableRow key={`${peer.ip}-${peer.port}-${i}`} className="border-zinc-800/50">
              <TableCell className="text-[11px] text-zinc-300 py-1.5 font-mono">
                {peer.ip}
              </TableCell>
              <TableCell className="text-[11px] text-zinc-400 py-1.5 truncate max-w-[120px]">
                {peer.client || "—"}
              </TableCell>
              <TableCell className="text-[11px] text-zinc-400 py-1.5 text-right tabular-nums">
                {(peer.progress * 100).toFixed(0)}%
              </TableCell>
              <TableCell className="text-[11px] text-zinc-400 py-1.5 text-right tabular-nums">
                {peer.down_rate > 0 ? formatBytes(peer.down_rate) : "—"}
              </TableCell>
              <TableCell className="text-[11px] text-zinc-400 py-1.5 text-right tabular-nums">
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
    return <p className="text-xs text-zinc-500 py-4 text-center">No file info available</p>;
  }
  return (
    <div className="mt-2 space-y-1.5">
      {detail.files.map((file, i) => {
        const pct = Math.min(100, (file.progress ?? 0) * 100);
        const name = file.path.split("/").pop() || file.path;
        return (
          <div key={i} className="rounded bg-zinc-800/40 p-2 space-y-1">
            <div className="flex justify-between items-center">
              <span
                className="text-[11px] text-zinc-300 truncate flex-1 mr-2"
                title={file.path}
              >
                {name}
              </span>
              <span className="text-[10px] text-zinc-500 tabular-nums shrink-0">
                {formatBytes(file.size)}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Progress value={pct} className="h-1 flex-1 bg-zinc-800" />
              <span className="text-[10px] text-zinc-500 tabular-nums w-9 text-right">
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
    return <p className="text-xs text-zinc-500 py-4 text-center">No trackers</p>;
  }
  return (
    <div className="mt-2 overflow-x-auto">
      <Table>
        <TableHeader>
          <TableRow className="border-zinc-800">
            <TableHead className="text-[10px] text-zinc-500">URL</TableHead>
            <TableHead className="text-[10px] text-zinc-500 text-right">Tier</TableHead>
            <TableHead className="text-[10px] text-zinc-500 text-right">Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {detail.trackers.map((tracker, i) => (
            <TableRow key={`${tracker.url}-${i}`} className="border-zinc-800/50">
              <TableCell className="text-[11px] text-zinc-300 py-1.5 truncate max-w-[280px] font-mono">
                {tracker.url}
              </TableCell>
              <TableCell className="text-[11px] text-zinc-400 py-1.5 text-right tabular-nums">
                {tracker.tier}
              </TableCell>
              <TableCell className="text-[11px] py-1.5 text-right">
                <Badge
                  variant="outline"
                  className={`text-[9px] ${
                    tracker.status === "working"
                      ? "text-green-400 border-green-800"
                      : tracker.status === "error"
                      ? "text-red-400 border-red-800"
                      : "text-zinc-500 border-zinc-700"
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
  const [selectedItem, setSelectedItem] = React.useState<QueueItem | null>(null);

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
          {refreshing ? <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5 mr-1.5" />}
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

      <Sheet open={!!selectedItem} onOpenChange={(open) => !open && setSelectedItem(null)}>
        <SheetContent side="right" className="w-[500px] sm:max-w-[500px] overflow-y-auto">
          <SheetHeader>
            <SheetTitle className="text-sm font-medium truncate pr-8">
              {selectedItem?.title}
            </SheetTitle>
          </SheetHeader>
          {selectedItem && <TorrentDetailPanel item={selectedItem} />}
        </SheetContent>
      </Sheet>
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
