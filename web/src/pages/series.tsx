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
import type { Series, QualityProfile, SeriesSortKey, ViewMode } from "@/components/series";

// ─── Skeletons ──────────────────────────────────────────────────────────

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
          <DialogTitle>Delete {count} Series</DialogTitle>
          <DialogDescription>
            This will remove {count} series from your library. This cannot be undone.
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

export function SeriesPage() {
  const { isAuthenticated } = useAuth();
  const [seriesList, setSeriesList] = useState<Series[]>([]);
  const { data: allLibraries = [] } = useLibraries();
  const libraries = allLibraries.filter(l => l.media_type === "series");
  const [qualityProfiles, setQualityProfiles] = useState<QualityProfile[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);

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

  const fetchAll = useCallback(async () => {
    if (!isAuthenticated) return;
    setIsLoading(true);
    try {
      const [seriesRes, profilesRes] = await Promise.all([
        fetch("/api/v1/series", { credentials: "include" }),
        fetch("/api/v1/quality-profiles", { credentials: "include" }),
      ]);
      if (seriesRes.ok) {
        const data = await seriesRes.json();
        setSeriesList(Array.isArray(data) ? data : data.data ?? []);
      }
      if (profilesRes.ok) {
        const data = await profilesRes.json();
        const profiles = data?.data ?? (Array.isArray(data) ? data : []);
        setQualityProfiles(profiles);
      }
    } catch { /* ignore */ } finally { setIsLoading(false); }
  }, [isAuthenticated]);

  useEffect(() => { fetchAll(); }, [fetchAll]);

  const existingTmdbIds = useMemo(
    () => new Set(seriesList.map(s => s.tmdbId).filter(Boolean) as string[]),
    [seriesList],
  );

  // Filter + sort pipeline
  const processed = useMemo(() => {
    let list = seriesList;
    if (filterText) {
      const q = filterText.toLowerCase();
      list = list.filter(s => s.title.toLowerCase().includes(q));
    }
    if (statusFilter !== "all") {
      list = list.filter(s => s.status === statusFilter);
    }
    if (monitoredFilter === "monitored") {
      list = list.filter(s => s.monitoringStatus === "monitored");
    } else if (monitoredFilter === "unmonitored") {
      list = list.filter(s => s.monitoringStatus === "unmonitored");
    } else if (monitoredFilter === "archived") {
      list = list.filter(s => s.monitoringStatus === "unmonitored");
    }
    return sortSeries(list, sortKey);
  }, [seriesList, filterText, statusFilter, monitoredFilter, sortKey]);

  // Selection helpers
  const selectMode = selectedIds.size > 0;
  const allSelected = processed.length > 0 && processed.every(s => selectedIds.has(s.id));

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
      setSelectedIds(new Set(processed.map(s => s.id)));
    }
  };

  const clearSelection = () => setSelectedIds(new Set());

  // Bulk actions
  const handleBulkMonitoring = async (status: "monitored" | "unmonitored") => {
    const ids = Array.from(selectedIds);
    await Promise.all(ids.map(id =>
      fetch(`/api/v1/series/${id}/monitoring`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ status }),
      }),
    ));
    setSeriesList(prev => prev.map(s => selectedIds.has(s.id) ? { ...s, monitoringStatus: status } : s));
    clearSelection();
    toast.success(`${ids.length} series set to ${status}`);
  };

  const handleBulkDelete = async () => {
    const ids = Array.from(selectedIds);
    await Promise.all(ids.map(id => fetch(`/api/v1/series/${id}`, { method: "DELETE", credentials: "include" })));
    setSeriesList(prev => prev.filter(s => !selectedIds.has(s.id)));
    clearSelection();
    toast.success(`${ids.length} series deleted`);
  };

  // Series update/delete from detail sheet
  const handleSeriesUpdated = (updated: Series) => {
    setSeriesList(prev => prev.map(s => s.id === updated.id ? updated : s));
    setDetailSeries(updated);
  };

  const handleSeriesDeleted = (id: string) => {
    setSeriesList(prev => prev.filter(s => s.id !== id));
    setSelectedIds(prev => { const next = new Set(prev); next.delete(id); return next; });
  };

  const openDetail = (series: Series) => {
    setDetailSeries(series);
    setDetailOpen(true);
  };

  // Stats
  const totalSeries = seriesList.length;
  const monitoredCount = seriesList.filter(s => s.monitoringStatus === "monitored").length;
  const continuingCount = seriesList.filter(s => s.status === "continuing").length;

  const subtitle = totalSeries > 0
    ? `${totalSeries} series • ${monitoredCount} monitored • ${continuingCount} continuing`
    : undefined;
  useSetPageHeader("TV Shows", subtitle);

  return (
    <div className="px-6 pt-2 pb-6">
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
          onAddSeries={() => setAddDialogOpen(true)}
          onImportLibrary={() => setImportDialogOpen(true)}
        />
      ) : null}

      {/* Content */}
      {isLoading ? (
        viewMode === "grid" ? <GridSkeletons /> : <ListSkeletons />
      ) : totalSeries === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 text-center">
          <div className="w-20 h-20 rounded-full bg-accent/10 flex items-center justify-center mb-6">
            <Tv className="w-10 h-10 text-accent" />
          </div>
          <h2 className="text-xl font-semibold mb-2">No series yet</h2>
          <p className="text-sm text-muted-foreground max-w-sm mb-6">
            Start building your library by adding TV series from TMDB.
          </p>
          {libraries.length === 0 ? (
            <p className="text-sm text-amber-500">⚠️ Add a library in Settings before adding series</p>
          ) : (
            <div className="flex gap-3">
              <Button variant="outline" size="lg" onClick={() => setImportDialogOpen(true)}>
                <FolderSearch className="w-4 h-4 mr-1.5" /> Import Existing
              </Button>
              <Button onClick={() => setAddDialogOpen(true)} size="lg">
                <Plus className="w-4 h-4 mr-1.5" /> Add Series
              </Button>
            </div>
          )}
        </div>
      ) : viewMode === "grid" ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-8 gap-4">
          {processed.map(s => (
            <div key={s.id} className={cn(s.monitoringStatus === "unmonitored" && "opacity-60")}>
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
        <div className="border border-border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox checked={allSelected} onCheckedChange={toggleSelectAll} />
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
              {processed.map(s => (
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
          <Search className="w-10 h-10 mb-3 opacity-30" />
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
