import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { apiFetch } from "@/lib/fetch";
import { useLibraries } from "@/lib/libraries-api";
import { MovieDetailSheet } from "@/components/movies/movie-detail-sheet";
import { SeriesDetailSheet } from "@/components/series/series-detail-sheet";
import type { Movie, QualityProfile } from "@/components/movies/types";
import type { Series } from "@/components/series/types";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Loader2 } from "lucide-react";

export type LibraryMediaTarget = {
  type: "movie" | "series";
  id: string;
};

async function get<T>(path: string, signal: AbortSignal): Promise<T> {
  const res = await apiFetch(path, { signal });
  if (!res.ok) throw new Error(`Unable to load details (${res.status})`);
  const data: T | { data: T } = await res.json();
  return data && typeof data === "object" && "data" in data ? data.data : data;
}

export function LibraryMediaSheet({
  target,
  onClose,
  onChanged,
}: {
  target: LibraryMediaTarget;
  onClose: () => void;
  onChanged?: () => void;
}) {
  const qc = useQueryClient();
  const opener = useRef(
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null,
  );
  const closing = useRef(false);
  const close = () => {
    closing.current = true;
    onClose();
  };
  const onCloseAutoFocus = (event: Event) => {
    event.preventDefault();
    if (closing.current) opener.current?.focus();
  };
  const detailKey = ["library-media", target.type, target.id];
  const libraries = useLibraries();
  const profiles = useQuery({
    queryKey: ["quality-profiles", "media-sheet"],
    queryFn: ({ signal }) =>
      get<QualityProfile[]>("/api/v1/quality-profiles", signal),
  });
  const detail = useQuery({
    queryKey: detailKey,
    queryFn: async ({ signal }) =>
      target.type === "movie"
        ? {
            type: "movie" as const,
            item: await get<Movie>(
              `/api/v1/movies/${encodeURIComponent(target.id)}`,
              signal,
            ),
          }
        : {
            type: "series" as const,
            item: await get<Series>(
              `/api/v1/series/${encodeURIComponent(target.id)}`,
              signal,
            ),
          },
  });
  const onOpenChange = (open: boolean) => {
    if (!open) close();
  };
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: detailKey });
    void qc.invalidateQueries({ queryKey: ["calendar"] });
    onChanged?.();
  };
  const onDeleted = () => {
    void qc.invalidateQueries({ queryKey: ["calendar"] });
    void qc.invalidateQueries({ queryKey: ["title-membership"] });
    onChanged?.();
    close();
  };
  const error = detail.error ?? profiles.error ?? libraries.error;
  if (!detail.data || !profiles.data || !libraries.data || error) {
    return (
      <Sheet open onOpenChange={onOpenChange}>
        <SheetContent
          className="w-full sm:max-w-3xl"
          onCloseAutoFocus={onCloseAutoFocus}
        >
          <SheetHeader>
            <SheetTitle>Media details</SheetTitle>
            <SheetDescription>
              {error
                ? "Could not load media details."
                : "Loading media details…"}
            </SheetDescription>
          </SheetHeader>
          {error ? (
            <div className="mt-6 space-y-3">
              <p role="alert" className="text-sm text-destructive">
                {error.message}
              </p>
              <Button
                onClick={() => {
                  void detail.refetch();
                  void profiles.refetch();
                  void libraries.refetch();
                }}
              >
                Retry
              </Button>
            </div>
          ) : (
            <Loader2
              aria-label="Loading details"
              className="mt-6 h-6 w-6 animate-spin"
            />
          )}
        </SheetContent>
      </Sheet>
    );
  }
  const common = {
    open: true,
    onOpenChange,
    onCloseAutoFocus,
    profiles: profiles.data,
    libraries: libraries.data.filter((l) => l.media_type === target.type),
    onDeleted,
    onRefresh: refresh,
  };
  return detail.data.type === "movie" ? (
    <MovieDetailSheet
      {...common}
      movie={detail.data.item}
      onUpdated={(item) => {
        qc.setQueryData(["library-media", target.type, target.id], {
          type: "movie",
          item,
        });
        void qc.invalidateQueries({ queryKey: ["calendar"] });
        onChanged?.();
      }}
    />
  ) : (
    <SeriesDetailSheet
      {...common}
      series={detail.data.item}
      onUpdated={(item) => {
        qc.setQueryData(["library-media", target.type, target.id], {
          type: "series",
          item,
        });
        void qc.invalidateQueries({ queryKey: ["calendar"] });
        onChanged?.();
      }}
    />
  );
}
