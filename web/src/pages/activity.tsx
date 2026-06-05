import * as React from "react";
import { apiFetch } from "@/lib/fetch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useSetPageHeader } from "@/hooks/use-page-header";
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Clock,
  Ban,
  RefreshCw,
  Trash2,
  Download,
  ArrowDown,
  ArrowUp,
  Pause,
  Play,
  ChevronsUp,
  ChevronsDown,
  ChevronUp,
  ChevronDown,
  Zap,
  ShieldCheck,
  Radio,
  MoreHorizontal,
  Gauge,
} from "lucide-react";
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
import { formatBytes, formatEta, formatSpeed, relativeTime } from "@/lib/utils";
import { downloadStatusConfig } from "@/lib/status-utils";

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

// ─── Queue action helpers ────────────────────────────────────────────────

async function queueAction(
  clientId: string,
  action: string,
  body: Record<string, unknown> = {},
): Promise<boolean> {
  try {
    const res = await apiFetch(
      `/api/v1/download-clients/${encodeURIComponent(clientId)}/${action}`,
      {
        method: "POST",
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
  const [removeTarget, setRemoveTarget] = React.useState<DownloadItem | null>(
    null,
  );
  const [deleteFiles, setDeleteFiles] = React.useState(false);
  const [speedTarget, setSpeedTarget] = React.useState<DownloadItem | null>(
    null,
  );
  const [speedLimit, setSpeedLimit] = React.useState("");

  const fetchQueue = React.useCallback(async () => {
    try {
      const res = await apiFetch("/api/v1/activity");
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
    async (
      item: DownloadItem,
      action: string,
      body: Record<string, unknown> = {},
    ) => {
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
    const limitBytes = Math.max(
      0,
      Math.floor(parseFloat(speedLimit || "0") * 1024),
    );
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
          <LoadingState label="Loading queue…" />
        </CardContent>
      </Card>
    );
  }

  if (items.length === 0) {
    return (
      <Card>
        <CardContent className="py-8">
          <EmptyState
            icon={<Download className="h-8 w-8" />}
            title="No active downloads"
            description="The download queue is empty."
          />
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-3">
          <CardTitle className="text-base">Download Queue</CardTitle>
          <Button
            variant="ghost"
            size="icon"
            onClick={fetchQueue}
            className="h-8 w-8"
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead className="w-24">Status</TableHead>
                <TableHead className="w-28">Progress</TableHead>
                <TableHead className="w-24">Size</TableHead>
                <TableHead className="w-24">Speed</TableHead>
                <TableHead className="w-20">ETA</TableHead>
                <TableHead className="w-10"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => {
                const sc = downloadStatusConfig(item.status);
                return (
                  <TableRow key={`${item.client_id}-${item.id}`}>
                    <TableCell>
                      <div className="max-w-md truncate font-medium">
                        {item.title}
                      </div>
                      {item.category && (
                        <div className="text-xs text-muted-foreground">
                          {item.category}
                        </div>
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={sc.variant}
                        className={`text-xs ${sc.className ?? ""}`}
                      >
                        {item.status === "downloading" && (
                          <ArrowDown className="mr-0.5 inline h-3 w-3" />
                        )}
                        {item.status === "seeding" && (
                          <ArrowUp className="mr-0.5 inline h-3 w-3" />
                        )}
                        {item.status === "paused" && (
                          <Pause className="mr-0.5 inline h-3 w-3" />
                        )}
                        {sc.label}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full rounded-full bg-accent transition-all"
                            style={{
                              width: `${Math.round(item.progress * 100)}%`,
                            }}
                          />
                        </div>
                        <span className="w-8 text-xs tabular-nums text-muted-foreground">
                          {Math.round(item.progress * 100)}%
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="text-xs tabular-nums text-muted-foreground">
                      {item.size_bytes ? formatBytes(item.size_bytes) : "—"}
                    </TableCell>
                    <TableCell className="text-xs tabular-nums text-muted-foreground">
                      {item.download_rate
                        ? formatSpeed(item.download_rate)
                        : "—"}
                    </TableCell>
                    <TableCell className="text-xs tabular-nums text-muted-foreground">
                      {item.eta_seconds ? formatEta(item.eta_seconds) : "—"}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                          >
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {item.status === "paused" ? (
                            <DropdownMenuItem
                              onClick={() =>
                                doAction(item, "resume", { ids: [item.id] })
                              }
                            >
                              <Play className="mr-2 h-3.5 w-3.5" /> Resume
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem
                              onClick={() =>
                                doAction(item, "pause", { ids: [item.id] })
                              }
                            >
                              <Pause className="mr-2 h-3.5 w-3.5" /> Pause
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem
                            onClick={() =>
                              doAction(item, "force-start", { ids: [item.id] })
                            }
                          >
                            <Zap className="mr-2 h-3.5 w-3.5" /> Force Start
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuSub>
                            <DropdownMenuSubTrigger>
                              <ChevronsUp className="mr-2 h-3.5 w-3.5" />{" "}
                              Priority
                            </DropdownMenuSubTrigger>
                            <DropdownMenuSubContent>
                              <DropdownMenuItem
                                onClick={() =>
                                  doAction(item, "set-priority", {
                                    ids: [item.id],
                                    priority: "top",
                                  })
                                }
                              >
                                <ChevronsUp className="mr-2 h-3.5 w-3.5" /> Move
                                to Top
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() =>
                                  doAction(item, "set-priority", {
                                    ids: [item.id],
                                    priority: "up",
                                  })
                                }
                              >
                                <ChevronUp className="mr-2 h-3.5 w-3.5" /> Move
                                Up
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() =>
                                  doAction(item, "set-priority", {
                                    ids: [item.id],
                                    priority: "down",
                                  })
                                }
                              >
                                <ChevronDown className="mr-2 h-3.5 w-3.5" />{" "}
                                Move Down
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onClick={() =>
                                  doAction(item, "set-priority", {
                                    ids: [item.id],
                                    priority: "bottom",
                                  })
                                }
                              >
                                <ChevronsDown className="mr-2 h-3.5 w-3.5" />{" "}
                                Move to Bottom
                              </DropdownMenuItem>
                            </DropdownMenuSubContent>
                          </DropdownMenuSub>
                          <DropdownMenuItem
                            onClick={() => setSpeedTarget(item)}
                          >
                            <Gauge className="mr-2 h-3.5 w-3.5" /> Speed Limit…
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onClick={() =>
                              doAction(item, "recheck", { ids: [item.id] })
                            }
                          >
                            <ShieldCheck className="mr-2 h-3.5 w-3.5" /> Recheck
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() =>
                              doAction(item, "reannounce", { ids: [item.id] })
                            }
                          >
                            <Radio className="mr-2 h-3.5 w-3.5" /> Reannounce
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-red-500 focus:text-red-500"
                            onClick={() => setRemoveTarget(item)}
                          >
                            <Trash2 className="mr-2 h-3.5 w-3.5" /> Remove…
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Remove confirmation dialog */}
      <Dialog
        open={!!removeTarget}
        onOpenChange={(open) => {
          if (!open) {
            setRemoveTarget(null);
            setDeleteFiles(false);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Remove Download</DialogTitle>
            <DialogDescription>
              Remove <span className="font-medium">{removeTarget?.title}</span>{" "}
              from the queue?
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <Checkbox
              id="delete-files"
              checked={deleteFiles}
              onCheckedChange={(v) => setDeleteFiles(!!v)}
            />
            <Label htmlFor="delete-files" className="cursor-pointer">
              Also delete downloaded files
            </Label>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setRemoveTarget(null);
                setDeleteFiles(false);
              }}
            >
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmRemove}>
              Remove
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Speed limit dialog */}
      <Dialog
        open={!!speedTarget}
        onOpenChange={(open) => {
          if (!open) {
            setSpeedTarget(null);
            setSpeedLimit("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Set Speed Limit</DialogTitle>
            <DialogDescription>
              Limit download speed for{" "}
              <span className="font-medium">{speedTarget?.title}</span>. Enter 0
              for unlimited.
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <Input
              type="number"
              min={0}
              step="any"
              value={speedLimit}
              onChange={(e) => setSpeedLimit(e.target.value)}
              placeholder="0"
            />
            <span className="whitespace-nowrap text-sm text-muted-foreground">
              KB/s
            </span>
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setSpeedTarget(null);
                setSpeedLimit("");
              }}
            >
              Cancel
            </Button>
            <Button onClick={confirmSpeedLimit}>Apply</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function DownloadHistory() {
  const [entries, setEntries] = React.useState<HistoryEntry[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    async function load() {
      try {
        const res = await apiFetch("/api/v1/downloads/history?limit=50");
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
          <LoadingState label="Loading history…" />
        </CardContent>
      </Card>
    );
  }

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="py-8">
          <EmptyState
            icon={<Clock className="h-8 w-8" />}
            title="No download history"
            description="No download history yet."
          />
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
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Title</TableHead>
              <TableHead>Category</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Completed</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((entry) => {
              const sc = downloadStatusConfig(entry.status);
              return (
                <TableRow key={entry.id}>
                  <TableCell>
                    <div className="max-w-xs truncate font-medium">
                      {entry.title}
                    </div>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {entry.category || "—"}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={sc.variant}
                      className={`text-xs ${sc.className ?? ""}`}
                    >
                      {sc.label}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {relativeTime(entry.completed_at)}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
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
      const res = await apiFetch("/api/v1/blocklist");
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
      await apiFetch(`/api/v1/blocklist/${id}`, { method: "DELETE" });
      setEntries((prev) => prev.filter((e) => e.id !== id));
    } catch {
      // silently fail
    }
  };

  const handleClearAll = async () => {
    try {
      await apiFetch("/api/v1/blocklist", { method: "DELETE" });
      setEntries([]);
    } catch {
      // silently fail
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-muted-foreground">
          <LoadingState label="Loading blocklist…" />
        </CardContent>
      </Card>
    );
  }

  if (entries.length === 0) {
    return (
      <Card>
        <CardContent className="py-8">
          <EmptyState
            icon={<Ban className="h-8 w-8" />}
            title="Blocklist is empty"
            description="No releases have been blocklisted."
          />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base">Blocklisted Releases</CardTitle>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={fetchBlocklist}
            className="h-8 w-8"
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button variant="destructive" size="sm" onClick={handleClearAll}>
            <Trash2 className="mr-1 h-3.5 w-3.5" /> Clear All
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Title</TableHead>
              <TableHead>Reason</TableHead>
              <TableHead>Date</TableHead>
              <TableHead className="w-20"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((entry) => (
              <TableRow key={entry.id}>
                <TableCell>
                  <div className="max-w-xs truncate font-medium">
                    {entry.title}
                  </div>
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {entry.reason || "—"}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {new Date(entry.created_at).toLocaleString()}
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => handleRemove(entry.id)}
                  >
                    <Trash2 className="h-3.5 w-3.5 text-red-500" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
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
      const res = await apiFetch("/api/v1/reviews");
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
      await apiFetch(`/api/v1/reviews/${id}/${action}`, { method: "POST" });
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
          <LoadingState label="Loading reviews…" />
        </CardContent>
      </Card>
    );
  }

  if (reviews.length === 0) {
    return (
      <Card>
        <CardContent className="py-8">
          <EmptyState
            icon={<CheckCircle2 className="h-8 w-8 text-green-500" />}
            title="No items pending review"
            description="All reviews have been processed."
          />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-3">
      {reviews.map((r) => (
        <Card key={r.id}>
          <CardContent className="flex items-start justify-between gap-4 py-4">
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 shrink-0 text-yellow-500" />
                <span className="truncate text-sm font-medium">
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
            <div className="flex shrink-0 gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={acting === r.id}
                onClick={() => handleAction(r.id, "approve")}
              >
                <CheckCircle2 className="mr-1 h-4 w-4 text-green-500" />
                Approve
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={acting === r.id}
                onClick={() => handleAction(r.id, "reject")}
              >
                <XCircle className="mr-1 h-4 w-4 text-red-500" />
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
    apiFetch("/api/v1/reviews/count")
      .then((r) => r.json())
      .then((b) => setReviewCount(b.count ?? 0))
      .catch((err) => console.error("fetch failed:", err));
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
      </Tabs>
    </div>
  );
}
