import { Checkbox } from "@/components/ui/checkbox";
import { TableRow, TableCell } from "@/components/ui/table";
import { Film, Star, Eye, EyeOff } from "lucide-react";
import { StatusBadge } from "./status-badge";
import type { Movie, QualityProfile } from "./types";
import { TMDB_IMG } from "./types";

export function MovieListRow({
  movie,
  profiles,
  selected,
  onToggleSelect,
  onClick,
}: {
  movie: Movie;
  profiles: QualityProfile[];
  selected: boolean;
  onToggleSelect: () => void;
  onClick: () => void;
}) {
  const profile = profiles.find((p) => p.id === movie.qualityProfileId);
  return (
    <TableRow
      className="cursor-pointer transition-colors hover:bg-accent/5"
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
        <div className="aspect-[2/3] w-8 shrink-0 overflow-hidden rounded bg-muted">
          {movie.posterPath ? (
            <img
              src={`${TMDB_IMG}/w92${movie.posterPath}`}
              alt=""
              className="h-full w-full object-cover"
              loading="lazy"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center">
              <Film className="h-3 w-3 text-muted-foreground/30" />
            </div>
          )}
        </div>
      </TableCell>
      <TableCell className="font-medium">{movie.title}</TableCell>
      <TableCell className="text-muted-foreground">{movie.year}</TableCell>
      <TableCell>
        <StatusBadge status={movie.status} />
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {profile?.name ?? "—"}
      </TableCell>
      <TableCell>
        {movie.monitoringStatus === "monitored" ? (
          <Eye className="h-4 w-4 text-accent" />
        ) : (
          <EyeOff className="h-4 w-4 text-muted-foreground/50" />
        )}
      </TableCell>
      <TableCell>
        {movie.rating > 0 ? (
          <span className="flex items-center gap-1 text-xs">
            <Star className="h-3 w-3 fill-yellow-400 text-yellow-400" />
            {movie.rating.toFixed(1)}
          </span>
        ) : (
          "—"
        )}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {movie.createdAt ? new Date(movie.createdAt).toLocaleDateString() : "—"}
      </TableCell>
    </TableRow>
  );
}
