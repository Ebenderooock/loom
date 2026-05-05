import { useEffect, useState, useCallback, useMemo } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Plus, Search, Film, FolderSearch } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuth } from "@/hooks/use-auth";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { toast } from "sonner";
import {
  MovieCard,
  MovieListRow,
  MovieToolbar,
  AddMovieDialog,
  MovieDetailSheet,
  sortMovies,
} from "@/components/movies";
import { LibraryImportDialog } from "@/components/movies/library-import-dialog";
import { OrganizeDialog } from "@/components/movies/organize-dialog";
import { useLibraries } from "@/lib/libraries-api";
import type { Movie, QualityProfile, SortKey, ViewMode } from "@/components/movies";


// ─── Small helpers (page-local) ──────────────────────────────────────

function CardSkeleton() {
  return <Skeleton className="aspect-[2/3] rounded-lg" />;
}

function GridSkeletons() {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8 gap-4">
      {Array.from({ length: 12 }).map((_, i) => <CardSkeleton key={i} />)}
    </div>
  );
}

function ListSkeletons() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-12 w-full rounded-md" />)}
    </div>
  );
}

function BulkDeleteDialog({
  open,
  onOpenChange,
  count,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  count: number;
  onConfirm: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Delete {count} Movie{count !== 1 ? "s" : ""}</DialogTitle>
          <DialogDescription>
            This will remove {count} movie{count !== 1 ? "s" : ""} from your library. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="flex justify-end gap-2 mt-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button
            variant="destructive"
            disabled={deleting}
            onClick={async () => {
              setDeleting(true);
              await onConfirm();
              setDeleting(false);
              onOpenChange(false);
            }}
          >
            Delete
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ─── Main Page ──────────────────────────────────────────────────────────

export function MoviesPage() {
  const { isAuthenticated } = useAuth();
  const [movies, setMovies] = useState<Movie[]>([]);
  const { data: allLibraries = [] } = useLibraries();
  const libraries = allLibraries.filter(l => l.media_type === "movie");
  const [qualityProfiles, setQualityProfiles] = useState<QualityProfile[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [organizeDialogOpen, setOrganizeDialogOpen] = useState(false);

  // Filters & sort
  const [filterText, setFilterText] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [profileFilter, setProfileFilter] = useState("all");
  const [monitoredFilter, setMonitoredFilter] = useState("all");
  const [sortKey, setSortKey] = useState<SortKey>("title-asc");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");

  // Selection
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

  // Detail sheet
  const [detailMovie, setDetailMovie] = useState<Movie | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);

  const fetchAll = useCallback(async () => {
    if (!isAuthenticated) return;
    setIsLoading(true);
    try {
      const [moviesRes, profilesRes] = await Promise.all([
        fetch("/api/v1/movies?limit=200", { credentials: "include" }),
        fetch("/api/v1/movies/quality-profiles", { credentials: "include" }),
      ]);
      if (moviesRes.ok) {
        const data = await moviesRes.json();
        setMovies(Array.isArray(data) ? data : data.data ?? []);
      }
      if (profilesRes.ok) {
        const data = await profilesRes.json();
        setQualityProfiles(Array.isArray(data) ? data : []);
      }
    } catch { /* ignore */ } finally { setIsLoading(false); }
  }, [isAuthenticated]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  const existingTmdbIds = useMemo(
    () => new Set(movies.map(m => m.tmdbId).filter(Boolean) as string[]),
    [movies],
  );

  // Filter + sort pipeline
  const processed = useMemo(() => {
    let list = movies;
    if (filterText) {
      const q = filterText.toLowerCase();
      list = list.filter(m => m.title.toLowerCase().includes(q));
    }
    if (statusFilter !== "all") {
      list = list.filter(m => m.status === statusFilter);
    }
    if (profileFilter !== "all") {
      list = list.filter(m => m.qualityProfileId === profileFilter);
    }
    if (monitoredFilter === "monitored") {
      list = list.filter(m => m.monitoringStatus === "monitored");
    } else if (monitoredFilter === "unmonitored") {
      list = list.filter(m => m.monitoringStatus === "unmonitored");
    } else if (monitoredFilter === "archived") {
      list = list.filter(m => m.monitoringStatus === "unmonitored");
    }
    return sortMovies(list, sortKey);
  }, [movies, filterText, statusFilter, profileFilter, monitoredFilter, sortKey]);

  // Selection helpers
  const selectMode = selectedIds.size > 0;
  const allSelected = processed.length > 0 && processed.every(m => selectedIds.has(m.id));

  const toggleSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(processed.map(m => m.id)));
    }
  };

  const clearSelection = () => setSelectedIds(new Set());

  // Bulk actions
  const handleBulkMonitoring = async (status: "monitored" | "unmonitored") => {
    const ids = Array.from(selectedIds);
    await Promise.all(ids.map(id =>
      fetch(`/api/v1/movies/${id}/monitoring`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ status }),
      }),
    ));
    setMovies(prev => prev.map(m => selectedIds.has(m.id) ? { ...m, monitoringStatus: status } : m));
    clearSelection();
    toast.success(`${ids.length} movie${ids.length !== 1 ? "s" : ""} set to ${status}`);
  };

  const handleBulkDelete = async () => {
    const ids = Array.from(selectedIds);
    await Promise.all(ids.map(id => fetch(`/api/v1/movies/${id}`, { method: "DELETE", credentials: "include" })));
    setMovies(prev => prev.filter(m => !selectedIds.has(m.id)));
    clearSelection();
    toast.success(`${ids.length} movie${ids.length !== 1 ? "s" : ""} deleted`);
  };

  // Movie update/delete from detail sheet
  const handleMovieUpdated = (updated: Movie) => {
    setMovies(prev => prev.map(m => m.id === updated.id ? updated : m));
    setDetailMovie(updated);
  };

  const handleMovieDeleted = (id: string) => {
    setMovies(prev => prev.filter(m => m.id !== id));
    setSelectedIds(prev => { const next = new Set(prev); next.delete(id); return next; });
  };

  const openDetail = (movie: Movie) => {
    setDetailMovie(movie);
    setDetailOpen(true);
  };

  // Stats
  const totalMovies = movies.length;
  const monitoredCount = movies.filter(m => m.monitoringStatus === "monitored").length;
  const missingCount = movies.filter(m => m.status === "missing").length;

  const subtitle = totalMovies > 0
    ? `${totalMovies} movie${totalMovies !== 1 ? "s" : ""} • ${monitoredCount} monitored • ${missingCount} missing`
    : undefined;
  useSetPageHeader("Movies", subtitle);

  return (
    <div className="px-6 pt-2 pb-6">
      {/* Toolbar */}
      {totalMovies > 0 ? (
        <MovieToolbar
          filterText={filterText}
          onFilterTextChange={setFilterText}
          statusFilter={statusFilter}
          onStatusFilterChange={setStatusFilter}
          profileFilter={profileFilter}
          onProfileFilterChange={setProfileFilter}
          monitoredFilter={monitoredFilter}
          onMonitoredFilterChange={setMonitoredFilter}
          sortKey={sortKey}
          onSortKeyChange={setSortKey}
          viewMode={viewMode}
          onViewModeChange={setViewMode}
          profiles={qualityProfiles}
          selectMode={selectMode}
          selectedCount={selectedIds.size}
          allSelected={allSelected}
          onToggleSelectAll={toggleSelectAll}
          onClearSelection={clearSelection}
          onBulkMonitor={() => handleBulkMonitoring("monitored")}
          onBulkUnmonitor={() => handleBulkMonitoring("unmonitored")}
          onBulkDelete={() => setBulkDeleteOpen(true)}
          onAddMovie={() => setAddDialogOpen(true)}
          onImportLibrary={() => setImportDialogOpen(true)}
          onOrganize={() => setOrganizeDialogOpen(true)}
        />
      ) : null}

      {/* Content */}
      {isLoading ? (
        viewMode === "grid" ? <GridSkeletons /> : <ListSkeletons />
      ) : totalMovies === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="w-20 h-20 rounded-full bg-accent/10 flex items-center justify-center mb-6">
            <Film className="w-10 h-10 text-accent" />
          </div>
          <h2 className="text-xl font-semibold mb-2">No movies yet</h2>
          <p className="text-sm text-muted-foreground max-w-sm mb-6">
            Start building your library by adding movies from TMDB, or import existing movies from your libraries.
          </p>
          {libraries.length === 0 ? (
            <p className="text-sm text-amber-500">⚠️ Add a library in Settings before adding movies</p>
          ) : (
            <div className="flex gap-3">
              <Button variant="outline" size="lg" onClick={() => setImportDialogOpen(true)}>
                <FolderSearch className="w-4 h-4 mr-1.5" /> Import Existing
              </Button>
              <Button onClick={() => setAddDialogOpen(true)} size="lg">
                <Plus className="w-4 h-4 mr-1.5" /> Add Movie
              </Button>
            </div>
          )}
        </div>
      ) : viewMode === "grid" ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8 gap-4">
          {processed.map(movie => (
            <div key={movie.id} className={cn(movie.monitoringStatus === "unmonitored" && "opacity-60")}>
              <MovieCard
                movie={movie}
                profiles={qualityProfiles}
                selected={selectedIds.has(movie.id)}
                selectMode={selectMode}
                onToggleSelect={() => toggleSelect(movie.id)}
                onClick={() => openDetail(movie)}
              />
            </div>
          ))}
        </div>
      ) : (
        <div className="border border-border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox checked={allSelected} onCheckedChange={toggleSelectAll} />
                </TableHead>
                <TableHead className="w-12" />
                <TableHead>Title</TableHead>
                <TableHead className="w-16">Year</TableHead>
                <TableHead className="w-28">Status</TableHead>
                <TableHead className="w-28">Quality</TableHead>
                <TableHead className="w-12">Mon.</TableHead>
                <TableHead className="w-16">Rating</TableHead>
                <TableHead className="w-24">Added</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {processed.map(movie => (
                <MovieListRow
                  key={movie.id}
                  movie={movie}
                  profiles={qualityProfiles}
                  selected={selectedIds.has(movie.id)}
                  onToggleSelect={() => toggleSelect(movie.id)}
                  onClick={() => openDetail(movie)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {processed.length === 0 && totalMovies > 0 && !isLoading && (
        <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
          <Search className="w-10 h-10 mb-3 opacity-30" />
          <p className="text-sm">No movies match the current filters</p>
        </div>
      )}

      {/* Dialogs & Sheets */}
      <AddMovieDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        libraries={libraries}
        qualityProfiles={qualityProfiles}
        existingTmdbIds={existingTmdbIds}
        onMovieAdded={fetchAll}
      />

      <MovieDetailSheet
        movie={detailMovie}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        profiles={qualityProfiles}
        libraries={libraries}
        onUpdated={handleMovieUpdated}
        onDeleted={handleMovieDeleted}
      />

      <BulkDeleteDialog
        open={bulkDeleteOpen}
        onOpenChange={setBulkDeleteOpen}
        count={selectedIds.size}
        onConfirm={handleBulkDelete}
      />

      <LibraryImportDialog
        open={importDialogOpen}
        onOpenChange={setImportDialogOpen}
        libraries={libraries}
        onImportComplete={fetchAll}
      />

      <OrganizeDialog
        open={organizeDialogOpen}
        onOpenChange={setOrganizeDialogOpen}
        movies={movies}
        onComplete={fetchAll}
      />
    </div>
  );
}
