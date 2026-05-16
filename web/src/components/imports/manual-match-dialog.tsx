import * as React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Film, Tv, Loader2, Search, Check } from "lucide-react";
import {
  useManualMatch,
  searchLocalMovies,
  searchLocalSeries,
  fetchEpisodes,
} from "@/lib/imports-api";
import type {
  ScanResult,
  LocalMovie,
  LocalSeries,
  SeriesEpisode,
} from "@/lib/imports-api";

interface ManualMatchDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scanResult: ScanResult | null;
  onMatched: () => void;
}

export function ManualMatchDialog({
  open,
  onOpenChange,
  scanResult,
  onMatched,
}: ManualMatchDialogProps) {
  const isSeries = !!(scanResult?.detected_season || scanResult?.detected_episode);
  const [mediaType, setMediaType] = React.useState<"movie" | "series">(
    isSeries ? "series" : "movie",
  );
  const [query, setQuery] = React.useState("");
  const [searching, setSearching] = React.useState(false);

  // Movie state
  const [movies, setMovies] = React.useState<LocalMovie[]>([]);
  const [selectedMovie, setSelectedMovie] = React.useState<LocalMovie | null>(null);

  // Series state
  const [seriesList, setSeriesList] = React.useState<LocalSeries[]>([]);
  const [selectedSeries, setSelectedSeries] = React.useState<LocalSeries | null>(null);
  const [episodes, setEpisodes] = React.useState<SeriesEpisode[]>([]);
  const [selectedEpisode, setSelectedEpisode] = React.useState<SeriesEpisode | null>(null);
  const [seasonNum, setSeasonNum] = React.useState<number>(1);
  const [loadingEpisodes, setLoadingEpisodes] = React.useState(false);

  const [error, setError] = React.useState("");
  const matchMut = useManualMatch();

  // Reset state when dialog opens with a new scan result
  React.useEffect(() => {
    if (open && scanResult) {
      const detected = scanResult.detected_title || "";
      const series = !!(scanResult.detected_season || scanResult.detected_episode);
      setMediaType(series ? "series" : "movie");
      setQuery(detected);
      setMovies([]);
      setSelectedMovie(null);
      setSeriesList([]);
      setSelectedSeries(null);
      setEpisodes([]);
      setSelectedEpisode(null);
      setSeasonNum(scanResult.detected_season || 1);
      setError("");
    }
  }, [open, scanResult]);

  const handleSearch = async () => {
    if (query.length < 2) return;
    setSearching(true);
    setError("");
    try {
      if (mediaType === "movie") {
        const results = await searchLocalMovies(query);
        setMovies(results);
        setSelectedMovie(null);
      } else {
        const results = await searchLocalSeries(query);
        setSeriesList(results);
        setSelectedSeries(null);
        setEpisodes([]);
        setSelectedEpisode(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Search failed");
    } finally {
      setSearching(false);
    }
  };

  // Load episodes when series + season is selected
  const handleSelectSeries = async (series: LocalSeries) => {
    setSelectedSeries(series);
    setSelectedEpisode(null);
    setLoadingEpisodes(true);
    try {
      const eps = await fetchEpisodes(series.id, seasonNum);
      setEpisodes(eps);
      // Auto-select if detected episode matches
      if (scanResult?.detected_episode) {
        const match = eps.find(
          (e) => e.episodeNumber === scanResult.detected_episode,
        );
        if (match) setSelectedEpisode(match);
      }
    } catch {
      setEpisodes([]);
    } finally {
      setLoadingEpisodes(false);
    }
  };

  // Reload episodes when season changes
  const handleSeasonChange = async (num: number) => {
    setSeasonNum(num);
    if (!selectedSeries) return;
    setSelectedEpisode(null);
    setLoadingEpisodes(true);
    try {
      const eps = await fetchEpisodes(selectedSeries.id, num);
      setEpisodes(eps);
    } catch {
      setEpisodes([]);
    } finally {
      setLoadingEpisodes(false);
    }
  };

  const handleConfirm = () => {
    if (!scanResult) return;
    setError("");

    let matchMediaType: string;
    let matchMediaId: string;

    if (mediaType === "movie") {
      if (!selectedMovie) return;
      matchMediaType = "movie";
      matchMediaId = selectedMovie.id;
    } else {
      if (!selectedEpisode) return;
      matchMediaType = "episode";
      matchMediaId = selectedEpisode.id;
    }

    matchMut.mutate(
      { path: scanResult.file_path, media_type: matchMediaType, media_id: matchMediaId },
      {
        onSuccess: () => {
          onMatched();
          onOpenChange(false);
        },
        onError: (err) => {
          setError(err instanceof Error ? err.message : "Match failed");
        },
      },
    );
  };

  const canConfirm =
    mediaType === "movie" ? !!selectedMovie : !!selectedEpisode;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Manual Match</DialogTitle>
          <DialogDescription>
            Search your library and select the correct match for{" "}
            <span className="font-mono text-xs">
              {scanResult?.file_path.split("/").pop()}
            </span>
          </DialogDescription>
        </DialogHeader>

        <Tabs
          value={mediaType}
          onValueChange={(v) => {
            setMediaType(v as "movie" | "series");
            setMovies([]);
            setSeriesList([]);
            setSelectedMovie(null);
            setSelectedSeries(null);
            setEpisodes([]);
            setSelectedEpisode(null);
          }}
        >
          <TabsList className="w-full">
            <TabsTrigger value="movie" className="flex-1">
              <Film className="mr-2 h-4 w-4" />
              Movie
            </TabsTrigger>
            <TabsTrigger value="series" className="flex-1">
              <Tv className="mr-2 h-4 w-4" />
              Series
            </TabsTrigger>
          </TabsList>

          {/* Search bar */}
          <div className="mt-4 flex gap-2">
            <Input
              placeholder="Search your library..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void handleSearch();
              }}
              className="flex-1"
            />
            <Button
              onClick={() => void handleSearch()}
              disabled={searching || query.length < 2}
              size="sm"
            >
              {searching ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Search className="h-4 w-4" />
              )}
            </Button>
          </div>

          {/* Movie results */}
          <TabsContent value="movie" className="mt-2">
            <div className="max-h-[300px] space-y-1 overflow-y-auto">
              {movies.length === 0 && !searching && (
                <p className="py-4 text-center text-sm text-muted-foreground">
                  Search for a movie in your library
                </p>
              )}
              {movies.map((m) => (
                <button
                  key={m.id}
                  type="button"
                  onClick={() => setSelectedMovie(m)}
                  className={`flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-colors ${
                    selectedMovie?.id === m.id
                      ? "bg-primary/10 ring-1 ring-primary"
                      : "hover:bg-muted"
                  }`}
                >
                  <div>
                    <span className="font-medium">{m.title}</span>
                    {m.year ? (
                      <span className="ml-2 text-muted-foreground">
                        ({m.year})
                      </span>
                    ) : null}
                  </div>
                  {selectedMovie?.id === m.id && (
                    <Check className="h-4 w-4 text-primary" />
                  )}
                </button>
              ))}
            </div>
          </TabsContent>

          {/* Series results */}
          <TabsContent value="series" className="mt-2 space-y-3">
            {/* Series list */}
            {!selectedSeries && (
              <div className="max-h-[200px] space-y-1 overflow-y-auto">
                {seriesList.length === 0 && !searching && (
                  <p className="py-4 text-center text-sm text-muted-foreground">
                    Search for a series in your library
                  </p>
                )}
                {seriesList.map((s) => (
                  <button
                    key={s.id}
                    type="button"
                    onClick={() => void handleSelectSeries(s)}
                    className="flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-muted"
                  >
                    <div>
                      <span className="font-medium">{s.title}</span>
                      {s.year ? (
                        <span className="ml-2 text-muted-foreground">
                          ({s.year})
                        </span>
                      ) : null}
                    </div>
                  </button>
                ))}
              </div>
            )}

            {/* Selected series → pick episode */}
            {selectedSeries && (
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">{selectedSeries.title}</Badge>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setSelectedSeries(null);
                        setEpisodes([]);
                        setSelectedEpisode(null);
                      }}
                    >
                      Change
                    </Button>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-muted-foreground">Season</span>
                    <Select
                      value={String(seasonNum)}
                      onValueChange={(v) => void handleSeasonChange(Number(v))}
                    >
                      <SelectTrigger className="w-[80px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {Array.from({ length: 30 }, (_, i) => i + 1).map((n) => (
                          <SelectItem key={n} value={String(n)}>
                            {n}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {loadingEpisodes ? (
                  <div className="flex justify-center py-4">
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                  </div>
                ) : (
                  <div className="max-h-[200px] space-y-1 overflow-y-auto">
                    {episodes.length === 0 ? (
                      <p className="py-4 text-center text-sm text-muted-foreground">
                        No episodes found for this season
                      </p>
                    ) : (
                      episodes.map((ep) => (
                        <button
                          key={ep.id}
                          type="button"
                          onClick={() => setSelectedEpisode(ep)}
                          className={`flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm transition-colors ${
                            selectedEpisode?.id === ep.id
                              ? "bg-primary/10 ring-1 ring-primary"
                              : "hover:bg-muted"
                          }`}
                        >
                          <div>
                            <span className="font-medium text-muted-foreground">
                              E{String(ep.episodeNumber).padStart(2, "0")}
                            </span>
                            <span className="ml-2">{ep.title}</span>
                          </div>
                          {selectedEpisode?.id === ep.id && (
                            <Check className="h-4 w-4 text-primary" />
                          )}
                        </button>
                      ))
                    )}
                  </div>
                )}
              </div>
            )}
          </TabsContent>
        </Tabs>

        {error && (
          <p className="text-sm text-destructive">{error}</p>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={!canConfirm || matchMut.isPending}
          >
            {matchMut.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : null}
            Import Match
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
