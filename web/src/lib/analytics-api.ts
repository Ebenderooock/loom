// Typed fetch wrappers + react-query hooks for media-server analytics.

import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

export interface LiveStream {
  connection_id: string;
  connection_name: string;
  provider: string;
  session_key: string;
  media_id: string;
  user: string;
  media_type: string;
  title: string;
  grandparent_title?: string;
  full_title: string;
  device?: string;
  state: string; // playing | paused
  position_ms: number;
  duration_ms: number;
  transcode: boolean;
  progress: number; // 0..100
}

export interface HistoryRecord {
  id: string;
  connection_id: string;
  provider: string;
  user: string;
  media_type: string;
  full_title: string;
  device: string;
  transcode: boolean;
  started_at: string;
  last_seen_at: string;
  duration_ms: number;
  watched_ms: number;
  ended_at?: string;
}

export interface UserStat {
  user: string;
  plays: number;
  watched_ms: number;
}

export interface MediaStat {
  media_id: string;
  title: string;
  media_type: string;
  plays: number;
  watched_ms: number;
}

export interface DayCount {
  day: string;
  plays: number;
}

export interface AnalyticsStats {
  window_days: number;
  totals: { plays: number; unique_users: number; watched_ms: number };
  top_users: UserStat[];
  top_media: MediaStat[];
  least_media: MediaStat[];
  plays_per_day: DayCount[];
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await apiFetch(path, { signal });
  if (!res.ok) throw new Error(`${path} failed: ${res.status}`);
  return (await res.json()) as T;
}

export function useActiveStreams() {
  return useQuery({
    queryKey: ["analytics", "streams"],
    queryFn: ({ signal }) =>
      getJSON<{ streams: LiveStream[] }>("/api/v1/analytics/streams", signal).then(
        (b) => b.streams ?? [],
      ),
    refetchInterval: 10_000,
  });
}

export function useAnalyticsHistory(limit = 50) {
  return useQuery({
    queryKey: ["analytics", "history", limit],
    queryFn: ({ signal }) =>
      getJSON<{ history: HistoryRecord[] }>(
        `/api/v1/analytics/history?limit=${limit}`,
        signal,
      ).then((b) => b.history ?? []),
  });
}

export function useAnalyticsStats(windowDays = 30) {
  return useQuery({
    queryKey: ["analytics", "stats", windowDays],
    queryFn: ({ signal }) =>
      getJSON<AnalyticsStats>(`/api/v1/analytics/stats?days=${windowDays}`, signal),
  });
}

export function formatWatched(ms: number): string {
  const totalMin = Math.round(ms / 60000);
  if (totalMin < 60) return `${totalMin}m`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}
