export interface QualityProfile {
  id: string;
  name: string;
}

export interface EpisodeStats {
  totalEpisodes: number;
  downloadedEpisodes: number;
  monitoredEpisodes: number;
  missingEpisodes: number;
  airedEpisodes: number;
}

export interface Series {
  id: string;
  title: string;
  year: number;
  imdbId?: string;
  tmdbId?: string;
  tvdbId?: string;
  overview: string;
  genres: string[];
  runtime: number;
  rating: number;
  backdropPath?: string;
  posterPath?: string;
  network: string;
  status: string;
  seriesType: string;
  qualityProfileId: string;
  libraryId: string;
  monitoringStatus: string;
  seasonFolder: boolean;
  releaseDate?: string;
  createdAt?: string;
  updatedAt?: string;
  seasons?: Season[];
  episodeStats?: EpisodeStats;
}

export interface Season {
  id: string;
  seriesId: string;
  seasonNumber: number;
  title: string;
  overview: string;
  posterPath?: string;
  monitored: boolean;
  episodeCount: number;
  episodeStats?: EpisodeStats;
}

export interface Episode {
  id: string;
  seriesId: string;
  seasonId: string;
  episodeNumber: number;
  title: string;
  overview: string;
  airDate?: string;
  runtime: number;
  stillPath?: string;
  monitored: boolean;
  hasFile: boolean;
  grabbed?: boolean;
}

export interface TMDBSeriesResult {
  tmdbId?: string;
  title: string;
  year: number;
  overview: string;
  posterPath?: string;
  network?: string;
  status?: string;
}

export interface CreditPerson {
  id: number;
  name: string;
  character?: string;
  role: string;
  department: string;
  profile_path: string;
  order: number;
}

export interface Credits {
  cast: CreditPerson[];
  crew: CreditPerson[];
}

export type SeriesSortKey =
  | "title-asc"
  | "title-desc"
  | "year-desc"
  | "year-asc"
  | "added-desc"
  | "network-asc"
  | "rating-desc";

export type ViewMode = "grid" | "list";

export const TMDB_IMG = "https://image.tmdb.org/t/p";

export const SERIES_STATUS_OPTIONS = [
  "all",
  "continuing",
  "ended",
  "upcoming",
  "cancelled",
] as const;

export const SERIES_STATUS_CONFIG: Record<
  string,
  { label: string; color: string; bg: string; border: string }
> = {
  continuing: { label: "Continuing", color: "text-emerald-400", bg: "bg-emerald-500/20", border: "#10b981" },
  ended:      { label: "Ended",      color: "text-blue-400",    bg: "bg-blue-500/20",    border: "#3b82f6" },
  upcoming:   { label: "Upcoming",   color: "text-amber-400",   bg: "bg-amber-500/20",   border: "#f59e0b" },
  cancelled:  { label: "Cancelled",  color: "text-red-400",     bg: "bg-red-500/20",     border: "#ef4444" },
};

export function seriesStatusLabel(s: string) {
  return SERIES_STATUS_CONFIG[s]?.label ?? s;
}

export const SERIES_SORT_OPTIONS: { value: SeriesSortKey; label: string }[] = [
  { value: "title-asc", label: "Title A–Z" },
  { value: "title-desc", label: "Title Z–A" },
  { value: "year-desc", label: "Year (Newest)" },
  { value: "year-asc", label: "Year (Oldest)" },
  { value: "added-desc", label: "Date Added" },
  { value: "network-asc", label: "Network" },
  { value: "rating-desc", label: "Rating" },
];

export function sortSeries(series: Series[], key: SeriesSortKey): Series[] {
  const sorted = [...series];
  switch (key) {
    case "title-asc":   return sorted.sort((a, b) => a.title.localeCompare(b.title));
    case "title-desc":  return sorted.sort((a, b) => b.title.localeCompare(a.title));
    case "year-desc":   return sorted.sort((a, b) => b.year - a.year);
    case "year-asc":    return sorted.sort((a, b) => a.year - b.year);
    case "added-desc":  return sorted.sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""));
    case "network-asc": return sorted.sort((a, b) => (a.network ?? "").localeCompare(b.network ?? ""));
    case "rating-desc": return sorted.sort((a, b) => b.rating - a.rating);
  }
}
