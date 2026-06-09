// Typed fetch wrappers + React Query hooks for the Loom request-bot endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export type Platform = "telegram" | "discord";

export interface BotConfig {
  telegram_enabled: boolean;
  telegram_token_set: boolean;
  discord_enabled: boolean;
  discord_token_set: boolean;
  default_movie_quality_profile_id: string;
  default_movie_library_id: string;
  default_series_quality_profile_id: string;
  default_series_library_id: string;
  default_music_quality_profile_id: string;
  default_music_library_id: string;
  updated_at: string;
}

export interface UpdateBotConfig {
  telegram_enabled?: boolean;
  telegram_bot_token?: string;
  discord_enabled?: boolean;
  discord_bot_token?: string;
  default_movie_quality_profile_id?: string;
  default_movie_library_id?: string;
  default_series_quality_profile_id?: string;
  default_series_library_id?: string;
  default_music_quality_profile_id?: string;
  default_music_library_id?: string;
}

export interface BotStatus {
  platform: Platform;
  running: boolean;
  last_error?: string;
}

export interface BotLink {
  id: string;
  platform: Platform;
  external_id: string;
  external_username: string;
  user_id: number;
  username: string;
  created_at: string;
}

export interface LinkPreview {
  platform: Platform;
  external_username: string;
  expires_at: string;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const init: RequestInit = { method, signal };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  const res = await apiFetch(path, init);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let parsed: unknown;
  if (text.length > 0) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = undefined;
    }
  }
  if (!res.ok) {
    const env = parsed as { error?: { message?: string } } | undefined;
    throw new ApiError(
      res.status,
      env?.error?.message ?? `${method} ${path} failed: ${res.status}`,
    );
  }
  return parsed as T;
}

// ---------- Hooks ----------

export function useBotConfig() {
  return useQuery({
    queryKey: ["bots", "config"],
    queryFn: ({ signal }) =>
      request<BotConfig>("GET", "/api/v1/bots/config", undefined, signal),
  });
}

export function useUpdateBotConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: UpdateBotConfig) =>
      request<BotConfig>("PUT", "/api/v1/bots/config", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["bots", "config"] });
      qc.invalidateQueries({ queryKey: ["bots", "status"] });
    },
  });
}

export function useBotStatus() {
  return useQuery({
    queryKey: ["bots", "status"],
    queryFn: ({ signal }) =>
      request<BotStatus[]>("GET", "/api/v1/bots/status", undefined, signal),
    refetchInterval: 15000,
  });
}

export function useBotLinks() {
  return useQuery({
    queryKey: ["bots", "links"],
    queryFn: ({ signal }) =>
      request<BotLink[]>("GET", "/api/v1/bots/links", undefined, signal),
  });
}

export function useDeleteBotLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      request<void>("DELETE", `/api/v1/bots/links/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["bots", "links"] }),
  });
}

export function usePreviewLinkCode() {
  return useMutation({
    mutationFn: (code: string) =>
      request<LinkPreview>("POST", "/api/v1/bots/link/preview", { code }),
  });
}

export function useRedeemLinkCode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (code: string) =>
      request<BotLink>("POST", "/api/v1/bots/link/redeem", { code }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["bots", "links"] }),
  });
}
