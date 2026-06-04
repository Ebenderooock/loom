// Typed fetch wrappers + react-query hooks for the media-requests REST API.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type RequestMediaType = "movie" | "series";

export type RequestStatus =
  | "pending"
  | "approving"
  | "approved"
  | "available"
  | "rejected"
  | "failed";

export interface MediaRequest {
  id: string;
  user_id: string;
  username: string;
  media_type: RequestMediaType;
  tmdb_id: string;
  title: string;
  year: number;
  poster_path?: string;
  overview?: string;
  status: RequestStatus;
  reason?: string;
  media_id?: string;
  decided_by?: string;
  decided_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateRequestInput {
  media_type: RequestMediaType;
  tmdb_id: string;
  title: string;
  year?: number;
  poster_path?: string;
  overview?: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public code?: string,
  ) {
    super(message);
    this.name = "ApiError";
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

export async function createRequest(
  body: CreateRequestInput,
): Promise<MediaRequest> {
  return request("POST", "/api/v1/requests", body);
}

export async function listMyRequests(
  signal?: AbortSignal,
): Promise<MediaRequest[]> {
  const data = await request<{ data: MediaRequest[] }>(
    "GET",
    "/api/v1/requests/mine",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function listAllRequests(
  status?: RequestStatus | "all",
  signal?: AbortSignal,
): Promise<MediaRequest[]> {
  const qs = status && status !== "all" ? `?status=${encodeURIComponent(status)}` : "";
  const data = await request<{ data: MediaRequest[] }>(
    "GET",
    `/api/v1/requests${qs}`,
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function approveRequest(
  id: string,
  qualityProfileId: string,
  libraryId: string,
): Promise<MediaRequest> {
  return request("POST", `/api/v1/requests/${encodeURIComponent(id)}/approve`, {
    quality_profile_id: qualityProfileId,
    library_id: libraryId,
  });
}

export async function rejectRequest(
  id: string,
  reason: string,
): Promise<MediaRequest> {
  return request("POST", `/api/v1/requests/${encodeURIComponent(id)}/reject`, {
    reason,
  });
}

// ---------- Query keys ----------

export const requestKeys = {
  all: ["requests"] as const,
  mine: () => [...requestKeys.all, "mine"] as const,
  list: (status?: string) => [...requestKeys.all, "list", status ?? "all"] as const,
};

// ---------- React Query hooks ----------

export function useMyRequests() {
  return useQuery<MediaRequest[], Error>({
    queryKey: requestKeys.mine(),
    queryFn: ({ signal }) => listMyRequests(signal),
  });
}

export function useAllRequests(status?: RequestStatus | "all") {
  return useQuery<MediaRequest[], Error>({
    queryKey: requestKeys.list(status),
    queryFn: ({ signal }) => listAllRequests(status, signal),
  });
}

export function useCreateRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createRequest,
    onSuccess: () => qc.invalidateQueries({ queryKey: requestKeys.all }),
  });
}

export function useApproveRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      qualityProfileId,
      libraryId,
    }: {
      id: string;
      qualityProfileId: string;
      libraryId: string;
    }) => approveRequest(id, qualityProfileId, libraryId),
    onSuccess: () => qc.invalidateQueries({ queryKey: requestKeys.all }),
  });
}

export function useRejectRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      rejectRequest(id, reason),
    onSuccess: () => qc.invalidateQueries({ queryKey: requestKeys.all }),
  });
}
