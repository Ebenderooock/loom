import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------------------------------------------------------------------------
// Dashboard data-fetching hooks (extracted for DRY + testability)
// ---------------------------------------------------------------------------

// Minimal types for dashboard counts — full types live in domain-specific modules.
interface IndexerSummary { id: string; name: string; enabled: boolean; }
interface DownloadClientSummary { id: string; name: string; enabled: boolean; }

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
  return useDashboardQuery<{ indexers: IndexerSummary[] }>(
    "indexers",
    "/api/v1/indexers",
  );
}

export function useDashboardDownloadClients() {
  return useDashboardQuery<{ download_clients: DownloadClientSummary[] }>(
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

// ---------------------------------------------------------------------------
// Active Downloads — polled for the dashboard activity widget
// ---------------------------------------------------------------------------

export interface DashboardQueueItem {
  id: string;
  title: string;
  status: string;
  progress: number;
  size_bytes: number;
  downloaded_bytes: number;
  download_rate: number;
  upload_rate: number;
}

export function useDashboardActivity() {
  return useQuery({
    queryKey: ["dashboard", "activity"],
    queryFn: async ({ signal }) => {
      const res = await apiFetch("/api/v1/activity", { signal });
      if (!res.ok) throw new Error("Failed to fetch activity");
      const json = (await res.json()) as { items: DashboardQueueItem[] };
      return json.items ?? [];
    },
    refetchInterval: 3000,
    staleTime: 2000,
    retry: 1,
  });
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
