import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { loadLibraryTitles } from "@/lib/collections-api";
import {
  useDiscover,
  type DiscoverItem,
  type MediaType,
} from "@/lib/import-lists-api";
import { usePageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Compass } from "lucide-react";
import { TitleDialog } from "@/components/titles/title-experience";
import { TitlePosterCard } from "@/components/titles/title-poster-card";
import { CardGridSkeleton } from "@/components/ui/skeletons";
import { RecommendationPanel } from "@/components/library-expansion";

export function DiscoverPage() {
  const { setHeader } = usePageHeader();
  React.useEffect(() => setHeader({ title: "Discover" }), [setHeader]);

  return (
    <div className="space-y-6 p-6">
      <Tabs defaultValue="recommendations">
        <TabsList className="h-auto flex-wrap" aria-label="Discovery sections">
          <TabsTrigger value="recommendations">Grow your library</TabsTrigger>
          <TabsTrigger value="lists">Import Lists</TabsTrigger>
        </TabsList>
        <TabsContent value="recommendations" className="mt-6">
          <RecommendationPanel />
        </TabsContent>
        <TabsContent value="lists" className="mt-6">
          <Tabs defaultValue="movie">
            <TabsList aria-label="Import list media type">
              <TabsTrigger value="movie">Movies</TabsTrigger>
              <TabsTrigger value="series">TV shows</TabsTrigger>
            </TabsList>
            <TabsContent value="movie" className="mt-4">
              <DiscoverGrid mediaType="movie" />
            </TabsContent>
            <TabsContent value="series" className="mt-4">
              <DiscoverGrid mediaType="series" />
            </TabsContent>
          </Tabs>
        </TabsContent>
      </Tabs>
    </div>
  );
}

type SortKey = "title-asc" | "title-desc" | "year-desc" | "year-asc";

const ALL = "__all__";

function DiscoverGrid({ mediaType }: { mediaType: MediaType }) {
  const { data: items, isLoading, isError, error } = useDiscover(mediaType);

  const [listFilter, setListFilter] = React.useState(ALL);
  const [genreFilter, setGenreFilter] = React.useState(ALL);
  const [yearFilter, setYearFilter] = React.useState(ALL);
  const [sort, setSort] = React.useState<SortKey>("title-asc");

  // Derive available filter options from the loaded items.
  const { lists, genres, years } = React.useMemo(() => {
    const listSet = new Set<string>();
    const genreSet = new Set<string>();
    const yearSet = new Set<number>();
    for (const it of items ?? []) {
      if (it.list_name) listSet.add(it.list_name);
      for (const g of it.genres ?? []) genreSet.add(g);
      if (it.year) yearSet.add(it.year);
    }
    return {
      lists: [...listSet].sort((a, b) => a.localeCompare(b)),
      genres: [...genreSet].sort((a, b) => a.localeCompare(b)),
      years: [...yearSet].sort((a, b) => b - a),
    };
  }, [items]);

  const visible = React.useMemo(() => {
    let out = [...(items ?? [])];
    if (listFilter !== ALL)
      out = out.filter((it) => it.list_name === listFilter);
    if (genreFilter !== ALL)
      out = out.filter((it) => (it.genres ?? []).includes(genreFilter));
    if (yearFilter !== ALL)
      out = out.filter((it) => String(it.year ?? "") === yearFilter);

    out.sort((a, b) => {
      switch (sort) {
        case "title-desc":
          return b.title.localeCompare(a.title);
        case "year-desc":
          return (
            (b.year ?? 0) - (a.year ?? 0) || a.title.localeCompare(b.title)
          );
        case "year-asc":
          return (
            (a.year ?? 0) - (b.year ?? 0) || a.title.localeCompare(b.title)
          );
        case "title-asc":
        default:
          return a.title.localeCompare(b.title);
      }
    });
    return out;
  }, [items, listFilter, genreFilter, yearFilter, sort]);

  if (isLoading) {
    return <CardGridSkeleton count={12} />;
  }
  if (isError) {
    return (
      <p className="text-sm text-destructive">
        {error?.message ?? "Failed to load discover feed."}
      </p>
    );
  }
  if (!items || items.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
        <Compass className="h-10 w-10 text-muted-foreground" />
        <p className="text-sm font-medium">Nothing to discover yet</p>
        <p className="max-w-md text-xs text-muted-foreground">
          Set an import list to <span className="font-medium">Discover</span>{" "}
          mode in Import Lists, then sync it. Its{" "}
          {mediaType === "series" ? "shows" : "movies"} will appear here for you
          to add.
        </p>
      </div>
    );
  }

  const hasFilters =
    listFilter !== ALL || genreFilter !== ALL || yearFilter !== ALL;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        {lists.length > 1 && (
          <Select value={listFilter} onValueChange={setListFilter}>
            <SelectTrigger className="h-9 w-[160px] text-xs">
              <SelectValue placeholder="List" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL} className="text-xs">
                All lists
              </SelectItem>
              {lists.map((l) => (
                <SelectItem key={l} value={l} className="text-xs">
                  {l}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        {genres.length > 0 && (
          <Select value={genreFilter} onValueChange={setGenreFilter}>
            <SelectTrigger className="h-9 w-[150px] text-xs">
              <SelectValue placeholder="Genre" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL} className="text-xs">
                All genres
              </SelectItem>
              {genres.map((g) => (
                <SelectItem key={g} value={g} className="text-xs">
                  {g}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        {years.length > 0 && (
          <Select value={yearFilter} onValueChange={setYearFilter}>
            <SelectTrigger className="h-9 w-[120px] text-xs">
              <SelectValue placeholder="Year" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL} className="text-xs">
                All years
              </SelectItem>
              {years.map((y) => (
                <SelectItem key={y} value={String(y)} className="text-xs">
                  {y}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        <Select value={sort} onValueChange={(v) => setSort(v as SortKey)}>
          <SelectTrigger className="h-9 w-[150px] text-xs">
            <SelectValue placeholder="Sort" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="title-asc" className="text-xs">
              Title (A-Z)
            </SelectItem>
            <SelectItem value="title-desc" className="text-xs">
              Title (Z-A)
            </SelectItem>
            <SelectItem value="year-desc" className="text-xs">
              Year (newest)
            </SelectItem>
            <SelectItem value="year-asc" className="text-xs">
              Year (oldest)
            </SelectItem>
          </SelectContent>
        </Select>

        {hasFilters && (
          <Button
            variant="ghost"
            size="sm"
            className="h-9 text-xs"
            onClick={() => {
              setListFilter(ALL);
              setGenreFilter(ALL);
              setYearFilter(ALL);
            }}
          >
            Clear filters
          </Button>
        )}

        <span className="ml-auto text-xs text-muted-foreground">
          {visible.length} of {items.length}
        </span>
      </div>

      {visible.length === 0 ? (
        <p className="py-12 text-center text-sm text-muted-foreground">
          No items match the selected filters.
        </p>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {visible.map((item) => (
            <DiscoverCard key={item.id} item={item} mediaType={mediaType} />
          ))}
        </div>
      )}
    </div>
  );
}

function DiscoverCard({
  item,
  mediaType,
}: {
  item: DiscoverItem;
  mediaType: MediaType;
}) {
  const [view, setView] = React.useState<"details" | "add" | null>(null);
  const [added, setAdded] = React.useState(false);

  const inLibrary = item.in_library || added;
  const provider =
    item.tmdb_id && item.tmdb_id !== "0"
      ? "tmdb"
      : item.imdb_id
        ? "imdb"
        : "tvdb";
  const providerId =
    provider === "tmdb"
      ? item.tmdb_id
      : provider === "imdb"
        ? item.imdb_id
        : item.tvdb_id;
  const libraryMedia = mediaType === "series" ? "series" : "movie";
  const library = useQuery({
    queryKey: ["collection-picker-library", mediaType],
    queryFn: ({ signal }) => loadLibraryTitles(libraryMedia, signal),
    enabled: inLibrary && mediaType !== "music",
    refetchInterval: 30_000,
  });
  const local = providerId
    ? library.data?.find((row) => row[provider] === providerId)
    : undefined;

  return (
    <div className="min-w-0">
      <TitlePosterCard
        title={item.title}
        poster={item.poster_path}
        year={item.year}
        inLibrary={inLibrary}
        mediaType={libraryMedia}
        status={local?.status}
        episodeStats={local?.episodeStats}
        monitoringStatus={local?.monitoringStatus}
        onOpen={() => setView("details")}
        onAdd={() => setView("add")}
        disabled={!providerId}
      />
      {inLibrary && library.error && (
        <button
          type="button"
          className="text-xs text-destructive"
          onClick={() => void library.refetch()}
        >
          Unable to load library status. Retry
        </button>
      )}
      {!providerId && (
        <p className="text-xs text-muted-foreground">
          No provider identity is available for this title.
        </p>
      )}
      {view && providerId && (
        <TitleDialog
          open
          onOpenChange={(open) => {
            if (!open) setView(null);
          }}
          identity={{
            media: mediaType === "series" ? "series" : "movie",
            id: providerId,
            provider,
            title: item.title,
            inLibrary,
          }}
          initialView={view}
          defaults={item.defaults}
          sourceItemId={item.id}
          sourceName={item.list_name}
          onAdded={() => {
            setAdded(true);
            setView(null);
          }}
        />
      )}
    </div>
  );
}
