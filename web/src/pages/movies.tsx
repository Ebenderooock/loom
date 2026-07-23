import { useEffect, useState, useCallback, useMemo } from "react";
import { useSearch, useNavigate } from "@tanstack/react-router";
import { apiFetch } from "@/lib/fetch";
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
import type {
  Movie,
  QualityProfile,
  SortKey,
  ViewMode,
} from "@/components/movies";

// ─── Small helpers (page-local) ──────────────────────────────────────

function CardSkeleton() {
  return <Skeleton className="aspect-[2/3] rounded-lg" />;
}

function GridSkeletons() {
  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
      {Array.from({ length: 12 }).map((_, i) => (
        <CardSkeleton key={i} />
      ))}
    </div>
  );
}

function ListSkeletons() {
  return (
    <div className="space-y-2">
      {Array.from({ length: 8 }).map((_, i) => (
        <Skeleton key={i} className="h-12 w-full rounded-md" />
      ))}
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
          <DialogTitle>
            Delete {count} Movie{count !== 1 ? "s" : ""}
          </DialogTitle>
          <DialogDescription>
            This will remove {count} movie{count !== 1 ? "s" : ""} from your
            library. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
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
  const libraries = allLibraries.filter((l) => l.media_type === "movie");
  const [qualityProfiles, setQualityProfiles] = useState<QualityProfile[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [organizeDialogOpen, setOrganizeDialogOpen] = useState(false);
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [rescanningLibraries, setRescanningLibraries] = useState(false);

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

  // Deep-link: open a specific movie's detail when navigated with ?focus=<id>
  // (e.g. from the global command palette).
  const navigate = useNavigate();
  const { focus } = useSearch({ strict: false }) as { focus?: string };

  const fetchAll = useCallback(
    async (background = false) => {
      if (!isAuthenticated) return;
      if (!background) setIsLoading(true);
      try {
        const [moviesRes, profilesRes] = await Promise.all([
          apiFetch("/api/v1/movies?limit=200"),
          apiFetch("/api/v1/quality-profiles"),
        ]);
        if (moviesRes.ok) {
          const data = await moviesRes.json();
          const fresh: Movie[] = Array.isArray(data) ? data : (data.data ?? []);
          setMovies(fresh);
          // Reconcile detail sheet with refreshed data
          setDetailMovie((prev) => {
            if (!prev) return null;
            return fresh.find((m) => m.id === prev.id) ?? prev;
          });
        }
        if (profilesRes.ok) {
          const data = await profilesRes.json();
          const profiles = data?.data ?? (Array.isArray(data) ? data : []);
          setQualityProfiles(profiles);
        }
      } catch {
        /* ignore */
      } finally {
        if (!background) setIsLoading(false);
      }
    },
    [isAuthenticated],
  );

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // Open the detail sheet for a deep-linked movie (e.g. from the global command
  // palette). Resolve from the loaded list, falling back to a by-ID fetch for
  // movies outside this page's window, then clear the param.
  useEffect(() => {
    if (!focus || isLoading) return;
    const clear = () =>
      void navigate({ to: "/movies", search: {}, replace: true });
    const existing = movies.find((m) => m.id === focus);
    if (existing) {
      setDetailMovie(existing);
      setDetailOpen(true);
      clear();
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await apiFetch(`/api/v1/movies/${focus}`);
        if (cancelled) return;
        if (res.ok) {
          const data = await res.json();
          const movie: Movie = data?.data ?? data;
          if (movie?.id) {
            setDetailMovie(movie);
            setDetailOpen(true);
          } else {
            toast.error("Movie not found");
          }
        } else {
          toast.error("Movie not found");
        }
      } catch {
        if (!cancelled) toast.error("Could not open movie");
      } finally {
        if (!cancelled) clear();
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [focus, isLoading, movies, navigate]);

  // Background polling every 30s to pick up status changes (grab → downloading → available)
  useEffect(() => {
    if (!isAuthenticated) return;
    const interval = setInterval(() => fetchAll(true), 30_000);
    return () => clearInterval(interval);
  }, [isAuthenticated, fetchAll]);

  const existingTmdbIds = useMemo(
    () => new Set(movies.map((m) => m.tmdbId).filter(Boolean) as string[]),
    [movies],
  );

  const existingMovieNumericIds = useMemo(
    () =>
      new Set(
        movies
          .map((m) => m.tmdbId)
          .filter(Boolean)
          .map(Number)
          .filter((n) => !isNaN(n)),
      ),
    [movies],
  );

  // Filter + sort pipeline
  const processed = useMemo(() => {
    let list = movies;
    if (filterText) {
      const q = filterText.toLowerCase();
      list = list.filter((m) => m.title.toLowerCase().includes(q));
    }
    if (statusFilter !== "all") {
      list = list.filter((m) => m.status === statusFilter);
    }
    if (profileFilter !== "all") {
      list = list.filter((m) => m.qualityProfileId === profileFilter);
    }
    if (monitoredFilter === "monitored") {
      list = list.filter((m) => m.monitoringStatus === "monitored");
    } else if (monitoredFilter === "unmonitored") {
      list = list.filter((m) => m.monitoringStatus === "unmonitored");
    } else if (monitoredFilter === "archived") {
      list = list.filter((m) => m.monitoringStatus === "unmonitored");
    }
    return sortMovies(list, sortKey);
  }, [
    movies,
    filterText,
    statusFilter,
    profileFilter,
    monitoredFilter,
    sortKey,
  ]);

  // Selection helpers
  const selectMode = selectedIds.size > 0;
  const allSelected =
    processed.length > 0 && processed.every((m) => selectedIds.has(m.id));

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(processed.map((m) => m.id)));
    }
  };

  const clearSelection = () => setSelectedIds(new Set());

  // Bulk actions
  const handleBulkMonitoring = async (status: "monitored" | "unmonitored") => {
    const ids = Array.from(selectedIds);
    await Promise.all(
      ids.map((id) =>
        apiFetch(`/api/v1/movies/${id}/monitoring`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ status }),
        }),
      ),
    );
    setMovies((prev) =>
      prev.map((m) =>
        selectedIds.has(m.id) ? { ...m, monitoringStatus: status } : m,
      ),
    );
    clearSelection();
    toast.success(
      `${ids.length} movie${ids.length !== 1 ? "s" : ""} set to ${status}`,
    );
  };

  const handleBulkDelete = async () => {
    const ids = Array.from(selectedIds);
    await Promise.all(
      ids.map((id) => apiFetch(`/api/v1/movies/${id}`, { method: "DELETE" })),
    );
    setMovies((prev) => prev.filter((m) => !selectedIds.has(m.id)));
    clearSelection();
    toast.success(`${ids.length} movie${ids.length !== 1 ? "s" : ""} deleted`);
  };

  const handleBulkQualityProfile = async (profileId: string) => {
    const ids = Array.from(selectedIds);
    const results = await Promise.allSettled(
      ids.map((id) =>
        apiFetch(`/api/v1/movies/${id}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ qualityProfileId: profileId }),
        }).then((r) => {
          if (!r.ok) throw new Error(`${r.status}`);
          return id;
        }),
      ),
    );
    const succeeded = results
      .filter((r) => r.status === "fulfilled")
      .map((r) => (r as PromiseFulfilledResult<string>).value);
    const failed = results.filter((r) => r.status === "rejected").length;
    if (succeeded.length > 0) {
      const succSet = new Set(succeeded);
      setMovies((prev) =>
        prev.map((m) =>
          succSet.has(m.id) ? { ...m, qualityProfileId: profileId } : m,
        ),
      );
    }
    clearSelection();
    const profile = qualityProfiles.find((p) => p.id === profileId);
    if (failed > 0) {
      toast.error(`${failed} movie${failed !== 1 ? "s" : ""} failed to update`);
    } else {
      toast.success(
        `${ids.length} movie${ids.length !== 1 ? "s" : ""} set to ${profile?.name ?? "profile"}`,
      );
    }
  };

  // Movie update/delete from detail sheet
  const handleMovieUpdated = (updated: Movie) => {
    setMovies((prev) => prev.map((m) => (m.id === updated.id ? updated : m)));
    setDetailMovie(updated);
  };

  const handleMovieDeleted = (id: string) => {
    setMovies((prev) => prev.filter((m) => m.id !== id));
    setSelectedIds((prev) => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };

  const openDetail = (movie: Movie) => {
    setDetailMovie(movie);
    setDetailOpen(true);
  };

  const handleRefreshAll = async () => {
    setRefreshingAll(true);
    try {
      const res = await apiFetch("/api/v1/movies/refresh", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = (await res.json()) as { count?: number };
      toast.success(
        `Refreshing ${data.count ?? movies.length} movie${(data.count ?? movies.length) === 1 ? "" : "s"} in the background`,
      );
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh movies",
      );
    } finally {
      setRefreshingAll(false);
    }
  };

  const handleRescanLibraries = async () => {
    setRescanningLibraries(true);
    try {
      const res = await apiFetch("/api/v1/movies/rescan", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = (await res.json()) as { libraryCount?: number };
      toast.success(
        `Rescanning ${data.libraryCount ?? libraries.length} movie librar${(data.libraryCount ?? libraries.length) === 1 ? "y" : "ies"} in the background`,
      );
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to rescan movie libraries",
      );
    } finally {
      setRescanningLibraries(false);
    }
  };

  // Stats
  const totalMovies = movies.length;
  const monitoredCount = movies.filter(
    (m) => m.monitoringStatus === "monitored",
  ).length;
  const missingCount = movies.filter((m) => m.status === "missing").length;

  const subtitle =
    totalMovies > 0
      ? `${totalMovies} movie${totalMovies !== 1 ? "s" : ""} • ${monitoredCount} monitored • ${missingCount} missing`
      : undefined;
  useSetPageHeader("Movies", subtitle);

  return (
    <div className="px-6 pb-6 pt-2">
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
          onBulkQualityProfile={handleBulkQualityProfile}
          onAddMovie={() => setAddDialogOpen(true)}
          onImportLibrary={() => setImportDialogOpen(true)}
          onOrganize={() => setOrganizeDialogOpen(true)}
          onRefreshAll={handleRefreshAll}
          onRescanLibraries={handleRescanLibraries}
          refreshingAll={refreshingAll}
          rescanningLibraries={rescanningLibraries}
        />
      ) : null}

      {/* Content */}
      {isLoading ? (
        viewMode === "grid" ? (
          <GridSkeletons />
        ) : (
          <ListSkeletons />
        )
      ) : totalMovies === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-6 flex h-20 w-20 items-center justify-center rounded-full bg-accent/10">
            <Film className="h-10 w-10 text-accent" />
          </div>
          <h2 className="mb-2 text-xl font-semibold">No movies yet</h2>
          <p className="mb-6 max-w-sm text-sm text-muted-foreground">
            Start building your library by adding movies from TMDB, or import
            existing movies from your libraries.
          </p>
          {libraries.length === 0 ? (
            <p className="text-sm text-amber-500">
              ⚠️ Add a library in Settings before adding movies
            </p>
          ) : (
            <div className="flex gap-3">
              <Button
                variant="outline"
                size="lg"
                onClick={handleRescanLibraries}
                disabled={rescanningLibraries}
              >
                <FolderSearch className="mr-1.5 h-4 w-4" />
                {rescanningLibraries ? "Rescanning..." : "Rescan Libraries"}
              </Button>
              <Button
                variant="outline"
                size="lg"
                onClick={() => setImportDialogOpen(true)}
              >
                <FolderSearch className="mr-1.5 h-4 w-4" /> Import Existing
              </Button>
              <Button onClick={() => setAddDialogOpen(true)} size="lg">
                <Plus className="mr-1.5 h-4 w-4" /> Add Movie
              </Button>
            </div>
          )}
        </div>
      ) : viewMode === "grid" ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
          {processed.map((movie) => (
            <div
              key={movie.id}
              className={cn(
                movie.monitoringStatus === "unmonitored" && "opacity-60",
              )}
            >
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
        <div className="overflow-hidden overflow-x-auto rounded-lg border border-border">
          <Table className="min-w-[700px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox
                    checked={allSelected}
                    onCheckedChange={toggleSelectAll}
                  />
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
              {processed.map((movie) => (
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
          <Search className="mb-3 h-10 w-10 opacity-30" />
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
        onRefresh={() => fetchAll(true)}
        existingMovieIds={existingMovieNumericIds}
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
