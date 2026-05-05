import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Plus, Search, Grid3X3, List, SortAsc, Eye, EyeOff, Trash2, X,
} from "lucide-react";
import type { QualityProfile, SeriesSortKey, ViewMode } from "./types";
import { SERIES_STATUS_OPTIONS, SERIES_STATUS_CONFIG, SERIES_SORT_OPTIONS } from "./types";

export function SeriesToolbar({
  filterText,
  onFilterTextChange,
  statusFilter,
  onStatusFilterChange,
  monitoredFilter,
  onMonitoredFilterChange,
  sortKey,
  onSortKeyChange,
  viewMode,
  onViewModeChange,
  profiles,
  selectMode,
  selectedCount,
  allSelected,
  onToggleSelectAll,
  onClearSelection,
  onBulkMonitor,
  onBulkUnmonitor,
  onBulkDelete,
  onAddSeries,
}: {
  filterText: string;
  onFilterTextChange: (v: string) => void;
  statusFilter: string;
  onStatusFilterChange: (v: string) => void;
  monitoredFilter: string;
  onMonitoredFilterChange: (v: string) => void;
  sortKey: SeriesSortKey;
  onSortKeyChange: (v: SeriesSortKey) => void;
  viewMode: ViewMode;
  onViewModeChange: (v: ViewMode) => void;
  profiles: QualityProfile[];
  selectMode: boolean;
  selectedCount: number;
  allSelected: boolean;
  onToggleSelectAll: () => void;
  onClearSelection: () => void;
  onBulkMonitor: () => void;
  onBulkUnmonitor: () => void;
  onBulkDelete: () => void;
  onAddSeries: () => void;
}) {
  return (
    <div className="mb-6 space-y-3">
      <div className="flex items-center gap-3 flex-wrap">
        {/* Filter */}
        <div className="relative flex-1 min-w-[200px] max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <Input
            placeholder="Filter series..."
            value={filterText}
            onChange={(e) => onFilterTextChange(e.target.value)}
            className="pl-9 h-9"
          />
        </div>

        {/* Status filter */}
        <Select value={statusFilter} onValueChange={onStatusFilterChange}>
          <SelectTrigger className="w-[140px] h-9 text-xs">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            {SERIES_STATUS_OPTIONS.map(s => (
              <SelectItem key={s} value={s} className="text-xs">
                {s === "all" ? "All Statuses" : (SERIES_STATUS_CONFIG[s]?.label ?? s)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Monitored filter */}
        <Select value={monitoredFilter} onValueChange={onMonitoredFilterChange}>
          <SelectTrigger className="w-[130px] h-9 text-xs">
            <SelectValue placeholder="Monitored" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all" className="text-xs">All</SelectItem>
            <SelectItem value="monitored" className="text-xs">Monitored</SelectItem>
            <SelectItem value="unmonitored" className="text-xs">Unmonitored</SelectItem>
          </SelectContent>
        </Select>

        {/* Sort */}
        <Select value={sortKey} onValueChange={(v) => onSortKeyChange(v as SeriesSortKey)}>
          <SelectTrigger className="w-[140px] h-9 text-xs">
            <SortAsc className="w-3.5 h-3.5 mr-1" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SERIES_SORT_OPTIONS.map(o => (
              <SelectItem key={o.value} value={o.value} className="text-xs">{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* View toggle */}
        <div className="flex items-center border border-border rounded-md">
          <Button
            variant={viewMode === "grid" ? "secondary" : "ghost"}
            size="sm"
            className="h-9 w-9 p-0 rounded-r-none"
            onClick={() => onViewModeChange("grid")}
          >
            <Grid3X3 className="w-4 h-4" />
          </Button>
          <Button
            variant={viewMode === "list" ? "secondary" : "ghost"}
            size="sm"
            className="h-9 w-9 p-0 rounded-l-none"
            onClick={() => onViewModeChange("list")}
          >
            <List className="w-4 h-4" />
          </Button>
        </div>

        {/* Add button */}
        <div className="flex items-center gap-2 ml-auto">
          <Button size="sm" className="h-9 gap-1.5" onClick={onAddSeries}>
            <Plus className="w-4 h-4" /> Add Series
          </Button>
        </div>
      </div>

      {/* Bulk action bar */}
      {selectMode && (
        <div className="flex items-center gap-3 px-3 py-2 rounded-lg bg-accent/10 border border-accent/20">
          <Checkbox
            checked={allSelected}
            onCheckedChange={onToggleSelectAll}
            className="data-[state=checked]:bg-accent"
          />
          <span className="text-sm font-medium">{selectedCount} selected</span>
          <div className="flex gap-2 ml-auto">
            <Button size="sm" variant="outline" className="h-7 text-xs gap-1" onClick={onBulkMonitor}>
              <Eye className="w-3.5 h-3.5" /> Monitor
            </Button>
            <Button size="sm" variant="outline" className="h-7 text-xs gap-1" onClick={onBulkUnmonitor}>
              <EyeOff className="w-3.5 h-3.5" /> Unmonitor
            </Button>
            <Button size="sm" variant="destructive" className="h-7 text-xs gap-1" onClick={onBulkDelete}>
              <Trash2 className="w-3.5 h-3.5" /> Delete
            </Button>
            <Button size="sm" variant="ghost" className="h-7 text-xs" onClick={onClearSelection}>
              <X className="w-3.5 h-3.5" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
