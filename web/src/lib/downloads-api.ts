// Typed fetch wrappers for the Loom download-client REST endpoints.
// The shapes mirror internal/downloads/{types,handlers}.go; keep them in
// sync if the contract changes.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { UseQueryOptions } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type DownloadKind =
  | "builtin/null"
  | "builtin/torrent"
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
  constructor(
    status: number,
    message: string,
    code?: string,
    details?: unknown,
  ) {
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
  const res = await apiFetch(path, init);
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
    throw new ApiError(
      res.status,
      message,
      env?.error?.code,
      env?.error?.details,
    );
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
export async function getDownload(
  id: string,
  signal?: AbortSignal,
): Promise<Download> {
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
  await request<void>(
    "DELETE",
    `/api/v1/download-clients/${encodeURIComponent(id)}`,
  );
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
  return request<TestResult>("POST", "/api/v1/download-clients/test", payload);
}

// ---------- Grab (send release to download client) ----------

export interface GrabRequest {
  magnet?: string;
  torrent_url?: string;
  nzb_url?: string;
  infohash?: string;
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

// ---------- Built-in torrent engine management ----------

export interface TorrentEngineSummary {
  total_torrents: number;
  downloading: number;
  seeding: number;
  paused: number;
  download_rate: number; // aggregate bytes/sec
  upload_rate: number; // aggregate bytes/sec
  download_limit: number; // bytes/sec, 0 = unlimited
  upload_limit: number; // bytes/sec, 0 = unlimited
  listen_port: number;
  dht: boolean;
  pex: boolean;
  upnp: boolean;
  save_path: string;
}

export async function getTorrentStatus(
  clientId: string,
  signal?: AbortSignal,
): Promise<TorrentEngineSummary> {
  return request<TorrentEngineSummary>(
    "GET",
    `/api/v1/download-clients/${encodeURIComponent(clientId)}/torrent/status`,
    undefined,
    signal,
  );
}

export async function setTorrentSpeedLimits(
  clientId: string,
  body: { download_limit: number; upload_limit: number },
): Promise<TorrentEngineSummary> {
  return request<TorrentEngineSummary>(
    "POST",
    `/api/v1/download-clients/${encodeURIComponent(clientId)}/torrent/speed-limits`,
    body,
  );
}

export async function torrentPauseAll(clientId: string): Promise<void> {
  await request<void>(
    "POST",
    `/api/v1/download-clients/${encodeURIComponent(clientId)}/torrent/pause-all`,
  );
}

export async function torrentResumeAll(clientId: string): Promise<void> {
  await request<void>(
    "POST",
    `/api/v1/download-clients/${encodeURIComponent(clientId)}/torrent/resume-all`,
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
 * Invalidates both download and movie queries so the UI reflects the grab.
 */
export function useGrabRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ clientId, ...body }: GrabRequest & { clientId: string }) =>
      grabRelease(clientId, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: downloadKeys.all });
      qc.invalidateQueries({ queryKey: ["movies"] });
    },
  });
}

// ---------- Built-in torrent engine hooks ----------

export const torrentEngineKeys = {
  status: (id: string) => [...downloadKeys.all, "torrent-status", id] as const,
};

/**
 * Poll the built-in torrent engine status. Only enabled when a clientId is
 * supplied for a builtin/torrent client.
 */
export function useTorrentStatus(
  clientId: string | undefined,
  options?: Omit<
    UseQueryOptions<TorrentEngineSummary, Error>,
    "queryKey" | "queryFn"
  >,
) {
  return useQuery<TorrentEngineSummary, Error>({
    queryKey: torrentEngineKeys.status(clientId ?? ""),
    queryFn: ({ signal }) => getTorrentStatus(clientId as string, signal),
    enabled: !!clientId,
    refetchInterval: 5000,
    ...options,
  });
}

export function useSetTorrentSpeedLimits() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      clientId,
      download_limit,
      upload_limit,
    }: {
      clientId: string;
      download_limit: number;
      upload_limit: number;
    }) => setTorrentSpeedLimits(clientId, { download_limit, upload_limit }),
    onSuccess: (data, { clientId }) => {
      // Seed the status cache with the authoritative summary the server
      // returned so the panel reflects the new limits immediately.
      qc.setQueryData(torrentEngineKeys.status(clientId), data);
      qc.invalidateQueries({ queryKey: torrentEngineKeys.status(clientId) });
      qc.invalidateQueries({ queryKey: downloadKeys.all });
    },
  });
}

export function useTorrentPauseAll() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clientId: string) => torrentPauseAll(clientId),
    onSuccess: (_data, clientId) => {
      qc.invalidateQueries({ queryKey: torrentEngineKeys.status(clientId) });
      qc.invalidateQueries({ queryKey: downloadKeys.all });
    },
  });
}

export function useTorrentResumeAll() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (clientId: string) => torrentResumeAll(clientId),
    onSuccess: (_data, clientId) => {
      qc.invalidateQueries({ queryKey: torrentEngineKeys.status(clientId) });
      qc.invalidateQueries({ queryKey: downloadKeys.all });
    },
  });
}
