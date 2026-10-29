import type { Credits } from "@/components/movies/types";
import { apiFetch } from "@/lib/fetch";

export type TitleMedia = "movie" | "series" | "artist";
export interface TitleIdentity {
  media: TitleMedia;
  id: string;
  provider?: "tmdb" | "imdb" | "tvdb";
  title: string;
  inLibrary?: boolean;
  poster?: string;
  year?: number;
}
export interface TitleDefaults {
  library_path?: string;
  quality_profile_id?: string;
  monitor_type?: string;
  search_on_add?: boolean;
}
export interface ExternalTitle {
  title?: string;
  name?: string;
  tmdb_id?: string;
  imdb_id?: string;
  mbid?: string;
  overview?: string;
  poster_path?: string;
  backdrop_path?: string;
  status?: string;
  network?: string;
  tagline?: string;
  image_url?: string;
  year?: number;
  release_date?: string;
  first_air_date?: string;
  runtime?: number;
  seasons?: number;
  rating?: number;
  genres?: string[];
  country?: string;
  type?: string;
  disambiguation?: string;
}
export interface ExternalAlbum {
  mbid: string;
  title: string;
  type?: string;
  release_date?: string;
  cover_art_url?: string;
  secondary_types?: string[];
  genres?: string[];
}
export interface TitleDetails {
  title: ExternalTitle;
  credits?: Credits;
  credits_error?: string;
  albums?: ExternalAlbum[];
}

export async function titleJSON<T>(
  url: string,
  init?: RequestInit,
): Promise<T> {
  const response = await apiFetch(url, init);
  if (!response.ok)
    throw new Error((await response.text()) || "Unable to complete request");
  return response.json();
}

export function titleKey(
  title: Pick<TitleIdentity, "media" | "id" | "provider">,
) {
  return [
    "external-title",
    title.media,
    title.provider ?? (title.media === "artist" ? "musicbrainz" : "tmdb"),
    title.id,
  ] as const;
}

export function loadTitle(identity: TitleIdentity, signal?: AbortSignal) {
  return titleJSON<TitleDetails>(
    `/api/v1/discover/${identity.media}/${encodeURIComponent(identity.id)}${identity.provider ? `?provider=${identity.provider}` : ""}`,
    { signal },
  );
}

export async function searchTitles(
  media: TitleMedia,
  term: string,
  signal?: AbortSignal,
): Promise<TitleIdentity[]> {
  const url =
    media === "movie"
      ? `/api/v1/movies/lookup?term=${encodeURIComponent(term)}`
      : media === "series"
        ? `/api/v1/series/search?q=${encodeURIComponent(term)}`
        : `/api/v1/artists/lookup?q=${encodeURIComponent(term)}`;
  const body = await titleJSON<
    Array<Record<string, unknown>> | { data: Array<Record<string, unknown>> }
  >(url, { signal });
  return (Array.isArray(body) ? body : (body.data ?? [])).flatMap((item) => {
    const id = item.tmdb_id ?? item.tmdbId ?? item.mbid;
    if (!id || id === "0") return [];
    return [
      {
        media,
        id: String(id),
        title: String(item.title ?? item.name ?? ""),
        inLibrary: item.already_added === true,
        poster: String(
          item.poster_path ?? item.posterPath ?? item.image_url ?? "",
        ),
        year: Number(item.year) || undefined,
      },
    ];
  });
}
