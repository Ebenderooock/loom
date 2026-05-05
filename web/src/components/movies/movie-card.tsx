import { Checkbox } from "@/components/ui/checkbox";
import { Film, Star } from "lucide-react";
import { cn } from "@/lib/utils";
import { StatusBadge } from "./status-badge";
import type { Movie, QualityProfile } from "./types";
import { TMDB_IMG, STATUS_CONFIG } from "./types";

export function MovieCard({
  movie,
  profiles,
  selected,
  selectMode,
  onToggleSelect,
  onClick,
}: {
  movie: Movie;
  profiles: QualityProfile[];
  selected: boolean;
  selectMode: boolean;
  onToggleSelect: () => void;
  onClick: () => void;
}) {
  const profile = profiles.find(p => p.id === movie.qualityProfileId);
  const statusColor = STATUS_CONFIG[movie.status]?.border ?? "#6b7280";

  return (
    <div
      className={cn(
        "group relative rounded-lg overflow-hidden shadow-lg transition-all duration-200 hover:scale-[1.03] hover:shadow-xl cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        selected && "ring-2 ring-accent ring-offset-2 ring-offset-background",
      )}
      tabIndex={0}
      role="button"
      onClick={onClick}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onClick(); } }}
    >
      {/* Status accent — thin gradient bar at top */}
      <div
        className="absolute top-0 left-0 right-0 h-[3px] z-10"
        style={{
          background: `linear-gradient(90deg, ${statusColor}, ${statusColor}cc 60%, transparent)`,
        }}
      />
      <div
        className="absolute top-[3px] left-0 right-0 h-3 z-10 pointer-events-none"
        style={{
          background: `linear-gradient(180deg, ${statusColor}25, transparent)`,
        }}
      />
      {/* Poster */}
      <div className="aspect-[2/3] bg-muted">
        {movie.posterPath ? (
          <img
            src={`${TMDB_IMG}/w300${movie.posterPath}`}
            alt={movie.title}
            className="w-full h-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center">
            <Film className="w-12 h-12 text-muted-foreground/30" />
          </div>
        )}
      </div>

      {/* Checkbox */}
      {(selectMode || selected) && (
        <div
          className="absolute top-2 left-2 z-10"
          onClick={(e) => { e.stopPropagation(); onToggleSelect(); }}
        >
          <Checkbox checked={selected} className="h-5 w-5 border-white/60 data-[state=checked]:bg-accent data-[state=checked]:border-accent" />
        </div>
      )}
      {!selectMode && !selected && (
        <div
          className="absolute top-2 left-2 z-10 opacity-0 group-hover:opacity-100 transition-opacity duration-200"
          onClick={(e) => { e.stopPropagation(); onToggleSelect(); }}
        >
          <Checkbox checked={false} className="h-5 w-5 border-white/60" />
        </div>
      )}

      {/* Status bar at bottom */}
      <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/90 via-black/60 to-transparent p-3 pt-8">
        <div className="flex items-center justify-between gap-1">
          <StatusBadge status={movie.status} />
          {movie.monitoringStatus === "unmonitored" && (
            <span className="text-[10px] text-gray-500 uppercase tracking-wider">Unmonitored</span>
          )}
        </div>
        <h3 className="text-sm font-semibold text-white truncate mt-1">{movie.title}</h3>
        <p className="text-xs text-gray-400">{movie.year}</p>
      </div>

      {/* Hover overlay with details */}
      <div className="absolute inset-0 bg-black/85 p-4 flex flex-col justify-between opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none">
        <div>
          <h3 className="text-sm font-bold text-white">{movie.title}</h3>
          <p className="text-xs text-gray-400 mt-0.5">{movie.year} • {movie.runtime ? `${movie.runtime}m` : "—"}</p>
          {movie.rating > 0 && (
            <div className="flex items-center gap-1 mt-1">
              <Star className="w-3 h-3 text-yellow-400 fill-yellow-400" />
              <span className="text-xs text-yellow-400">{movie.rating.toFixed(1)}</span>
            </div>
          )}
          {movie.genres?.length > 0 && (
            <p className="text-[10px] text-gray-500 mt-1.5">{movie.genres.slice(0, 3).join(" • ")}</p>
          )}
          <p className="text-xs text-gray-300 mt-2 line-clamp-4 leading-relaxed">
            {movie.overview || "No overview available."}
          </p>
        </div>
        <div className="space-y-1.5">
          <StatusBadge status={movie.status} />
          {profile && (
            <p className="text-[10px] text-gray-500">Quality: {profile.name}</p>
          )}
        </div>
      </div>
    </div>
  );
}
