import { MonitoringLabel } from "@/components/movies/status-badge";
import { isSeriesMonitored, type Series } from "./types";

export function SeriesMonitoringLabel({ status }: { status?: string }) {
  if (!status || isSeriesMonitored(status)) return null;
  return <MonitoringLabel status="unmonitored" />;
}

type SeriesState = Pick<Series, "status" | "episodeStats">;

export function getEpisodeProgressInfo(series: SeriesState) {
  const stats = series.episodeStats;
  if (!stats || stats.airedEpisodes === 0) {
    return {
      percent: 0,
      downloaded: 0,
      total: 0,
      color: "#6b7280",
      label: "No episodes",
    };
  }
  const downloaded = stats.downloadedEpisodes;
  const total = stats.airedEpisodes;
  const percent = Math.round((downloaded / total) * 100);
  const color =
    downloaded === 0 ? "#ef4444" : downloaded < total ? "#f59e0b" : "#10b981";
  return { percent, downloaded, total, color, label: `${downloaded}/${total}` };
}

export function SeriesStatusBadge({ series }: { series: SeriesState }) {
  const progress = getEpisodeProgressInfo(series);
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-medium"
      style={{ color: progress.color, backgroundColor: `${progress.color}20` }}
    >
      {progress.total > 0 ? `${progress.label} episodes` : series.status}
    </span>
  );
}

export function SeriesStatusAccent({ series }: { series: SeriesState }) {
  const progress = getEpisodeProgressInfo(series);
  return (
    <>
      <div className="absolute left-0 right-0 top-0 z-10 h-[3px] bg-gray-700/50">
        <div
          className="h-full transition-all duration-300"
          style={{
            width: `${progress.percent}%`,
            backgroundColor: progress.color,
          }}
        />
      </div>
      <div
        className="pointer-events-none absolute left-0 right-0 top-[3px] z-10 h-3"
        style={{
          background: `linear-gradient(180deg, ${progress.color}25, transparent)`,
        }}
      />
    </>
  );
}
