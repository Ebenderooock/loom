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
  FolderSearch, Loader2, CheckCircle2, AlertCircle, FileVideo,
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
  const pollTimeoutRef = useRef<ReturnType<typeof setTimeout>>();
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
            const unmatchedRes = await apiFetch("/api/v1/series/scan/unmatched");
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
      ? ((scanResult.matched + scanResult.unmatched) / scanResult.totalFiles) * 100
      : 0
    : 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FolderSearch className="w-5 h-5" />
            Import Existing Series
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-auto space-y-4">
          {/* Library selection */}
          {!scanResult && !scanning && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Scan a library to discover TV shows. Show folders will be matched
                against TMDB and added automatically, then episode files will be linked.
              </p>

              <div className="space-y-2">
                <span className="text-sm font-medium">Library</span>
                {libraries.length === 0 ? (
                  <p className="text-sm text-destructive">
                    No series libraries configured. Add one in Settings first.
                  </p>
                ) : (
                  <div className="space-y-1">
                    {libraries.map(lib => (
                      <button
                        key={lib.id}
                        onClick={() => setSelectedLibrary(lib.id)}
                        className={`w-full text-left px-3 py-2 rounded-md text-sm transition-colors ${
                          selectedLibrary === lib.id
                            ? "bg-primary/10 border border-primary/30 text-primary"
                            : "bg-muted/30 hover:bg-muted/50 border border-transparent"
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
                <FolderSearch className="w-4 h-4" />
                Start Scan
              </Button>
            </div>
          )}

          {/* Scanning progress */}
          {scanning && scanResult && (
            <div className="space-y-4">
              <div className="flex items-center gap-2 text-sm">
                <Loader2 className="w-4 h-4 animate-spin text-primary" />
                <span>Scanning for TV shows...</span>
              </div>
              <Progress value={progress} className="h-2" />
              <div className="grid grid-cols-3 gap-3 text-center">
                <div className="p-3 rounded-lg bg-muted/30">
                  <div className="text-2xl font-bold">{scanResult.totalFiles}</div>
                  <div className="text-xs text-muted-foreground">Files Found</div>
                </div>
                <div className="p-3 rounded-lg bg-emerald-500/10">
                  <div className="text-2xl font-bold text-emerald-500">{scanResult.matched}</div>
                  <div className="text-xs text-muted-foreground">Matched</div>
                </div>
                <div className="p-3 rounded-lg bg-amber-500/10">
                  <div className="text-2xl font-bold text-amber-500">{scanResult.unmatched}</div>
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
                  <CheckCircle2 className="w-5 h-5 text-emerald-500" />
                ) : (
                  <AlertCircle className="w-5 h-5 text-destructive" />
                )}
                <span className="font-medium">
                  Scan {scanResult.status === "completed" ? "Complete" : "Failed"}
                </span>
              </div>

              <div className="grid grid-cols-4 gap-3 text-center">
                <div className="p-3 rounded-lg bg-muted/30">
                  <div className="text-xl font-bold">{scanResult.totalFiles}</div>
                  <div className="text-xs text-muted-foreground">Total</div>
                </div>
                <div className="p-3 rounded-lg bg-emerald-500/10">
                  <div className="text-xl font-bold text-emerald-500">{scanResult.imported}</div>
                  <div className="text-xs text-muted-foreground">Imported</div>
                </div>
                <div className="p-3 rounded-lg bg-blue-500/10">
                  <div className="text-xl font-bold text-blue-500">{scanResult.matched}</div>
                  <div className="text-xs text-muted-foreground">Matched</div>
                </div>
                <div className="p-3 rounded-lg bg-amber-500/10">
                  <div className="text-xl font-bold text-amber-500">{scanResult.unmatched}</div>
                  <div className="text-xs text-muted-foreground">Unmatched</div>
                </div>
              </div>

              {scanResult.errors && scanResult.errors.length > 0 && (
                <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                  <div className="text-sm font-medium text-destructive mb-1">Errors</div>
                  <div className="text-xs text-destructive/80 space-y-0.5">
                    {scanResult.errors.slice(0, 5).map((e, i) => (
                      <div key={i} className="truncate">{e}</div>
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
                  <h3 className="text-sm font-medium flex items-center gap-2">
                    <AlertCircle className="w-4 h-4 text-amber-500" />
                    Unmatched Files ({unmatchedFiles.length})
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    These episode files couldn&apos;t be matched to any series or episode.
                  </p>
                  <ScrollArea className="max-h-[300px]">
                    <div className="space-y-1">
                      {unmatchedFiles.map(f => (
                        <div
                          key={f.id}
                          className="w-full text-left px-3 py-2 rounded-md text-sm bg-muted/20 flex items-center gap-2"
                        >
                          <FileVideo className="w-4 h-4 text-muted-foreground shrink-0" />
                          <div className="flex-1 min-w-0">
                            <div className="truncate font-mono text-xs">
                              {f.filePath.split("/").pop()}
                            </div>
                            <div className="flex gap-2 mt-0.5">
                              {f.parsedTitle && (
                                <Badge variant="outline" className="text-[10px] h-4">
                                  {f.parsedTitle}
                                </Badge>
                              )}
                              {f.quality && f.quality !== "unknown" && (
                                <Badge variant="secondary" className="text-[10px] h-4">
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
                <Button onClick={() => { setScanResult(null); setScanning(false); }}>
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
