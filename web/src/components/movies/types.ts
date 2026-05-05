export interface RootFolder {
  id: string;
  path: string;
}

export interface QualityProfile {
  id: string;
  name: string;
}

export interface Movie {
  id: string;
  title: string;
  year: number;
  overview: string;
  tmdbId?: string;
  imdbId?: string;
  posterPath?: string;
  backdropPath?: string;
  rating: number;
  runtime: number;
  genres: string[];
  releaseDate: string;
  status: string;
  monitoringStatus: string;
  qualityProfileId: string;
  rootFolderId: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CreditPerson {
  id: number;
  name: string;
  role: string;
  department: string;
  profile_path: string;
  order: number;
}

export interface Credits {
  cast: CreditPerson[];
  crew: CreditPerson[];
}

export interface TMDBResult {
  tmdb_id?: string;
  title: string;
  year: number;
  overview: string;
  poster_path: string;
  backdrop_path?: string;
  release_date: string;
  rating: number;
  runtime?: number;
  genres?: string[];
  imdb_id?: string;
}

export type SortKey = "title-asc" | "title-desc" | "year-desc" | "year-asc" | "added-desc" | "rating-desc";
export type ViewMode = "grid" | "list";

export const TMDB_IMG = "https://image.tmdb.org/t/p";

export const STATUS_OPTIONS = [
  "all", "missing", "unreleased", "downloading", "storing",
  "available_wrong_quality", "available_right_quality", "available_higher_quality",
] as const;

export const STATUS_CONFIG: Record<string, { label: string; color: string; bg: string; border: string }> = {
  missing:                  { label: "Missing",          color: "text-red-400",    bg: "bg-red-500/20",     border: "#ef4444" },
  unreleased:               { label: "Unreleased",       color: "text-blue-400",   bg: "bg-blue-500/20",    border: "#3b82f6" },
  downloading:              { label: "Downloading",      color: "text-yellow-400", bg: "bg-yellow-500/20",  border: "#eab308" },
  storing:                  { label: "Importing",        color: "text-orange-400", bg: "bg-orange-500/20",  border: "#f97316" },
  available_wrong_quality:  { label: "Wrong Quality",    color: "text-amber-400",  bg: "bg-amber-500/20",   border: "#f59e0b" },
  available_right_quality:  { label: "Available",        color: "text-green-400",  bg: "bg-green-500/20",   border: "#22c55e" },
  available_higher_quality: { label: "HD Available",     color: "text-emerald-400",bg: "bg-emerald-500/20", border: "#10b981" },
};

export function statusLabel(s: string) {
  return STATUS_CONFIG[s]?.label ?? s;
}

export const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: "title-asc", label: "Title A–Z" },
  { value: "title-desc", label: "Title Z–A" },
  { value: "year-desc", label: "Year (Newest)" },
  { value: "year-asc", label: "Year (Oldest)" },
  { value: "added-desc", label: "Date Added" },
  { value: "rating-desc", label: "Rating" },
];

export function sortMovies(movies: Movie[], key: SortKey): Movie[] {
  const sorted = [...movies];
  switch (key) {
    case "title-asc":   return sorted.sort((a, b) => a.title.localeCompare(b.title));
    case "title-desc":  return sorted.sort((a, b) => b.title.localeCompare(a.title));
    case "year-desc":   return sorted.sort((a, b) => b.year - a.year);
    case "year-asc":    return sorted.sort((a, b) => a.year - b.year);
    case "added-desc":  return sorted.sort((a, b) => (b.createdAt ?? "").localeCompare(a.createdAt ?? ""));
    case "rating-desc": return sorted.sort((a, b) => b.rating - a.rating);
  }
}
