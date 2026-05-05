import { useEffect, useState, useCallback, useRef } from "react";
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
  Plus, Search, Loader2, Film, Star, Check, Calendar, Clock, ArrowLeft,
} from "lucide-react";
import { cn } from "@/lib/utils";
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
  const [selectedLibrary, setSelectedLibrary] = useState(libraries[0]?.id ?? "");
  const [selectedProfile, setSelectedProfile] = useState(qualityProfiles[0]?.id ?? "");
  const [monitored, setMonitored] = useState(true);
  const [searchOnAdd, setSearchOnAdd] = useState(true);
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setSearchTerm("");
      setResults([]);
      setSelectedMovie(null);
      setAddError("");
      const firstLib = libraries[0];
      setSelectedLibrary(firstLib?.id ?? "");
      setSelectedProfile(firstLib?.quality_profile_id || (qualityProfiles[0]?.id ?? ""));
      setMonitored(true);
      setSearchOnAdd(true);
      setTimeout(() => searchInputRef.current?.focus(), 100);
    }
  }, [open, qualityProfiles, libraries]);

  const handleLibraryChange = useCallback((libId: string) => {
    setSelectedLibrary(libId);
    const lib = libraries.find(l => l.id === libId);
    if (lib?.quality_profile_id) {
      setSelectedProfile(lib.quality_profile_id);
    }
  }, [libraries]);

  const doSearch = useCallback(async (term: string) => {
    if (term.length < 2) { setResults([]); return; }
    setSearching(true);
    try {
      const res = await fetch(`/api/v1/movies/lookup?term=${encodeURIComponent(term)}`, { credentials: "include" });
      if (res.ok) { const data = await res.json(); setResults(data ?? []); }
    } catch { /* ignore */ } finally { setSearching(false); }
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
      const res = await fetch("/api/v1/movies", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
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
      if (!res.ok) { setAddError((await res.text()) || "Failed to add movie"); return; }
      onMovieAdded();
      onOpenChange(false);
    } catch { setAddError("Network error adding movie"); } finally { setAdding(false); }
  };

  const isAlreadyInLibrary = (r: TMDBResult) => r.tmdb_id ? existingTmdbIds.has(r.tmdb_id) : false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[85vh] flex flex-col p-0 gap-0">
        <DialogHeader className="p-6 pb-4 border-b border-border/50">
          <DialogTitle className="text-xl flex items-center gap-2">
            <Film className="w-5 h-5 text-accent" />
            {selectedMovie ? "Add Movie" : "Search Movies"}
          </DialogTitle>
        </DialogHeader>

        {selectedMovie ? (
          <div className="flex-1 overflow-y-auto">
            <div className="relative h-48 bg-muted overflow-hidden">
              {selectedMovie.backdrop_path ? (
                <img src={`${TMDB_IMG}/w780${selectedMovie.backdrop_path}`} className="w-full h-full object-cover opacity-60" alt="" />
              ) : selectedMovie.poster_path ? (
                <img src={`${TMDB_IMG}/w780${selectedMovie.poster_path}`} className="w-full h-full object-cover opacity-30 blur-sm" alt="" />
              ) : null}
              <div className="absolute inset-0 bg-gradient-to-t from-background to-transparent" />
              <button onClick={() => setSelectedMovie(null)} className="absolute top-4 left-4 flex items-center gap-1 text-sm text-white/80 hover:text-white bg-black/40 rounded-full px-3 py-1.5">
                <ArrowLeft className="w-4 h-4" /> Back to results
              </button>
            </div>
            <div className="p-6 -mt-16 relative z-10">
              <div className="flex gap-5">
                <div className="shrink-0 w-32 rounded-lg overflow-hidden shadow-xl border-2 border-background">
                  {selectedMovie.poster_path ? (
                    <img src={`${TMDB_IMG}/w300${selectedMovie.poster_path}`} alt={selectedMovie.title} className="w-full aspect-[2/3] object-cover" />
                  ) : (
                    <div className="w-full aspect-[2/3] bg-muted flex items-center justify-center"><Film className="w-8 h-8 text-muted-foreground/30" /></div>
                  )}
                </div>
                <div className="flex-1 min-w-0 pt-12">
                  <h2 className="text-2xl font-bold truncate">{selectedMovie.title}</h2>
                  <div className="flex items-center gap-3 mt-1 text-sm text-muted-foreground">
                    {selectedMovie.year > 0 && <span>{selectedMovie.year}</span>}
                    {selectedMovie.runtime && selectedMovie.runtime > 0 && (
                      <span className="flex items-center gap-1"><Clock className="w-3.5 h-3.5" /> {selectedMovie.runtime}m</span>
                    )}
                    {selectedMovie.rating > 0 && (
                      <span className="flex items-center gap-1"><Star className="w-3.5 h-3.5 text-yellow-400 fill-yellow-400" /> {selectedMovie.rating.toFixed(1)}</span>
                    )}
                    {selectedMovie.release_date && (
                      <span className="flex items-center gap-1"><Calendar className="w-3.5 h-3.5" /> {selectedMovie.release_date}</span>
                    )}
                  </div>
                  {selectedMovie.genres && selectedMovie.genres.length > 0 && (
                    <div className="flex gap-1.5 mt-2 flex-wrap">
                      {selectedMovie.genres.map(g => <Badge key={g} variant="secondary" className="text-[10px]">{g}</Badge>)}
                    </div>
                  )}
                  <p className="text-sm text-muted-foreground mt-3 line-clamp-3 leading-relaxed">{selectedMovie.overview || "No overview available."}</p>
                </div>
              </div>
              <div className="mt-6 grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">Library</label>
                  <Select value={selectedLibrary} onValueChange={handleLibraryChange}>
                    <SelectTrigger><SelectValue placeholder="Select library" /></SelectTrigger>
                    <SelectContent>{libraries.map(lib => <SelectItem key={lib.id} value={lib.id}>{lib.name}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">Quality Profile</label>
                  <Select value={selectedProfile} onValueChange={setSelectedProfile}>
                    <SelectTrigger><SelectValue placeholder="Select quality profile" /></SelectTrigger>
                    <SelectContent>{qualityProfiles.map(qp => <SelectItem key={qp.id} value={qp.id}>{qp.name}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
              </div>
              <div className="flex items-center gap-2 mt-4">
                <Checkbox id="monitored" checked={monitored} onCheckedChange={(v) => setMonitored(v === true)} />
                <label htmlFor="monitored" className="text-sm font-medium cursor-pointer">Monitor for downloads</label>
              </div>
              <div className="flex items-center gap-2 mt-2">
                <Checkbox id="searchOnAdd" checked={searchOnAdd} onCheckedChange={(v) => setSearchOnAdd(v === true)} />
                <label htmlFor="searchOnAdd" className="text-sm font-medium cursor-pointer">Search after adding</label>
              </div>
              {addError && <p className="text-sm text-destructive mt-3">{addError}</p>}
              <div className="flex justify-end gap-3 mt-6 pt-4 border-t border-border/50">
                <Button variant="outline" onClick={() => setSelectedMovie(null)}>Cancel</Button>
                <Button onClick={handleAdd} disabled={adding || !selectedLibrary || !selectedProfile} className="min-w-[120px]">
                  {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : <><Plus className="w-4 h-4 mr-1" /> Add Movie</>}
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <>
            <div className="px-6 pt-4">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                <Input ref={searchInputRef} placeholder="Search for a movie to add..." value={searchTerm} onChange={(e) => handleSearchChange(e.target.value)} className="pl-9 h-11" />
                {searching && <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 animate-spin text-muted-foreground" />}
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-6 pb-6">
              {results.length === 0 && searchTerm.length >= 2 && !searching ? (
                <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                  <Film className="w-12 h-12 mb-3 opacity-30" />
                  <p className="text-sm">No movies found for &ldquo;{searchTerm}&rdquo;</p>
                </div>
              ) : results.length === 0 && searchTerm.length < 2 ? (
                <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                  <Search className="w-12 h-12 mb-3 opacity-30" />
                  <p className="text-sm">Start typing to search TMDB</p>
                </div>
              ) : (
                <div className="space-y-2 mt-3">
                  {results.map((r, i) => {
                    const inLibrary = isAlreadyInLibrary(r);
                    return (
                      <button
                        key={r.tmdb_id ?? i}
                        onClick={() => !inLibrary && setSelectedMovie(r)}
                        disabled={inLibrary}
                        className={cn(
                          "w-full flex items-start gap-4 p-3 rounded-lg border text-left transition-colors",
                          inLibrary ? "border-border/30 opacity-50 cursor-not-allowed" : "border-border/50 hover:border-accent/50 hover:bg-accent/5 cursor-pointer",
                        )}
                      >
                        <div className="shrink-0 w-12 aspect-[2/3] rounded overflow-hidden bg-muted">
                          {r.poster_path ? (
                            <img src={`${TMDB_IMG}/w92${r.poster_path}`} alt={r.title} className="w-full h-full object-cover" />
                          ) : (
                            <div className="w-full h-full flex items-center justify-center"><Film className="w-4 h-4 text-muted-foreground/30" /></div>
                          )}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <h4 className="text-sm font-semibold truncate">{r.title}</h4>
                            {r.year > 0 && <span className="text-xs text-muted-foreground shrink-0">({r.year})</span>}
                          </div>
                          <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">{r.overview}</p>
                          <div className="flex items-center gap-3 mt-1.5">
                            {r.rating > 0 && (
                              <span className="flex items-center gap-0.5 text-xs text-yellow-500"><Star className="w-3 h-3 fill-yellow-500" /> {r.rating.toFixed(1)}</span>
                            )}
                            {r.genres && r.genres.length > 0 && (
                              <span className="text-[10px] text-muted-foreground">{r.genres.slice(0, 3).join(" • ")}</span>
                            )}
                          </div>
                        </div>
                        <div className="shrink-0 flex items-center">
                          {inLibrary ? (
                            <span className="flex items-center gap-1 text-xs text-green-500"><Check className="w-3.5 h-3.5" /> In Library</span>
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
