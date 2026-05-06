export interface AutoSearchRequest {
  media_type: "movie" | "series";
  media_id: string;
  title: string;
  quality_profile_id: string;
  imdb_id?: string;
  tmdb_id?: string;
  tvdb_id?: string;
  season?: number;
  episode?: number;
}

export interface GrabbedRelease {
  title: string;
  indexer_id: string;
  size: number;
  quality_tier: number;
  format_score: number;
  composite_score: number;
  client_id: string;
  download_id: string;
}

export interface AutoSearchResult {
  grabbed?: GrabbedRelease;
  considered: number;
  rejected: number;
  reason?: string;
  top_rejects?: { reason: string; count: number }[];
}

export async function autoSearch(
  req: AutoSearchRequest
): Promise<AutoSearchResult> {
  const res = await fetch("/api/v1/autosearch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || "Auto search failed");
  }
  return res.json();
}
