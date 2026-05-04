// Typed fetch wrappers for the Loom metadata search, import, and cache endpoints.
// Implements TanStack Query patterns for search, import, stats, and provider tests.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { UseQueryOptions } from "@tanstack/react-query";

// ---------- Types ----------

export interface MovieMetadata {
  tmdb_id?: string;
  imdb_id?: string;
  tvdb_id?: string;
  title: string;
  year?: number;
  overview?: string;
  poster_path?: string;
  release_date?: string;
  runtime?: number;
  genres?: string[];
  rating?: number;
  cached_at?: string;
}

export interface SeriesMetadata {
  tmdb_id?: string;
  imdb_id?: string;
  tvdb_id?: string;
  title: string;
  overview?: string;
  poster_path?: string;
  first_air_date?: string;
  genres?: string[];
  rating?: number;
  seasons?: number;
  cached_at?: string;
}

export interface CacheStats {
  hit_rate: number;
  miss_rate: number;
  cache_size: number;
  entries: number;
  ttl_remaining_seconds?: number;
}

export interface ProviderStatus {
  name: string;
  status: "ok" | "unconfigured" | "error";
  configured_api_key: boolean;
  last_test_time?: string;
  last_test_error?: string;
  last_test_latency_ms?: number;
}

export interface TestResult {
  ok: boolean;
  latency_ms: number;
  error?: string;
  result?: MovieMetadata | SeriesMetadata;
}

// ---------- HTTP helpers ----------

export class ApiError extends Error {
  status: number;
  code?: string;
  details?: unknown;
  constructor(status: number, message: string, code?: string, details?: unknown) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
    Object.setPrototypeOf(this, ApiError.prototype);
  }
}

async function httpGet<T>(path: string): Promise<T> {
  const resp = await fetch(path, { method: "GET" });
  if (!resp.ok) {
    const text = await resp.text();
    throw new ApiError(resp.status, text);
  }
  return resp.json();
}

async function httpPost<T>(path: string, body: unknown): Promise<T> {
  const resp = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new ApiError(resp.status, text);
  }
  return resp.json();
}

// ---------- React Query Hooks ----------

export function useMetadataSearch(query: string, type: string, year?: number) {
  return useMutation({
    mutationFn: async () => {
      if (!query) throw new Error("Query cannot be empty");
      return httpPost<MovieMetadata[] | SeriesMetadata[]>(
        "/api/metadata/search",
        { query, type, year }
      );
    },
  });
}

export function useMetadataImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (payload: {
      type: string;
      metadata: MovieMetadata | SeriesMetadata;
    }) => {
      return httpPost<{ id: string; type: string }>("/api/metadata/import", {
        type: payload.type,
        metadata: payload.metadata,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["cache-stats"] });
    },
  });
}

export function useMetadataStats() {
  return useQuery({
    queryKey: ["cache-stats"],
    queryFn: () => httpGet<CacheStats>("/api/metadata/cache/stats"),
    refetchInterval: 30000, // Refetch every 30s
  });
}

export function useProviderStatus(provider: string) {
  return useQuery({
    queryKey: ["provider-status", provider],
    queryFn: () =>
      httpGet<ProviderStatus>(`/api/metadata/providers/${provider}/status`),
    enabled: !!provider,
  });
}

export function useProviderTest(provider: string) {
  return useMutation({
    mutationFn: async () => {
      return httpPost<TestResult>(
        `/api/metadata/providers/${provider}/test`,
        {}
      );
    },
  });
}
