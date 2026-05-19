import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ─── Types ──────────────────────────────────────────────────────────────

export interface SearchDebugEntry {
  id: string;
  created_at: string;
  media_type: string;
  media_id: string;
  title: string;
  year: number;
  season: number;
  episode: number;
  imdb_id: string;
  tvdb_id: string;
  tmdb_id: string;
  quality_profile_id: string;
  request?: any;
  tiers?: TierDetail[];
  indexer_results?: IndexerResult[];
  evaluation?: EvalResult[];
  total_results: number;
  total_rejected: number;
  grabbed_title: string;
  outcome: string;
  duration_ms: number;
  error_message?: string;
}

export interface TierDetail {
  tier_index: number;
  queries: QueryDetail[];
  result_count: number;
  accepted_count: number;
  rejected_count: number;
  stopped_here: boolean;
}

export interface QueryDetail {
  term?: string;
  mode?: string;
  imdb_id?: string;
  tvdb_id?: string;
  tmdb_id?: string;
  season?: number;
  episode?: number;
  year?: number;
  categories?: number[];
}

export interface IndexerResult {
  indexer_id: string;
  indexer_name: string;
  status: string;
  result_count: number;
  latency_ms: number;
  error?: string;
  results?: ResultEntry[];
}

export interface ResultEntry {
  title: string;
  size: number;
  seeders?: number;
  peers?: number;
  quality?: string;
  pub_date?: string;
  freeleech?: boolean;
  internal?: boolean;
  scene?: boolean;
  indexer_id: string;
}

export interface EvalResult {
  title: string;
  indexer_id: string;
  rejected: boolean;
  reject_reason?: string;
  parsed_title?: string;
  parsed_source?: string;
  parsed_resolution?: number;
  quality_name?: string;
  quality_tier: number;
  format_score: number;
  composite_score: number;
  size: number;
  seeders?: number;
}

export interface SearchDebugListResult {
  entries: SearchDebugEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface SearchDebugStats {
  total_searches: number;
  outcome_counts: Record<string, number>;
  top_reject_reasons?: { reason: string; count: number }[];
}

export interface SearchDebugParams {
  media_type?: string;
  media_id?: string;
  outcome?: string;
  limit?: number;
  offset?: number;
}

// ─── Fetch ──────────────────────────────────────────────────────────────

export async function fetchSearchDebugList(
  params: SearchDebugParams = {},
  signal?: AbortSignal,
): Promise<SearchDebugListResult> {
  const qs = new URLSearchParams();
  if (params.media_type) qs.set("media_type", params.media_type);
  if (params.media_id) qs.set("media_id", params.media_id);
  if (params.outcome) qs.set("outcome", params.outcome);
  if (params.limit != null) qs.set("limit", String(params.limit));
  if (params.offset != null) qs.set("offset", String(params.offset));
  const url = `/api/v1/search-debug${qs.toString() ? `?${qs}` : ""}`;
  const res = await apiFetch(url, { signal });
  if (!res.ok) throw new Error(`search debug: ${res.status}`);
  return (await res.json()) as SearchDebugListResult;
}

export async function fetchSearchDebugEntry(
  id: string,
  signal?: AbortSignal,
): Promise<SearchDebugEntry> {
  const res = await apiFetch(`/api/v1/search-debug/${id}`, { signal });
  if (!res.ok) throw new Error(`search debug entry: ${res.status}`);
  return (await res.json()) as SearchDebugEntry;
}

export async function fetchSearchDebugStats(
  signal?: AbortSignal,
): Promise<SearchDebugStats> {
  const res = await apiFetch("/api/v1/search-debug/stats", { signal });
  if (!res.ok) throw new Error(`search debug stats: ${res.status}`);
  return (await res.json()) as SearchDebugStats;
}

// ─── Hooks ──────────────────────────────────────────────────────────────

export function useSearchDebugList(params: SearchDebugParams = {}) {
  return useQuery({
    queryKey: ["search-debug", "list", params],
    queryFn: ({ signal }) => fetchSearchDebugList(params, signal),
    refetchInterval: 15_000,
    staleTime: 10_000,
  });
}

export function useSearchDebugEntry(id: string | null) {
  return useQuery({
    queryKey: ["search-debug", "entry", id],
    queryFn: ({ signal }) => fetchSearchDebugEntry(id!, signal),
    enabled: !!id,
  });
}

export function useSearchDebugStats() {
  return useQuery({
    queryKey: ["search-debug", "stats"],
    queryFn: ({ signal }) => fetchSearchDebugStats(signal),
    refetchInterval: 30_000,
    staleTime: 20_000,
  });
}
