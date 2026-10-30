import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";
import { useAuth } from "@/hooks/use-auth";

export type DiscoveryMediaType = "movie" | "series";

export interface Recommendation {
  tmdb_id: number;
  media_type: DiscoveryMediaType;
  title: string;
  year: number;
  poster_path?: string;
  overview?: string;
  rating: number;
  popularity: number;
  release_date?: string;
  reasons: string[];
  source: string;
  collection_id?: number;
}

export interface DiscoveryStatus {
  warnings: string[];
  generated_at: string;
  stale: boolean;
}

export interface RecommendationsResponse extends DiscoveryStatus {
  items: Recommendation[];
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(`/api/v1/discover${path}`, init);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `Discovery request failed (${res.status})`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const discoveryKeys = {
  all: ["discovery"] as const,
  recommendations: (
    userID: number | null,
    mediaType: DiscoveryMediaType,
    includeUpcoming: boolean,
  ) =>
    [
      "discovery",
      "recommendations",
      userID,
      mediaType,
      includeUpcoming,
    ] as const,
};

export function useRecommendations(
  mediaType: DiscoveryMediaType,
  includeUpcoming: boolean,
) {
  const { user } = useAuth();
  return useQuery({
    queryKey: discoveryKeys.recommendations(
      user?.id ?? null,
      mediaType,
      includeUpcoming,
    ),
    queryFn: ({ signal }) =>
      request<RecommendationsResponse>(
        `/recommendations?media_type=${mediaType}&include_upcoming=${includeUpcoming}`,
        { signal },
      ),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

export function useDismissRecommendation() {
  const client = useQueryClient();
  const { user } = useAuth();
  const recommendationKey = ["discovery", "recommendations", user?.id ?? null];
  return useMutation({
    mutationFn: (item: Pick<Recommendation, "tmdb_id" | "media_type">) =>
      request<void>("/dismissals", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(item),
      }),
    onSuccess: (_data, item) => {
      client.setQueriesData<RecommendationsResponse>(
        { queryKey: recommendationKey },
        (previous) =>
          previous
            ? {
                ...previous,
                items: previous.items.filter(
                  (candidate) =>
                    candidate.tmdb_id !== item.tmdb_id ||
                    candidate.media_type !== item.media_type,
                ),
              }
            : previous,
      );
      void client.invalidateQueries({
        queryKey: recommendationKey,
      });
    },
  });
}
