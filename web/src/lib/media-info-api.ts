// Typed fetch wrappers for the Loom media-info / media-preferences REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// ---------- Types ----------

export interface MediaInfo {
  audio_codec: string;
  audio_channels: string;
  video_codec: string;
  resolution: string;
  hdr: string;
  audio_languages: string[];
  sub_languages: string[];
  sub_type: string;
  source: string;
}

export interface MediaPreferences {
  id: string;
  preferred_audio: string[];
  preferred_sub_languages: string[];
  require_subtitles: boolean;
  prefer_hdr: boolean;
  prefer_atmos: boolean;
  created_at?: string;
  updated_at?: string;
}

// ---------- HTTP helpers ----------

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const init: RequestInit = { method, signal, credentials: "include" };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let parsed: unknown;
  if (text.length > 0) {
    try { parsed = JSON.parse(text); } catch { parsed = undefined; }
  }
  if (!res.ok) {
    const env = parsed as { error?: string } | undefined;
    throw new ApiError(
      res.status,
      env?.error ?? `${method} ${path} failed: ${res.status}`,
    );
  }
  return parsed as T;
}

// ---------- API functions ----------

export async function getMediaPreferences(signal?: AbortSignal): Promise<MediaPreferences> {
  return request<MediaPreferences>("GET", "/api/v1/media-info/preferences", undefined, signal);
}

export async function updateMediaPreferences(prefs: Partial<MediaPreferences>): Promise<MediaPreferences> {
  return request<MediaPreferences>("PUT", "/api/v1/media-info/preferences", prefs);
}

export async function parseReleaseName(name: string): Promise<MediaInfo> {
  return request<MediaInfo>("POST", "/api/v1/media-info/parse", { name });
}

// ---------- React Query hooks ----------

export function useMediaPreferences() {
  return useQuery({
    queryKey: ["media-preferences"],
    queryFn: ({ signal }) => getMediaPreferences(signal),
  });
}

export function useUpdateMediaPreferences() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: updateMediaPreferences,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["media-preferences"] }),
  });
}

export function useParseReleaseName() {
  return useMutation({
    mutationFn: parseReleaseName,
  });
}
