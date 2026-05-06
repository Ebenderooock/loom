// Typed fetch wrappers for the Loom download-client REST endpoints.
// The shapes mirror internal/downloads/{types,handlers}.go; keep them in
// sync if the contract changes.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { UseQueryOptions } from "@tanstack/react-query";

// ---------- Types ----------

export type DownloadKind =
  | "builtin/null"
  | "qbittorrent"
  | "transmission"
  | "deluge"
  | "sabnzbd"
  | "nzbget";
export type DownloadProtocol = "torrent" | "usenet";
export type DownloadHealthStatus = "ok" | "degraded" | "failed" | "unknown";

export interface DownloadHealth {
  client_id: string;
  status: DownloadHealthStatus;
  last_checked_at: string;
  last_success_at?: string;
  last_failure_at?: string;
  last_error?: string;
  consecutive_failures: number;
  last_free_space_bytes?: number;
  last_categories?: Array<{ name: string; save_path?: string }>;
}

export interface Download {
  id: string;
  kind: DownloadKind | string;
  name: string;
  protocol: DownloadProtocol;
  enabled: boolean;
  priority: number;
  host?: string;
  port?: number;
  tls?: boolean;
  username?: string;
  password?: string;
  config?: Record<string, unknown>;
  category_default?: string;
  save_path_default?: string;
  remove_completed?: boolean;
  remove_failed?: boolean;
  created_at?: string;
  updated_at?: string;
  health?: DownloadHealth;
}

export interface DownloadCreate {
  id?: string;
  kind: DownloadKind | string;
  name: string;
  protocol: DownloadProtocol;
  enabled?: boolean;
  priority?: number;
  host?: string;
  port?: number;
  tls?: boolean;
  username?: string;
  password?: string;
  config?: Record<string, unknown>;
  category_default?: string;
  save_path_default?: string;
  remove_completed?: boolean;
  remove_failed?: boolean;
}

export interface DownloadPatch {
  name?: string;
  enabled?: boolean;
  priority?: number;
  host?: string;
  port?: number;
  tls?: boolean;
  username?: string;
  password?: string;
  config?: Record<string, unknown>;
  category_default?: string;
  save_path_default?: string;
  remove_completed?: boolean;
  remove_failed?: boolean;
}

export interface TestResult {
  ok: boolean;
  error?: string;
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
  }
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
  const res = await fetch(path, init);
  if (res.status === 204) {
    return undefined as T;
  }
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
    const env = parsed as
      | { error?: { code?: string; message?: string; details?: unknown } }
      | undefined;
    const message =
      env?.error?.message ??
      (typeof parsed === "string" ? parsed : undefined) ??
      `${method} ${path} failed: ${res.status} ${res.statusText}`;
    throw new ApiError(res.status, message, env?.error?.code, env?.error?.details);
  }
  return parsed as T;
}

// ---------- Download endpoints ----------

export const downloadKeys = {
  all: ["downloads"] as const,
  list: () => [...downloadKeys.all, "list"] as const,
  detail: (id: string) => [...downloadKeys.all, "detail", id] as const,
};

/**
 * Fetch all configured download clients.
 */
export async function listDownloads(signal?: AbortSignal): Promise<Download[]> {
  const env = await request<{ download_clients: Download[] }>(
    "GET",
    "/api/v1/download-clients/",
    undefined,
    signal,
  );
  return env.download_clients ?? [];
}

/**
 * Fetch a single download client by ID.
 */
export async function getDownload(id: string, signal?: AbortSignal): Promise<Download> {
  return request<Download>(
    "GET",
    `/api/v1/download-clients/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

/**
 * Create a new download client.
 */
export async function createDownload(body: DownloadCreate): Promise<Download> {
  return request<Download>("POST", "/api/v1/download-clients/", body);
}

/**
 * Update a download client via PATCH.
 */
export async function patchDownload(
  id: string,
  body: DownloadPatch,
): Promise<Download> {
  return request<Download>(
    "PATCH",
    `/api/v1/download-clients/${encodeURIComponent(id)}`,
    body,
  );
}

/**
 * Delete a download client.
 */
export async function deleteDownload(id: string): Promise<void> {
  await request<void>("DELETE", `/api/v1/download-clients/${encodeURIComponent(id)}`);
}

/**
 * Test connectivity and authentication for a download client.
 */
export async function testDownload(id: string): Promise<TestResult> {
  return request<TestResult>(
    "POST",
    `/api/v1/download-clients/${encodeURIComponent(id)}/test`,
  );
}

export async function testDownloadConfig(
  payload: Record<string, unknown>,
): Promise<TestResult> {
  return request<TestResult>(
    "POST",
    "/api/v1/download-clients/test",
    payload,
  );
}

// ---------- Grab (send release to download client) ----------

export interface GrabRequest {
  magnet?: string;
  torrent_url?: string;
  nzb_url?: string;
  title?: string;
  category?: string;
  save_path?: string;
  tags?: string[];
  // Media context for grab tracking (import pipeline matching)
  media_type?: "movie" | "episode";
  series_id?: string;
  episode_ids?: string[];
  movie_id?: string;
}

export interface GrabResult {
  client_id: string;
  item_id: string;
}

export async function grabRelease(
  clientId: string,
  body: GrabRequest,
): Promise<GrabResult> {
  return request<GrabResult>(
    "POST",
    `/api/v1/download-clients/${encodeURIComponent(clientId)}/items`,
    body,
  );
}

// ---------- React Query hooks ----------

/**
 * Query hook to fetch all download clients.
 */
export function useDownloads(
  options?: Omit<UseQueryOptions<Download[], Error>, "queryKey" | "queryFn">,
) {
  return useQuery<Download[], Error>({
    queryKey: downloadKeys.list(),
    queryFn: ({ signal }) => listDownloads(signal),
    ...options,
  });
}

/**
 * Query hook to fetch a single download client.
 */
export function useDownload(
  id: string,
  options?: Omit<UseQueryOptions<Download, Error>, "queryKey" | "queryFn">,
) {
  return useQuery<Download, Error>({
    queryKey: downloadKeys.detail(id),
    queryFn: ({ signal }) => getDownload(id, signal),
    enabled: !!id,
    ...options,
  });
}

/**
 * Mutation hook to create a new download client.
 */
export function useCreateDownload() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createDownload,
    onSuccess: () => qc.invalidateQueries({ queryKey: downloadKeys.all }),
  });
}

/**
 * Mutation hook to update a download client.
 */
export function usePatchDownload() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: DownloadPatch }) =>
      patchDownload(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: downloadKeys.all }),
  });
}

/**
 * Mutation hook to delete a download client.
 */
export function useDeleteDownload() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteDownload,
    onSuccess: () => qc.invalidateQueries({ queryKey: downloadKeys.all }),
  });
}

/**
 * Mutation hook to test a download client.
 */
export function useTestDownload() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: testDownload,
    onSuccess: () => qc.invalidateQueries({ queryKey: downloadKeys.all }),
  });
}

export function useTestDownloadConfig() {
  return useMutation({ mutationFn: testDownloadConfig });
}

/**
 * Mutation hook to grab a release (send to a download client).
 */
export function useGrabRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ clientId, ...body }: GrabRequest & { clientId: string }) =>
      grabRelease(clientId, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: downloadKeys.all }),
  });
}
