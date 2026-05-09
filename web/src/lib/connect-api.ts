// Typed fetch wrappers for the Loom connect (media server integrations) REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type ProviderType = "plex" | "emby" | "jellyfin" | "trakt";

export interface ProviderSettings {
  host?: string;
  api_key?: string;
  // Trakt OAuth2
  client_id?: string;
  client_secret?: string;
  access_token?: string;
  refresh_token?: string;
  token_expiry?: string;
}

export interface ConnectConnection {
  id: string;
  name: string;
  provider: ProviderType;
  enabled: boolean;
  settings: ProviderSettings;
  notify_on_import: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateConnectRequest {
  name: string;
  provider: ProviderType;
  enabled?: boolean;
  settings: ProviderSettings;
  notify_on_import?: boolean;
}

export interface UpdateConnectRequest {
  name?: string;
  provider?: ProviderType;
  enabled?: boolean;
  settings?: ProviderSettings;
  notify_on_import?: boolean;
}

export interface TestResult {
  message: string;
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

export async function listConnections(
  signal?: AbortSignal,
): Promise<ConnectConnection[]> {
  const data = await request<ConnectConnection[]>(
    "GET",
    "/api/v1/connect",
    undefined,
    signal,
  );
  return data ?? [];
}

export async function getConnection(
  id: string,
  signal?: AbortSignal,
): Promise<ConnectConnection> {
  return request<ConnectConnection>(
    "GET",
    `/api/v1/connect/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createConnection(
  body: CreateConnectRequest,
): Promise<ConnectConnection> {
  return request<ConnectConnection>("POST", "/api/v1/connect", body);
}

export async function updateConnection(
  id: string,
  body: UpdateConnectRequest,
): Promise<ConnectConnection> {
  return request<ConnectConnection>(
    "PUT",
    `/api/v1/connect/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteConnection(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/connect/${encodeURIComponent(id)}`,
  );
}

export async function testConnection(id: string): Promise<TestResult> {
  return request<TestResult>(
    "POST",
    `/api/v1/connect/${encodeURIComponent(id)}/test`,
  );
}

export async function testConnectionConfig(
  body: CreateConnectRequest,
): Promise<TestResult> {
  return request<TestResult>("POST", "/api/v1/connect/test", body);
}

// ---------- Query keys ----------

export const connectKeys = {
  all: ["connect"] as const,
  list: () => [...connectKeys.all, "list"] as const,
  detail: (id: string) => [...connectKeys.all, "detail", id] as const,
};

// ---------- React Query hooks ----------

export function useConnections() {
  return useQuery<ConnectConnection[], Error>({
    queryKey: connectKeys.list(),
    queryFn: ({ signal }) => listConnections(signal),
  });
}

export function useCreateConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createConnection,
    onSuccess: () => qc.invalidateQueries({ queryKey: connectKeys.all }),
  });
}

export function useUpdateConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: UpdateConnectRequest;
    }) => updateConnection(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: connectKeys.all }),
  });
}

export function useDeleteConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteConnection,
    onSuccess: () => qc.invalidateQueries({ queryKey: connectKeys.all }),
  });
}

export function useTestConnection() {
  return useMutation({ mutationFn: testConnection });
}

export function useTestConnectionConfig() {
  return useMutation({ mutationFn: testConnectionConfig });
}

// ---------- Provider type metadata ----------

export const PROVIDER_TYPES: {
  value: ProviderType;
  label: string;
  description: string;
  fields: (keyof ProviderSettings)[];
}[] = [
  {
    value: "plex",
    label: "Plex",
    description: "Plex Media Server — library refresh on import",
    fields: ["host", "api_key"],
  },
  {
    value: "emby",
    label: "Emby",
    description: "Emby Media Server — library refresh on import",
    fields: ["host", "api_key"],
  },
  {
    value: "jellyfin",
    label: "Jellyfin",
    description: "Jellyfin Media Server — library refresh on import",
    fields: ["host", "api_key"],
  },
  {
    value: "trakt",
    label: "Trakt",
    description: "Trakt.tv — sync watched, collections, and watchlists",
    fields: ["client_id", "client_secret"],
  },
];

// ---------- Trakt OAuth & Sync API ----------

export async function traktGetAuthorizeUrl(body: {
  client_id: string;
  redirect_uri: string;
}): Promise<{ authorize_url: string }> {
  return request("POST", "/api/v1/connect/trakt/oauth/authorize", body);
}

export async function traktCallback(body: {
  code: string;
  client_id: string;
  client_secret: string;
  redirect_uri: string;
  connection_id: string;
}): Promise<ConnectConnection> {
  return request("POST", "/api/v1/connect/trakt/oauth/callback", body);
}

export async function traktRefreshToken(connectionId: string): Promise<ConnectConnection> {
  return request("POST", `/api/v1/connect/trakt/oauth/refresh/${encodeURIComponent(connectionId)}`);
}

export async function traktSyncWatched(connectionId: string): Promise<{ movies: number; shows: number }> {
  return request("POST", `/api/v1/connect/trakt/sync/watched/${encodeURIComponent(connectionId)}`);
}

export async function traktSyncCollection(connectionId: string): Promise<{ movies: number; shows: number }> {
  return request("POST", `/api/v1/connect/trakt/sync/collection/${encodeURIComponent(connectionId)}`);
}

export async function traktSyncWatchlist(connectionId: string): Promise<{ movies: number; shows: number }> {
  return request("POST", `/api/v1/connect/trakt/sync/watchlist/${encodeURIComponent(connectionId)}`);
}

// ---------- Trakt React Query hooks ----------

export function useTraktAuthorize() {
  return useMutation({ mutationFn: traktGetAuthorizeUrl });
}

export function useTraktCallback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: traktCallback,
    onSuccess: () => qc.invalidateQueries({ queryKey: connectKeys.all }),
  });
}

export function useTraktRefreshToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: traktRefreshToken,
    onSuccess: () => qc.invalidateQueries({ queryKey: connectKeys.all }),
  });
}

export function useTraktSyncWatched() {
  return useMutation({ mutationFn: traktSyncWatched });
}

export function useTraktSyncCollection() {
  return useMutation({ mutationFn: traktSyncCollection });
}

export function useTraktSyncWatchlist() {
  return useMutation({ mutationFn: traktSyncWatchlist });
}
