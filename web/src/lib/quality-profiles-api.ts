// Typed fetch wrappers for the Loom quality profiles endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export interface FormatItem {
  custom_format_id: string;
  score: number;
}

export interface QualityProfile {
  id: string;
  name: string;
  cutoff: string;
  items: string;
  upgrade_allowed: boolean;
  format_items: FormatItem[];
  created_at?: string;
  updated_at?: string;
}

export interface CreateQualityProfileRequest {
  name: string;
  cutoff: string;
  items: string;
  upgrade_allowed: boolean;
  format_items?: FormatItem[];
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

// ---------- API functions ----------

export async function listQualityProfiles(
  signal?: AbortSignal,
): Promise<QualityProfile[]> {
  const data = await request<{ data: QualityProfile[] }>(
    "GET",
    "/api/v1/quality-profiles",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function getQualityProfile(
  id: string,
  signal?: AbortSignal,
): Promise<QualityProfile> {
  return request<QualityProfile>(
    "GET",
    `/api/v1/quality-profiles/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createQualityProfile(
  body: CreateQualityProfileRequest,
): Promise<QualityProfile> {
  return request<QualityProfile>("POST", "/api/v1/quality-profiles", body);
}

export async function updateQualityProfile(
  id: string,
  body: Partial<QualityProfile>,
): Promise<QualityProfile> {
  return request<QualityProfile>(
    "PUT",
    `/api/v1/quality-profiles/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteQualityProfile(id: string): Promise<void> {
  return request<void>(
    "DELETE",
    `/api/v1/quality-profiles/${encodeURIComponent(id)}`,
  );
}

export async function getFormatScores(
  profileId: string,
  signal?: AbortSignal,
): Promise<FormatItem[]> {
  const data = await request<{ data: FormatItem[] }>(
    "GET",
    `/api/v1/quality-profiles/${encodeURIComponent(profileId)}/format-scores`,
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function setFormatScores(
  profileId: string,
  items: FormatItem[],
): Promise<FormatItem[]> {
  const data = await request<{ data: FormatItem[] }>(
    "PUT",
    `/api/v1/quality-profiles/${encodeURIComponent(profileId)}/format-scores`,
    items,
  );
  return data?.data ?? [];
}

// ---------- React Query hooks ----------

export function useQualityProfiles() {
  return useQuery({
    queryKey: ["quality-profiles"],
    queryFn: ({ signal }) => listQualityProfiles(signal),
  });
}

export function useQualityProfile(id: string) {
  return useQuery({
    queryKey: ["quality-profiles", id],
    queryFn: ({ signal }) => getQualityProfile(id, signal),
    enabled: !!id,
  });
}

export function useCreateQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createQualityProfile,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["quality-profiles"] }),
  });
}

export function useUpdateQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<QualityProfile> }) =>
      updateQualityProfile(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["quality-profiles"] }),
  });
}

export function useDeleteQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteQualityProfile,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["quality-profiles"] }),
  });
}

export function useFormatScores(profileId: string) {
  return useQuery({
    queryKey: ["quality-profiles", profileId, "format-scores"],
    queryFn: ({ signal }) => getFormatScores(profileId, signal),
    enabled: !!profileId,
  });
}

export function useSetFormatScores() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      profileId,
      items,
    }: {
      profileId: string;
      items: FormatItem[];
    }) => setFormatScores(profileId, items),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["quality-profiles"] }),
  });
}
