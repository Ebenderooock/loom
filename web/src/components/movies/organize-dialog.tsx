import { useState, useEffect, useCallback } from "react";
import { apiFetch } from "@/lib/fetch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  FolderSync,
  Check,
  X,
  AlertTriangle,
  ArrowRight,
  Loader2,
} from "lucide-react";
import type { Movie } from "./types";

interface RenamePreview {
  file_id: string;
  movie_id: string;
  movie_title: string;
  current_path: string;
  new_path: string;
  changed: boolean;
  collision?: boolean;
  error?: string;
}

interface RenameResult {
  file_id: string;
  movie_id: string;
  old_path: string;
  new_path: string;
  success: boolean;
  error?: string;
}

interface OrganizeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  movies: Movie[];
  onComplete?: () => void;
}

export function OrganizeDialog({
  open,
  onOpenChange,
  movies,
  onComplete,
}: OrganizeDialogProps) {
  const [step, setStep] = useState<"preview" | "executing" | "done">("preview");
  const [previews, setPreviews] = useState<RenamePreview[]>([]);
  const [results, setResults] = useState<RenameResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const movieIds = movies.map((m) => m.id);

  const fetchPreviews = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiFetch("/api/v1/movies/organize/preview", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ movie_ids: movieIds }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data: RenamePreview[] = await res.json();
      setPreviews(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to preview");
    } finally {
      setLoading(false);
    }
  }, [movieIds.join(",")]);

  useEffect(() => {
    if (open && movies.length > 0) {
      setStep("preview");
      setResults([]);
      fetchPreviews();
    }
  }, [open, fetchPreviews, movies.length]);

  const executeRenames = async () => {
    setStep("executing");
    setError(null);
    try {
      const res = await apiFetch("/api/v1/movies/organize/rename", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ movie_ids: movieIds }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data: RenameResult[] = await res.json();
      setResults(data);
      setStep("done");
      onComplete?.();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to rename");
      setStep("preview");
    }
  };

  const changedPreviews = previews.filter((p) => p.changed);
  const unchangedCount = previews.length - changedPreviews.length;
  const collisionCount = changedPreviews.filter((p) => p.collision).length;

  const successCount = results.filter((r) => r.success).length;
  const failCount = results.filter((r) => !r.success).length;

  const shortenPath = (path: string) => {
    const parts = path.split("/");
    if (parts.length > 4) {
      return ".../" + parts.slice(-3).join("/");
    }
    return path;
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[80vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FolderSync className="h-5 w-5 text-teal-400" />
            Organize Files
          </DialogTitle>
          <DialogDescription>
            Rename and organize movie files according to your naming configuration.
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 overflow-y-auto space-y-4">
          {error && (
            <div className="flex items-center gap-2 p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-sm">
              <AlertTriangle className="h-4 w-4 flex-shrink-0" />
              {error}
            </div>
          )}

          {/* Preview Step */}
          {step === "preview" && (
            <>
              {loading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="h-6 w-6 animate-spin text-teal-400" />
                  <span className="ml-2 text-zinc-400">Computing renames...</span>
                </div>
              ) : (
                <>
                  {/* Summary */}
                  <div className="flex gap-3 text-sm">
                    <Badge variant="outline" className="border-teal-500/30 text-teal-400">
                      {changedPreviews.length} to rename
                    </Badge>
                    {unchangedCount > 0 && (
                      <Badge variant="outline" className="border-zinc-500/30 text-zinc-400">
                        {unchangedCount} already correct
                      </Badge>
                    )}
                    {collisionCount > 0 && (
                      <Badge variant="outline" className="border-amber-500/30 text-amber-400">
                        {collisionCount} collisions
                      </Badge>
                    )}
                  </div>

                  {/* File list */}
                  {changedPreviews.length === 0 ? (
                    <div className="text-center py-8 text-zinc-500">
                      <Check className="h-8 w-8 mx-auto mb-2 text-green-400" />
                      All files are already correctly named.
                    </div>
                  ) : (
                    <div className="space-y-2">
                      {changedPreviews.map((p) => (
                        <div
                          key={p.file_id}
                          className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800 space-y-1"
                        >
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-medium text-zinc-300">
                              {p.movie_title}
                            </span>
                            {p.collision && (
                              <Badge variant="outline" className="border-amber-500/30 text-amber-400 text-xs">
                                collision
                              </Badge>
                            )}
                            {p.error && (
                              <Badge variant="outline" className="border-red-500/30 text-red-400 text-xs">
                                error
                              </Badge>
                            )}
                          </div>
                          <div className="flex items-center gap-2 text-xs">
                            <span className="text-red-400/70 font-mono truncate" title={p.current_path}>
                              {shortenPath(p.current_path)}
                            </span>
                            <ArrowRight className="h-3 w-3 text-zinc-500 flex-shrink-0" />
                            <span className="text-green-400/70 font-mono truncate" title={p.new_path}>
                              {shortenPath(p.new_path)}
                            </span>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
            </>
          )}

          {/* Executing Step */}
          {step === "executing" && (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-6 w-6 animate-spin text-teal-400" />
              <span className="ml-2 text-zinc-400">Renaming files...</span>
            </div>
          )}

          {/* Done Step */}
          {step === "done" && (
            <>
              <div className="flex gap-3 text-sm">
                {successCount > 0 && (
                  <Badge variant="outline" className="border-green-500/30 text-green-400">
                    <Check className="h-3 w-3 mr-1" /> {successCount} renamed
                  </Badge>
                )}
                {failCount > 0 && (
                  <Badge variant="outline" className="border-red-500/30 text-red-400">
                    <X className="h-3 w-3 mr-1" /> {failCount} failed
                  </Badge>
                )}
              </div>

              <div className="space-y-2">
                {results.map((r) => (
                  <div
                    key={r.file_id}
                    className={`p-3 rounded-lg border space-y-1 ${
                      r.success
                        ? "bg-green-500/5 border-green-500/20"
                        : "bg-red-500/5 border-red-500/20"
                    }`}
                  >
                    <div className="flex items-center gap-2 text-xs">
                      {r.success ? (
                        <Check className="h-3 w-3 text-green-400" />
                      ) : (
                        <X className="h-3 w-3 text-red-400" />
                      )}
                      <span className="font-mono text-zinc-400 truncate" title={r.new_path}>
                        {shortenPath(r.new_path)}
                      </span>
                    </div>
                    {r.error && (
                      <p className="text-xs text-red-400 ml-5">{r.error}</p>
                    )}
                  </div>
                ))}
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 pt-4 border-t border-zinc-800">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="border-zinc-700"
          >
            {step === "done" ? "Close" : "Cancel"}
          </Button>
          {step === "preview" && changedPreviews.length > 0 && (
            <Button
              onClick={executeRenames}
              disabled={loading}
              className="bg-teal-600 hover:bg-teal-700"
            >
              <FolderSync className="h-4 w-4 mr-2" />
              Rename {changedPreviews.length} Files
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
