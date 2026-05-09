// Typed fetch wrappers for the Loom API key management endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export interface APIKey {
  id: string;
  name: string;
  key: string;
  scopes: string;
  last_used: string;
  created_at: string;
}

export interface CreateAPIKeyRequest {
  name: string;
  scopes: string;
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
    try { parsed = JSON.parse(text); } catch { parsed = undefined; }
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

// ---------- API functions ----------

export async function listAPIKeys(signal?: AbortSignal): Promise<APIKey[]> {
  const data = await request<{ data: APIKey[] }>("GET", "/api/v1/api-keys", undefined, signal);
  return data?.data ?? [];
}

export async function createAPIKey(body: CreateAPIKeyRequest): Promise<APIKey> {
  return request<APIKey>("POST", "/api/v1/api-keys", body);
}

export async function deleteAPIKey(id: string): Promise<void> {
  return request<void>("DELETE", `/api/v1/api-keys/${encodeURIComponent(id)}`);
}

// ---------- React Query hooks ----------

export function useAPIKeys() {
  return useQuery({
    queryKey: ["api-keys"],
    queryFn: ({ signal }) => listAPIKeys(signal),
  });
}

export function useCreateAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createAPIKey,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["api-keys"] }),
  });
}

export function useDeleteAPIKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteAPIKey,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["api-keys"] }),
  });
}
