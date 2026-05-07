import * as React from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { CheckCircle2, XCircle, AlertTriangle, Loader2, Clock, Ban, RefreshCw, Trash2, Download, ArrowDown, ArrowUp, Pause, Play, ChevronsUp, ChevronsDown, ChevronUp, ChevronDown, Zap, ShieldCheck, Radio, MoreHorizontal, Gauge } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { SearchDiagnostics } from "@/components/search/search-diagnostics";

// ─── Types ──────────────────────────────────────────────────────────────

interface DownloadItem {
  id: string;
  client_id?: string;
  title: string;
  category?: string;
  status: string;
  progress: number;
  size_bytes?: number;
  downloaded_bytes?: number;
  eta_seconds?: number;
  download_rate?: number;
  upload_rate?: number;
  ratio?: number;
  message?: string;
}

interface ReviewItem {
  id: string;
  media_type: string;
  media_id: string;
  download_path: string;
  reason: string;
  status: string;
  created_at: string;
}

// ─── Types: Download History ─────────────────────────────────────────────

interface HistoryEntry {
  id: string;
  download_id: string;
  client_id: string;
  title: string;
  category: string;
  status: string;
  grabbed_at?: string;
  completed_at: string;
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

function formatRate(bytesPerSec: number): string {
  return `${formatBytes(bytesPerSec)}/s`;
}

function formatEta(seconds: number): string {
  if (seconds <= 0) return "—";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function statusColor(status: string): string {
  switch (status) {
    case "downloading": return "text-blue-500";
    case "completed": return "text-green-500";
    case "paused": return "text-yellow-500";
    case "stalled": return "text-orange-500";
    case "error": case "failed": return "text-red-500";
    case "seeding": return "text-emerald-500";
    default: return "text-muted-foreground";
  }
}

// ─── Queue action helpers ────────────────────────────────────────────────

async function queueAction(
  clientId: string,
  action: string,
  body: Record<string, unknown> = {},
): Promise<boolean> {
  try {
    const res = await fetch(
      `/api/v1/download-clients/${encodeURIComponent(clientId)}/${action}`,
      {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      },
    );
    return res.ok;
  } catch {
    return false;
  }
}

function DownloadQueue() {
  const [items, setItems] = React.useState<DownloadItem[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [removeTarget, setRemoveTarget] = React.useState<DownloadItem | null>(null);
  const [deleteFiles, setDeleteFiles] = React.useState(false);
  const [speedTarget, setSpeedTarget] = React.useState<DownloadItem | null>(null);
  const [speedLimit, setSpeedLimit] = React.useState("");

  const fetchQueue = React.useCallback(async () => {
    try {
      const res = await fetch("/api/v1/activity", { credentials: "include" });
      if (res.ok) {
        const body = await res.json();
        setItems(body.items ?? []);
      }
    } catch {
      // silently fail
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    fetchQueue();
    const interval = setInterval(fetchQueue, 5000);
    return () => clearInterval(interval);
  }, [fetchQueue]);

  const doAction = React.useCallback(
    async (item: DownloadItem, action: string, body: Record<string, unknown> = {}) => {
      if (!item.client_id) return;
      await queueAction(item.client_id, action, body);
      fetchQueue();
    },
    [fetchQueue],
  );

  const confirmRemove = React.useCallback(async () => {
    if (!removeTarget?.client_id) return;
    await queueAction(removeTarget.client_id, "remove", {
      ids: [removeTarget.id],
      delete_files: deleteFiles,
    });
    setRemoveTarget(null);
    setDeleteFiles(false);
    fetchQueue();
  }, [removeTarget, deleteFiles, fetchQueue]);

  const confirmSpeedLimit = React.useCallback(async () => {
    if (!speedTarget?.client_id) return;
    const limitBytes = Math.max(0, Math.floor(parseFloat(speedLimit || "0") * 1024));
    await queueAction(speedTarget.client_id, "set-speed-limit", {
      ids: [speedTarget.id],
      limit_bytes_per_sec: limitBytes,
    });
    setSpeedTarget(null);
    setSpeedLimit("");
    fetchQueue();
  }, [speedTarget, speedLimit, fetchQueue]);

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin mx-auto mb-2" />
          Loading queue…
        </CardContent>
      </Card>
    );
  }

  if (items.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Download className="h-5 w-5 mx-auto mb-2 opacity-50" />
          No active downloads.
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-3">
          <CardTitle className="text-base">Download Queue</CardTitle>
          <Button variant="ghost" size="icon" onClick={fetchQueue} className="h-8 w-8">
            <RefreshCw className="h-4 w-4" />
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-muted-foreground text-xs">
                <th className="py-2 px-4 font-medium">Title</th>
                <th className="py-2 px-4 font-medium w-24">Status</th>
                <th className="py-2 px-4 font-medium w-28">Progress</th>
                <th className="py-2 px-4 font-medium w-24">Size</th>
                <th className="py-2 px-4 font-medium w-24">Speed</th>
                <th className="py-2 px-4 font-medium w-20">ETA</th>
                <th className="py-2 px-4 font-medium w-10"></th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={`${item.client_id}-${item.id}`} className="border-b border-border/50 last:border-0">
                  <td className="py-3 px-4">
                    <div className="font-medium truncate max-w-md">{item.title}</div>
                    {item.category && (
                      <div className="text-xs text-muted-foreground">{item.category}</div>
                    )}
                  </td>
                  <td className="py-3 px-4">
                    <span className={`text-xs font-medium ${statusColor(item.status)}`}>
                      {item.status === "downloading" && <ArrowDown className="inline h-3 w-3 mr-0.5" />}
                      {item.status === "seeding" && <ArrowUp className="inline h-3 w-3 mr-0.5" />}
                      {item.status === "paused" && <Pause className="inline h-3 w-3 mr-0.5" />}
                      {item.status}
                    </span>
                  </td>
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-2">
                      <div className="flex-1 h-1.5 bg-muted rounded-full overflow-hidden">
                        <div
                          className="h-full bg-accent rounded-full transition-all"
                          style={{ width: `${Math.round(item.progress * 100)}%` }}
                        />
                      </div>
                      <span className="text-xs tabular-nums text-muted-foreground w-8">
                        {Math.round(item.progress * 100)}%
                      </span>
                    </div>
                  </td>
                  <td className="py-3 px-4 text-xs text-muted-foreground tabular-nums">
                    {item.size_bytes ? formatBytes(item.size_bytes) : "—"}
                  </td>
                  <td className="py-3 px-4 text-xs text-muted-foreground tabular-nums">
                    {item.download_rate ? formatRate(item.download_rate) : "—"}
                  </td>
                  <td className="py-3 px-4 text-xs text-muted-foreground tabular-nums">
                    {item.eta_seconds ? formatEta(item.eta_seconds) : "—"}
                  </td>
                  <td className="py-3 px-4">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-7 w-7">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {item.status === "paused" ? (
                          <DropdownMenuItem onClick={() => doAction(item, "resume", { ids: [item.id] })}>
                            <Play className="h-3.5 w-3.5 mr-2" /> Resume
                          </DropdownMenuItem>
                        ) : (
                          <DropdownMenuItem onClick={() => doAction(item, "pause", { ids: [item.id] })}>
                            <Pause className="h-3.5 w-3.5 mr-2" /> Pause
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem onClick={() => doAction(item, "force-start", { ids: [item.id] })}>
                          <Zap className="h-3.5 w-3.5 mr-2" /> Force Start
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuSub>
                          <DropdownMenuSubTrigger>
                            <ChevronsUp className="h-3.5 w-3.5 mr-2" /> Priority
                          </DropdownMenuSubTrigger>
                          <DropdownMenuSubContent>
                            <DropdownMenuItem onClick={() => doAction(item, "set-priority", { ids: [item.id], priority: "top" })}>
                              <ChevronsUp className="h-3.5 w-3.5 mr-2" /> Move to Top
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => doAction(item, "set-priority", { ids: [item.id], priority: "up" })}>
                              <ChevronUp className="h-3.5 w-3.5 mr-2" /> Move Up
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => doAction(item, "set-priority", { ids: [item.id], priority: "down" })}>
                              <ChevronDown className="h-3.5 w-3.5 mr-2" /> Move Down
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => doAction(item, "set-priority", { ids: [item.id], priority: "bottom" })}>
                              <ChevronsDown className="h-3.5 w-3.5 mr-2" /> Move to Bottom
                            </DropdownMenuItem>
                          </DropdownMenuSubContent>
                        </DropdownMenuSub>
                        <DropdownMenuItem onClick={() => setSpeedTarget(item)}>
                          <Gauge className="h-3.5 w-3.5 mr-2" /> Speed Limit…
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={() => doAction(item, "recheck", { ids: [item.id] })}>
                          <ShieldCheck className="h-3.5 w-3.5 mr-2" /> Recheck
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => doAction(item, "reannounce", { ids: [item.id] })}>
                          <Radio className="h-3.5 w-3.5 mr-2" /> Reannounce
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className="text-red-500 focus:text-red-500"
                          onClick={() => setRemoveTarget(item)}
                        >
                          <Trash2 className="h-3.5 w-3.5 mr-2" /> Remove…
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </CardContent>
      </Card>

      {/* Remove confirmation dialog */}
      <Dialog open={!!removeTarget} onOpenChange={(open) => { if (!open) { setRemoveTarget(null); setDeleteFiles(false); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove Download</DialogTitle>
            <DialogDescription>
              Remove <span className="font-medium">{removeTarget?.title}</span> from the queue?
            </DialogDescription>
          </DialogHeader>
          <label className="flex items-center gap-2 text-sm cursor-pointer">
            <input
              type="checkbox"
              checked={deleteFiles}
              onChange={(e) => setDeleteFiles(e.target.checked)}
              className="rounded border-border"
            />
            Also delete downloaded files
          </label>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setRemoveTarget(null); setDeleteFiles(false); }}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmRemove}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Speed limit dialog */}
      <Dialog open={!!speedTarget} onOpenChange={(open) => { if (!open) { setSpeedTarget(null); setSpeedLimit(""); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Set Speed Limit</DialogTitle>
            <DialogDescription>
              Limit download speed for <span className="font-medium">{speedTarget?.title}</span>. Enter 0 for unlimited.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <input
              type="number"
              min="0"
              step="any"
              value={speedLimit}
              onChange={(e) => setSpeedLimit(e.target.value)}
              placeholder="0"
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            />
            <span className="text-sm text-muted-foreground whitespace-nowrap">KB/s</span>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setSpeedTarget(null); setSpeedLimit(""); }}>
              Cancel
            </Button>
            <Button onClick={confirmSpeedLimit}>Apply</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function relativeTime(iso: string): string {
  const now = Date.now();
  const then = new Date(iso).getTime();
  const diffSec = Math.floor((now - then) / 1000);
  if (diffSec < 60) return "just now";
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

function DownloadHistory() {
  const [entries, setEntries] = React.useState<HistoryEntry[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    async function load() {
      try {
        const res = await fetch("/api/v1/downloads/history?limit=50", { credentials: "include" });
        if (res.ok) {
          const body = await res.json();
          setEntries(body ?? []);
        }
      } catch {
        // silently fail
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin mx-auto mb-2" />
          Loading history…
        </CardContent>
      </Card>
    );
  }

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Clock className="h-5 w-5 mx-auto mb-2 opacity-50" />
          No download history yet.
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Download History</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground text-xs">
              <th className="py-2 px-4 font-medium">Title</th>
              <th className="py-2 px-4 font-medium">Category</th>
              <th className="py-2 px-4 font-medium">Status</th>
              <th className="py-2 px-4 font-medium">Completed</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr key={entry.id} className="border-b border-border/50 last:border-0">
                <td className="py-3 px-4">
                  <div className="font-medium truncate max-w-xs">{entry.title}</div>
                </td>
                <td className="py-3 px-4 text-xs text-muted-foreground">
                  {entry.category || "—"}
                </td>
                <td className="py-3 px-4">
                  {entry.status === "completed" ? (
                    <Badge variant="default" className="bg-green-600 text-xs">completed</Badge>
                  ) : entry.status === "failed" ? (
                    <Badge variant="destructive" className="text-xs">failed</Badge>
                  ) : (
                    <Badge variant="secondary" className="text-xs">{entry.status}</Badge>
                  )}
                </td>
                <td className="py-3 px-4 text-xs text-muted-foreground">
                  {relativeTime(entry.completed_at)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}

// ─── Blocklist Viewer ────────────────────────────────────────────────────

interface BlocklistEntry {
  id: string;
  title: string;
  indexer_id: string;
  release_hash: string;
  reason: string;
  created_at: string;
}

function BlocklistViewer() {
  const [entries, setEntries] = React.useState<BlocklistEntry[]>([]);
  const [loading, setLoading] = React.useState(true);

  const fetchBlocklist = React.useCallback(async () => {
    try {
      const res = await fetch("/api/v1/blocklist", { credentials: "include" });
      if (res.ok) {
        const body = await res.json();
        setEntries(body.data ?? []);
      }
    } catch {
      // silently fail
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    fetchBlocklist();
  }, [fetchBlocklist]);

  const handleRemove = async (id: string) => {
    try {
      await fetch(`/api/v1/blocklist/${id}`, { method: "DELETE", credentials: "include" });
      setEntries((prev) => prev.filter((e) => e.id !== id));
    } catch {
      // silently fail
    }
  };

  const handleClearAll = async () => {
    try {
      await fetch("/api/v1/blocklist", { method: "DELETE", credentials: "include" });
      setEntries([]);
    } catch {
      // silently fail
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin mx-auto mb-2" />
          Loading blocklist…
        </CardContent>
      </Card>
    );
  }

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Ban className="h-5 w-5 mx-auto mb-2 opacity-50" />
          Blocklist is empty.
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base">Blocklisted Releases</CardTitle>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" onClick={fetchBlocklist} className="h-8 w-8">
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button variant="destructive" size="sm" onClick={handleClearAll}>
            <Trash2 className="h-3.5 w-3.5 mr-1" /> Clear All
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-muted-foreground text-xs">
              <th className="py-2 px-4 font-medium">Title</th>
              <th className="py-2 px-4 font-medium">Reason</th>
              <th className="py-2 px-4 font-medium">Date</th>
              <th className="py-2 px-4 font-medium w-20"></th>
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr key={entry.id} className="border-b border-border/50 last:border-0">
                <td className="py-3 px-4">
                  <div className="font-medium truncate max-w-xs">{entry.title}</div>
                </td>
                <td className="py-3 px-4 text-xs text-muted-foreground">
                  {entry.reason || "—"}
                </td>
                <td className="py-3 px-4 text-xs text-muted-foreground">
                  {new Date(entry.created_at).toLocaleString()}
                </td>
                <td className="py-3 px-4">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => handleRemove(entry.id)}
                  >
                    <Trash2 className="h-3.5 w-3.5 text-red-500" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}

// ─── Review Queue ───────────────────────────────────────────────────────

function ReviewQueue() {
  const [reviews, setReviews] = React.useState<ReviewItem[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [acting, setActing] = React.useState<string | null>(null);

  const fetchReviews = React.useCallback(async () => {
    try {
      const res = await fetch("/api/v1/reviews");
      if (res.ok) {
        const body = await res.json();
        setReviews(body.data ?? []);
      }
    } catch {
      // silently fail
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    fetchReviews();
  }, [fetchReviews]);

  const handleAction = async (id: string, action: "approve" | "reject") => {
    setActing(id);
    try {
      await fetch(`/api/v1/reviews/${id}/${action}`, { method: "POST" });
      setReviews((prev) => prev.filter((r) => r.id !== id));
    } catch {
      // silently fail
    } finally {
      setActing(null);
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin mx-auto mb-2" />
          Loading reviews…
        </CardContent>
      </Card>
    );
  }

  if (reviews.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <CheckCircle2 className="h-5 w-5 mx-auto mb-2 text-green-500" />
          No items pending review.
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      {reviews.map((r) => (
        <Card key={r.id}>
          <CardContent className="py-4 flex items-start justify-between gap-4">
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-yellow-500 shrink-0" />
                <span className="text-sm font-medium truncate">
                  {r.download_path || r.media_id}
                </span>
                <Badge variant="secondary" className="text-xs">
                  {r.media_type}
                </Badge>
              </div>
              <p className="text-xs text-muted-foreground">{r.reason}</p>
              <p className="text-xs text-muted-foreground">
                {new Date(r.created_at).toLocaleString()}
              </p>
            </div>
            <div className="flex gap-2 shrink-0">
              <Button
                size="sm"
                variant="outline"
                disabled={acting === r.id}
                onClick={() => handleAction(r.id, "approve")}
              >
                <CheckCircle2 className="h-4 w-4 mr-1 text-green-500" />
                Approve
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={acting === r.id}
                onClick={() => handleAction(r.id, "reject")}
              >
                <XCircle className="h-4 w-4 mr-1 text-red-500" />
                Reject
              </Button>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

export function ActivityPage() {
  useSetPageHeader("Activity");

  const [reviewCount, setReviewCount] = React.useState(0);

  React.useEffect(() => {
    fetch("/api/v1/reviews/count")
      .then((r) => r.json())
      .then((b) => setReviewCount(b.count ?? 0))
      .catch(() => {});
  }, []);

  return (
    <div className="space-y-6">
      <Tabs defaultValue="queue">
        <TabsList>
          <TabsTrigger value="queue">Queue</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="blocklist">Blocklist</TabsTrigger>
          <TabsTrigger value="reviews" className="flex items-center gap-1.5">
            Reviews
            {reviewCount > 0 && (
              <Badge
                variant="destructive"
                className="ml-1 h-5 min-w-[1.25rem] px-1 text-xs"
              >
                {reviewCount}
              </Badge>
            )}
          </TabsTrigger>
          <TabsTrigger value="search-log">Search Log</TabsTrigger>
        </TabsList>
        <TabsContent value="queue">
          <DownloadQueue />
        </TabsContent>
        <TabsContent value="history">
          <DownloadHistory />
        </TabsContent>
        <TabsContent value="blocklist">
          <BlocklistViewer />
        </TabsContent>
        <TabsContent value="reviews">
          <ReviewQueue />
        </TabsContent>
        <TabsContent value="search-log">
          <SearchDiagnostics />
        </TabsContent>
      </Tabs>
    </div>
  );
}
