// Typed fetch wrappers + react-query hooks for the media-requests REST API.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type RequestMediaType = "movie" | "series" | "artist";
export type RequestAvailability = "present" | "missing" | "unknown";

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
  availability?: RequestAvailability;
  availability_reason?: string;
  availability_checked_at?: string;
  fulfilled_at?: string;
  re_request_of?: string;
  media_server_availability?: RequestAvailability;
  monitored?: boolean | null;
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
  re_request_of?: string;
}

export interface ReconciliationOutcome {
  request_id: string;
  before?: RequestAvailability | string;
  after?: RequestAvailability | string;
  reason: string;
  changed: boolean;
  error?: string;
}

export interface ReconcileResponse {
  dry_run: boolean;
  outcomes: ReconciliationOutcome[];
}

export interface ReconciliationRun {
  id: string;
  actor: string;
  dry_run: boolean;
  created_at: string;
  outcomes: ReconciliationOutcome[];
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
  const qs =
    status && status !== "all" ? `?status=${encodeURIComponent(status)}` : "";
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

export async function clearRequests(): Promise<void> {
  return request("DELETE", "/api/v1/requests");
}

export async function reconcileRequests(
  dryRun: boolean,
): Promise<ReconcileResponse> {
  return request("POST", "/api/v1/requests/reconcile", { dry_run: dryRun });
}

export async function listReconciliationHistory(
  signal?: AbortSignal,
): Promise<ReconciliationRun[]> {
  const data = await request<{ data: ReconciliationRun[] }>(
    "GET",
    "/api/v1/requests/reconciliation",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

// ---------- Quotas ----------

export interface MediaQuota {
  limit: number;
  used: number;
  remaining: number; // -1 = unlimited
  unlimited: boolean;
}

export interface QuotaStatus {
  window_days: number;
  movie: MediaQuota;
  series: MediaQuota;
  music: MediaQuota;
}

export interface QuotaConfig {
  movie_limit: number;
  series_limit: number;
  music_limit: number;
  window_days: number;
}

export async function getQuotaStatus(
  signal?: AbortSignal,
): Promise<QuotaStatus> {
  return request("GET", "/api/v1/requests/quota", undefined, signal);
}

export async function getQuotaConfig(
  signal?: AbortSignal,
): Promise<QuotaConfig> {
  return request("GET", "/api/v1/requests/quota/config", undefined, signal);
}

export async function updateQuotaConfig(
  cfg: QuotaConfig,
): Promise<QuotaConfig> {
  return request("PUT", "/api/v1/requests/quota/config", cfg);
}

// ---------- Query keys ----------

export const requestKeys = {
  all: ["requests"] as const,
  mine: () => [...requestKeys.all, "mine"] as const,
  list: (status?: string) =>
    [...requestKeys.all, "list", status ?? "all"] as const,
  quota: () => [...requestKeys.all, "quota"] as const,
  quotaConfig: () => [...requestKeys.all, "quota", "config"] as const,
  reconciliation: () => [...requestKeys.all, "reconciliation"] as const,
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

export function useClearRequests() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: clearRequests,
    onSuccess: () => qc.invalidateQueries({ queryKey: requestKeys.all }),
  });
}

export function useReconcileRequests() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ dryRun }: { dryRun: boolean }) => reconcileRequests(dryRun),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: requestKeys.all });
      qc.invalidateQueries({ queryKey: requestKeys.reconciliation() });
    },
  });
}

export function useReconciliationHistory(enabled = true) {
  return useQuery<ReconciliationRun[], Error>({
    queryKey: requestKeys.reconciliation(),
    queryFn: ({ signal }) => listReconciliationHistory(signal),
    enabled,
  });
}

export function useQuotaStatus(enabled = true) {
  return useQuery<QuotaStatus, Error>({
    queryKey: requestKeys.quota(),
    queryFn: ({ signal }) => getQuotaStatus(signal),
    enabled,
  });
}

export function useQuotaConfig(enabled = true) {
  return useQuery<QuotaConfig, Error>({
    queryKey: requestKeys.quotaConfig(),
    queryFn: ({ signal }) => getQuotaConfig(signal),
    enabled,
  });
}

export function useUpdateQuotaConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: updateQuotaConfig,
    onSuccess: () => qc.invalidateQueries({ queryKey: requestKeys.all }),
  });
}
