import { useState, useEffect, useCallback, useRef } from "react";
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
  Search, ChevronRight,
} from "lucide-react";
import { Input } from "@/components/ui/input";
import type { Library } from "../../lib/libraries-api";

interface ScanResult {
  id: string;
  libraryId: string;
  libraryPath: string;  status: "running" | "completed" | "failed";
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

interface TmdbResult {
  tmdb_id: string;
  title: string;
  year: number;
  poster_path: string;
  overview: string;
}

export function LibraryImportDialog({
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
  const [selectedFolder, setSelectedFolder] = useState<string>("");
  const [matchingFile, setMatchingFile] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<TmdbResult[]>([]);
  const [searching, setSearching] = useState(false);
  const pollTimeoutRef = useRef<ReturnType<typeof setTimeout>>();
  const mountedRef = useRef(true);

  // Clean up polling on unmount
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      clearTimeout(pollTimeoutRef.current);
    };
  }, []);

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setScanResult(null);
      setUnmatchedFiles([]);
      setScanning(false);
      setMatchingFile(null);
      if (libraries.length === 1) {
        setSelectedFolder(libraries[0]!.id);
      }
    }
  }, [open, libraries]);

  const startScan = useCallback(async () => {
    if (!selectedFolder) return;
    setScanning(true);
    setScanResult(null);
    setUnmatchedFiles([]);

    try {
      const res = await fetch("/api/v1/movies/scan", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ libraryId: selectedFolder }),
      });
      const data = await res.json();
      const scanId = data.scanId;

      // Poll for completion
      const poll = async () => {
        if (!mountedRef.current) return;
        try {
          const statusRes = await fetch(`/api/v1/movies/scan/${scanId}`, {
            credentials: "include",
          });
          const result: ScanResult = await statusRes.json();
          if (!mountedRef.current) return;
          setScanResult(result);

          if (result.status === "running") {
            pollTimeoutRef.current = setTimeout(poll, 1500);
          } else {
            setScanning(false);
            const unmatchedRes = await fetch("/api/v1/movies/scan/unmatched", {
              credentials: "include",
            });
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
  }, [selectedFolder, onImportComplete]);

  const searchTmdb = useCallback(async (query: string) => {
    if (!query.trim()) return;
    setSearching(true);
    try {
      const res = await fetch(
        `/api/v1/movies/lookup?term=${encodeURIComponent(query)}`,
        { credentials: "include" }
      );
      const data = await res.json();
      setSearchResults(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error("TMDB search failed:", err);
    } finally {
      setSearching(false);
    }
  }, []);

  const matchFile = useCallback(async (unmatchedId: string, tmdbId: string) => {
    try {
      await fetch("/api/v1/movies/scan/match", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          unmatchedId,
          tmdbId,
          libraryId: selectedFolder,
          qualityProfileId: "",
        }),
      });
      // Remove from list
      setUnmatchedFiles(prev => prev.filter(f => f.id !== unmatchedId));
      setMatchingFile(null);
      setSearchQuery("");
      setSearchResults([]);
      onImportComplete();
    } catch (err) {
      console.error("Match failed:", err);
    }
  }, [selectedFolder, onImportComplete]);

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
            Import Existing Media
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-auto space-y-4">
          {/* Folder selection */}
          {!scanResult && !scanning && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Scan a library to discover and import existing movies. 
                Files will be matched against TMDB automatically.
              </p>

              <div className="space-y-2">
                <label className="text-sm font-medium">Library</label>
                {libraries.length === 0 ? (
                  <p className="text-sm text-destructive">
                    No libraries configured. Add one in Settings first.
                  </p>
                ) : (
                  <div className="space-y-1">
                    {libraries.map(f => (
                      <button
                        key={f.id}
                        onClick={() => setSelectedFolder(f.id)}
                        className={`w-full text-left px-3 py-2 rounded-md text-sm transition-colors ${
                          selectedFolder === f.id
                            ? "bg-primary/10 border border-primary/30 text-primary"
                            : "bg-muted/30 hover:bg-muted/50 border border-transparent"
                        }`}
                      >
                        {f.path}
                      </button>
                    ))}
                  </div>
                )}

              </div>

              <Button
                onClick={startScan}
                disabled={!selectedFolder}
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
                <span>Scanning {scanResult.libraryPath}...</span>
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
          {scanResult && scanResult.status !== "running" && !matchingFile && (
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
                    These files couldn&apos;t be auto-matched. Click to manually match.
                  </p>
                  <ScrollArea className="max-h-[300px]">
                    <div className="space-y-1">
                      {unmatchedFiles.map(f => (
                        <button
                          key={f.id}
                          onClick={() => {
                            setMatchingFile(f.id);
                            setSearchQuery(f.parsedTitle || "");
                            if (f.parsedTitle) searchTmdb(f.parsedTitle);
                          }}
                          className="w-full text-left px-3 py-2 rounded-md text-sm bg-muted/20 hover:bg-muted/40 transition-colors flex items-center gap-2"
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
                              {f.parsedYear > 0 && (
                                <Badge variant="outline" className="text-[10px] h-4">
                                  {f.parsedYear}
                                </Badge>
                              )}
                              {f.quality && f.quality !== "unknown" && (
                                <Badge variant="secondary" className="text-[10px] h-4">
                                  {f.quality}
                                </Badge>
                              )}
                            </div>
                          </div>
                          <ChevronRight className="w-4 h-4 text-muted-foreground" />
                        </button>
                      ))}
                    </div>
                  </ScrollArea>
                </div>
              )}

              {unmatchedFiles.length === 0 && (
                <div className="flex justify-end gap-2 pt-2">
                  <Button variant="outline" onClick={() => onOpenChange(false)}>
                    Close
                  </Button>
                  <Button onClick={() => { setScanResult(null); setScanning(false); }}>
                    Scan Again
                  </Button>
                </div>
              )}
            </div>
          )}

          {/* Manual matching UI */}
          {matchingFile && (
            <div className="space-y-4">
              <Button
                variant="ghost"
                size="sm"
                className="h-7 text-xs"
                onClick={() => {
                  setMatchingFile(null);
                  setSearchQuery("");
                  setSearchResults([]);
                }}
              >
                ← Back to unmatched files
              </Button>

              <div className="p-3 rounded-lg bg-muted/20">
                <div className="text-xs text-muted-foreground">Matching file:</div>
                <div className="text-sm font-mono truncate">
                  {unmatchedFiles.find(f => f.id === matchingFile)?.filePath.split("/").pop()}
                </div>
              </div>

              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                  <Input
                    placeholder="Search TMDB..."
                    value={searchQuery}
                    onChange={e => setSearchQuery(e.target.value)}
                    onKeyDown={e => e.key === "Enter" && searchTmdb(searchQuery)}
                    className="pl-9 h-9"
                  />
                </div>
                <Button
                  size="sm"
                  className="h-9"
                  onClick={() => searchTmdb(searchQuery)}
                  disabled={searching}
                >
                  {searching ? <Loader2 className="w-4 h-4 animate-spin" /> : "Search"}
                </Button>
              </div>

              <ScrollArea className="max-h-[300px]">
                <div className="space-y-1">
                  {searchResults.map((r) => (
                    <button
                      key={r.tmdb_id}
                      onClick={() => matchFile(matchingFile, r.tmdb_id)}
                      className="w-full text-left px-3 py-2 rounded-md text-sm bg-muted/20 hover:bg-muted/40 transition-colors flex items-center gap-3"
                    >
                      {r.poster_path ? (
                        <img
                          src={`https://image.tmdb.org/t/p/w92${r.poster_path}`}
                          className="w-10 h-14 object-cover rounded"
                          alt=""
                        />
                      ) : (
                        <div className="w-10 h-14 bg-muted rounded flex items-center justify-center">
                          <FileVideo className="w-4 h-4 text-muted-foreground" />
                        </div>
                      )}
                      <div className="flex-1 min-w-0">
                        <div className="font-medium truncate">{r.title}</div>
                        <div className="text-xs text-muted-foreground">
                          {r.year > 0 && r.year} · TMDB: {r.tmdb_id}
                        </div>
                      </div>
                    </button>
                  ))}
                  {!searching && searchResults.length === 0 && searchQuery && (
                    <div className="text-center text-sm text-muted-foreground py-4">
                      No results found
                    </div>
                  )}
                </div>
              </ScrollArea>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
