import * as React from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Progress } from "@/components/ui/progress";
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
  FileAudio,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ArrowUpDown,
  Link2,
} from "lucide-react";
import {
  cancelImportScan,
  useScanFolder,
  useImportDecisions,
  useImportJobs,
  useImportJob,
  useQueueImportFiles,
  useRetryImportJob,
} from "@/lib/imports-api";
import type {
  ImportJob,
  ImportScan,
  QueueImportResult,
  ScanResult,
} from "@/lib/imports-api";
import { ManualMatchDialog } from "./manual-match-dialog";

// ─── Helpers ────────────────────────────────────────────────────────────

import { formatBytes } from "@/lib/utils";

function confidenceBadge(c: number) {
  if (c >= 0.8) return <Badge className="bg-green-600 text-white">High</Badge>;
  if (c >= 0.5)
    return <Badge className="bg-yellow-500 text-black">Medium</Badge>;
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
    case "already_imported":
    case "imported":
      return (
        <Badge className="bg-green-600 text-white">
          <CheckCircle2 className="mr-1 h-3 w-3" />
          Imported
        </Badge>
      );
    case "queued":
      return <Badge variant="secondary">Queued</Badge>;
    case "importing":
      return (
        <Badge variant="outline">
          <Loader2 className="mr-1 h-3 w-3 animate-spin" />
          Importing
        </Badge>
      );
    case "retry_pending":
      return <Badge variant="outline">Retry scheduled</Badge>;
    case "failed":
      return <Badge variant="destructive">Failed</Badge>;
    case "pending_review":
    case "manual_review":
      return <Badge variant="destructive">Needs review</Badge>;
    case "reimport":
      return (
        <Badge className="bg-blue-600 text-white">
          <RefreshCw className="mr-1 h-3 w-3" />
          Re-import
        </Badge>
      );
    case "skip":
    case "skipped":
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
        <Badge className="bg-primary text-primary-foreground">Keep Both</Badge>
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

function ImportJobFiles({ id }: { id: string }) {
  const query = useImportJob(id);
  if (query.isPending) {
    return (
      <p className="text-sm text-muted-foreground">Loading file outcomes...</p>
    );
  }
  if (query.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {query.error.message}
      </p>
    );
  }
  return (
    <div className="max-h-64 space-y-2 overflow-y-auto border-t pt-2">
      {(query.data.files ?? []).map((file) => (
        <div key={file.id} className="space-y-1 text-xs">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="break-all font-mono">{file.source_path}</span>
            {actionBadge(file.status)}
          </div>
          {file.dest_path && (
            <p className="break-all text-muted-foreground">
              Destination: {file.dest_path}
            </p>
          )}
          {file.error && (
            <p className="break-words text-destructive">{file.error}</p>
          )}
        </div>
      ))}
    </div>
  );
}

function ImportQueue({ jobs }: { jobs: ImportJob[] }) {
  const finishedToggleId = React.useId();
  const retryMut = useRetryImportJob();
  const active = jobs.filter((job) =>
    ["queued", "importing", "retry_pending"].includes(job.status),
  );
  const imported = jobs.filter((job) => job.status === "imported").length;
  const failed = jobs.filter((job) =>
    ["failed", "pending_review"].includes(job.status),
  ).length;
  const [showFinished, setShowFinished] = React.useState(false);
  const [expanded, setExpanded] = React.useState<string | null>(null);
  const [page, setPage] = React.useState(0);
  const visible = jobs.filter(
    (job) => showFinished || job.status !== "imported",
  );
  const pageSize = 25;
  const pageCount = Math.max(1, Math.ceil(visible.length / pageSize));
  const currentPage = Math.min(page, pageCount - 1);
  const pageJobs = visible.slice(
    currentPage * pageSize,
    (currentPage + 1) * pageSize,
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle>Import queue</CardTitle>
        <p className="text-sm text-muted-foreground" aria-live="polite">
          {active.length} active · {imported} imported · {failed} need attention
        </p>
        <p className="text-xs text-muted-foreground">
          Files stay queued while workers are busy. Retries resume
          automatically; successful files are not imported again.
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <label
          htmlFor={finishedToggleId}
          className="flex items-center gap-2 text-sm"
        >
          <Checkbox
            id={finishedToggleId}
            checked={showFinished}
            onCheckedChange={(checked) => {
              setShowFinished(checked === true);
              setPage(0);
            }}
          />
          Show recent successful imports
        </label>
        {visible.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No imports waiting. Scan a folder to find missing files.
          </p>
        ) : (
          <div className="max-h-96 space-y-3 overflow-y-auto">
            {pageJobs.map((job) => (
              <div key={job.id} className="space-y-2 border p-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="min-w-0 break-all font-mono text-xs">
                    {job.source_path}
                  </span>
                  {actionBadge(job.status)}
                </div>
                {job.total_files > 0 && (
                  <>
                    <Progress
                      value={(job.processed_files / job.total_files) * 100}
                      aria-valuenow={
                        (job.processed_files / job.total_files) * 100
                      }
                      aria-label={`Import progress for ${job.source_path}`}
                    />
                    <p className="text-xs text-muted-foreground">
                      {job.processed_files} / {job.total_files} files processed
                      {" · "}
                      {job.imported_files} imported
                      {" · "}
                      {job.failed_files} failed
                      {job.skipped_files
                        ? ` · ${job.skipped_files} skipped`
                        : ""}
                    </p>
                  </>
                )}
                {job.error && (
                  <p className="break-words text-sm text-destructive">
                    {job.error}
                  </p>
                )}
                {job.status === "pending_review" && (
                  <p className="text-sm text-muted-foreground">
                    Approve this import in Activity &gt; Reviews, then retry.
                    Rejected reviews cannot be retried.
                  </p>
                )}
                <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                  <span>Attempts: {job.attempts}</span>
                  {job.status === "retry_pending" && job.next_retry_at && (
                    <span>
                      Next check: {new Date(job.next_retry_at).toLocaleString()}
                    </span>
                  )}
                  {(job.status === "failed" ||
                    job.status === "pending_review") && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={retryMut.isPending}
                      onClick={() => retryMut.mutate(job.id)}
                      aria-label={`Retry ${job.source_path}${job.status === "pending_review" ? " after approval" : ""}`}
                    >
                      <RefreshCw className="mr-1 h-3 w-3" />
                      {job.status === "pending_review"
                        ? "Retry after approval"
                        : "Retry"}
                    </Button>
                  )}
                  {job.total_files > 1 && (
                    <Button
                      size="sm"
                      variant="outline"
                      aria-expanded={expanded === job.id}
                      onClick={() =>
                        setExpanded(expanded === job.id ? null : job.id)
                      }
                    >
                      {expanded === job.id
                        ? "Hide file outcomes"
                        : "Show file outcomes"}
                    </Button>
                  )}
                </div>
                {expanded === job.id && <ImportJobFiles id={job.id} />}
              </div>
            ))}
          </div>
        )}
        {pageCount > 1 && (
          <div className="flex items-center justify-between gap-2">
            <Button
              size="sm"
              variant="outline"
              aria-label="Previous queue page"
              disabled={currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              Previous
            </Button>
            <span className="text-xs text-muted-foreground">
              Page {currentPage + 1} of {pageCount}
            </span>
            <Button
              size="sm"
              variant="outline"
              aria-label="Next queue page"
              disabled={currentPage + 1 >= pageCount}
              onClick={() => setPage(currentPage + 1)}
            >
              Next
            </Button>
          </div>
        )}
        {retryMut.isError && (
          <p role="alert" className="text-sm text-destructive">
            {retryMut.error.message}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function ScanTab() {
  const importedToggleId = React.useId();
  const [folderPath, setFolderPath] = React.useState("");
  const scanMut = useScanFolder();
  const queueMut = useQueueImportFiles();
  const jobsQuery = useImportJobs();
  const [results, setResults] = React.useState<ScanResult[]>([]);
  const [scan, setScan] = React.useState<ImportScan | null>(null);
  const scanController = React.useRef<AbortController | null>(null);
  const cancelController = React.useRef<AbortController | null>(null);
  const [cancelPending, setCancelPending] = React.useState(false);
  const [cancelError, setCancelError] = React.useState<string | null>(null);
  const [cancelRequested, setCancelRequested] = React.useState(false);
  React.useEffect(
    () => () => {
      scanController.current?.abort();
      cancelController.current?.abort();
    },
    [],
  );
  const [scanStartedAt, setScanStartedAt] = React.useState(0);
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const [search, setSearch] = React.useState("");
  const [showImported, setShowImported] = React.useState(false);
  const [page, setPage] = React.useState(0);
  const [queueErrors, setQueueErrors] = React.useState<
    QueueImportResult["errors"]
  >([]);
  const [matchTarget, setMatchTarget] = React.useState<ScanResult | null>(null);
  const [matchOpen, setMatchOpen] = React.useState(false);
  const pageSize = 50;
  const jobsByPath = new Map<string, ImportJob>();
  for (const job of jobsQuery.data ?? []) {
    const existing = jobsByPath.get(job.source_path);
    if (
      !existing ||
      Date.parse(job.updated_at) > Date.parse(existing.updated_at)
    ) {
      jobsByPath.set(job.source_path, job);
    }
  }
  const statusOf = (result: ScanResult) => {
    const job = jobsByPath.get(result.file_path);
    // A fresh scan verifies destination/source identity. Older successful jobs
    // must not hide a changed source or a file missing from the library.
    if (
      job &&
      (job.status !== "imported" || Date.parse(job.updated_at) >= scanStartedAt)
    ) {
      return job.status;
    }
    return result.import_status ?? result.suggested_action;
  };
  const canQueue = (result: ScanResult) =>
    !!result.matched_media_id &&
    ["import", "reimport", "failed"].includes(statusOf(result));
  const eligible = results.filter(canQueue);
  const filtered = results.filter(
    (result) =>
      (showImported ||
        !["already_imported", "imported"].includes(statusOf(result))) &&
      `${result.file_path} ${result.matched_media ?? ""}`
        .toLowerCase()
        .includes(search.toLowerCase()),
  );
  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, pageCount - 1);
  const visible = filtered.slice(
    currentPage * pageSize,
    (currentPage + 1) * pageSize,
  );
  const selectedPaths = eligible
    .filter((result) => selected.has(result.file_path))
    .map((result) => result.file_path);
  const visibleEligible = visible.filter(canQueue);
  const allVisibleSelected =
    visibleEligible.length > 0 &&
    visibleEligible.every((result) => selected.has(result.file_path));

  const watchScan = (controller: AbortController, resume?: ImportScan) => {
    scanController.current = controller;
    scanMut.mutate(
      {
        path: resume?.path ?? folderPath.trim(),
        signal: controller.signal,
        resume,
        onProgress: (progress) => {
          if (controller.signal.aborted) return;
          setScan(progress);
          setResults((previous) => {
            // Keep queue/manual-match edits and selection when later pages arrive.
            const known = new Set(previous.map((result) => result.file_path));
            return [
              ...previous,
              ...progress.results.filter(
                (result) => !known.has(result.file_path),
              ),
            ];
          });
        },
      },
      {
        onSettled: () => {
          if (scanController.current === controller)
            scanController.current = null;
        },
      },
    );
  };

  const handleScan = () => {
    // Refs guard Enter/button submissions before React renders pending state.
    if (
      !folderPath.trim() ||
      scanController.current ||
      cancelController.current ||
      queueMut.isPending
    )
      return;
    setScanStartedAt(Date.now());
    queueMut.reset();
    setQueueErrors([]);
    setResults([]);
    setScan(null);
    setSelected(new Set());
    setPage(0);
    setMatchOpen(false);
    setMatchTarget(null);
    setCancelError(null);
    setCancelRequested(false);
    watchScan(new AbortController());
  };

  const handleCancel = async () => {
    if (
      !scan ||
      scan.status !== "running" ||
      cancelController.current ||
      cancelRequested
    )
      return;
    const controller = new AbortController();
    cancelController.current = controller;
    setCancelPending(true);
    setCancelError(null);
    try {
      await cancelImportScan(scan.id, controller.signal);
      if (controller.signal.aborted) return;
      setCancelRequested(true);
      // Keep the existing poller alive to drain unread cancelled pages. If
      // polling already failed, resume this session's cursor, never POST again.
      if (!scanController.current) watchScan(new AbortController(), scan);
    } catch (error) {
      if (!controller.signal.aborted) {
        setCancelError(
          error instanceof Error ? error.message : "Request failed",
        );
      }
    } finally {
      if (!controller.signal.aborted) setCancelPending(false);
      if (cancelController.current === controller)
        cancelController.current = null;
    }
  };

  const queuePaths = (paths: string[]) => {
    setQueueErrors([]);
    queueMut.mutate(paths, {
      onSuccess: (outcome) => {
        const accepted = new Map(
          outcome.data.map((job) => [job.source_path, job.status]),
        );
        setQueueErrors(outcome.errors);
        setSelected(
          (previous) =>
            new Set([...previous].filter((path) => !accepted.has(path))),
        );
        setResults((previous) =>
          previous.map((result) =>
            accepted.has(result.file_path)
              ? { ...result, import_status: accepted.get(result.file_path) }
              : result,
          ),
        );
      },
    });
  };

  return (
    <div className="space-y-4">
      {jobsQuery.isError && (
        <p role="alert" className="text-sm text-destructive">
          Import status unavailable: {jobsQuery.error.message}
        </p>
      )}
      {jobsQuery.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          Loading import queue...
        </p>
      ) : (
        <ImportQueue jobs={jobsQuery.data ?? []} />
      )}
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
              aria-label="Download folder"
              value={folderPath}
              onChange={(e) => setFolderPath(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleScan();
              }}
              className="flex-1"
            />
            <Button
              onClick={handleScan}
              disabled={
                scanMut.isPending ||
                cancelPending ||
                queueMut.isPending ||
                !folderPath.trim()
              }
            >
              {scanMut.isPending ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <FolderSearch className="mr-2 h-4 w-4" />
              )}
              Scan
            </Button>
            {scan?.status === "running" && (
              <Button
                variant="outline"
                onClick={() => void handleCancel()}
                disabled={cancelPending || cancelRequested}
              >
                {cancelPending
                  ? "Cancelling..."
                  : cancelRequested
                    ? "Cancellation requested"
                    : "Cancel scan"}
              </Button>
            )}
          </div>
          {cancelError && (
            <p role="alert" className="mt-2 text-sm text-destructive">
              Could not cancel scan: {cancelError}. You can retry cancellation.
            </p>
          )}
          {(scanMut.isPending || scan) && (
            <div
              role="status"
              className="mt-2 space-y-1 text-sm text-muted-foreground"
            >
              <p>
                {scanMut.isError
                  ? "Scan stopped. Partial results are still available."
                  : !scan
                    ? "Starting scan..."
                    : scan.status === "running"
                      ? scan.phase === "discovering"
                        ? `Discovering files: ${scan.total_files} found · ${scan.processed_files} processed`
                        : `Matching files: ${scan.processed_files} / ${scan.total_files} processed`
                      : scanMut.isPending
                        ? "Loading remaining scan results..."
                        : scan.status === "completed"
                          ? `Scan completed: ${scan.processed_files} / ${scan.total_files} files processed`
                          : scan.status === "failed"
                            ? "Scan failed. Partial results are still available."
                            : "Scan cancelled. Partial results are still available."}
              </p>
              {scanMut.isPending &&
                scan?.status === "running" &&
                scan.current_file && (
                  <p className="break-all">Current file: {scan.current_file}</p>
                )}
            </div>
          )}
          {scan?.status === "failed" && (
            <p role="alert" className="mt-2 text-sm text-destructive">
              {scan.error ||
                "The scan failed before all files could be processed."}
            </p>
          )}
          {scanMut.isError && (
            <p role="alert" className="mt-2 text-sm text-destructive">
              Scan could not continue: {scanMut.error.message}. Partial results
              are still available. Start a new scan to try again.
            </p>
          )}
        </CardContent>
      </Card>

      {results.length > 0 && (
        <Card>
          <CardHeader className="space-y-3">
            <CardTitle>
              Scan Results ({results.length} file
              {results.length !== 1 ? "s" : ""})
            </CardTitle>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                onClick={() => queuePaths(selectedPaths)}
                disabled={queueMut.isPending || selectedPaths.length === 0}
              >
                <Import className="mr-2 h-4 w-4" />
                Import selected ({selectedPaths.length})
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => queuePaths(eligible.map((r) => r.file_path))}
                disabled={queueMut.isPending || eligible.length === 0}
              >
                Import all matched ({eligible.length})
              </Button>
              <Input
                placeholder="Filter files or titles..."
                aria-label="Filter scan results"
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value);
                  setPage(0);
                }}
                className="max-w-xs"
              />
              <label
                htmlFor={importedToggleId}
                className="flex items-center gap-2 text-sm"
              >
                <Checkbox
                  id={importedToggleId}
                  checked={showImported}
                  onCheckedChange={(checked) => {
                    setShowImported(checked === true);
                    setPage(0);
                  }}
                />
                Show already imported files
              </label>
            </div>
            {queueMut.isError && (
              <p role="alert" className="text-sm text-destructive">
                {queueMut.error.message}
              </p>
            )}
            {queueMut.isSuccess && queueMut.data.data.length > 0 && (
              <p role="status" className="text-sm text-muted-foreground">
                {queueMut.data.data.length}{" "}
                {queueMut.data.data.length === 1 ? "file" : "files"} queued, not
                completed. Follow their progress in the import queue.
              </p>
            )}
            {queueErrors.length > 0 && (
              <div
                role="alert"
                className="max-h-48 space-y-1 overflow-y-auto text-sm text-destructive"
              >
                <p>{queueErrors.length} files could not be queued:</p>
                {queueErrors.map((failure) => (
                  <p key={failure.path} className="break-all">
                    {failure.path}: {failure.error}
                  </p>
                ))}
              </div>
            )}
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>
                    <Checkbox
                      aria-label="Select all eligible files on this page"
                      checked={
                        allVisibleSelected
                          ? true
                          : visibleEligible.some((r) =>
                                selected.has(r.file_path),
                              )
                            ? "indeterminate"
                            : false
                      }
                      disabled={visibleEligible.length === 0}
                      onCheckedChange={(checked) =>
                        setSelected((previous) => {
                          const next = new Set(previous);
                          for (const result of visibleEligible) {
                            if (checked === true) next.add(result.file_path);
                            else next.delete(result.file_path);
                          }
                          return next;
                        })
                      }
                    />
                  </TableHead>
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
                {visible.map((r) => (
                  <TableRow key={r.file_path}>
                    <TableCell>
                      <Checkbox
                        aria-label={`Select ${r.file_path.split("/").pop()}`}
                        checked={canQueue(r) && selected.has(r.file_path)}
                        disabled={!canQueue(r)}
                        onCheckedChange={(checked) =>
                          setSelected((previous) => {
                            const next = new Set(previous);
                            if (checked === true) next.add(r.file_path);
                            else next.delete(r.file_path);
                            return next;
                          })
                        }
                      />
                    </TableCell>
                    <TableCell className="max-w-[200px] truncate font-mono text-xs">
                      <div className="flex items-center gap-1">
                        {r.media_type === "music" ? (
                          <FileAudio
                            aria-label="Audio file"
                            className="h-4 w-4 shrink-0 text-muted-foreground"
                          />
                        ) : (
                          <FileVideo
                            aria-label="Video file"
                            className="h-4 w-4 shrink-0 text-muted-foreground"
                          />
                        )}
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
                      {r.media_type === "music" && (
                        <div className="text-xs text-muted-foreground">
                          {[
                            r.detected_artist,
                            r.detected_album,
                            r.detected_disc != null
                              ? `Disc ${r.detected_disc}`
                              : null,
                            r.detected_track != null
                              ? `Track ${r.detected_track}`
                              : null,
                          ]
                            .filter(Boolean)
                            .join(" · ")}
                        </div>
                      )}
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
                    <TableCell>
                      {actionBadge(statusOf(r))}
                      {r.import_error && (
                        <p className="mt-1 text-xs text-destructive">
                          {r.import_error}
                        </p>
                      )}
                    </TableCell>
                    <TableCell>
                      {canQueue(r) && (
                        <Button
                          size="sm"
                          variant="outline"
                          aria-label={`Import ${r.file_path.split("/").pop()}`}
                          onClick={() => queuePaths([r.file_path])}
                          disabled={queueMut.isPending}
                        >
                          {queueMut.isPending ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Import className="h-4 w-4" />
                          )}
                        </Button>
                      )}
                      {["no_match", "manual_review"].includes(statusOf(r)) && (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => {
                            setMatchTarget(r);
                            setMatchOpen(true);
                          }}
                        >
                          <Link2 className="mr-1 h-4 w-4" />
                          Match
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {filtered.length === 0 && (
              <p className="py-4 text-sm text-muted-foreground">
                No files need importing for this filter.
              </p>
            )}
            <div className="mt-4 flex items-center justify-between gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={currentPage === 0}
                onClick={() => setPage(currentPage - 1)}
              >
                Previous
              </Button>
              <span className="text-sm text-muted-foreground">
                {filtered.length} files · Page {currentPage + 1} of {pageCount}
              </span>
              <Button
                size="sm"
                variant="outline"
                disabled={currentPage + 1 >= pageCount}
                onClick={() => setPage(currentPage + 1)}
              >
                Next
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      <ManualMatchDialog
        open={matchOpen}
        onOpenChange={setMatchOpen}
        scanResult={matchTarget}
        onMatched={() => {
          // Mark the item as queued instead of removing it
          setResults((prev) =>
            prev.map((r) =>
              r.file_path === matchTarget?.file_path
                ? { ...r, import_status: "queued" }
                : r,
            ),
          );
          setMatchTarget(null);
        }}
      />
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
            <Select value={String(pageSize)} onValueChange={() => setPage(0)}>
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
                        <TableCell
                          className="max-w-[150px] truncate font-mono text-xs"
                          title={d.source_path}
                        >
                          {d.source_path.split("/").pop()}
                        </TableCell>
                        <TableCell
                          className="max-w-[150px] truncate font-mono text-xs"
                          title={d.dest_path}
                        >
                          {d.dest_path.split("/").pop()}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1">
                            <Badge variant="outline" className="text-xs">
                              {d.media_type}
                            </Badge>
                            <span
                              className="max-w-[100px] truncate text-xs"
                              title={d.media_id}
                            >
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
