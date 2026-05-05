import { useState, useEffect, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Languages, Plus, X, Loader2, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

interface AltTitle {
  id: string;
  media_id: string;
  media_type: string;
  title: string;
  language: string;
  source: string;
  created_at: string;
}

interface AltTitlesSectionProps {
  mediaId: string;
  mediaType: "movie" | "series";
}

export function AltTitlesSection({ mediaId, mediaType }: AltTitlesSectionProps) {
  const [open, setOpen] = useState(false);
  const [titles, setTitles] = useState<AltTitle[]>([]);
  const [loading, setLoading] = useState(false);
  const [adding, setAdding] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const [newLanguage, setNewLanguage] = useState("");

  const fetchTitles = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(
        `/api/v1/alt-titles?media_id=${encodeURIComponent(mediaId)}&media_type=${encodeURIComponent(mediaType)}`,
        { credentials: "include" },
      );
      if (res.ok) {
        const data = await res.json();
        setTitles(Array.isArray(data) ? data : data.data ?? []);
      }
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  }, [mediaId, mediaType]);

  useEffect(() => {
    if (open && mediaId) fetchTitles();
  }, [open, mediaId, fetchTitles]);

  const handleAdd = async () => {
    if (!newTitle.trim()) return;
    setAdding(true);
    try {
      const res = await fetch("/api/v1/alt-titles", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          media_id: mediaId,
          media_type: mediaType,
          title: newTitle.trim(),
          language: newLanguage.trim() || undefined,
        }),
      });
      if (!res.ok) throw new Error("Failed to add alt title");
      toast.success("Alt title added");
      setNewTitle("");
      setNewLanguage("");
      fetchTitles();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to add alt title");
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      const res = await fetch(`/api/v1/alt-titles/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to remove alt title");
      setTitles((prev) => prev.filter((t) => t.id !== id));
      toast.success("Alt title removed");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to remove alt title");
    }
  };

  return (
    <div className="border-t border-border/40">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-2 w-full py-3 text-sm font-semibold text-muted-foreground hover:text-foreground transition-colors"
      >
        <Languages className="w-4 h-4" />
        Alt Titles
        <ChevronRight
          className={cn(
            "w-4 h-4 ml-auto transition-transform duration-200",
            open && "rotate-90",
          )}
        />
      </button>
      {open && (
        <div className="pb-4 space-y-3">
          {loading ? (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="w-4 h-4 animate-spin text-muted-foreground" />
            </div>
          ) : titles.length === 0 ? (
            <p className="text-sm text-muted-foreground">No alternate titles</p>
          ) : (
            <div className="space-y-1.5">
              {titles.map((t) => (
                <div
                  key={t.id}
                  className="flex items-center gap-2 text-sm group"
                >
                  <span className="flex-1 truncate">{t.title}</span>
                  {t.language && (
                    <Badge variant="secondary" className="text-[10px]">
                      {t.language}
                    </Badge>
                  )}
                  {t.source && (
                    <Badge variant="outline" className="text-[10px]">
                      {t.source}
                    </Badge>
                  )}
                  <button
                    onClick={() => handleDelete(t.id)}
                    className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-destructive"
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Add form */}
          <div className="flex items-end gap-2">
            <div className="flex-1 space-y-1">
              <Input
                placeholder="Title"
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                className="h-8 text-sm"
              />
            </div>
            <div className="w-20 space-y-1">
              <Input
                placeholder="Lang"
                value={newLanguage}
                onChange={(e) => setNewLanguage(e.target.value)}
                className="h-8 text-sm"
              />
            </div>
            <Button
              size="sm"
              variant="outline"
              className="h-8 gap-1"
              disabled={adding || !newTitle.trim()}
              onClick={handleAdd}
            >
              {adding ? (
                <Loader2 className="w-3 h-3 animate-spin" />
              ) : (
                <Plus className="w-3 h-3" />
              )}
              Add
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
