import { Checkbox } from "@/components/ui/checkbox";
import { TableRow, TableCell } from "@/components/ui/table";
import { Tv, Star, Eye, EyeOff } from "lucide-react";
import { SeriesStatusBadge } from "./series-status-badge";
import type { Series, QualityProfile } from "./types";
import { TMDB_IMG } from "./types";

export function SeriesListRow({
  series,
  profiles,
  selected,
  onToggleSelect,
  onClick,
}: {
  series: Series;
  profiles: QualityProfile[];
  selected: boolean;
  onToggleSelect: () => void;
  onClick: () => void;
}) {
  const profile = profiles.find((p) => p.id === series.qualityProfileId);
  const seasonCount = series.seasons?.length ?? 0;

  return (
    <TableRow
      className="hover:bg-accent/5 cursor-pointer transition-colors"
      onClick={onClick}
    >
      <TableCell
        className="w-10"
        onClick={(e) => {
          e.stopPropagation();
          onToggleSelect();
        }}
      >
        <Checkbox checked={selected} />
      </TableCell>
      <TableCell className="w-12">
        <div className="bg-muted aspect-[2/3] w-8 shrink-0 overflow-hidden rounded">
          {series.posterPath ? (
            <img
              src={`${TMDB_IMG}/w92${series.posterPath}`}
              alt=""
              className="h-full w-full object-cover"
              loading="lazy"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center">
              <Tv className="text-muted-foreground/30 h-3 w-3" />
            </div>
          )}
        </div>
      </TableCell>
      <TableCell className="font-medium">{series.title}</TableCell>
      <TableCell className="text-muted-foreground">
        {series.network || "—"}
      </TableCell>
      <TableCell className="text-muted-foreground text-xs">
        {seasonCount > 0 ? `${seasonCount} seasons` : "—"}
      </TableCell>
      <TableCell>
        <SeriesStatusBadge status={series.status} />
      </TableCell>
      <TableCell className="text-muted-foreground text-xs">
        {profile?.name ?? "—"}
      </TableCell>
      <TableCell>
        {series.monitoringStatus === "monitored" ? (
          <Eye className="text-accent h-4 w-4" />
        ) : (
          <EyeOff className="text-muted-foreground/50 h-4 w-4" />
        )}
      </TableCell>
      <TableCell>
        {series.rating > 0 ? (
          <span className="flex items-center gap-1 text-xs">
            <Star className="h-3 w-3 fill-yellow-400 text-yellow-400" />
            {series.rating.toFixed(1)}
          </span>
        ) : (
          "—"
        )}
      </TableCell>
      <TableCell className="text-muted-foreground text-xs">
        {series.createdAt
          ? new Date(series.createdAt).toLocaleDateString()
          : "—"}
      </TableCell>
    </TableRow>
  );
}
