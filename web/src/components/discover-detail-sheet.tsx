import { useQuery } from "@tanstack/react-query";
import type { DiscoverItem, MediaType } from "@/lib/import-lists-api";
import type { MovieMetadata, SeriesMetadata } from "@/lib/metadata-api";
import { apiFetch } from "@/lib/fetch";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { Check, ExternalLink, Loader2, Plus, Star } from "lucide-react";

type DiscoverMetadata = MovieMetadata &
  Pick<SeriesMetadata, "first_air_date" | "seasons">;

export function DiscoverDetailSheet({
  item,
  mediaType,
  inLibrary,
  adding,
  addError,
  onAdd,
  onClose,
  onRestoreFocus,
}: {
  item: DiscoverItem;
  mediaType: MediaType;
  inLibrary: boolean;
  adding: boolean;
  addError?: string;
  onAdd: () => void;
  onClose: () => void;
  onRestoreFocus: () => void;
}) {
  const details = useQuery({
    queryKey: ["discover-details", mediaType, item.tmdb_id],
    queryFn: async ({ signal }): Promise<DiscoverMetadata> => {
      const res = await apiFetch(
        `/api/v1/discover/${mediaType}/${encodeURIComponent(item.tmdb_id!)}`,
        { signal },
      );
      if (!res.ok)
        throw new Error(`Could not load full details (${res.status}).`);
      return res.json();
    },
    enabled: !!item.tmdb_id && item.tmdb_id !== "0",
    staleTime: 5 * 60_000,
  });
  const data = details.data;
  const title = data?.title ?? item.title;
  const poster = data?.poster_path || item.poster_path;
  const overview = data?.overview || item.overview;
  const genres = data?.genres ?? item.genres ?? [];
  const releaseDate = data?.release_date || data?.first_air_date;
  const year = data?.year || item.year || releaseDate?.slice(0, 4);

  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent
        className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          onRestoreFocus();
        }}
      >
        <div className="flex-1 space-y-6 overflow-y-auto p-6">
          <SheetHeader className="pr-6 text-left">
            <SheetTitle className="text-2xl">{title}</SheetTitle>
            <SheetDescription>
              {mediaType === "series" ? "TV Show" : "Movie"}
              {year ? ` · ${year}` : ""} · From {item.list_name}
            </SheetDescription>
          </SheetHeader>

          <div className="flex items-start gap-5">
            {poster && (
              <img
                src={
                  poster.startsWith("/")
                    ? `https://image.tmdb.org/t/p/w342${poster}`
                    : poster
                }
                alt={`${title} poster`}
                className="aspect-[2/3] w-28 shrink-0 rounded-lg object-cover sm:w-44"
              />
            )}
            <div className="min-w-0 space-y-4">
              <Badge variant="secondary">
                {inLibrary ? "In Library" : "Not in library"}
              </Badge>
              {data?.rating ? (
                <p className="flex items-center gap-1.5 text-sm">
                  <Star aria-hidden="true" className="h-4 w-4 text-amber-500" />
                  {data.rating.toFixed(1)} / 10
                </p>
              ) : null}
              <dl className="space-y-3 text-sm">
                {releaseDate && (
                  <div>
                    <dt className="text-muted-foreground">
                      {mediaType === "series" ? "First aired" : "Release date"}
                    </dt>
                    <dd>{releaseDate}</dd>
                  </div>
                )}
                {data?.runtime ? (
                  <div>
                    <dt className="text-muted-foreground">Runtime</dt>
                    <dd>{data.runtime} min</dd>
                  </div>
                ) : null}
                {data?.seasons ? (
                  <div>
                    <dt className="text-muted-foreground">Seasons</dt>
                    <dd>{data.seasons}</dd>
                  </div>
                ) : null}
              </dl>
            </div>
          </div>
          {details.isFetching && (
            <p
              role="status"
              className="flex items-center gap-2 text-sm text-muted-foreground"
            >
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading full details…
            </p>
          )}
          {details.error && (
            <div role="alert" className="space-y-2">
              <p className="text-sm text-destructive">
                {details.error.message}
              </p>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void details.refetch()}
              >
                Retry
              </Button>
            </div>
          )}
          {genres.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {genres.map((genre) => (
                <Badge key={genre} variant="outline">
                  {genre}
                </Badge>
              ))}
            </div>
          )}
          <section className="space-y-2">
            <h3 className="text-sm font-semibold">Overview</h3>
            <p className="whitespace-pre-line text-sm leading-relaxed text-muted-foreground">
              {overview || "No overview available."}
            </p>
          </section>
          <div className="flex flex-wrap gap-4 text-sm">
            {item.tmdb_id && item.tmdb_id !== "0" && (
              <a
                className="inline-flex items-center gap-1 text-foreground underline decoration-muted-foreground underline-offset-4 hover:decoration-foreground"
                href={`https://www.themoviedb.org/${mediaType === "series" ? "tv" : "movie"}/${encodeURIComponent(item.tmdb_id)}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                TMDB <ExternalLink className="h-3 w-3" />
              </a>
            )}
            {item.imdb_id && (
              <a
                className="inline-flex items-center gap-1 text-foreground underline decoration-muted-foreground underline-offset-4 hover:decoration-foreground"
                href={`https://www.imdb.com/title/${encodeURIComponent(item.imdb_id)}/`}
                target="_blank"
                rel="noopener noreferrer"
              >
                IMDb <ExternalLink className="h-3 w-3" />
              </a>
            )}
          </div>
        </div>
        <div className="space-y-3 border-t p-6">
          <p className="text-xs text-muted-foreground">
            Adds using this import list’s library, quality profile, monitoring,
            and search-on-add settings.
          </p>
          {addError && (
            <p role="alert" className="text-sm text-destructive">
              {addError}
            </p>
          )}
          <Button
            className="min-h-11 w-full"
            disabled={inLibrary || adding}
            onClick={onAdd}
          >
            {adding ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : inLibrary ? (
              <Check className="mr-2 h-4 w-4" />
            ) : (
              <Plus className="mr-2 h-4 w-4" />
            )}
            {adding ? "Adding…" : inLibrary ? "In Library" : "Add to library"}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
