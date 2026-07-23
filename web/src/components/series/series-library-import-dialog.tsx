import { useState, useEffect, useCallback, useRef } from "react";
import { apiFetch } from "@/lib/fetch";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  FolderSearch,
  Loader2,
  CheckCircle2,
  AlertCircle,
  FileVideo,
} from "lucide-react";
import type { Library } from "../../lib/libraries-api";

interface ScanResult {
  id: string;
  libraryId: string;
  rootFolderPath: string;
  status: "running" | "completed" | "failed";
  totalFiles: number;
  matched: number;
  unmatched: number;
  imported: number;
  errors?: string[];
  startedAt: string;
  completedAt?: string;
}

interface UnmatchedFile {
  id: string;
  scanId: string;
  filePath: string;
  size: number;
  parsedTitle: string;
  parsedYear: number;
  quality: string;
  source: string;
}

export function SeriesLibraryImportDialog({
  open,
  onOpenChange,
  libraries,
  onImportComplete,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  onImportComplete: () => void;
}) {
  const [scanning, setScanning] = useState(false);
  const [scanResult, setScanResult] = useState<ScanResult | null>(null);
  const [unmatchedFiles, setUnmatchedFiles] = useState<UnmatchedFile[]>([]);
  const [selectedLibrary, setSelectedLibrary] = useState<string>("");
  const pollTimeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      clearTimeout(pollTimeoutRef.current);
    };
  }, []);

  useEffect(() => {
    if (open) {
      setScanResult(null);
      setUnmatchedFiles([]);
      setScanning(false);
      if (libraries.length === 1) {
        setSelectedLibrary(libraries[0]!.id);
      }
    }
  }, [open, libraries]);

  const startScan = useCallback(async () => {
    if (!selectedLibrary) return;
    setScanning(true);
    setScanResult(null);
    setUnmatchedFiles([]);

    try {
      const res = await apiFetch("/api/v1/series/scan", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ libraryId: selectedLibrary }),
      });
      const data = await res.json();
      const scanId = data.scanId;

      const poll = async () => {
        if (!mountedRef.current) return;
        try {
          const statusRes = await apiFetch(`/api/v1/series/scan/${scanId}`);
          const result: ScanResult = await statusRes.json();
          if (!mountedRef.current) return;
          setScanResult(result);

          if (result.status === "running") {
            pollTimeoutRef.current = setTimeout(poll, 1500);
          } else {
            setScanning(false);
            const unmatchedRes = await apiFetch(
              "/api/v1/series/scan/unmatched",
            );
            const files: UnmatchedFile[] = await unmatchedRes.json();
            if (mountedRef.current) {
              setUnmatchedFiles(files);
              onImportComplete();
            }
          }
        } catch (err) {
          if (mountedRef.current) {
            setScanning(false);
            console.error("Poll failed:", err);
          }
        }
      };
      pollTimeoutRef.current = setTimeout(poll, 1000);
    } catch (err) {
      setScanning(false);
      console.error("Scan failed:", err);
    }
  }, [selectedLibrary, onImportComplete]);

  const progress = scanResult
    ? scanResult.totalFiles > 0
      ? ((scanResult.matched + scanResult.unmatched) / scanResult.totalFiles) *
        100
      : 0
    : 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] max-w-2xl flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FolderSearch className="h-5 w-5" />
            Import Existing Series
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 space-y-4 overflow-auto">
          {/* Library selection */}
          {!scanResult && !scanning && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Scan a library to discover TV shows. Show folders will be
                matched against TMDB and added automatically, then episode files
                will be linked.
              </p>

              <div className="space-y-2">
                <span className="text-sm font-medium">Library</span>
                {libraries.length === 0 ? (
                  <p className="text-sm text-destructive">
                    No series libraries configured. Add one in Settings first.
                  </p>
                ) : (
                  <div className="space-y-1">
                    {libraries.map((lib) => (
                      <button
                        key={lib.id}
                        onClick={() => setSelectedLibrary(lib.id)}
                        className={`w-full rounded-md px-3 py-2 text-left text-sm transition-colors ${
                          selectedLibrary === lib.id
                            ? "border border-primary/30 bg-primary/10 text-primary"
                            : "border border-transparent bg-muted/30 hover:bg-muted/50"
                        }`}
                      >
                        {lib.path}
                      </button>
                    ))}
                  </div>
                )}
              </div>

              <Button
                onClick={startScan}
                disabled={!selectedLibrary}
                className="w-full gap-2"
              >
                <FolderSearch className="h-4 w-4" />
                Start Scan
              </Button>
            </div>
          )}

          {/* Scanning progress */}
          {scanning && scanResult && (
            <div className="space-y-4">
              <div className="flex items-center gap-2 text-sm">
                <Loader2 className="h-4 w-4 animate-spin text-primary" />
                <span>Scanning for TV shows...</span>
              </div>
              <Progress value={progress} className="h-2" />
              <div className="grid grid-cols-3 gap-3 text-center">
                <div className="rounded-lg bg-muted/30 p-3">
                  <div className="text-2xl font-bold">
                    {scanResult.totalFiles}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    Files Found
                  </div>
                </div>
                <div className="rounded-lg bg-emerald-500/10 p-3">
                  <div className="text-2xl font-bold text-emerald-500">
                    {scanResult.matched}
                  </div>
                  <div className="text-xs text-muted-foreground">Matched</div>
                </div>
                <div className="rounded-lg bg-amber-500/10 p-3">
                  <div className="text-2xl font-bold text-amber-500">
                    {scanResult.unmatched}
                  </div>
                  <div className="text-xs text-muted-foreground">Unmatched</div>
                </div>
              </div>
            </div>
          )}

          {/* Scan complete */}
          {scanResult && scanResult.status !== "running" && (
            <div className="space-y-4">
              <div className="flex items-center gap-2">
                {scanResult.status === "completed" ? (
                  <CheckCircle2 className="h-5 w-5 text-emerald-500" />
                ) : (
                  <AlertCircle className="h-5 w-5 text-destructive" />
                )}
                <span className="font-medium">
                  Scan{" "}
                  {scanResult.status === "completed" ? "Complete" : "Failed"}
                </span>
              </div>

              <div className="grid grid-cols-4 gap-3 text-center">
                <div className="rounded-lg bg-muted/30 p-3">
                  <div className="text-xl font-bold">
                    {scanResult.totalFiles}
                  </div>
                  <div className="text-xs text-muted-foreground">Total</div>
                </div>
                <div className="rounded-lg bg-emerald-500/10 p-3">
                  <div className="text-xl font-bold text-emerald-500">
                    {scanResult.imported}
                  </div>
                  <div className="text-xs text-muted-foreground">Imported</div>
                </div>
                <div className="rounded-lg bg-blue-500/10 p-3">
                  <div className="text-xl font-bold text-blue-500">
                    {scanResult.matched}
                  </div>
                  <div className="text-xs text-muted-foreground">Matched</div>
                </div>
                <div className="rounded-lg bg-amber-500/10 p-3">
                  <div className="text-xl font-bold text-amber-500">
                    {scanResult.unmatched}
                  </div>
                  <div className="text-xs text-muted-foreground">Unmatched</div>
                </div>
              </div>

              {scanResult.errors && scanResult.errors.length > 0 && (
                <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-3">
                  <div className="mb-1 text-sm font-medium text-destructive">
                    Errors
                  </div>
                  <div className="space-y-0.5 text-xs text-destructive/80">
                    {scanResult.errors.slice(0, 5).map((e, i) => (
                      <div key={i} className="truncate">
                        {e}
                      </div>
                    ))}
                    {scanResult.errors.length > 5 && (
                      <div>...and {scanResult.errors.length - 5} more</div>
                    )}
                  </div>
                </div>
              )}

              {/* Unmatched files list */}
              {unmatchedFiles.length > 0 && (
                <div className="space-y-2">
                  <h3 className="flex items-center gap-2 text-sm font-medium">
                    <AlertCircle className="h-4 w-4 text-amber-500" />
                    Unmatched Files ({unmatchedFiles.length})
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    These episode files couldn&apos;t be matched to any series
                    or episode.
                  </p>
                  <ScrollArea className="max-h-[300px]">
                    <div className="space-y-1">
                      {unmatchedFiles.map((f) => (
                        <div
                          key={f.id}
                          className="flex w-full items-center gap-2 rounded-md bg-muted/20 px-3 py-2 text-left text-sm"
                        >
                          <FileVideo className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <div className="min-w-0 flex-1">
                            <div className="truncate font-mono text-xs">
                              {f.filePath.split("/").pop()}
                            </div>
                            <div className="mt-0.5 flex gap-2">
                              {f.parsedTitle && (
                                <Badge
                                  variant="outline"
                                  className="h-4 text-[10px]"
                                >
                                  {f.parsedTitle}
                                </Badge>
                              )}
                              {f.quality && f.quality !== "unknown" && (
                                <Badge
                                  variant="secondary"
                                  className="h-4 text-[10px]"
                                >
                                  {f.quality}
                                </Badge>
                              )}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </ScrollArea>
                </div>
              )}

              <div className="flex justify-end gap-2 pt-2">
                <Button variant="outline" onClick={() => onOpenChange(false)}>
                  Close
                </Button>
                <Button
                  onClick={() => {
                    setScanResult(null);
                    setScanning(false);
                  }}
                >
                  Scan Again
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
