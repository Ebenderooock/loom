// Typed fetch wrappers for the Loom sync profiles REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// ---------- Types ----------

export interface SyncProfileIndexer {
  indexer_id: string;
  enabled: boolean;
}

export interface SyncProfileCategory {
  category: string;
  mapped_to: string;
}

export interface SyncProfile {
  id: string;
  name: string;
  app_type: string;
  enabled: boolean;
  indexers?: SyncProfileIndexer[];
  categories?: SyncProfileCategory[];
  created_at: string;
  updated_at: string;
}

export interface CreateSyncProfileRequest {
  name: string;
  app_type?: string;
  enabled?: boolean;
  indexers?: SyncProfileIndexer[];
  categories?: SyncProfileCategory[];
}

export interface UpdateSyncProfileRequest {
  name?: string;
  app_type?: string;
  enabled?: boolean;
  indexers?: SyncProfileIndexer[];
  categories?: SyncProfileCategory[];
}

// ---------- HTTP helpers ----------

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(status: number, message: string, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
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
      | { error?: { code?: string; message?: string } }
      | undefined;
    const message =
      env?.error?.message ??
      `${method} ${path} failed: ${res.status} ${res.statusText}`;
    throw new ApiError(res.status, message, env?.error?.code);
  }
  return parsed as T;
}

// ---------- API functions ----------

export async function listSyncProfiles(
  signal?: AbortSignal,
): Promise<SyncProfile[]> {
  const data = await request<SyncProfile[]>(
    "GET",
    "/api/v1/sync-profiles",
    undefined,
    signal,
  );
  return data ?? [];
}

export async function getSyncProfile(
  id: string,
  signal?: AbortSignal,
): Promise<SyncProfile> {
  return request<SyncProfile>(
    "GET",
    `/api/v1/sync-profiles/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createSyncProfile(
  body: CreateSyncProfileRequest,
): Promise<SyncProfile> {
  return request<SyncProfile>("POST", "/api/v1/sync-profiles", body);
}

export async function updateSyncProfile(
  id: string,
  body: UpdateSyncProfileRequest,
): Promise<SyncProfile> {
  return request<SyncProfile>(
    "PUT",
    `/api/v1/sync-profiles/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteSyncProfile(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/sync-profiles/${encodeURIComponent(id)}`,
  );
}

// ---------- Query keys ----------

export const syncProfileKeys = {
  all: ["sync-profiles"] as const,
  list: () => [...syncProfileKeys.all, "list"] as const,
  detail: (id: string) => [...syncProfileKeys.all, "detail", id] as const,
};

// ---------- React Query hooks ----------

export function useSyncProfiles() {
  return useQuery<SyncProfile[], Error>({
    queryKey: syncProfileKeys.list(),
    queryFn: ({ signal }) => listSyncProfiles(signal),
  });
}

export function useSyncProfile(id: string) {
  return useQuery<SyncProfile, Error>({
    queryKey: syncProfileKeys.detail(id),
    queryFn: ({ signal }) => getSyncProfile(id, signal),
    enabled: !!id,
  });
}

export function useCreateSyncProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createSyncProfile,
    onSuccess: () => qc.invalidateQueries({ queryKey: syncProfileKeys.all }),
  });
}

export function useUpdateSyncProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: UpdateSyncProfileRequest;
    }) => updateSyncProfile(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: syncProfileKeys.all }),
  });
}

export function useDeleteSyncProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteSyncProfile,
    onSuccess: () => qc.invalidateQueries({ queryKey: syncProfileKeys.all }),
  });
}
