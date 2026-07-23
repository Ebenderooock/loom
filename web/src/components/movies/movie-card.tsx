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
  const profile = profiles.find((p) => p.id === movie.qualityProfileId);
  const statusColor = STATUS_CONFIG[movie.status]?.border ?? "#6b7280";

  return (
    <div
      className={cn(
        "group relative cursor-pointer overflow-hidden rounded-lg border border-transparent shadow-lg transition-all duration-200 hover:-translate-y-1 hover:scale-[1.02] hover:border-accent/10 hover:shadow-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-background",
        selected && "ring-2 ring-accent ring-offset-2 ring-offset-background",
      )}
      tabIndex={0}
      role="button"
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onClick();
        }
      }}
    >
      {/* Status accent — thin gradient bar at top */}
      <div
        className="absolute left-0 right-0 top-0 z-10 h-[3px]"
        style={{
          background: `linear-gradient(90deg, ${statusColor}, ${statusColor}cc 60%, transparent)`,
        }}
      />
      <div
        className="pointer-events-none absolute left-0 right-0 top-[3px] z-10 h-3"
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
            className="h-full w-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center">
            <Film className="h-12 w-12 text-muted-foreground/30" />
          </div>
        )}
      </div>

      {/* Checkbox */}
      {(selectMode || selected) && (
        <div className="absolute left-2 top-2 z-10">
          <Checkbox
            checked={selected}
            onClick={(e) => e.stopPropagation()}
            onCheckedChange={() => onToggleSelect()}
            className="h-5 w-5 border-white/60 data-[state=checked]:border-accent data-[state=checked]:bg-accent"
          />
        </div>
      )}
      {!selectMode && !selected && (
        <div className="absolute left-2 top-2 z-10 opacity-0 transition-opacity duration-200 group-hover:opacity-100">
          <Checkbox
            checked={false}
            onClick={(e) => e.stopPropagation()}
            onCheckedChange={() => onToggleSelect()}
            className="h-5 w-5 border-white/60"
          />
        </div>
      )}

      {/* Status bar at bottom */}
      <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/95 via-black/70 to-transparent p-3 pt-8">
        <div className="flex items-center justify-between gap-1">
          <StatusBadge status={movie.status} />
          {movie.monitoringStatus === "unmonitored" && (
            <span className="text-[10px] uppercase tracking-wider text-gray-500">
              Unmonitored
            </span>
          )}
        </div>
        <h3 className="mt-1 truncate text-sm font-semibold text-white">
          {movie.title}
        </h3>
        <p className="text-xs text-gray-400">{movie.year}</p>
      </div>

      {/* Hover overlay with details */}
      <div className="pointer-events-none absolute inset-0 flex flex-col justify-between bg-black/80 p-4 opacity-0 backdrop-blur-sm transition-opacity duration-200 group-hover:opacity-100">
        <div>
          <h3 className="text-sm font-bold text-white">{movie.title}</h3>
          <p className="mt-0.5 text-xs text-gray-400">
            {movie.year} • {movie.runtime ? `${movie.runtime}m` : "—"}
          </p>
          {movie.rating > 0 && (
            <div className="mt-1 flex items-center gap-1">
              <Star className="h-3 w-3 fill-yellow-400 text-yellow-400" />
              <span className="text-xs text-yellow-400">
                {movie.rating.toFixed(1)}
              </span>
            </div>
          )}
          {movie.genres?.length > 0 && (
            <p className="mt-1.5 text-[10px] text-gray-500">
              {movie.genres.slice(0, 3).join(" • ")}
            </p>
          )}
          <p className="mt-2 line-clamp-4 text-xs leading-relaxed text-gray-300">
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
