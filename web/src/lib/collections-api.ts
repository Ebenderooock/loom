import { apiFetch } from "@/lib/fetch";
import type { TitleIdentity } from "@/lib/title-api";
import type { EpisodeStats } from "@/components/series/types";

export interface CollectionSource {
  provider: "tmdb" | "imdb" | "tvdb";
  kind: "collection" | "list";
  external_id: string;
  last_sync?: string;
  error?: string;
}

export interface CollectionMember {
  id: string;
  media_type: "movie" | "series";
  title: string;
  year?: number;
  poster_path?: string;
  overview?: string;
  release_date?: string;
  external_ids: Partial<Record<"tmdb" | "imdb" | "tvdb", string>>;
  sources: string[];
  provenance: string[];
  local: boolean;
  excluded: boolean;
  media_id?: string;
  library_id?: string;
  in_library: boolean;
  monitored: boolean;
  has_file?: boolean;
  status?: string;
  monitoring_status?: string;
  episode_stats?: EpisodeStats;
}

export interface LoomCollection {
  id: string;
  name: string;
  overview: string;
  sources: CollectionSource[];
  members: CollectionMember[];
  revision: number;
  total: number;
  in_library: number;
  quality_profile_id?: string;
}

export type BulkStatus =
  | "eligible"
  | "skipped"
  | "queued"
  | "running"
  | "done"
  | "failed";

export interface BulkItem {
  member_id: string;
  media_type: "movie" | "series";
  title: string;
  year?: number;
  media_id?: string;
  status: BulkStatus;
  reason?: string;
  detail?: string;
  current_profile_id?: string;
  new_profile_id?: string;
  missing_episodes?: number;
}

export interface CollectionOption {
  id: string;
  name: string;
  quality_profile_id?: string;
}

export interface AddSettings {
  library_id?: string;
  quality_profile_id?: string;
  monitored?: boolean;
  search?: boolean;
}

export interface CollectionJob {
  id: string;
  kind: "add" | "search";
  collection_id: string;
  status: "running" | "done";
  items: BulkItem[];
  counts: Partial<Record<BulkStatus, number>>;
  started_at: string;
  finished_at?: string;
}

export interface BulkPlan {
  kind: "add" | "search" | "quality";
  items: BulkItem[];
  counts: Partial<Record<BulkStatus, number>>;
  settings?: Partial<Record<"movie" | "series", AddSettings>>;
  libraries?: Partial<Record<"movie" | "series", CollectionOption[]>>;
  profiles?: CollectionOption[];
  job?: CollectionJob;
  note?: string;
}

export type MemberInput =
  | { media_type: "movie" | "series"; media_id: string }
  | {
      media_type: "movie" | "series";
      title: string;
      year?: number;
      poster_path?: string;
      external_ids: Partial<Record<"tmdb" | "imdb" | "tvdb", string>>;
    };

export const SKIP_REASONS: Record<string, string> = {
  excluded: "Excluded",
  in_library: "Already in library",
  unresolved: "Needs a TMDB match",
  unreleased: "Not released",
  no_library: "Choose a library",
  no_profile: "Choose a quality profile",
  already_running: "Already running",
  not_in_library: "Not in library",
  unmonitored: "Not monitored",
  has_file: "Already downloaded",
  nothing_missing: "Nothing missing",
  unchanged: "Already uses profile",
};

export interface LibraryTitle {
  media_type: "movie" | "series";
  id: string;
  title: string;
  year?: number;
  poster?: string;
  tmdb?: string;
  imdb?: string;
  tvdb?: string;
  status?: string;
  monitoringStatus?: string;
  episodeStats?: EpisodeStats;
}

/** Loads every movie (paginated) or show in the library for local search. */
export async function loadLibraryTitles(
  media: "movie" | "series",
  signal?: AbortSignal,
): Promise<LibraryTitle[]> {
  const out: LibraryTitle[] = [];
  for (let offset = 0; ; ) {
    const url =
      media === "movie"
        ? `/api/v1/movies?limit=1000&offset=${offset}`
        : "/api/v1/series";
    const response = await apiFetch(url, { signal });
    if (!response.ok)
      throw new Error((await response.text()) || "Unable to load library");
    const body = (await response.json()) as
      | Array<Record<string, unknown>>
      | { data?: Array<Record<string, unknown>> };
    const rows = Array.isArray(body) ? body : (body.data ?? []);
    for (const row of rows) {
      const tmdb = row.tmdbId ?? row.tmdb_id;
      out.push({
        media_type: media,
        id: String(row.id),
        title: String(row.title ?? ""),
        year: Number(row.year) || undefined,
        poster: String(row.posterPath ?? row.poster_path ?? "") || undefined,
        tmdb: tmdb ? String(tmdb) : undefined,
        imdb: typeof row.imdbId === "string" ? row.imdbId : undefined,
        tvdb: typeof row.tvdbId === "string" ? row.tvdbId : undefined,
        status: typeof row.status === "string" ? row.status : undefined,
        monitoringStatus:
          typeof row.monitoringStatus === "string"
            ? row.monitoringStatus
            : undefined,
        episodeStats: row.episodeStats as EpisodeStats | undefined,
      });
    }
    if (media !== "movie" || rows.length < 1000) return out;
    offset += rows.length;
  }
}

export function collectionPath(id: string, path = "") {
  return `/${encodeURIComponent(id)}${path}`;
}

/** Adds real titles to an existing collection. */
export function addCollectionMembers(
  collection: Pick<LoomCollection, "id">,
  items: MemberInput[],
) {
  return collectionRequest<LoomCollection>(
    collectionPath(collection.id, "/members"),
    { items },
  );
}

/** True when the member already represents this exact local or provider title. */
export function hasTitle(
  collection: Pick<LoomCollection, "members">,
  media: "movie" | "series",
  ids: { media_id?: string; tmdb?: string },
) {
  return collection.members.some(
    (m) =>
      !m.excluded &&
      m.media_type === media &&
      ((!!ids.media_id && m.media_id === ids.media_id) ||
        (!!ids.tmdb && m.external_ids.tmdb === ids.tmdb)),
  );
}

export const collectionKeys = {
  all: ["collections"] as const,
  detail: (id: string) => ["collections", id] as const,
};

export async function collectionRequest<T>(
  path = "",
  body?: unknown,
  method = "POST",
  signal?: AbortSignal,
): Promise<T> {
  const response = await apiFetch(`/api/v1/collections${path || "/"}`, {
    method: body === undefined && method === "POST" ? "GET" : method,
    signal,
    ...(body === undefined
      ? {}
      : {
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }),
  });
  if (!response.ok)
    throw new Error((await response.text()) || "Collection operation failed");
  return response.json();
}

export function memberIdentity(member: CollectionMember): TitleIdentity {
  const provider = member.external_ids.tmdb
    ? "tmdb"
    : member.external_ids.imdb
      ? "imdb"
      : "tvdb";
  return {
    media: member.media_type,
    provider,
    id: member.external_ids[provider] ?? "",
    title: member.title,
    poster: member.poster_path,
    year: member.year,
  };
}

export function matchesMember(
  member: CollectionMember,
  library: string,
  media: string,
  status: string,
) {
  return (
    (!library || member.library_id === library) &&
    (!media || member.media_type === media) &&
    (status === "excluded"
      ? member.excluded
      : !member.excluded &&
        (!status ||
          (status === "added" && member.in_library) ||
          (status === "external" && !member.in_library) ||
          (member.in_library &&
            status === "missing" &&
            (member.media_type === "movie"
              ? member.status === "missing"
              : !!member.episode_stats &&
                member.episode_stats.airedEpisodes >
                  member.episode_stats.downloadedEpisodes)) ||
          (member.in_library &&
            status === "available" &&
            (member.media_type === "movie"
              ? member.status?.startsWith("available_")
              : !!member.episode_stats &&
                member.episode_stats.airedEpisodes > 0 &&
                member.episode_stats.downloadedEpisodes >=
                  member.episode_stats.airedEpisodes)) ||
          (member.in_library &&
            (status === "downloading" || status === "storing") &&
            member.media_type === "movie" &&
            member.status === status) ||
          (status === "upcoming" &&
            (member.in_library
              ? member.status === "unreleased"
              : !!member.release_date &&
                member.release_date.slice(0, 10) >
                  new Date().toISOString().slice(0, 10))) ||
          (status === "monitored" && member.monitored) ||
          (status === "files" && member.has_file === true)))
  );
}
