// Typed fetch wrappers for the Loom episode-ordering & packs REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ─── Episode Ordering Types ────────────────────────────────────────────

export type OrderingType = "aired" | "dvd" | "absolute" | "scene";
export type MappingSource = "manual" | "tvdb" | "xem" | "anidb";

export interface EpisodeMapping {
  id: string;
  seriesId: string;
  orderingType: OrderingType;
  seasonFrom?: number | null;
  episodeFrom?: number | null;
  absoluteFrom?: number | null;
  seasonTo?: number | null;
  episodeTo?: number | null;
  absoluteTo?: number | null;
  source: MappingSource;
  createdAt: string;
}

export interface CreateMappingRequest {
  orderingType: string;
  seasonFrom?: number | null;
  episodeFrom?: number | null;
  absoluteFrom?: number | null;
  seasonTo?: number | null;
  episodeTo?: number | null;
  absoluteTo?: number | null;
  source?: string;
}

// ─── Pack Types ─────────────────────────────────────────────────────────

export type PackType =
  | "single_season"
  | "multi_season"
  | "complete_series"
  | "episode_range";

export interface DetectedPack {
  type: PackType;
  seasonStart: number;
  seasonEnd: number;
  episodeStart: number;
  episodeEnd: number;
  title: string;
  isPack: boolean;
}

export interface PackHistory {
  id: string;
  seriesId: string;
  season: number;
  packTitle: string;
  episodesIncluded: number[];
  quality: string;
  grabbedAt: string;
}

// ─── Episode Ordering API ───────────────────────────────────────────────

export async function fetchEpisodeMappings(
  seriesId: string,
  orderingType?: OrderingType,
  signal?: AbortSignal,
): Promise<EpisodeMapping[]> {
  const params = new URLSearchParams();
  if (orderingType) params.set("type", orderingType);
  const url = `/api/v1/episode-order/series/${seriesId}/mappings${params.toString() ? `?${params}` : ""}`;
  const res = await apiFetch(url, { signal });
  if (!res.ok) throw new Error(`episode mappings: ${res.status}`);
  const json = await res.json();
  return json.data as EpisodeMapping[];
}

export async function createEpisodeMapping(
  seriesId: string,
  mapping: CreateMappingRequest,
): Promise<EpisodeMapping> {
  const res = await apiFetch(
    `/api/v1/episode-order/series/${seriesId}/mappings`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(mapping),
    },
  );
  if (!res.ok) throw new Error(`create mapping: ${res.status}`);
  return (await res.json()) as EpisodeMapping;
}

export async function deleteEpisodeMapping(id: string): Promise<void> {
  const res = await apiFetch(`/api/v1/episode-order/mappings/${id}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error(`delete mapping: ${res.status}`);
}

export async function setOrderingType(
  seriesId: string,
  orderingType: OrderingType,
): Promise<void> {
  const res = await apiFetch(
    `/api/v1/episode-order/series/${seriesId}/ordering-type`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ orderingType }),
    },
  );
  if (!res.ok) throw new Error(`set ordering type: ${res.status}`);
}

// ─── Pack API ───────────────────────────────────────────────────────────

export async function fetchPackHistory(
  seriesId?: string,
  signal?: AbortSignal,
): Promise<PackHistory[]> {
  const params = new URLSearchParams();
  if (seriesId) params.set("seriesId", seriesId);
  const url = `/api/v1/packs/history${params.toString() ? `?${params}` : ""}`;
  const res = await apiFetch(url, { signal });
  if (!res.ok) throw new Error(`pack history: ${res.status}`);
  const json = await res.json();
  return json.data as PackHistory[];
}

export async function detectPack(title: string): Promise<DetectedPack> {
  const res = await apiFetch("/api/v1/packs/detect", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title }),
  });
  if (!res.ok) throw new Error(`detect pack: ${res.status}`);
  return (await res.json()) as DetectedPack;
}

// ─── React Query Hooks ──────────────────────────────────────────────────

export const EPISODE_MAPPINGS_KEY = "episode-mappings";
export const PACK_HISTORY_KEY = "pack-history";

export function useEpisodeMappings(
  seriesId: string,
  orderingType?: OrderingType,
) {
  return useQuery({
    queryKey: [EPISODE_MAPPINGS_KEY, seriesId, orderingType],
    queryFn: ({ signal }) =>
      fetchEpisodeMappings(seriesId, orderingType, signal),
    enabled: !!seriesId,
  });
}

export function useCreateMapping(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mapping: CreateMappingRequest) =>
      createEpisodeMapping(seriesId, mapping),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: [EPISODE_MAPPINGS_KEY, seriesId] }),
  });
}

export function useDeleteMapping(seriesId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteEpisodeMapping(id),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: [EPISODE_MAPPINGS_KEY, seriesId] }),
  });
}

export function usePackHistory(seriesId?: string) {
  return useQuery({
    queryKey: [PACK_HISTORY_KEY, seriesId],
    queryFn: ({ signal }) => fetchPackHistory(seriesId, signal),
  });
}
