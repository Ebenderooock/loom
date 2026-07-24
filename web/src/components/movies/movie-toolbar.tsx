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
  Plus,
  Search,
  Grid3X3,
  List,
  SortAsc,
  Eye,
  EyeOff,
  RefreshCw,
  Trash2,
  X,
  FolderSearch,
  FolderSync,
  Settings2,
  MoreHorizontal,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { QualityProfile, SortKey, ViewMode } from "./types";
import { STATUS_OPTIONS, STATUS_CONFIG, SORT_OPTIONS } from "./types";

export function MovieToolbar({
  filterText,
  onFilterTextChange,
  statusFilter,
  onStatusFilterChange,
  profileFilter,
  onProfileFilterChange,
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
  onBulkQualityProfile,
  onAddMovie,
  onImportLibrary,
  onOrganize,
  onRefreshAll,
  onRescanLibraries,
  refreshingAll,
  rescanningLibraries,
}: {
  filterText: string;
  onFilterTextChange: (v: string) => void;
  statusFilter: string;
  onStatusFilterChange: (v: string) => void;
  profileFilter: string;
  onProfileFilterChange: (v: string) => void;
  monitoredFilter: string;
  onMonitoredFilterChange: (v: string) => void;
  sortKey: SortKey;
  onSortKeyChange: (v: SortKey) => void;
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
  onBulkQualityProfile: (profileId: string) => void;
  onAddMovie: () => void;
  onImportLibrary: () => void;
  onOrganize: () => void;
  onRefreshAll: () => void;
  onRescanLibraries: () => void;
  refreshingAll: boolean;
  rescanningLibraries: boolean;
}) {
  return (
    <div className="mb-6 space-y-3">
      {/* Main toolbar row */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Filter */}
        <div className="relative w-full sm:min-w-[220px] sm:max-w-sm sm:flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Filter movies..."
            value={filterText}
            onChange={(e) => onFilterTextChange(e.target.value)}
            className="h-9 pl-9"
          />
        </div>

        {/* Status filter */}
        <Select value={statusFilter} onValueChange={onStatusFilterChange}>
          <SelectTrigger className="h-9 w-full text-xs sm:w-[140px]">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((s) => (
              <SelectItem key={s} value={s} className="text-xs">
                {s === "all" ? "All Statuses" : (STATUS_CONFIG[s]?.label ?? s)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Quality profile filter */}
        <Select value={profileFilter} onValueChange={onProfileFilterChange}>
          <SelectTrigger className="h-9 w-full text-xs sm:w-[140px]">
            <SelectValue placeholder="Profile" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all" className="text-xs">
              All Profiles
            </SelectItem>
            {profiles.map((p) => (
              <SelectItem key={p.id} value={p.id} className="text-xs">
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Monitored filter */}
        <Select value={monitoredFilter} onValueChange={onMonitoredFilterChange}>
          <SelectTrigger className="h-9 w-full text-xs sm:w-[130px]">
            <SelectValue placeholder="Monitored" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all" className="text-xs">
              All
            </SelectItem>
            <SelectItem value="monitored" className="text-xs">
              Monitored
            </SelectItem>
            <SelectItem value="unmonitored" className="text-xs">
              Unmonitored
            </SelectItem>
          </SelectContent>
        </Select>

        {/* Sort */}
        <Select
          value={sortKey}
          onValueChange={(v) => onSortKeyChange(v as SortKey)}
        >
          <SelectTrigger className="h-9 w-full text-xs sm:w-[140px]">
            <SortAsc className="mr-1 h-3.5 w-3.5" />
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SORT_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value} className="text-xs">
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* View toggle */}
        <div className="hidden items-center rounded-md border border-border sm:flex">
          <Button
            variant={viewMode === "grid" ? "secondary" : "ghost"}
            size="sm"
            className="h-9 w-9 rounded-r-none p-0"
            onClick={() => onViewModeChange("grid")}
          >
            <Grid3X3 className="h-4 w-4" />
          </Button>
          <Button
            variant={viewMode === "list" ? "secondary" : "ghost"}
            size="sm"
            className="h-9 w-9 rounded-l-none p-0"
            onClick={() => onViewModeChange("list")}
          >
            <List className="h-4 w-4" />
          </Button>
        </div>

        {/* Add button and actions */}
        <div className="flex w-full items-center gap-2 sm:ml-auto sm:w-auto">
          <Button
            size="sm"
            className="h-9 flex-1 gap-1.5 sm:flex-none"
            onClick={onAddMovie}
          >
            <Plus className="h-4 w-4" /> Add Movie
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" className="h-9 px-3 sm:hidden">
                <MoreHorizontal className="h-4 w-4" />
                <span className="sr-only">More actions</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" sideOffset={4}>
              <DropdownMenuItem onClick={onRefreshAll} disabled={refreshingAll}>
                {refreshingAll ? "Refreshing..." : "Refresh All"}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={onRescanLibraries}
                disabled={rescanningLibraries}
              >
                {rescanningLibraries ? "Rescanning..." : "Rescan Libraries"}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={onImportLibrary}>Import</DropdownMenuItem>
              <DropdownMenuItem onClick={onOrganize}>Organize</DropdownMenuItem>
              <DropdownMenuItem onClick={() => onViewModeChange("grid")}>
                Grid View
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => onViewModeChange("list")}>
                List View
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <div className="hidden items-center gap-2 sm:flex">
            <Button
              size="sm"
              variant="outline"
              className="h-9 gap-1.5"
              onClick={onRefreshAll}
              disabled={refreshingAll}
            >
              <RefreshCw className="h-4 w-4" />
              {refreshingAll ? "Refreshing..." : "Refresh All"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-9 gap-1.5"
              onClick={onRescanLibraries}
              disabled={rescanningLibraries}
            >
              <FolderSync className="h-4 w-4" />
              {rescanningLibraries ? "Rescanning..." : "Rescan Libraries"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-9 gap-1.5"
              onClick={onImportLibrary}
            >
              <FolderSearch className="h-4 w-4" /> Import
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-9 gap-1.5"
              onClick={onOrganize}
            >
              <FolderSync className="h-4 w-4" /> Organize
            </Button>
          </div>
        </div>
      </div>

      {/* Bulk action bar */}
      {selectMode && (
        <div className="flex flex-wrap items-center gap-3 rounded-lg border border-accent/20 bg-accent/10 px-3 py-2">
          <Checkbox
            checked={allSelected}
            onCheckedChange={onToggleSelectAll}
            className="data-[state=checked]:bg-accent"
          />
          <span className="text-sm font-medium">{selectedCount} selected</span>
          <div className="flex w-full flex-wrap gap-2 sm:ml-auto sm:w-auto">
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1 text-xs"
              onClick={onBulkMonitor}
            >
              <Eye className="h-3.5 w-3.5" /> Monitor
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="h-7 gap-1 text-xs"
              onClick={onBulkUnmonitor}
            >
              <EyeOff className="h-3.5 w-3.5" /> Unmonitor
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 gap-1 text-xs"
                >
                  <Settings2 className="h-3.5 w-3.5" /> Quality Profile
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {profiles.map((p) => (
                  <DropdownMenuItem
                    key={p.id}
                    onClick={() => onBulkQualityProfile(p.id)}
                  >
                    {p.name}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              size="sm"
              variant="destructive"
              className="h-7 gap-1 text-xs"
              onClick={onBulkDelete}
            >
              <Trash2 className="h-3.5 w-3.5" /> Delete
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="h-7 text-xs"
              onClick={onClearSelection}
            >
              <X className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
