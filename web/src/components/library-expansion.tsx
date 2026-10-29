import { useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ChevronRight,
  Film,
  Layers,
  Loader2,
  Plus,
  X,
} from "lucide-react";
import {
  discoveryKeys,
  useCollection,
  useCollections,
  useDismissRecommendation,
  useRecommendations,
  type CollectionItem,
  type DiscoveryMediaType,
  type DiscoveryStatus,
  type MovieCollection,
  type Recommendation,
} from "@/lib/discovery-api";
import { useLibraries, type Library } from "@/lib/libraries-api";
import {
  useQualityProfiles,
  type QualityProfile,
} from "@/lib/quality-profiles-api";
import { AddMovieDialog } from "@/components/movies/add-movie-dialog";
import { AddSeriesDialog } from "@/components/series/add-series-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { CardGridSkeleton } from "@/components/ui/skeletons";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

const EMPTY_IDS = new Set<string>();
const EMPTY_LIBRARIES: Library[] = [];
const EMPTY_PROFILES: QualityProfile[] = [];
const GRID =
  "grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6";

export function RecommendationPanel() {
  const [mediaType, setMediaType] = useState<DiscoveryMediaType>("movie");
  const [includeUpcoming, setIncludeUpcoming] = useState(false);
  const [selected, setSelected] = useState<Recommendation | null>(null);
  const [collectionID, setCollectionID] = useState<number | null>(null);
  const query = useRecommendations(mediaType, includeUpcoming);

  if (collectionID !== null) {
    return (
      <CollectionDetail
        id={collectionID}
        onBack={() => setCollectionID(null)}
      />
    );
  }

  return (
    <section className="space-y-5" aria-label="Grow your library">
      <div>
        <h2 className="text-xl font-semibold tracking-tight">
          What to add next
        </h2>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          New additions connected to your collection, cast, creators, and
          genres. Sourced from TMDB; no Import Lists needed.
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Tabs
          value={mediaType}
          onValueChange={(value) => {
            if (value === "movie" || value === "series") setMediaType(value);
          }}
        >
          <TabsList aria-label="Recommendation media type">
            <TabsTrigger value="movie">Movies</TabsTrigger>
            <TabsTrigger value="series">TV shows</TabsTrigger>
          </TabsList>
        </Tabs>
        <label
          htmlFor="discovery-upcoming"
          className="flex items-center gap-2 text-sm"
        >
          <Checkbox
            id="discovery-upcoming"
            checked={includeUpcoming}
            onCheckedChange={(checked) => setIncludeUpcoming(checked === true)}
          />
          Include upcoming titles
        </label>
      </div>
      {query.isLoading ? (
        <CardGridSkeleton count={6} />
      ) : query.isError ? (
        <LoadError error={query.error} onRetry={() => void query.refetch()} />
      ) : query.data ? (
        <>
          <SourceStatus status={query.data} />
          {query.data.items.length === 0 ? (
            <EmptyState
              title="No new suggestions yet"
              description="Add movies or shows with TMDB IDs to build your library's connections. Suggestions exclude titles already in Loom and titles you've dismissed."
            />
          ) : (
            <div className={GRID}>
              {query.data.items.map((item) => (
                <CatalogCard
                  key={`${item.media_type}:${item.tmdb_id}`}
                  item={item}
                  onAdd={() => setSelected(item)}
                  onCollection={
                    item.collection_id
                      ? () => setCollectionID(item.collection_id ?? null)
                      : undefined
                  }
                  dismissible
                />
              ))}
            </div>
          )}
        </>
      ) : null}
      {selected && (
        <CatalogAddDialog item={selected} onClose={() => setSelected(null)} />
      )}
      <Attribution />
    </section>
  );
}

export function CollectionsPanel() {
  const query = useCollections();
  const [selectedID, setSelectedID] = useState<number | null>(null);

  if (selectedID !== null) {
    return (
      <CollectionDetail id={selectedID} onBack={() => setSelectedID(null)} />
    );
  }

  return (
    <section className="space-y-5" aria-label="Movie collections">
      <div>
        <h2 className="text-xl font-semibold tracking-tight">
          Complete the set
        </h2>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          Movie sets identified by TMDB from titles in your library. See which
          entries are missing and choose what to add.
        </p>
        <p className="mt-2 text-xs text-muted-foreground">
          These are provider-defined movie sets, not every title from a studio
          such as Disney or a wider universe such as the MCU.
        </p>
      </div>
      {query.isLoading ? (
        <CardGridSkeleton count={6} />
      ) : query.isError ? (
        <LoadError error={query.error} onRetry={() => void query.refetch()} />
      ) : query.data ? (
        <>
          <SourceStatus status={query.data} />
          {query.data.items.length === 0 ? (
            <EmptyState
              title="No movie sets found yet"
              description="Add a movie belonging to a TMDB collection, such as a Saw film. Its collection and missing entries will appear here."
            />
          ) : (
            <div className={GRID}>
              {query.data.items.map((collection) => (
                <CollectionCard
                  key={collection.id}
                  collection={collection}
                  onOpen={() => setSelectedID(collection.id)}
                />
              ))}
            </div>
          )}
        </>
      ) : null}
      <Attribution />
    </section>
  );
}

function CollectionCard({
  collection,
  onOpen,
}: {
  collection: MovieCollection;
  onOpen: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="overflow-hidden rounded-lg border bg-card text-left transition-colors hover:border-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      aria-label={`View ${collection.name}`}
    >
      <Poster path={collection.poster_path} title={collection.name} />
      <div className="space-y-3 p-3">
        <h3 className="line-clamp-2 text-sm font-semibold">
          {collection.name}
        </h3>
        <div className="space-y-1.5">
          <div
            role="progressbar"
            aria-label={`${collection.name} catalog coverage`}
            aria-valuenow={collection.in_library}
            aria-valuemin={0}
            aria-valuemax={collection.total || 1}
            className="h-1.5 overflow-hidden rounded-full bg-muted"
          >
            <div
              className="h-full bg-accent"
              style={{
                width: `${collection.total ? (collection.in_library / collection.total) * 100 : 0}%`,
              }}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {collection.in_library} of {collection.total} added to Loom
          </p>
        </div>
        <p className="flex items-center gap-1 text-xs font-medium text-accent">
          {collection.missing} missing
          {collection.upcoming > 0 && ` / ${collection.upcoming} upcoming`}
          <ChevronRight className="ml-auto h-4 w-4 shrink-0" />
        </p>
      </div>
    </button>
  );
}

function CollectionDetail({ id, onBack }: { id: number; onBack: () => void }) {
  const query = useCollection(id);
  const [selected, setSelected] = useState<Recommendation | null>(null);

  return (
    <section className="space-y-5" aria-label="Collection details">
      <Button variant="ghost" size="sm" onClick={onBack}>
        <ArrowLeft className="mr-2 h-4 w-4" /> Back
      </Button>
      {query.isLoading ? (
        <CardGridSkeleton count={6} />
      ) : query.isError ? (
        <LoadError error={query.error} onRetry={() => void query.refetch()} />
      ) : query.data ? (
        <>
          <div className="max-w-3xl space-y-2">
            <h2 className="text-2xl font-semibold tracking-tight">
              {query.data.collection.name}
            </h2>
            <p className="text-sm text-muted-foreground">
              {query.data.collection.overview}
            </p>
            <p className="text-sm font-medium">
              {query.data.collection.in_library} of{" "}
              {query.data.collection.total} added to Loom
            </p>
            <p className="text-xs text-muted-foreground">
              Ordered by release date. Added to Loom does not mean a file is
              available or visible on your media server.
            </p>
          </div>
          <SourceStatus status={query.data} />
          <div className={GRID}>
            {query.data.items.map((item) => (
              <CatalogCard
                key={item.tmdb_id}
                item={item}
                status={item}
                onAdd={() => setSelected(item)}
              />
            ))}
          </div>
        </>
      ) : null}
      {selected && (
        <CatalogAddDialog item={selected} onClose={() => setSelected(null)} />
      )}
      <Attribution />
    </section>
  );
}

function CatalogCard({
  item,
  status,
  onAdd,
  onCollection,
  dismissible = false,
}: {
  item: Recommendation;
  status?: CollectionItem;
  onAdd: () => void;
  onCollection?: () => void;
  dismissible?: boolean;
}) {
  const dismiss = useDismissRecommendation();
  const upcoming = item.release_date
    ? item.release_date.slice(0, 10) > new Date().toISOString().slice(0, 10)
    : false;

  return (
    <article className="flex flex-col overflow-hidden rounded-lg border bg-card">
      <div className="relative">
        <Poster path={item.poster_path} title={item.title} />
        {dismissible && (
          <Button
            variant="secondary"
            size="icon"
            className="absolute right-2 top-2 h-8 w-8 shadow"
            aria-label={`Dismiss ${item.title}`}
            disabled={dismiss.isPending}
            onClick={() =>
              dismiss.mutate({
                tmdb_id: item.tmdb_id,
                media_type: item.media_type,
              })
            }
          >
            {dismiss.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <X className="h-4 w-4" />
            )}
          </Button>
        )}
      </div>
      <div className="flex flex-1 flex-col gap-3 p-3">
        <div>
          <h3 className="line-clamp-2 text-sm font-semibold">{item.title}</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {item.year || "Date unknown"}
            {item.rating > 0 && ` / ${item.rating.toFixed(1)} TMDB`}
          </p>
        </div>
        {upcoming && (
          <Badge variant="outline" className="w-fit">
            Upcoming
          </Badge>
        )}
        {status?.in_library ? (
          <div className="text-xs">
            <Badge variant="secondary">Added to Loom</Badge>
            <p className="mt-2 text-muted-foreground">
              {status.has_file
                ? "Imported file recorded"
                : status.has_file === false
                  ? "No imported file recorded"
                  : "File availability not checked"}
              {status.monitored ? " / Monitored" : " / Unmonitored"}
            </p>
          </div>
        ) : null}
        {item.reasons.length > 0 && (
          <p className="text-xs leading-relaxed text-muted-foreground">
            {item.reasons.join(" · ")}
          </p>
        )}
        <div className="mt-auto space-y-2">
          {!status?.in_library && (
            <Button size="sm" className="w-full" onClick={onAdd}>
              <Plus className="mr-1 h-4 w-4" /> Add to library
            </Button>
          )}
          {onCollection && (
            <Button
              variant="outline"
              size="sm"
              className="w-full"
              onClick={onCollection}
            >
              <Layers className="mr-1 h-4 w-4" /> View collection
            </Button>
          )}
          {dismiss.isError && (
            <p role="alert" className="text-xs text-destructive">
              {dismiss.error.message}
            </p>
          )}
        </div>
      </div>
    </article>
  );
}

function CatalogAddDialog({
  item,
  onClose,
}: {
  item: Recommendation;
  onClose: () => void;
}) {
  const client = useQueryClient();
  const libraryQuery = useLibraries();
  const profileQuery = useQualityProfiles();
  const allLibraries = libraryQuery.data ?? EMPTY_LIBRARIES;
  const profiles = profileQuery.data ?? EMPTY_PROFILES;
  const libraries = useMemo(
    () =>
      allLibraries.filter((library) => library.media_type === item.media_type),
    [allLibraries, item.media_type],
  );
  const poster = (item.poster_path ?? "").replace(
    /^https:\/\/image\.tmdb\.org\/t\/p\/[^/]+/,
    "",
  );
  const movie = useMemo(
    () => ({
      tmdb_id: String(item.tmdb_id),
      title: item.title,
      year: item.year,
      overview: item.overview ?? "",
      poster_path: poster,
      rating: item.rating,
      release_date: item.release_date ?? "",
    }),
    [item, poster],
  );
  const series = useMemo(
    () => ({
      tmdbId: String(item.tmdb_id),
      title: item.title,
      year: item.year,
      overview: item.overview ?? "",
      posterPath: poster,
    }),
    [item, poster],
  );
  const onAdded = () => {
    void client.invalidateQueries({ queryKey: discoveryKeys.all });
    void client.invalidateQueries({ queryKey: ["libraries"] });
    onClose();
  };
  const error = libraryQuery.error ?? profileQuery.error;
  const loading = libraryQuery.isLoading || profileQuery.isLoading;

  if (loading || error || libraries.length === 0 || profiles.length === 0) {
    return (
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add {item.title}</DialogTitle>
          </DialogHeader>
          {loading ? (
            <p role="status" className="text-sm text-muted-foreground">
              Loading library settings...
            </p>
          ) : error ? (
            <div className="space-y-3">
              <p role="alert" className="text-sm text-destructive">
                {error.message}
              </p>
              <Button
                variant="outline"
                onClick={() => {
                  void libraryQuery.refetch();
                  void profileQuery.refetch();
                }}
              >
                Try again
              </Button>
            </div>
          ) : (
            <p role="alert" className="text-sm text-muted-foreground">
              {libraries.length === 0
                ? `Create a ${item.media_type === "movie" ? "movie" : "series"} library in Settings before adding this title.`
                : "Create a quality profile in Settings before adding this title."}
            </p>
          )}
        </DialogContent>
      </Dialog>
    );
  }

  return item.media_type === "movie" ? (
    <AddMovieDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      libraries={libraries}
      qualityProfiles={profiles}
      existingTmdbIds={EMPTY_IDS}
      initialMovie={movie}
      onMovieAdded={onAdded}
    />
  ) : (
    <AddSeriesDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      libraries={libraries}
      qualityProfiles={profiles}
      existingTmdbIds={EMPTY_IDS}
      initialSeries={series}
      onSeriesAdded={onAdded}
    />
  );
}

function SourceStatus({ status }: { status: DiscoveryStatus }) {
  if (!status.stale && status.warnings.length === 0) return null;
  return (
    <div
      role="status"
      className="space-y-1 rounded-lg border border-border bg-muted/40 p-3 text-sm"
    >
      {status.stale && (
        <p>
          Showing cached TMDB data
          {status.generated_at &&
            ` from ${new Date(status.generated_at).toLocaleString()}`}
          . Library status is checked separately.
        </p>
      )}
      {status.warnings.map((warning) => (
        <p key={warning}>{warning}</p>
      ))}
    </div>
  );
}

function Poster({ path, title }: { path?: string; title: string }) {
  return path ? (
    <img
      src={path}
      alt=""
      loading="lazy"
      className="aspect-[2/3] w-full bg-muted object-cover"
    />
  ) : (
    <div className="flex aspect-[2/3] items-center justify-center bg-muted p-4 text-center text-xs text-muted-foreground">
      <Film className="mr-2 h-5 w-5 shrink-0" aria-hidden="true" />
      {title}
    </div>
  );
}

function EmptyState({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="rounded-lg border border-dashed px-6 py-16 text-center">
      <p className="font-medium">{title}</p>
      <p className="mx-auto mt-2 max-w-lg text-sm text-muted-foreground">
        {description}
      </p>
    </div>
  );
}

function LoadError({ error, onRetry }: { error: Error; onRetry: () => void }) {
  return (
    <div role="alert" className="space-y-3 rounded-lg border p-4">
      <p className="text-sm text-destructive">{error.message}</p>
      <Button variant="outline" size="sm" onClick={onRetry}>
        Try again
      </Button>
    </div>
  );
}

function Attribution() {
  return (
    <p className="text-xs text-muted-foreground">
      Catalog and recommendations by{" "}
      <a
        href="https://www.themoviedb.org/"
        target="_blank"
        rel="noreferrer"
        className="underline underline-offset-2"
      >
        TMDB
      </a>
      . This product uses the TMDB API but is not endorsed or certified by TMDB.
      Nothing is added or downloaded until you choose.
    </p>
  );
}
