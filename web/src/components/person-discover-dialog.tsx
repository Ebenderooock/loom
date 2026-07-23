import { useEffect, useState, useCallback } from "react";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Checkbox } from "@/components/ui/checkbox";
import {
  User,
  Film,
  Tv,
  Star,
  Loader2,
  Plus,
  Check,
  Calendar,
  ArrowLeft,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { Library } from "@/lib/libraries-api";
import type { QualityProfile } from "@/components/movies/types";

// ─── Types ────────────────────────────────────────────────────────────

interface PersonDetail {
  id: number;
  name: string;
  biography?: string;
  profile_path?: string;
  birthday?: string;
  deathday?: string;
  known_for?: string;
}

interface CreditItem {
  tmdb_id: number;
  media_type: "movie" | "tv";
  title: string;
  year?: number;
  poster_path?: string;
  overview?: string;
  rating: number;
  popularity: number;
  credit_type: "cast" | "crew";
  character?: string;
  job?: string;
  department?: string;
  release_date?: string;
}

interface PersonFilmography {
  person: PersonDetail;
  credits: CreditItem[];
}

// ─── Component ────────────────────────────────────────────────────────

export function PersonDiscoverDialog({
  open,
  onOpenChange,
  personId,
  personName,
  libraries,
  qualityProfiles,
  existingMovieIds,
  existingSeriesIds,
  onAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  personId: number;
  personName: string;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingMovieIds: Set<number>;
  existingSeriesIds: Set<number>;
  onAdded?: () => void;
}) {
  const [data, setData] = useState<PersonFilmography | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [selectedItem, setSelectedItem] = useState<CreditItem | null>(null);
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState("");
  const [selectedLibrary, setSelectedLibrary] = useState(
    libraries[0]?.id ?? "",
  );
  const [selectedProfile, setSelectedProfile] = useState(
    qualityProfiles[0]?.id ?? "",
  );
  const [monitored, setMonitored] = useState(true);
  const [searchOnAdd, setSearchOnAdd] = useState(true);
  const [tab, setTab] = useState<"movies" | "tv">("movies");

  const fetchFilmography = useCallback(async () => {
    if (!personId) return;
    setLoading(true);
    setError("");
    try {
      const res = await apiFetch(`/api/v1/discover/people/${personId}`);
      if (!res.ok) throw new Error(await res.text());
      const json: PersonFilmography = await res.json();
      setData(json);
      // Auto-select tab based on known_for
      if (
        json.person.known_for === "Directing" ||
        json.person.known_for === "Acting"
      ) {
        const movieCount = json.credits.filter(
          (c) => c.media_type === "movie",
        ).length;
        const tvCount = json.credits.filter(
          (c) => c.media_type === "tv",
        ).length;
        setTab(movieCount >= tvCount ? "movies" : "tv");
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load filmography");
    } finally {
      setLoading(false);
    }
  }, [personId]);

  useEffect(() => {
    if (open && personId) {
      setData(null);
      setSelectedItem(null);
      setAddError("");
      fetchFilmography();
    }
  }, [open, personId, fetchFilmography]);

  // Guarantee a valid quality profile is always selected once profiles load.
  useEffect(() => {
    if (qualityProfiles.length === 0) return;
    setSelectedProfile((prev) => {
      if (prev && qualityProfiles.some((p) => p.id === prev)) return prev;
      return libraries[0]?.quality_profile_id || qualityProfiles[0]?.id || "";
    });
  }, [qualityProfiles, libraries]);

  const movieCredits =
    data?.credits.filter((c) => c.media_type === "movie") ?? [];
  const tvCredits = data?.credits.filter((c) => c.media_type === "tv") ?? [];

  const isInLibrary = (item: CreditItem) =>
    item.media_type === "movie"
      ? existingMovieIds.has(item.tmdb_id)
      : existingSeriesIds.has(item.tmdb_id);

  const handleAdd = async () => {
    if (!selectedItem || !selectedLibrary || !selectedProfile) return;
    setAdding(true);
    setAddError("");
    try {
      if (selectedItem.media_type === "movie") {
        const res = await apiFetch("/api/v1/movies", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            title: selectedItem.title,
            year: selectedItem.year ?? 0,
            tmdb_id: String(selectedItem.tmdb_id),
            overview: selectedItem.overview ?? "",
            poster_path: selectedItem.poster_path ?? "",
            release_date: selectedItem.release_date ?? "",
            rating: selectedItem.rating,
            metadata_provider: "tmdb",
            quality_profile_id: selectedProfile,
            library_id: selectedLibrary,
            monitoring_status: monitored ? "monitored" : "unmonitored",
            search: searchOnAdd,
          }),
        });
        if (!res.ok)
          throw new Error((await res.text()) || "Failed to add movie");
      } else {
        const res = await apiFetch("/api/v1/series", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            tmdbId: String(selectedItem.tmdb_id),
            qualityProfileId: selectedProfile,
            libraryId: selectedLibrary,
            monitoringStatus: monitored ? "monitored" : "unmonitored",
            search: searchOnAdd,
          }),
        });
        if (!res.ok)
          throw new Error((await res.text()) || "Failed to add series");
      }
      onAdded?.();
      setSelectedItem(null);
    } catch (e) {
      setAddError(e instanceof Error ? e.message : "Failed to add");
    } finally {
      setAdding(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] max-w-3xl flex-col gap-0 p-0">
        <DialogHeader className="border-b border-border/50 p-6 pb-4">
          <DialogTitle className="flex items-center gap-2 text-xl">
            <User className="h-5 w-5 text-accent" />
            {selectedItem ? (
              <span className="flex items-center gap-2">
                <button
                  onClick={() => {
                    setSelectedItem(null);
                    setAddError("");
                  }}
                  className="transition-colors hover:text-accent"
                >
                  <ArrowLeft className="h-5 w-5" />
                </button>
                Add {selectedItem.media_type === "movie" ? "Movie" : "Series"}
              </span>
            ) : (
              personName
            )}
          </DialogTitle>
        </DialogHeader>

        {selectedItem ? (
          /* ── Add-to-Library view ── */
          <div className="flex-1 space-y-4 overflow-y-auto p-6">
            <div className="flex gap-4">
              {selectedItem.poster_path && (
                <img
                  src={selectedItem.poster_path}
                  alt={selectedItem.title}
                  className="h-36 w-24 rounded-lg object-cover"
                />
              )}
              <div className="min-w-0 flex-1">
                <h3 className="text-lg font-semibold">{selectedItem.title}</h3>
                {selectedItem.year && (
                  <p className="flex items-center gap-1 text-sm text-muted-foreground">
                    <Calendar className="h-3.5 w-3.5" /> {selectedItem.year}
                  </p>
                )}
                {selectedItem.rating > 0 && (
                  <p className="mt-0.5 flex items-center gap-1 text-sm text-muted-foreground">
                    <Star className="h-3.5 w-3.5 text-yellow-500" />{" "}
                    {selectedItem.rating.toFixed(1)}
                  </p>
                )}
                {selectedItem.character && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    as {selectedItem.character}
                  </p>
                )}
                {selectedItem.overview && (
                  <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">
                    {selectedItem.overview}
                  </p>
                )}
              </div>
            </div>

            <div className="space-y-3">
              <div>
                <label
                  htmlFor="person-discover-library"
                  className="mb-1 block text-sm font-medium"
                >
                  Library
                </label>
                <Select
                  value={selectedLibrary}
                  onValueChange={setSelectedLibrary}
                >
                  <SelectTrigger id="person-discover-library">
                    <SelectValue placeholder="Select library" />
                  </SelectTrigger>
                  <SelectContent>
                    {libraries.map((l) => (
                      <SelectItem key={l.id} value={l.id}>
                        {l.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div>
                <label
                  htmlFor="person-discover-profile"
                  className="mb-1 block text-sm font-medium"
                >
                  Quality Profile
                </label>
                <Select
                  value={selectedProfile}
                  onValueChange={setSelectedProfile}
                >
                  <SelectTrigger id="person-discover-profile">
                    <SelectValue placeholder="Select profile" />
                  </SelectTrigger>
                  <SelectContent>
                    {qualityProfiles.map((p) => (
                      <SelectItem key={p.id} value={p.id}>
                        {p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex items-center gap-4">
                <label
                  htmlFor="person-discover-monitored"
                  className="flex cursor-pointer items-center gap-2 text-sm"
                >
                  <Checkbox
                    id="person-discover-monitored"
                    checked={monitored}
                    onCheckedChange={(c) => setMonitored(!!c)}
                  />
                  Monitored
                </label>
                <label
                  htmlFor="person-discover-search"
                  className="flex cursor-pointer items-center gap-2 text-sm"
                >
                  <Checkbox
                    id="person-discover-search"
                    checked={searchOnAdd}
                    onCheckedChange={(c) => setSearchOnAdd(!!c)}
                  />
                  Search on add
                </label>
              </div>
            </div>

            {addError && <p className="text-sm text-destructive">{addError}</p>}

            <Button
              onClick={handleAdd}
              disabled={adding || !selectedLibrary || !selectedProfile}
              className="w-full"
            >
              {adding ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Plus className="mr-2 h-4 w-4" />
              )}
              Add {selectedItem.media_type === "movie" ? "Movie" : "Series"}
            </Button>
          </div>
        ) : (
          /* ── Browse filmography ── */
          <div className="flex min-h-0 flex-1 flex-col">
            {/* Person bio header */}
            {data?.person && (
              <div className="flex gap-4 p-6 pb-3">
                {data.person.profile_path && (
                  <img
                    src={data.person.profile_path}
                    alt={data.person.name}
                    className="h-20 w-20 flex-shrink-0 rounded-full object-cover"
                  />
                )}
                <div className="min-w-0 flex-1">
                  {data.person.known_for && (
                    <Badge variant="secondary" className="mb-1">
                      {data.person.known_for}
                    </Badge>
                  )}
                  {data.person.biography && (
                    <p className="line-clamp-3 text-sm text-muted-foreground">
                      {data.person.biography}
                    </p>
                  )}
                </div>
              </div>
            )}

            {loading ? (
              <div className="flex items-center justify-center py-12">
                <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              </div>
            ) : error ? (
              <div className="flex flex-col items-center py-12 text-center">
                <p className="text-sm text-destructive">{error}</p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={fetchFilmography}
                  className="mt-2"
                >
                  Retry
                </Button>
              </div>
            ) : data ? (
              <Tabs
                value={tab}
                onValueChange={(v) => setTab(v as "movies" | "tv")}
                className="flex min-h-0 flex-1 flex-col"
              >
                <TabsList className="mx-6 mb-2 w-fit">
                  <TabsTrigger value="movies" className="gap-1.5">
                    <Film className="h-3.5 w-3.5" /> Movies (
                    {movieCredits.length})
                  </TabsTrigger>
                  <TabsTrigger value="tv" className="gap-1.5">
                    <Tv className="h-3.5 w-3.5" /> TV ({tvCredits.length})
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="movies" className="mt-0 flex-1">
                  <ScrollArea className="h-[50vh]">
                    <CreditGrid
                      items={movieCredits}
                      isInLibrary={isInLibrary}
                      onSelect={setSelectedItem}
                    />
                  </ScrollArea>
                </TabsContent>

                <TabsContent value="tv" className="mt-0 flex-1">
                  <ScrollArea className="h-[50vh]">
                    <CreditGrid
                      items={tvCredits}
                      isInLibrary={isInLibrary}
                      onSelect={setSelectedItem}
                    />
                  </ScrollArea>
                </TabsContent>
              </Tabs>
            ) : null}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ─── Credit Grid ──────────────────────────────────────────────────────

function CreditGrid({
  items,
  isInLibrary,
  onSelect,
}: {
  items: CreditItem[];
  isInLibrary: (item: CreditItem) => boolean;
  onSelect: (item: CreditItem) => void;
}) {
  if (items.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <Film className="mb-3 h-10 w-10 text-muted-foreground/20" />
        <p className="text-sm text-muted-foreground">No credits found</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 p-6 pt-2 sm:grid-cols-3 md:grid-cols-4">
      {items.map((item) => {
        const inLib = isInLibrary(item);
        return (
          <button
            key={`${item.tmdb_id}-${item.credit_type}-${item.character ?? item.job ?? ""}`}
            onClick={() => !inLib && onSelect(item)}
            disabled={inLib}
            className={cn(
              "group relative overflow-hidden rounded-lg text-left transition-all",
              "hover:ring-2 hover:ring-accent/50 focus-visible:ring-2 focus-visible:ring-accent",
              inLib && "cursor-default opacity-60",
            )}
          >
            <div className="relative aspect-[2/3] bg-muted/30">
              {item.poster_path ? (
                <img
                  src={item.poster_path}
                  alt={item.title}
                  className="h-full w-full object-cover"
                  loading="lazy"
                />
              ) : (
                <div className="flex h-full w-full items-center justify-center text-muted-foreground/30">
                  <Film className="h-8 w-8" />
                </div>
              )}
              {inLib && (
                <div className="absolute inset-0 flex items-center justify-center bg-black/40">
                  <Badge variant="secondary" className="gap-1">
                    <Check className="h-3 w-3" /> In Library
                  </Badge>
                </div>
              )}
              {!inLib && (
                <div className="absolute inset-0 flex items-center justify-center bg-black/0 opacity-0 transition-colors group-hover:bg-black/30 group-hover:opacity-100">
                  <Plus className="h-8 w-8 text-white drop-shadow-lg" />
                </div>
              )}
              {item.rating > 0 && (
                <div className="absolute right-1.5 top-1.5 flex items-center gap-0.5 rounded bg-black/60 px-1.5 py-0.5 text-[10px] font-medium text-white">
                  <Star className="h-2.5 w-2.5 text-yellow-400" />
                  {item.rating.toFixed(1)}
                </div>
              )}
            </div>
            <div className="p-2">
              <p className="truncate text-xs font-medium" title={item.title}>
                {item.title}
              </p>
              <p className="truncate text-[11px] text-muted-foreground">
                {item.year ?? "TBA"}
                {item.character && ` · ${item.character}`}
                {item.job && ` · ${item.job}`}
              </p>
            </div>
          </button>
        );
      })}
    </div>
  );
}
