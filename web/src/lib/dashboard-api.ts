import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------------------------------------------------------------------------
// Dashboard data-fetching hooks (extracted for DRY + testability)
// ---------------------------------------------------------------------------

function useDashboardQuery<T>(key: string, path: string) {
  return useQuery({
    queryKey: ["dashboard", key],
    queryFn: async ({ signal }) => {
      const res = await apiFetch(path, { signal });
      if (!res.ok) throw new Error(`Failed to fetch ${key}`);
      return (await res.json()) as T;
    },
    staleTime: 60_000,
    retry: 1,
  });
}

export function useDashboardMovies() {
  return useDashboardQuery<{ data: unknown[]; total: number }>(
    "movies",
    "/api/v1/movies?limit=1",
  );
}

export function useDashboardSeries() {
  return useDashboardQuery<{ data: unknown[]; total: number }>(
    "series",
    "/api/v1/series?limit=1",
  );
}

export function useDashboardIndexers() {
  return useDashboardQuery<{ indexers: unknown[] }>(
    "indexers",
    "/api/v1/indexers",
  );
}

export function useDashboardDownloadClients() {
  return useDashboardQuery<{ download_clients: unknown[] }>(
    "download-clients",
    "/api/v1/download-clients",
  );
}

export interface HealthIssue {
  id: string;
  name: string;
  message: string;
  severity: "error" | "degraded";
}

interface IndexerHealthResponse {
  data: {
    indexer_id: string;
    indexer_name: string;
    status: string;
    last_error: string;
    success_rate: number;
    fail_count: number;
  }[];
}

export function useDashboardIndexerHealth() {
  return useQuery({
    queryKey: ["dashboard", "indexer-health"],
    queryFn: async ({ signal }) => {
      const res = await apiFetch("/api/v1/indexers/health", { signal });
      if (!res.ok) throw new Error("Failed to fetch indexer health");
      const json = (await res.json()) as IndexerHealthResponse;
      const issues: HealthIssue[] = (json.data ?? [])
        .filter((h) => h.status === "error" || h.status === "degraded")
        .map((h) => ({
          id: h.indexer_id,
          name: h.indexer_name || h.indexer_id,
          message: h.last_error || `${h.status} — ${h.fail_count} failed searches`,
          severity: h.status === "error" ? "error" : "degraded",
        }));
      return { data: issues };
    },
    staleTime: 60_000,
    retry: 1,
  });
}
