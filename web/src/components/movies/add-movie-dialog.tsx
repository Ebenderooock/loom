import { useEffect, useState, useCallback, useRef } from "react";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  Plus,
  Search,
  Loader2,
  Film,
  Star,
  Check,
  Calendar,
  Clock,
  ArrowLeft,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useMediaPreferences } from "@/lib/media-info-api";
import type { Library } from "../../lib/libraries-api";
import type { QualityProfile, TMDBResult } from "./types";
import { TMDB_IMG } from "./types";

export function AddMovieDialog({
  open,
  onOpenChange,
  libraries,
  qualityProfiles,
  existingTmdbIds,
  onMovieAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingTmdbIds: Set<string>;
  onMovieAdded: () => void;
}) {
  const [searchTerm, setSearchTerm] = useState("");
  const [results, setResults] = useState<TMDBResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [selectedMovie, setSelectedMovie] = useState<TMDBResult | null>(null);
  const [selectedLibrary, setSelectedLibrary] = useState(
    libraries[0]?.id ?? "",
  );
  const [selectedProfile, setSelectedProfile] = useState(
    qualityProfiles[0]?.id ?? "",
  );
  const [monitored, setMonitored] = useState(true);
  const [searchOnAdd, setSearchOnAdd] = useState(true);
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const { data: mediaPrefs } = useMediaPreferences();

  useEffect(() => {
    if (open) {
      setSearchTerm("");
      setResults([]);
      setSelectedMovie(null);
      setAddError("");
      const firstLib = libraries[0];
      setSelectedLibrary(firstLib?.id ?? "");
      setSelectedProfile(
        firstLib?.quality_profile_id ||
          mediaPrefs?.default_quality_profile_id ||
          (qualityProfiles[0]?.id ?? ""),
      );
      setMonitored(true);
      setSearchOnAdd(true);
      setTimeout(() => searchInputRef.current?.focus(), 100);
    }
  }, [open, qualityProfiles, libraries, mediaPrefs]);

  // Guarantee a valid quality profile is always selected once profiles load,
  // independent of dialog open state. Preserves any still-valid selection.
  useEffect(() => {
    if (qualityProfiles.length === 0) return;
    setSelectedProfile((prev) => {
      if (prev && qualityProfiles.some((p) => p.id === prev)) return prev;
      return (
        libraries[0]?.quality_profile_id ||
        mediaPrefs?.default_quality_profile_id ||
        qualityProfiles[0]?.id ||
        ""
      );
    });
  }, [qualityProfiles, libraries, mediaPrefs]);

  const handleLibraryChange = useCallback(
    (libId: string) => {
      setSelectedLibrary(libId);
      const lib = libraries.find((l) => l.id === libId);
      if (lib?.quality_profile_id) {
        setSelectedProfile(lib.quality_profile_id);
      }
    },
    [libraries],
  );

  const doSearch = useCallback(async (term: string) => {
    if (term.length < 2) {
      setResults([]);
      return;
    }
    setSearching(true);
    try {
      const res = await apiFetch(
        `/api/v1/movies/lookup?term=${encodeURIComponent(term)}`,
      );
      if (res.ok) {
        const data = await res.json();
        setResults(data ?? []);
      }
    } catch {
      /* ignore */
    } finally {
      setSearching(false);
    }
  }, []);

  const handleSearchChange = (val: string) => {
    setSearchTerm(val);
    setSelectedMovie(null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => doSearch(val), 400);
  };

  const handleAdd = async () => {
    if (!selectedMovie || !selectedLibrary || !selectedProfile) return;
    setAdding(true);
    setAddError("");
    try {
      const res = await apiFetch("/api/v1/movies", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          title: selectedMovie.title,
          year: selectedMovie.year,
          tmdb_id: selectedMovie.tmdb_id ?? "",
          imdb_id: selectedMovie.imdb_id ?? null,
          overview: selectedMovie.overview,
          poster_path: selectedMovie.poster_path,
          backdrop_path: selectedMovie.backdrop_path ?? "",
          rating: selectedMovie.rating,
          runtime: selectedMovie.runtime ?? 0,
          genres: selectedMovie.genres ?? [],
          release_date: selectedMovie.release_date ?? "",
          metadata_provider: "tmdb",
          quality_profile_id: selectedProfile,
          library_id: selectedLibrary,
          monitoring_status: monitored ? "monitored" : "unmonitored",
          search: searchOnAdd,
        }),
      });
      if (!res.ok) {
        setAddError((await res.text()) || "Failed to add movie");
        return;
      }
      onMovieAdded();
      onOpenChange(false);
    } catch {
      setAddError("Network error adding movie");
    } finally {
      setAdding(false);
    }
  };

  const isAlreadyInLibrary = (r: TMDBResult) =>
    r.tmdb_id ? existingTmdbIds.has(r.tmdb_id) : false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] max-w-3xl flex-col gap-0 p-0">
        <DialogHeader className="border-b border-border/50 p-6 pb-4">
          <DialogTitle className="flex items-center gap-2 text-xl">
            <Film className="h-5 w-5 text-accent" />
            {selectedMovie ? "Add Movie" : "Search Movies"}
          </DialogTitle>
        </DialogHeader>

        {selectedMovie ? (
          <div className="flex-1 overflow-y-auto">
            <div className="relative h-48 overflow-hidden bg-muted">
              {selectedMovie.backdrop_path ? (
                <img
                  src={`${TMDB_IMG}/w780${selectedMovie.backdrop_path}`}
                  className="h-full w-full object-cover opacity-60"
                  alt=""
                />
              ) : selectedMovie.poster_path ? (
                <img
                  src={`${TMDB_IMG}/w780${selectedMovie.poster_path}`}
                  className="h-full w-full object-cover opacity-30 blur-sm"
                  alt=""
                />
              ) : null}
              <div className="absolute inset-0 bg-gradient-to-t from-background to-transparent" />
              <button
                onClick={() => setSelectedMovie(null)}
                className="absolute left-4 top-4 flex items-center gap-1 rounded-full bg-black/40 px-3 py-1.5 text-sm text-white/80 hover:text-white"
              >
                <ArrowLeft className="h-4 w-4" /> Back to results
              </button>
            </div>
            <div className="relative z-10 -mt-16 p-6">
              <div className="flex gap-5">
                <div className="w-32 shrink-0 overflow-hidden rounded-lg border-2 border-background shadow-xl">
                  {selectedMovie.poster_path ? (
                    <img
                      src={`${TMDB_IMG}/w300${selectedMovie.poster_path}`}
                      alt={selectedMovie.title}
                      className="aspect-[2/3] w-full object-cover"
                    />
                  ) : (
                    <div className="flex aspect-[2/3] w-full items-center justify-center bg-muted">
                      <Film className="h-8 w-8 text-muted-foreground/30" />
                    </div>
                  )}
                </div>
                <div className="min-w-0 flex-1 pt-12">
                  <h2 className="truncate text-2xl font-bold">
                    {selectedMovie.title}
                  </h2>
                  <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
                    {selectedMovie.year > 0 && (
                      <span>{selectedMovie.year}</span>
                    )}
                    {selectedMovie.runtime && selectedMovie.runtime > 0 && (
                      <span className="flex items-center gap-1">
                        <Clock className="h-3.5 w-3.5" />{" "}
                        {selectedMovie.runtime}m
                      </span>
                    )}
                    {selectedMovie.rating > 0 && (
                      <span className="flex items-center gap-1">
                        <Star className="h-3.5 w-3.5 fill-yellow-400 text-yellow-400" />{" "}
                        {selectedMovie.rating.toFixed(1)}
                      </span>
                    )}
                    {selectedMovie.release_date && (
                      <span className="flex items-center gap-1">
                        <Calendar className="h-3.5 w-3.5" />{" "}
                        {selectedMovie.release_date}
                      </span>
                    )}
                  </div>
                  {selectedMovie.genres && selectedMovie.genres.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {selectedMovie.genres.map((g) => (
                        <Badge
                          key={g}
                          variant="secondary"
                          className="text-[10px]"
                        >
                          {g}
                        </Badge>
                      ))}
                    </div>
                  )}
                  <p className="mt-3 line-clamp-3 text-sm leading-relaxed text-muted-foreground">
                    {selectedMovie.overview || "No overview available."}
                  </p>
                </div>
              </div>
              <div className="mt-6 grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label
                    htmlFor="add-movie-library"
                    className="text-sm font-medium"
                  >
                    Library
                  </label>
                  <Select
                    value={selectedLibrary}
                    onValueChange={handleLibraryChange}
                  >
                    <SelectTrigger id="add-movie-library">
                      <SelectValue placeholder="Select library" />
                    </SelectTrigger>
                    <SelectContent>
                      {libraries.map((lib) => (
                        <SelectItem key={lib.id} value={lib.id}>
                          {lib.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <label
                    htmlFor="add-movie-profile"
                    className="text-sm font-medium"
                  >
                    Quality Profile
                  </label>
                  <Select
                    value={selectedProfile}
                    onValueChange={setSelectedProfile}
                  >
                    <SelectTrigger id="add-movie-profile">
                      <SelectValue placeholder="Select quality profile" />
                    </SelectTrigger>
                    <SelectContent>
                      {qualityProfiles.map((qp) => (
                        <SelectItem key={qp.id} value={qp.id}>
                          {qp.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="mt-4 flex items-center gap-2">
                <Checkbox
                  id="monitored"
                  checked={monitored}
                  onCheckedChange={(v) => setMonitored(v === true)}
                />
                <label
                  htmlFor="monitored"
                  className="cursor-pointer text-sm font-medium"
                >
                  Monitor for downloads
                </label>
              </div>
              <div className="mt-2 flex items-center gap-2">
                <Checkbox
                  id="searchOnAdd"
                  checked={searchOnAdd}
                  onCheckedChange={(v) => setSearchOnAdd(v === true)}
                />
                <label
                  htmlFor="searchOnAdd"
                  className="cursor-pointer text-sm font-medium"
                >
                  Search after adding
                </label>
              </div>
              {addError && (
                <p className="mt-3 text-sm text-destructive">{addError}</p>
              )}
              <div className="mt-6 flex justify-end gap-3 border-t border-border/50 pt-4">
                <Button
                  variant="outline"
                  onClick={() => setSelectedMovie(null)}
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleAdd}
                  disabled={adding || !selectedLibrary || !selectedProfile}
                  className="min-w-[120px]"
                >
                  {adding ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <>
                      <Plus className="mr-1 h-4 w-4" /> Add Movie
                    </>
                  )}
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <>
            <div className="px-6 pt-4">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  ref={searchInputRef}
                  placeholder="Search for a movie to add..."
                  value={searchTerm}
                  onChange={(e) => handleSearchChange(e.target.value)}
                  className="h-11 pl-9"
                />
                {searching && (
                  <Loader2 className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
                )}
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-6 pb-6">
              {results.length === 0 && searchTerm.length >= 2 && !searching ? (
                <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                  <Film className="mb-3 h-12 w-12 opacity-30" />
                  <p className="text-sm">
                    No movies found for &ldquo;{searchTerm}&rdquo;
                  </p>
                </div>
              ) : results.length === 0 && searchTerm.length < 2 ? (
                <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                  <Search className="mb-3 h-12 w-12 opacity-30" />
                  <p className="text-sm">Start typing to search TMDB</p>
                </div>
              ) : (
                <div className="mt-3 space-y-2">
                  {results.map((r, i) => {
                    const inLibrary = isAlreadyInLibrary(r);
                    return (
                      <button
                        key={r.tmdb_id ?? i}
                        onClick={() => !inLibrary && setSelectedMovie(r)}
                        disabled={inLibrary}
                        className={cn(
                          "flex w-full items-start gap-4 rounded-lg border p-3 text-left transition-colors",
                          inLibrary
                            ? "cursor-not-allowed border-border/30 opacity-50"
                            : "cursor-pointer border-border/50 hover:border-accent/50 hover:bg-accent/5",
                        )}
                      >
                        <div className="aspect-[2/3] w-12 shrink-0 overflow-hidden rounded bg-muted">
                          {r.poster_path ? (
                            <img
                              src={`${TMDB_IMG}/w92${r.poster_path}`}
                              alt={r.title}
                              className="h-full w-full object-cover"
                            />
                          ) : (
                            <div className="flex h-full w-full items-center justify-center">
                              <Film className="h-4 w-4 text-muted-foreground/30" />
                            </div>
                          )}
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <h4 className="truncate text-sm font-semibold">
                              {r.title}
                            </h4>
                            {r.year > 0 && (
                              <span className="shrink-0 text-xs text-muted-foreground">
                                ({r.year})
                              </span>
                            )}
                          </div>
                          <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                            {r.overview}
                          </p>
                          <div className="mt-1.5 flex items-center gap-3">
                            {r.rating > 0 && (
                              <span className="flex items-center gap-0.5 text-xs text-yellow-500">
                                <Star className="h-3 w-3 fill-yellow-500" />{" "}
                                {r.rating.toFixed(1)}
                              </span>
                            )}
                            {r.genres && r.genres.length > 0 && (
                              <span className="text-[10px] text-muted-foreground">
                                {r.genres.slice(0, 3).join(" • ")}
                              </span>
                            )}
                          </div>
                        </div>
                        <div className="flex shrink-0 items-center">
                          {inLibrary ? (
                            <span className="flex items-center gap-1 text-xs text-green-500">
                              <Check className="h-3.5 w-3.5" /> In Library
                            </span>
                          ) : (
                            <span className="text-xs text-accent">Add →</span>
                          )}
                        </div>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
