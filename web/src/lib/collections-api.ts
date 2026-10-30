import { apiFetch } from "@/lib/fetch";
import type { TitleIdentity } from "@/lib/title-api";

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
          (status === "missing" && !member.in_library) ||
          (status === "upcoming" &&
            !!member.release_date &&
            member.release_date.slice(0, 10) >
              new Date().toISOString().slice(0, 10)) ||
          (status === "monitored" && member.monitored) ||
          (status === "files" && member.has_file === true)))
  );
}
