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
  Search,
  ChevronRight,
} from "lucide-react";
import { Input } from "@/components/ui/input";
import type { Library } from "../../lib/libraries-api";

interface ScanResult {
  id: string;
  libraryId: string;
  libraryPath: string;
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
      const res = await apiFetch("/api/v1/movies/scan", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ libraryId: selectedFolder }),
      });
      const data = await res.json();
      const scanId = data.scanId;

      // Poll for completion
      const poll = async () => {
        if (!mountedRef.current) return;
        try {
          const statusRes = await apiFetch(`/api/v1/movies/scan/${scanId}`);
          const result: ScanResult = await statusRes.json();
          if (!mountedRef.current) return;
          setScanResult(result);

          if (result.status === "running") {
            pollTimeoutRef.current = setTimeout(poll, 1500);
          } else {
            setScanning(false);
            const unmatchedRes = await apiFetch(
              "/api/v1/movies/scan/unmatched",
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
  }, [selectedFolder, onImportComplete]);

  const searchTmdb = useCallback(async (query: string) => {
    if (!query.trim()) return;
    setSearching(true);
    try {
      const res = await apiFetch(
        `/api/v1/movies/lookup?term=${encodeURIComponent(query)}`,
      );
      const data = await res.json();
      setSearchResults(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error("TMDB search failed:", err);
    } finally {
      setSearching(false);
    }
  }, []);

  const matchFile = useCallback(
    async (unmatchedId: string, tmdbId: string) => {
      try {
        await apiFetch("/api/v1/movies/scan/match", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            unmatchedId,
            tmdbId,
            libraryId: selectedFolder,
            qualityProfileId: "",
          }),
        });
        // Remove from list
        setUnmatchedFiles((prev) => prev.filter((f) => f.id !== unmatchedId));
        setMatchingFile(null);
        setSearchQuery("");
        setSearchResults([]);
        onImportComplete();
      } catch (err) {
        console.error("Match failed:", err);
      }
    },
    [selectedFolder, onImportComplete],
  );

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
            Import Existing Media
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 space-y-4 overflow-auto">
          {/* Folder selection */}
          {!scanResult && !scanning && (
            <div className="space-y-4">
              <p className="text-sm text-muted-foreground">
                Scan a library to discover and import existing movies. Files
                will be matched against TMDB automatically.
              </p>

              <div className="space-y-2">
                <span className="text-sm font-medium">Library</span>
                {libraries.length === 0 ? (
                  <p className="text-sm text-destructive">
                    No libraries configured. Add one in Settings first.
                  </p>
                ) : (
                  <div className="space-y-1">
                    {libraries.map((f) => (
                      <button
                        key={f.id}
                        onClick={() => setSelectedFolder(f.id)}
                        className={`w-full rounded-md px-3 py-2 text-left text-sm transition-colors ${
                          selectedFolder === f.id
                            ? "border border-primary/30 bg-primary/10 text-primary"
                            : "border border-transparent bg-muted/30 hover:bg-muted/50"
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
                <span>Scanning {scanResult.libraryPath}...</span>
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
          {scanResult && scanResult.status !== "running" && !matchingFile && (
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
                    These files couldn&apos;t be auto-matched. Click to manually
                    match.
                  </p>
                  <ScrollArea className="max-h-[300px]">
                    <div className="space-y-1">
                      {unmatchedFiles.map((f) => (
                        <button
                          key={f.id}
                          onClick={() => {
                            setMatchingFile(f.id);
                            setSearchQuery(f.parsedTitle || "");
                            if (f.parsedTitle) searchTmdb(f.parsedTitle);
                          }}
                          className="flex w-full items-center gap-2 rounded-md bg-muted/20 px-3 py-2 text-left text-sm transition-colors hover:bg-muted/40"
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
                              {f.parsedYear > 0 && (
                                <Badge
                                  variant="outline"
                                  className="h-4 text-[10px]"
                                >
                                  {f.parsedYear}
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
                          <ChevronRight className="h-4 w-4 text-muted-foreground" />
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
                  <Button
                    onClick={() => {
                      setScanResult(null);
                      setScanning(false);
                    }}
                  >
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

              <div className="rounded-lg bg-muted/20 p-3">
                <div className="text-xs text-muted-foreground">
                  Matching file:
                </div>
                <div className="truncate font-mono text-sm">
                  {unmatchedFiles
                    .find((f) => f.id === matchingFile)
                    ?.filePath.split("/")
                    .pop()}
                </div>
              </div>

              <div className="flex gap-2">
                <div className="relative flex-1">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder="Search TMDB..."
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    onKeyDown={(e) =>
                      e.key === "Enter" && searchTmdb(searchQuery)
                    }
                    className="h-9 pl-9"
                  />
                </div>
                <Button
                  size="sm"
                  className="h-9"
                  onClick={() => searchTmdb(searchQuery)}
                  disabled={searching}
                >
                  {searching ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    "Search"
                  )}
                </Button>
              </div>

              <ScrollArea className="max-h-[300px]">
                <div className="space-y-1">
                  {searchResults.map((r) => (
                    <button
                      key={r.tmdb_id}
                      onClick={() => matchFile(matchingFile, r.tmdb_id)}
                      className="flex w-full items-center gap-3 rounded-md bg-muted/20 px-3 py-2 text-left text-sm transition-colors hover:bg-muted/40"
                    >
                      {r.poster_path ? (
                        <img
                          src={`https://image.tmdb.org/t/p/w92${r.poster_path}`}
                          className="h-14 w-10 rounded object-cover"
                          alt=""
                        />
                      ) : (
                        <div className="flex h-14 w-10 items-center justify-center rounded bg-muted">
                          <FileVideo className="h-4 w-4 text-muted-foreground" />
                        </div>
                      )}
                      <div className="min-w-0 flex-1">
                        <div className="truncate font-medium">{r.title}</div>
                        <div className="text-xs text-muted-foreground">
                          {r.year > 0 && r.year} · TMDB: {r.tmdb_id}
                        </div>
                      </div>
                    </button>
                  ))}
                  {!searching && searchResults.length === 0 && searchQuery && (
                    <div className="py-4 text-center text-sm text-muted-foreground">
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
