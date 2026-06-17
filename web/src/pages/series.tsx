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
import { Plus, Search, Tv, FolderSearch } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuth } from "@/hooks/use-auth";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { toast } from "sonner";
import {
  SeriesCard,
  SeriesListRow,
  SeriesToolbar,
  AddSeriesDialog,
  SeriesDetailSheet,
  sortSeries,
} from "@/components/series";
import { SeriesLibraryImportDialog } from "@/components/series/series-library-import-dialog";
import { useLibraries } from "@/lib/libraries-api";
import type {
  Series,
  QualityProfile,
  SeriesSortKey,
  ViewMode,
} from "@/components/series";

// ─── Skeletons ──────────────────────────────────────────────────────────

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
          <DialogTitle>Delete {count} Series</DialogTitle>
          <DialogDescription>
            This will remove {count} series from your library. This cannot be
            undone.
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

export function SeriesPage() {
  const { isAuthenticated } = useAuth();
  const [seriesList, setSeriesList] = useState<Series[]>([]);
  const { data: allLibraries = [] } = useLibraries();
  const libraries = allLibraries.filter((l) => l.media_type === "series");
  const [qualityProfiles, setQualityProfiles] = useState<QualityProfile[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [rescanningLibraries, setRescanningLibraries] = useState(false);

  // Filters & sort
  const [filterText, setFilterText] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [monitoredFilter, setMonitoredFilter] = useState("all");
  const [sortKey, setSortKey] = useState<SeriesSortKey>("title-asc");
  const [viewMode, setViewMode] = useState<ViewMode>("grid");

  // Selection
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);

  // Detail sheet
  const [detailSeries, setDetailSeries] = useState<Series | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);

  // Deep-link: open a specific series' detail when navigated with ?focus=<id>
  // (e.g. from the global command palette).
  const navigate = useNavigate();
  const { focus } = useSearch({ strict: false }) as { focus?: string };

  const fetchAll = useCallback(async () => {
    if (!isAuthenticated) return;
    setIsLoading(true);
    try {
      const [seriesRes, profilesRes] = await Promise.all([
        apiFetch("/api/v1/series"),
        apiFetch("/api/v1/quality-profiles"),
      ]);
      if (seriesRes.ok) {
        const data = await seriesRes.json();
        setSeriesList(Array.isArray(data) ? data : (data.data ?? []));
      }
      if (profilesRes.ok) {
        const data = await profilesRes.json();
        const profiles = data?.data ?? (Array.isArray(data) ? data : []);
        setQualityProfiles(profiles);
      }
    } catch {
      /* ignore */
    } finally {
      setIsLoading(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  // Open the detail sheet for a deep-linked series (e.g. from the global command
  // palette). Resolve from the loaded list, falling back to a by-ID fetch, then
  // clear the param.
  useEffect(() => {
    if (!focus || isLoading) return;
    const clear = () =>
      void navigate({ to: "/series", search: {}, replace: true });
    const existing = seriesList.find((s) => s.id === focus);
    if (existing) {
      setDetailSeries(existing);
      setDetailOpen(true);
      clear();
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await apiFetch(`/api/v1/series/${focus}`);
        if (cancelled) return;
        if (res.ok) {
          const data = await res.json();
          const s: Series = data?.data ?? data;
          if (s?.id) {
            setDetailSeries(s);
            setDetailOpen(true);
          } else {
            toast.error("Series not found");
          }
        } else {
          toast.error("Series not found");
        }
      } catch {
        if (!cancelled) toast.error("Could not open series");
      } finally {
        if (!cancelled) clear();
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [focus, isLoading, seriesList, navigate]);

  const existingTmdbIds = useMemo(
    () => new Set(seriesList.map((s) => s.tmdbId).filter(Boolean) as string[]),
    [seriesList],
  );

  const existingSeriesNumericIds = useMemo(
    () =>
      new Set(
        seriesList
          .map((s) => s.tmdbId)
          .filter(Boolean)
          .map(Number)
          .filter((n) => !isNaN(n)),
      ),
    [seriesList],
  );

  // Filter + sort pipeline
  const processed = useMemo(() => {
    let list = seriesList;
    if (filterText) {
      const q = filterText.toLowerCase();
      list = list.filter((s) => s.title.toLowerCase().includes(q));
    }
    if (statusFilter !== "all") {
      list = list.filter((s) => s.status === statusFilter);
    }
    if (monitoredFilter === "monitored") {
      list = list.filter((s) => s.monitoringStatus === "monitored");
    } else if (monitoredFilter === "unmonitored") {
      list = list.filter((s) => s.monitoringStatus === "unmonitored");
    } else if (monitoredFilter === "archived") {
      list = list.filter((s) => s.monitoringStatus === "unmonitored");
    }
    return sortSeries(list, sortKey);
  }, [seriesList, filterText, statusFilter, monitoredFilter, sortKey]);

  // Selection helpers
  const selectMode = selectedIds.size > 0;
  const allSelected =
    processed.length > 0 && processed.every((s) => selectedIds.has(s.id));

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
      setSelectedIds(new Set(processed.map((s) => s.id)));
    }
  };

  const clearSelection = () => setSelectedIds(new Set());

  // Bulk actions
  const handleBulkMonitoring = async (status: "monitored" | "unmonitored") => {
    const ids = Array.from(selectedIds);
    await Promise.all(
      ids.map((id) =>
        apiFetch(`/api/v1/series/${id}/monitoring`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ status }),
        }),
      ),
    );
    setSeriesList((prev) =>
      prev.map((s) =>
        selectedIds.has(s.id) ? { ...s, monitoringStatus: status } : s,
      ),
    );
    clearSelection();
    toast.success(`${ids.length} series set to ${status}`);
  };

  const handleBulkDelete = async () => {
    const ids = Array.from(selectedIds);
    await Promise.all(
      ids.map((id) => apiFetch(`/api/v1/series/${id}`, { method: "DELETE" })),
    );
    setSeriesList((prev) => prev.filter((s) => !selectedIds.has(s.id)));
    clearSelection();
    toast.success(`${ids.length} series deleted`);
  };

  const handleBulkQualityProfile = async (profileId: string) => {
    const ids = Array.from(selectedIds);
    const results = await Promise.allSettled(
      ids.map((id) =>
        apiFetch(`/api/v1/series/${id}`, {
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
      setSeriesList((prev) =>
        prev.map((s) =>
          succSet.has(s.id) ? { ...s, qualityProfileId: profileId } : s,
        ),
      );
    }
    clearSelection();
    const profile = qualityProfiles.find((p) => p.id === profileId);
    if (failed > 0) {
      toast.error(`${failed} series failed to update`);
    } else {
      toast.success(
        `${ids.length} series set to ${profile?.name ?? "profile"}`,
      );
    }
  };

  // Series update/delete from detail sheet
  const handleSeriesUpdated = (updated: Series) => {
    setSeriesList((prev) =>
      prev.map((s) => (s.id === updated.id ? updated : s)),
    );
    setDetailSeries(updated);
  };

  const handleSeriesDeleted = (id: string) => {
    setSeriesList((prev) => prev.filter((s) => s.id !== id));
    setSelectedIds((prev) => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };

  const openDetail = (series: Series) => {
    setDetailSeries(series);
    setDetailOpen(true);
  };

  const handleRefreshAll = async () => {
    setRefreshingAll(true);
    try {
      const res = await apiFetch("/api/v1/series/refresh", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = (await res.json()) as { count?: number };
      toast.success(
        `Refreshing ${data.count ?? seriesList.length} series in the background`,
      );
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh series",
      );
    } finally {
      setRefreshingAll(false);
    }
  };

  const handleRescanLibraries = async () => {
    setRescanningLibraries(true);
    try {
      const res = await apiFetch("/api/v1/series/rescan", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = (await res.json()) as { libraryCount?: number };
      toast.success(
        `Rescanning ${data.libraryCount ?? libraries.length} TV librar${(data.libraryCount ?? libraries.length) === 1 ? "y" : "ies"} in the background`,
      );
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to rescan TV libraries",
      );
    } finally {
      setRescanningLibraries(false);
    }
  };

  // Stats
  const totalSeries = seriesList.length;
  const monitoredCount = seriesList.filter(
    (s) => s.monitoringStatus === "monitored",
  ).length;
  const continuingCount = seriesList.filter(
    (s) => s.status === "continuing",
  ).length;

  const subtitle =
    totalSeries > 0
      ? `${totalSeries} series • ${monitoredCount} monitored • ${continuingCount} continuing`
      : undefined;
  useSetPageHeader("TV Shows", subtitle);

  return (
    <div className="px-6 pb-6 pt-2">
      {/* Toolbar */}
      {totalSeries > 0 ? (
        <SeriesToolbar
          filterText={filterText}
          onFilterTextChange={setFilterText}
          statusFilter={statusFilter}
          onStatusFilterChange={setStatusFilter}
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
          onAddSeries={() => setAddDialogOpen(true)}
          onImportLibrary={() => setImportDialogOpen(true)}
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
      ) : totalSeries === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="mb-6 flex h-20 w-20 items-center justify-center rounded-full bg-accent/10">
            <Tv className="h-10 w-10 text-accent" />
          </div>
          <h2 className="mb-2 text-xl font-semibold">No series yet</h2>
          <p className="mb-6 max-w-sm text-sm text-muted-foreground">
            Start building your library by adding TV series from TMDB.
          </p>
          {libraries.length === 0 ? (
            <p className="text-sm text-amber-500">
              ⚠️ Add a library in Settings before adding series
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
                <Plus className="mr-1.5 h-4 w-4" /> Add Series
              </Button>
            </div>
          )}
        </div>
      ) : viewMode === "grid" ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8">
          {processed.map((s) => (
            <div
              key={s.id}
              className={cn(
                s.monitoringStatus === "unmonitored" && "opacity-60",
              )}
            >
              <SeriesCard
                series={s}
                profiles={qualityProfiles}
                selected={selectedIds.has(s.id)}
                selectMode={selectMode}
                onToggleSelect={() => toggleSelect(s.id)}
                onClick={() => openDetail(s)}
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
                <TableHead className="w-28">Network</TableHead>
                <TableHead className="w-24">Seasons</TableHead>
                <TableHead className="w-28">Status</TableHead>
                <TableHead className="w-28">Quality</TableHead>
                <TableHead className="w-12">Mon.</TableHead>
                <TableHead className="w-16">Rating</TableHead>
                <TableHead className="w-24">Added</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {processed.map((s) => (
                <SeriesListRow
                  key={s.id}
                  series={s}
                  profiles={qualityProfiles}
                  selected={selectedIds.has(s.id)}
                  onToggleSelect={() => toggleSelect(s.id)}
                  onClick={() => openDetail(s)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {processed.length === 0 && totalSeries > 0 && !isLoading && (
        <div className="flex flex-col items-center justify-center py-16 text-muted-foreground">
          <Search className="mb-3 h-10 w-10 opacity-30" />
          <p className="text-sm">No series match the current filters</p>
        </div>
      )}

      {/* Dialogs & Sheets */}
      <AddSeriesDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        libraries={libraries}
        qualityProfiles={qualityProfiles}
        existingTmdbIds={existingTmdbIds}
        onSeriesAdded={fetchAll}
      />

      <SeriesDetailSheet
        series={detailSeries}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        profiles={qualityProfiles}
        libraries={libraries}
        onUpdated={handleSeriesUpdated}
        onDeleted={handleSeriesDeleted}
        onRefresh={fetchAll}
        existingSeriesIds={existingSeriesNumericIds}
      />

      <BulkDeleteDialog
        open={bulkDeleteOpen}
        onOpenChange={setBulkDeleteOpen}
        count={selectedIds.size}
        onConfirm={handleBulkDelete}
      />

      <SeriesLibraryImportDialog
        open={importDialogOpen}
        onOpenChange={setImportDialogOpen}
        libraries={libraries}
        onImportComplete={fetchAll}
      />
    </div>
  );
}
