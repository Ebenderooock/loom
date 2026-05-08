import * as React from "react";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  FolderSearch,
  Import,
  Loader2,
  RefreshCw,
  FileVideo,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ArrowUpDown,
} from "lucide-react";
import {
  useScanFolder,
  useManualImport,
  useImportDecisions,
  useReimportFile,
} from "@/lib/imports-api";
import type { ScanResult } from "@/lib/imports-api";

// ─── Helpers ────────────────────────────────────────────────────────────

import { formatBytes } from "@/lib/utils";

function confidenceBadge(c: number) {
  if (c >= 0.8)
    return <Badge className="bg-green-600 text-white">High</Badge>;
  if (c >= 0.5)
    return (
      <Badge className="bg-yellow-500 text-black">Medium</Badge>
    );
  return <Badge variant="secondary">Low</Badge>;
}

function actionBadge(action: string) {
  switch (action) {
    case "import":
      return (
        <Badge className="bg-green-600 text-white">
          <CheckCircle2 className="mr-1 h-3 w-3" />
          Import
        </Badge>
      );
    case "reimport":
      return (
        <Badge className="bg-blue-600 text-white">
          <RefreshCw className="mr-1 h-3 w-3" />
          Re-import
        </Badge>
      );
    case "skip":
      return (
        <Badge variant="secondary">
          <XCircle className="mr-1 h-3 w-3" />
          Skipped
        </Badge>
      );
    case "replace":
      return (
        <Badge className="bg-orange-500 text-white">
          <ArrowUpDown className="mr-1 h-3 w-3" />
          Replace
        </Badge>
      );
    case "keep_both":
      return (
        <Badge className="bg-purple-600 text-white">Keep Both</Badge>
      );
    case "no_match":
      return (
        <Badge variant="destructive">
          <AlertTriangle className="mr-1 h-3 w-3" />
          No Match
        </Badge>
      );
    default:
      return <Badge variant="outline">{action}</Badge>;
  }
}

// ─── Scan Tab ───────────────────────────────────────────────────────────

function ScanTab() {
  const [folderPath, setFolderPath] = React.useState("");
  const scanMut = useScanFolder();
  const manualImportMut = useManualImport();
  const reimportMut = useReimportFile();
  const [results, setResults] = React.useState<ScanResult[]>([]);

  const handleScan = () => {
    if (!folderPath.trim()) return;
    scanMut.mutate(folderPath.trim(), {
      onSuccess: (data) => setResults(data),
    });
  };

  const handleImportAll = () => {
    if (!folderPath.trim()) return;
    manualImportMut.mutate(folderPath.trim());
  };

  const handleReimport = (result: ScanResult) => {
    if (!result.matched_media_id || !result.media_type) return;
    reimportMut.mutate({
      media_type: result.media_type,
      media_id: result.matched_media_id,
      source_path: result.file_path,
      conflict_policy: "replace_if_better",
    });
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FolderSearch className="h-5 w-5" />
            Scan Folder
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2">
            <Input
              placeholder="/downloads/complete"
              value={folderPath}
              onChange={(e) => setFolderPath(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleScan();
              }}
              className="flex-1"
            />
            <Button
              onClick={handleScan}
              disabled={scanMut.isPending || !folderPath.trim()}
            >
              {scanMut.isPending ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <FolderSearch className="mr-2 h-4 w-4" />
              )}
              Scan
            </Button>
          </div>
          {scanMut.isError && (
            <p className="mt-2 text-sm text-destructive">
              {scanMut.error.message}
            </p>
          )}
        </CardContent>
      </Card>

      {results.length > 0 && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle>
              Scan Results ({results.length} file
              {results.length !== 1 ? "s" : ""})
            </CardTitle>
            <Button
              size="sm"
              onClick={handleImportAll}
              disabled={
                manualImportMut.isPending ||
                results.every((r) => r.suggested_action === "no_match")
              }
            >
              {manualImportMut.isPending ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Import className="mr-2 h-4 w-4" />
              )}
              Import All Matched
            </Button>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>File</TableHead>
                  <TableHead>Detected</TableHead>
                  <TableHead>Match</TableHead>
                  <TableHead>Confidence</TableHead>
                  <TableHead>Quality</TableHead>
                  <TableHead>Size</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead />
                </TableRow>
              </TableHeader>
              <TableBody>
                {results.map((r) => (
                  <TableRow key={r.file_path}>
                    <TableCell className="max-w-[200px] truncate font-mono text-xs">
                      <div className="flex items-center gap-1">
                        <FileVideo className="h-4 w-4 shrink-0 text-muted-foreground" />
                        <span className="truncate" title={r.file_path}>
                          {r.file_path.split("/").pop()}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="text-sm">{r.detected_title}</span>
                      {r.detected_year ? (
                        <span className="ml-1 text-xs text-muted-foreground">
                          ({r.detected_year})
                        </span>
                      ) : null}
                      {r.detected_season ? (
                        <span className="ml-1 text-xs text-muted-foreground">
                          S{String(r.detected_season).padStart(2, "0")}E
                          {String(r.detected_episode).padStart(2, "0")}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      {r.matched_media ? (
                        <span className="text-sm font-medium">
                          {r.matched_media}
                        </span>
                      ) : (
                        <span className="text-sm text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>{confidenceBadge(r.confidence)}</TableCell>
                    <TableCell>
                      <span className="text-xs text-muted-foreground">
                        {r.quality || "—"}
                      </span>
                    </TableCell>
                    <TableCell className="text-xs">
                      {formatBytes(r.file_size)}
                    </TableCell>
                    <TableCell>{actionBadge(r.suggested_action)}</TableCell>
                    <TableCell>
                      {(r.suggested_action === "reimport" ||
                        r.suggested_action === "import") &&
                        r.matched_media_id && (
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => handleReimport(r)}
                            disabled={reimportMut.isPending}
                          >
                            {reimportMut.isPending ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Import className="h-4 w-4" />
                            )}
                          </Button>
                        )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {manualImportMut.isSuccess && (
              <p className="mt-2 text-sm text-green-600">
                Import triggered successfully.
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}

// ─── Decision Log Tab ───────────────────────────────────────────────────

function DecisionLogTab() {
  const [mediaFilter, setMediaFilter] = React.useState("");
  const [page, setPage] = React.useState(0);
  const pageSize = 25;

  const { data: decisions, isLoading } = useImportDecisions(
    pageSize,
    page * pageSize,
    mediaFilter || undefined,
  );

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>Import Decision Log</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="mb-4 flex items-center gap-2">
            <Input
              placeholder="Filter by media ID..."
              value={mediaFilter}
              onChange={(e) => {
                setMediaFilter(e.target.value);
                setPage(0);
              }}
              className="max-w-xs"
            />
            <Select
              value={String(pageSize)}
              onValueChange={() => setPage(0)}
            >
              <SelectTrigger className="w-[120px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="25">25 / page</SelectItem>
                <SelectItem value="50">50 / page</SelectItem>
                <SelectItem value="100">100 / page</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Time</TableHead>
                    <TableHead>Source</TableHead>
                    <TableHead>Destination</TableHead>
                    <TableHead>Media</TableHead>
                    <TableHead>Action</TableHead>
                    <TableHead>Policy</TableHead>
                    <TableHead>Reason</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {(decisions ?? []).length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        className="text-center text-muted-foreground"
                      >
                        No decisions recorded yet.
                      </TableCell>
                    </TableRow>
                  ) : (
                    (decisions ?? []).map((d) => (
                      <TableRow key={d.id}>
                        <TableCell className="whitespace-nowrap text-xs">
                          {new Date(d.created_at).toLocaleString()}
                        </TableCell>
                        <TableCell className="max-w-[150px] truncate font-mono text-xs" title={d.source_path}>
                          {d.source_path.split("/").pop()}
                        </TableCell>
                        <TableCell className="max-w-[150px] truncate font-mono text-xs" title={d.dest_path}>
                          {d.dest_path.split("/").pop()}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1">
                            <Badge variant="outline" className="text-xs">
                              {d.media_type}
                            </Badge>
                            <span className="max-w-[100px] truncate text-xs" title={d.media_id}>
                              {d.media_id}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell>{actionBadge(d.action)}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className="text-xs">
                            {d.conflict_policy || "—"}
                          </Badge>
                        </TableCell>
                        <TableCell className="max-w-[200px] text-xs text-muted-foreground">
                          {d.reason}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>

              <div className="mt-4 flex items-center justify-between">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={page === 0}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  Previous
                </Button>
                <span className="text-sm text-muted-foreground">
                  Page {page + 1}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={(decisions ?? []).length < pageSize}
                  onClick={() => setPage((p) => p + 1)}
                >
                  Next
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Main Component ─────────────────────────────────────────────────────

export function ImportManager() {
  return (
    <Tabs defaultValue="scan" className="space-y-4">
      <TabsList>
        <TabsTrigger value="scan">
          <FolderSearch className="mr-2 h-4 w-4" />
          Scan &amp; Import
        </TabsTrigger>
        <TabsTrigger value="decisions">
          <ArrowUpDown className="mr-2 h-4 w-4" />
          Decision Log
        </TabsTrigger>
      </TabsList>

      <TabsContent value="scan">
        <ScanTab />
      </TabsContent>

      <TabsContent value="decisions">
        <DecisionLogTab />
      </TabsContent>
    </Tabs>
  );
}
