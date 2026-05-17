// Typed fetch wrappers for the Loom import-lists REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type ListType =
  | "trakt_list"
  | "trakt_watchlist"
  | "imdb_list"
  | "imdb_watchlist"
  | "tmdb_list"
  | "tmdb_popular"
  | "plex_watchlist"
  | "rss"
  | "sonarr"
  | "radarr";

export type MediaType = "movie" | "series";
export type MonitorType = "all" | "future" | "missing" | "none";
export type ItemStatus = "pending" | "added" | "excluded" | "failed";

export interface ImportList {
  id: string;
  name: string;
  list_type: ListType;
  enabled: boolean;
  url?: string;
  api_key?: string;
  access_token?: string;
  sync_interval_minutes: number;
  root_folder_path?: string;
  quality_profile_id: string;
  media_type: MediaType;
  monitor_type: MonitorType;
  search_on_add: boolean;
  last_sync?: string;
  settings: string;
  created_at: string;
  updated_at: string;
  item_count?: number;
}

export interface ImportListItem {
  id: string;
  list_id: string;
  external_id: string;
  title: string;
  year?: number;
  imdb_id?: string;
  tmdb_id?: string;
  tvdb_id?: string;
  status: ItemStatus;
  last_seen: string;
  created_at: string;
}

export interface ImportListExclusion {
  id: string;
  tmdb_id?: string;
  tvdb_id?: string;
  imdb_id?: string;
  title: string;
  year?: number;
  created_at: string;
}

export interface CreateImportListRequest {
  name: string;
  list_type: ListType;
  enabled?: boolean;
  url?: string;
  api_key?: string;
  access_token?: string;
  sync_interval_minutes?: number;
  root_folder_path?: string;
  quality_profile_id?: string;
  media_type?: MediaType;
  monitor_type?: MonitorType;
  search_on_add?: boolean;
  settings?: string;
}

export interface UpdateImportListRequest {
  name?: string;
  list_type?: ListType;
  enabled?: boolean;
  url?: string;
  api_key?: string;
  access_token?: string;
  sync_interval_minutes?: number;
  root_folder_path?: string;
  quality_profile_id?: string;
  media_type?: MediaType;
  monitor_type?: MonitorType;
  search_on_add?: boolean;
  settings?: string;
}

export interface CreateExclusionRequest {
  title: string;
  tmdb_id?: string;
  tvdb_id?: string;
  imdb_id?: string;
  year?: number;
}

export interface ImportListDetail {
  list: ImportList;
  items: ImportListItem[];
}

// ---------- HTTP helpers ----------

export class ApiError extends Error {
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
    const env = parsed as { error?: string } | undefined;
    throw new ApiError(
      res.status,
      env?.error ?? `${method} ${path}: ${res.status} ${res.statusText}`,
    );
  }
  return parsed as T;
}

// ---------- API functions ----------

export async function listImportLists(
  signal?: AbortSignal,
): Promise<ImportList[]> {
  const data = await request<{ data: ImportList[] }>(
    "GET",
    "/api/v1/import-lists",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function getImportList(
  id: string,
  signal?: AbortSignal,
): Promise<ImportListDetail> {
  return request<ImportListDetail>(
    "GET",
    `/api/v1/import-lists/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createImportList(
  body: CreateImportListRequest,
): Promise<ImportList> {
  return request<ImportList>("POST", "/api/v1/import-lists", body);
}

export async function updateImportList(
  id: string,
  body: UpdateImportListRequest,
): Promise<ImportList> {
  return request<ImportList>(
    "PUT",
    `/api/v1/import-lists/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteImportList(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/import-lists/${encodeURIComponent(id)}`,
  );
}

export async function syncImportList(
  id: string,
): Promise<{ message: string }> {
  return request<{ message: string }>(
    "POST",
    `/api/v1/import-lists/${encodeURIComponent(id)}/sync`,
  );
}

export async function listExclusions(
  signal?: AbortSignal,
): Promise<ImportListExclusion[]> {
  const data = await request<{ data: ImportListExclusion[] }>(
    "GET",
    "/api/v1/import-lists/exclusions",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function createExclusion(
  body: CreateExclusionRequest,
): Promise<ImportListExclusion> {
  return request<ImportListExclusion>(
    "POST",
    "/api/v1/import-lists/exclusions",
    body,
  );
}

export async function deleteExclusion(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/import-lists/exclusions/${encodeURIComponent(id)}`,
  );
}

// ---------- Query keys ----------

export const importListKeys = {
  all: ["import-lists"] as const,
  list: () => [...importListKeys.all, "list"] as const,
  detail: (id: string) => [...importListKeys.all, "detail", id] as const,
  exclusions: () => [...importListKeys.all, "exclusions"] as const,
};

// ---------- React Query hooks ----------

export function useImportLists() {
  return useQuery<ImportList[], Error>({
    queryKey: importListKeys.list(),
    queryFn: ({ signal }) => listImportLists(signal),
  });
}

export function useImportListDetail(id: string) {
  return useQuery<ImportListDetail, Error>({
    queryKey: importListKeys.detail(id),
    queryFn: ({ signal }) => getImportList(id, signal),
    enabled: !!id,
  });
}

export function useCreateImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createImportList,
    onSuccess: () => qc.invalidateQueries({ queryKey: importListKeys.all }),
  });
}

export function useUpdateImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: UpdateImportListRequest;
    }) => updateImportList(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: importListKeys.all }),
  });
}

export function useDeleteImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteImportList,
    onSuccess: () => qc.invalidateQueries({ queryKey: importListKeys.all }),
  });
}

export function useSyncImportList() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: syncImportList,
    onSuccess: () => qc.invalidateQueries({ queryKey: importListKeys.all }),
  });
}

export function useExclusions() {
  return useQuery<ImportListExclusion[], Error>({
    queryKey: importListKeys.exclusions(),
    queryFn: ({ signal }) => listExclusions(signal),
  });
}

export function useCreateExclusion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createExclusion,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: importListKeys.exclusions() }),
  });
}

export function useDeleteExclusion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteExclusion,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: importListKeys.exclusions() }),
  });
}

// ---------- List type metadata ----------

export const LIST_TYPES: {
  value: ListType;
  label: string;
  mediaType: MediaType;
  fields: string[];
}[] = [
  {
    value: "trakt_list",
    label: "Trakt List",
    mediaType: "movie",
    fields: ["url"],
  },
  {
    value: "trakt_watchlist",
    label: "Trakt Watchlist",
    mediaType: "movie",
    fields: [],
  },
  {
    value: "imdb_list",
    label: "IMDb List",
    mediaType: "movie",
    fields: ["url"],
  },
  {
    value: "imdb_watchlist",
    label: "IMDb Watchlist",
    mediaType: "movie",
    fields: ["url"],
  },
  {
    value: "tmdb_list",
    label: "TMDb List",
    mediaType: "movie",
    fields: ["url", "api_key"],
  },
  {
    value: "tmdb_popular",
    label: "TMDb Popular",
    mediaType: "movie",
    fields: ["api_key"],
  },
  {
    value: "plex_watchlist",
    label: "Plex Watchlist",
    mediaType: "movie",
    fields: ["url"],
  },
  {
    value: "rss",
    label: "RSS Feed",
    mediaType: "movie",
    fields: ["url"],
  },
  {
    value: "sonarr",
    label: "Sonarr",
    mediaType: "series",
    fields: ["url", "api_key"],
  },
  {
    value: "radarr",
    label: "Radarr",
    mediaType: "movie",
    fields: ["url", "api_key"],
  },
];

export const MONITOR_TYPES: { value: MonitorType; label: string }[] = [
  { value: "all", label: "All" },
  { value: "future", label: "Future Only" },
  { value: "missing", label: "Missing Only" },
  { value: "none", label: "None" },
];

export const SYNC_INTERVALS: { value: number; label: string }[] = [
  { value: 60, label: "Every Hour" },
  { value: 360, label: "Every 6 Hours" },
  { value: 720, label: "Every 12 Hours" },
  { value: 1440, label: "Every 24 Hours" },
];
