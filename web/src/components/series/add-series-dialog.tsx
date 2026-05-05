import { useEffect, useState, useCallback, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
  Plus, Search, Loader2, Tv, Check, ArrowLeft,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { Library } from "../../lib/libraries-api";
import type { QualityProfile, TMDBSeriesResult } from "./types";
import { TMDB_IMG } from "./types";

export function AddSeriesDialog({
  open,
  onOpenChange,
  libraries,
  qualityProfiles,
  existingTmdbIds,
  onSeriesAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingTmdbIds: Set<string>;
  onSeriesAdded: () => void;
}) {
  const [searchTerm, setSearchTerm] = useState("");
  const [results, setResults] = useState<TMDBSeriesResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [selectedSeries, setSelectedSeries] = useState<TMDBSeriesResult | null>(null);
  const [selectedProfile, setSelectedProfile] = useState(qualityProfiles[0]?.id ?? "");
  const [selectedLibrary, setSelectedLibrary] = useState(libraries[0]?.id ?? "");
  const [seriesType, setSeriesType] = useState("standard");
  const [seasonFolder, setSeasonFolder] = useState(true);
  const [monitoringStatus, setMonitoringStatus] = useState("all");
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState("");
  const [searchOnAdd, setSearchOnAdd] = useState(true);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>();
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setSearchTerm("");
      setResults([]);
      setSelectedSeries(null);
      setAddError("");
      const firstLib = libraries[0];
      setSelectedLibrary(firstLib?.id ?? "");
      setSelectedProfile(firstLib?.quality_profile_id || (qualityProfiles[0]?.id ?? ""));
      setSeriesType("standard");
      setSeasonFolder(true);
      setMonitoringStatus("all");
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
      const res = await fetch(`/api/v1/series/search?q=${encodeURIComponent(term)}`, { credentials: "include" });
      if (res.ok) { const data = await res.json(); setResults(Array.isArray(data) ? data : data.data ?? []); }
    } catch { /* ignore */ } finally { setSearching(false); }
  }, []);

  const handleSearchChange = (val: string) => {
    setSearchTerm(val);
    setSelectedSeries(null);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => doSearch(val), 400);
  };

  const handleAdd = async () => {
    if (!selectedSeries || !selectedLibrary || !selectedProfile) return;
    setAdding(true);
    setAddError("");
    try {
      const res = await fetch("/api/v1/series", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          tmdbId: selectedSeries.tmdbId ?? "",
          qualityProfileId: selectedProfile,
          libraryId: selectedLibrary,
          monitoringStatus,
          seasonFolder,
          seriesType,
          search: searchOnAdd,
        }),
      });
      if (!res.ok) { setAddError((await res.text()) || "Failed to add series"); return; }
      onSeriesAdded();
      onOpenChange(false);
    } catch { setAddError("Network error adding series"); } finally { setAdding(false); }
  };

  const isAlreadyInLibrary = (r: TMDBSeriesResult) => r.tmdbId ? existingTmdbIds.has(r.tmdbId) : false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl max-h-[85vh] flex flex-col p-0 gap-0">
        <DialogHeader className="p-6 pb-4 border-b border-border/50">
          <DialogTitle className="text-xl flex items-center gap-2">
            <Tv className="w-5 h-5 text-accent" />
            {selectedSeries ? "Add Series" : "Search Series"}
          </DialogTitle>
        </DialogHeader>

        {selectedSeries ? (
          <div className="flex-1 overflow-y-auto">
            <div className="relative h-48 bg-muted overflow-hidden">
              {selectedSeries.posterPath && (
                <img src={`${TMDB_IMG}/w780${selectedSeries.posterPath}`} className="w-full h-full object-cover opacity-30 blur-sm" alt="" />
              )}
              <div className="absolute inset-0 bg-gradient-to-t from-background to-transparent" />
              <button onClick={() => setSelectedSeries(null)} className="absolute top-4 left-4 flex items-center gap-1 text-sm text-white/80 hover:text-white bg-black/40 rounded-full px-3 py-1.5">
                <ArrowLeft className="w-4 h-4" /> Back to results
              </button>
            </div>
            <div className="p-6 -mt-16 relative z-10">
              <div className="flex gap-5">
                <div className="shrink-0 w-32 rounded-lg overflow-hidden shadow-xl border-2 border-background">
                  {selectedSeries.posterPath ? (
                    <img src={`${TMDB_IMG}/w300${selectedSeries.posterPath}`} alt={selectedSeries.title} className="w-full aspect-[2/3] object-cover" />
                  ) : (
                    <div className="w-full aspect-[2/3] bg-muted flex items-center justify-center"><Tv className="w-8 h-8 text-muted-foreground/30" /></div>
                  )}
                </div>
                <div className="flex-1 min-w-0 pt-12">
                  <h2 className="text-2xl font-bold truncate">{selectedSeries.title}</h2>
                  <div className="flex items-center gap-3 mt-1 text-sm text-muted-foreground">
                    {selectedSeries.year > 0 && <span>{selectedSeries.year}</span>}
                    {selectedSeries.network && <span>{selectedSeries.network}</span>}
                    {selectedSeries.status && <span className="capitalize">{selectedSeries.status}</span>}
                  </div>
                  <p className="text-sm text-muted-foreground mt-3 line-clamp-3 leading-relaxed">{selectedSeries.overview || "No overview available."}</p>
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
                <div className="space-y-1.5">
                  <label className="text-sm font-medium">Series Type</label>
                  <Select value={seriesType} onValueChange={setSeriesType}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="standard">Standard</SelectItem>
                      <SelectItem value="daily">Daily</SelectItem>
                      <SelectItem value="anime">Anime</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="flex items-center gap-6 mt-4">
                <div className="flex-1">
                  <label className="text-xs text-muted-foreground mb-1 block">Monitor</label>
                  <Select value={monitoringStatus} onValueChange={setMonitoringStatus}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Episodes</SelectItem>
                      <SelectItem value="future">Future Episodes</SelectItem>
                      <SelectItem value="missing">Missing Episodes</SelectItem>
                      <SelectItem value="existing">Existing Episodes</SelectItem>
                      <SelectItem value="pilot">Pilot Only</SelectItem>
                      <SelectItem value="firstSeason">First Season</SelectItem>
                      <SelectItem value="lastSeason">Latest Season</SelectItem>
                      <SelectItem value="none">None</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="flex items-center gap-2 pt-5">
                  <Checkbox id="seasonFolder" checked={seasonFolder} onCheckedChange={(v) => setSeasonFolder(v === true)} />
                  <label htmlFor="seasonFolder" className="text-sm font-medium cursor-pointer">Season folders</label>
                </div>
              </div>

              <div className="flex items-center gap-2 mt-2">
                <Checkbox id="searchOnAdd" checked={searchOnAdd} onCheckedChange={(v) => setSearchOnAdd(v === true)} />
                <label htmlFor="searchOnAdd" className="text-sm font-medium cursor-pointer">Search after adding</label>
              </div>

              {addError && <p className="text-sm text-destructive mt-3">{addError}</p>}

              <div className="flex justify-end gap-3 mt-6 pt-4 border-t border-border/50">
                <Button variant="outline" onClick={() => setSelectedSeries(null)}>Cancel</Button>
                <Button onClick={handleAdd} disabled={adding || !selectedLibrary || !selectedProfile} className="min-w-[120px]">
                  {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : <><Plus className="w-4 h-4 mr-1" /> Add Series</>}
                </Button>
              </div>
            </div>
          </div>
        ) : (
          <>
            <div className="px-6 pt-4">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                <Input ref={searchInputRef} placeholder="Search for a series to add..." value={searchTerm} onChange={(e) => handleSearchChange(e.target.value)} className="pl-9 h-11" />
                {searching && <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 animate-spin text-muted-foreground" />}
              </div>
            </div>
            <div className="flex-1 overflow-y-auto px-6 pb-6">
              {results.length === 0 && searchTerm.length >= 2 && !searching ? (
                <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
                  <Tv className="w-12 h-12 mb-3 opacity-30" />
                  <p className="text-sm">No series found for &ldquo;{searchTerm}&rdquo;</p>
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
                        key={r.tmdbId ?? i}
                        onClick={() => !inLibrary && setSelectedSeries(r)}
                        disabled={inLibrary}
                        className={cn(
                          "w-full flex items-start gap-4 p-3 rounded-lg border text-left transition-colors",
                          inLibrary ? "border-border/30 opacity-50 cursor-not-allowed" : "border-border/50 hover:border-accent/50 hover:bg-accent/5 cursor-pointer",
                        )}
                      >
                        <div className="shrink-0 w-12 aspect-[2/3] rounded overflow-hidden bg-muted">
                          {r.posterPath ? (
                            <img src={`${TMDB_IMG}/w92${r.posterPath}`} alt={r.title} className="w-full h-full object-cover" />
                          ) : (
                            <div className="w-full h-full flex items-center justify-center"><Tv className="w-4 h-4 text-muted-foreground/30" /></div>
                          )}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <h4 className="text-sm font-semibold truncate">{r.title}</h4>
                            {r.year > 0 && <span className="text-xs text-muted-foreground shrink-0">({r.year})</span>}
                          </div>
                          {r.network && <p className="text-xs text-muted-foreground mt-0.5">{r.network}</p>}
                          <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">{r.overview}</p>
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
