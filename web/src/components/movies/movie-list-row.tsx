import { Checkbox } from "@/components/ui/checkbox";
import {
  TableRow,
  TableCell,
} from "@/components/ui/table";
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
  const profile = profiles.find(p => p.id === movie.qualityProfileId);
  return (
    <TableRow
      className="cursor-pointer hover:bg-accent/5 transition-colors"
      onClick={onClick}
    >
      <TableCell className="w-10" onClick={(e) => { e.stopPropagation(); onToggleSelect(); }}>
        <Checkbox checked={selected} />
      </TableCell>
      <TableCell className="w-12">
        <div className="w-8 aspect-[2/3] rounded overflow-hidden bg-muted shrink-0">
          {movie.posterPath ? (
            <img src={`${TMDB_IMG}/w92${movie.posterPath}`} alt="" className="w-full h-full object-cover" loading="lazy" />
          ) : (
            <div className="w-full h-full flex items-center justify-center"><Film className="w-3 h-3 text-muted-foreground/30" /></div>
          )}
        </div>
      </TableCell>
      <TableCell className="font-medium">{movie.title}</TableCell>
      <TableCell className="text-muted-foreground">{movie.year}</TableCell>
      <TableCell><StatusBadge status={movie.status} /></TableCell>
      <TableCell className="text-muted-foreground text-xs">{profile?.name ?? "—"}</TableCell>
      <TableCell>
        {movie.monitoringStatus === "monitored" ? (
          <Eye className="w-4 h-4 text-accent" />
        ) : (
          <EyeOff className="w-4 h-4 text-muted-foreground/50" />
        )}
      </TableCell>
      <TableCell>
        {movie.rating > 0 ? (
          <span className="flex items-center gap-1 text-xs">
            <Star className="w-3 h-3 text-yellow-400 fill-yellow-400" />{movie.rating.toFixed(1)}
          </span>
        ) : "—"}
      </TableCell>
      <TableCell className="text-muted-foreground text-xs">
        {movie.createdAt ? new Date(movie.createdAt).toLocaleDateString() : "—"}
      </TableCell>
    </TableRow>
  );
}
