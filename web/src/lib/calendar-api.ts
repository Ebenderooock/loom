// Typed fetch wrappers for the Loom calendar REST endpoint.

import { useQuery } from "@tanstack/react-query";

// ---------- Types ----------

export interface CalendarEvent {
  id: string;
  title: string;
  type: "movie" | "episode";
  date: string;
  status: "missing" | "downloaded";
  year?: number;
  seriesTitle?: string;
  season?: number;
  episode?: number;
  episodeTitle?: string;
}

// ---------- HTTP helpers ----------

async function request<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, { method: "GET", signal, credentials: "include" });
  if (!res.ok) {
    throw new Error(`GET ${path} failed: ${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

// ---------- API functions ----------

export async function fetchCalendarEvents(
  start: string,
  end: string,
  signal?: AbortSignal,
): Promise<CalendarEvent[]> {
  const params = new URLSearchParams({ start, end });
  const data = await request<CalendarEvent[]>(
    `/api/v1/calendar?${params.toString()}`,
    signal,
  );
  return data ?? [];
}

// ---------- React Query hooks ----------

export function useCalendarEvents(start: string, end: string) {
  return useQuery({
    queryKey: ["calendar", start, end],
    queryFn: ({ signal }) => fetchCalendarEvents(start, end, signal),
    enabled: !!start && !!end,
  });
}
