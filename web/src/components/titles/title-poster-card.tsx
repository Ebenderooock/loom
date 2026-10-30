import { Film, Plus } from "lucide-react";
import { titleImage } from "./title-metadata";
import {
  MonitoringLabel,
  StatusAccent,
  StatusBadge,
} from "@/components/movies/status-badge";
import {
  SeriesStatusAccent,
  SeriesStatusBadge,
} from "@/components/series/series-status";
import type { EpisodeStats } from "@/components/series/types";

// Poster presentation follows the library MovieCard; actions remain separate
// buttons so opening details never accidentally adds or changes membership.
export function TitlePosterCard({
  title,
  poster,
  year,
  status,
  mediaType = "movie",
  episodeStats,
  monitoringStatus,
  inLibrary = false,
  onOpen,
  onAdd,
  label = `View details for ${title}`,
  disabled = false,
}: {
  title: string;
  poster?: string;
  year?: number;
  status?: string;
  mediaType?: "movie" | "series";
  episodeStats?: EpisodeStats;
  monitoringStatus?: string;
  inLibrary?: boolean;
  onOpen: () => void;
  onAdd?: () => void;
  label?: string;
  disabled?: boolean;
}) {
  const series =
    mediaType === "series" && inLibrary && status
      ? { status, episodeStats }
      : undefined;
  const displayStatus = inLibrary ? status : "missing";
  return (
    <div className="group relative min-w-0">
      <button
        type="button"
        disabled={disabled}
        aria-label={label}
        onClick={onOpen}
        className="relative block w-full overflow-hidden rounded-lg border border-transparent text-left shadow-lg transition-shadow hover:border-accent/20 hover:shadow-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        {series ? (
          <SeriesStatusAccent series={series} />
        ) : (
          <StatusAccent status={displayStatus ?? ""} />
        )}
        <div className="aspect-[2/3] bg-muted">
          {poster ? (
            <img
              src={titleImage(poster, "w342")}
              alt=""
              loading="lazy"
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="flex h-full items-center justify-center">
              <Film className="h-12 w-12 text-muted-foreground/30" />
            </div>
          )}
        </div>
        <div className="absolute inset-x-0 bottom-0 min-w-0 bg-gradient-to-t from-black/95 via-black/70 to-transparent p-3 pt-10">
          <div className="flex items-center justify-between gap-1">
            {!displayStatus && inLibrary ? (
              <span className="rounded border border-white/20 bg-black/50 px-2 py-0.5 text-xs text-white">
                In library
              </span>
            ) : series ? (
              <SeriesStatusBadge series={series} />
            ) : (
              <StatusBadge status={displayStatus ?? "missing"} />
            )}
            <MonitoringLabel status={monitoringStatus} />
          </div>
          <h3
            className="mt-1 truncate text-sm font-semibold text-white"
            title={title}
          >
            {title}
          </h3>
          <p className="text-xs text-gray-400">{year || "Date unknown"}</p>
        </div>
      </button>
      {onAdd && !inLibrary && (
        <button
          type="button"
          disabled={disabled}
          aria-label={`Add ${title}`}
          onClick={onAdd}
          className="absolute right-2 top-2 flex h-11 w-11 items-center justify-center rounded-full border border-white/20 bg-black/70 text-white shadow hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          <Plus className="h-5 w-5" />
        </button>
      )}
    </div>
  );
}
