import { Checkbox } from "@/components/ui/checkbox";
import { Tv, Star } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Series, QualityProfile } from "./types";
import { TMDB_IMG } from "./types";

function getEpisodeProgressInfo(series: Series) {
  const stats = series.episodeStats;
  if (!stats || stats.airedEpisodes === 0) {
    return { percent: 0, downloaded: 0, total: 0, color: "#6b7280", label: "No episodes" };
  }
  const downloaded = stats.downloadedEpisodes;
  const total = stats.airedEpisodes;
  const percent = total > 0 ? Math.round((downloaded / total) * 100) : 0;

  let color: string;
  if (downloaded === 0) {
    color = "#ef4444"; // red – nothing downloaded
  } else if (downloaded < total) {
    color = "#f59e0b"; // amber – partial
  } else {
    color = "#10b981"; // green – all downloaded
  }

  return { percent, downloaded, total, color, label: `${downloaded}/${total}` };
}

export function SeriesCard({
  series,
  profiles,
  selected,
  selectMode,
  onToggleSelect,
  onClick,
}: {
  series: Series;
  profiles: QualityProfile[];
  selected: boolean;
  selectMode: boolean;
  onToggleSelect: () => void;
  onClick: () => void;
}) {
  const profile = profiles.find(p => p.id === series.qualityProfileId);
  const progress = getEpisodeProgressInfo(series);
  const seasonCount = series.seasons?.length ?? 0;

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
      {/* Episode progress bar at top */}
      <div className="absolute top-0 left-0 right-0 h-[3px] z-10 bg-gray-700/50">
        <div
          className="h-full transition-all duration-300"
          style={{ width: `${progress.percent}%`, backgroundColor: progress.color }}
        />
      </div>
      <div
        className="absolute top-[3px] left-0 right-0 h-3 z-10 pointer-events-none"
        style={{ background: `linear-gradient(180deg, ${progress.color}25, transparent)` }}
      />

      {/* Poster */}
      <div className="aspect-[2/3] bg-muted">
        {series.posterPath ? (
          <img
            src={`${TMDB_IMG}/w300${series.posterPath}`}
            alt={series.title}
            className="w-full h-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center">
            <Tv className="w-12 h-12 text-muted-foreground/30" />
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

      {/* Season count badge */}
      {seasonCount > 0 && (
        <div className="absolute top-2 right-2 z-10 bg-black/60 text-white text-[10px] font-medium px-1.5 py-0.5 rounded">
          {seasonCount} {seasonCount === 1 ? "Season" : "Seasons"}
        </div>
      )}

      {/* Status bar at bottom */}
      <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/90 via-black/60 to-transparent p-3 pt-8">
        <div className="flex items-center justify-between gap-1">
          <span className="text-[10px] font-medium px-1.5 py-0.5 rounded" style={{ color: progress.color, backgroundColor: `${progress.color}20` }}>
            {progress.total > 0 ? `${progress.label} episodes` : series.status}
          </span>
          {series.monitoringStatus === "unmonitored" && (
            <span className="text-[10px] text-gray-500 uppercase tracking-wider">Unmonitored</span>
          )}
        </div>
        <h3 className="text-sm font-semibold text-white truncate mt-1">{series.title}</h3>
        <p className="text-xs text-gray-400">{series.year}{series.network ? ` • ${series.network}` : ""}</p>
      </div>

      {/* Hover overlay */}
      <div className="absolute inset-0 bg-black/85 p-4 flex flex-col justify-between opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none">
        <div>
          <h3 className="text-sm font-bold text-white">{series.title}</h3>
          <p className="text-xs text-gray-400 mt-0.5">{series.year}{series.network ? ` • ${series.network}` : ""}</p>
          {series.rating > 0 && (
            <div className="flex items-center gap-1 mt-1">
              <Star className="w-3 h-3 text-yellow-400 fill-yellow-400" />
              <span className="text-xs text-yellow-400">{series.rating.toFixed(1)}</span>
            </div>
          )}
          {series.genres?.length > 0 && (
            <p className="text-[10px] text-gray-500 mt-1.5">{series.genres.slice(0, 3).join(" • ")}</p>
          )}
          <p className="text-xs text-gray-300 mt-2 line-clamp-4 leading-relaxed">
            {series.overview || "No overview available."}
          </p>
        </div>
        <div className="space-y-1.5">
          {/* Episode progress in hover */}
          {progress.total > 0 && (
            <div>
              <div className="flex items-center justify-between mb-0.5">
                <span className="text-[10px] font-medium" style={{ color: progress.color }}>
                  {progress.label} episodes
                </span>
                <span className="text-[10px] text-gray-500">{progress.percent}%</span>
              </div>
              <div className="w-full h-1 rounded-full bg-gray-700/50">
                <div className="h-full rounded-full transition-all duration-300" style={{ width: `${progress.percent}%`, backgroundColor: progress.color }} />
              </div>
            </div>
          )}
          {profile && (
            <p className="text-[10px] text-gray-500">Quality: {profile.name}</p>
          )}
        </div>
      </div>
    </div>
  );
}
