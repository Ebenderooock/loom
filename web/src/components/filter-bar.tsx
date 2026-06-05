import { useState, useEffect, useRef } from "react";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Search, X } from "lucide-react";

interface FilterBarProps {
  searchText: string;
  onSearchChange: (text: string) => void;
  statusFilter: string;
  onStatusChange: (status: string) => void;
  statusOptions: readonly string[];
  statusLabels?: Record<string, string>;
  monitoredFilter: string;
  onMonitoredChange: (v: string) => void;
  sortKey: string;
  onSortChange: (key: string) => void;
  sortOptions: { value: string; label: string }[];
  children?: React.ReactNode;
}

const MONITORED_OPTIONS = [
  { value: "all", label: "All" },
  { value: "monitored", label: "Monitored" },
  { value: "unmonitored", label: "Unmonitored" },
  { value: "archived", label: "Archived" },
];

export function FilterBar({
  searchText,
  onSearchChange,
  statusFilter,
  onStatusChange,
  statusOptions,
  statusLabels,
  monitoredFilter,
  onMonitoredChange,
  sortKey,
  onSortChange,
  sortOptions,
  children,
}: FilterBarProps) {
  const [localSearch, setLocalSearch] = useState(searchText);
  const timerRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    setLocalSearch(searchText);
  }, [searchText]);

  const handleSearchInput = (value: string) => {
    setLocalSearch(value);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      onSearchChange(value);
    }, 300);
  };

  // Determine active (non-default) filters for pill display
  const pills: { label: string; onClear: () => void }[] = [];
  if (statusFilter !== "all") {
    pills.push({
      label: `Status: ${statusLabels?.[statusFilter] ?? statusFilter}`,
      onClear: () => onStatusChange("all"),
    });
  }
  if (monitoredFilter !== "all") {
    const monLabel =
      MONITORED_OPTIONS.find((o) => o.value === monitoredFilter)?.label ??
      monitoredFilter;
    pills.push({
      label: monLabel,
      onClear: () => onMonitoredChange("all"),
    });
  }
  if (searchText) {
    pills.push({
      label: `"${searchText}"`,
      onClear: () => {
        onSearchChange("");
        setLocalSearch("");
      },
    });
  }

  return (
    <div className="mb-4 space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        {/* Search */}
        <div className="relative min-w-[200px] max-w-xs flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={localSearch}
            onChange={(e) => handleSearchInput(e.target.value)}
            placeholder="Search..."
            className="h-9 pl-8 text-sm"
          />
        </div>

        {/* Status */}
        <Select value={statusFilter} onValueChange={onStatusChange}>
          <SelectTrigger className="h-9 w-[140px] text-sm">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Statuses</SelectItem>
            {statusOptions
              .filter((s) => s !== "all")
              .map((s) => (
                <SelectItem key={s} value={s}>
                  {statusLabels?.[s] ?? s.charAt(0).toUpperCase() + s.slice(1)}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>

        {/* Monitored */}
        <Select value={monitoredFilter} onValueChange={onMonitoredChange}>
          <SelectTrigger className="h-9 w-[150px] text-sm">
            <SelectValue placeholder="Monitored" />
          </SelectTrigger>
          <SelectContent>
            {MONITORED_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Sort */}
        <Select value={sortKey} onValueChange={onSortChange}>
          <SelectTrigger className="h-9 w-[160px] text-sm">
            <SelectValue placeholder="Sort by" />
          </SelectTrigger>
          <SelectContent>
            {sortOptions.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {children}
      </div>

      {/* Active filter pills */}
      {pills.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {pills.map((pill) => (
            <Badge
              key={pill.label}
              variant="secondary"
              className="cursor-pointer gap-1 text-xs hover:bg-destructive/10"
              onClick={pill.onClear}
            >
              {pill.label}
              <X className="h-3 w-3" />
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}
