// Typed fetch wrappers for import management endpoints.
// Mirrors internal/imports/ types; keep in sync if the contract changes.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// ---------- Types ----------

export interface ImportRecord {
  id: string;
  media_type: string;
  media_id: string;
  source_path: string;
  dest_path: string;
  import_mode: string;
  status: string;
  error?: string;
  imported_at: string;
}

export interface ScanResult {
  file_path: string;
  file_size: number;
  detected_title: string;
  detected_year?: number;
  detected_season?: number;
  detected_episode?: number;
  matched_media?: string;
  matched_media_id?: string;
  media_type?: string;
  confidence: number;
  suggested_action: string;
  quality?: string;
}

export interface ImportDecision {
  id: string;
  timestamp: string;
  source_path: string;
  dest_path: string;
  media_type: string;
  media_id: string;
  action: string;
  reason: string;
  conflict_policy: string;
  file_size: number;
  file_quality: string;
  created_at: string;
}

// ---------- API calls ----------

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(
      (body as Record<string, string>).error ?? `${res.status} ${res.statusText}`,
    );
  }
  return (await res.json()) as T;
}

export async function fetchImportHistory(
  limit = 50,
  offset = 0,
): Promise<ImportRecord[]> {
  const r = await fetchJSON<{ data: ImportRecord[] }>(
    `/api/v1/imports/history?limit=${limit}&offset=${offset}`,
  );
  return r.data;
}

export async function scanFolder(path: string): Promise<ScanResult[]> {
  const r = await fetchJSON<{ data: ScanResult[] }>("/api/v1/imports/scan", {
    method: "POST",
    body: JSON.stringify({ path }),
  });
  return r.data;
}

export async function reimportFile(params: {
  media_type: string;
  media_id: string;
  source_path: string;
  conflict_policy?: string;
}): Promise<ImportRecord> {
  const r = await fetchJSON<{ data: ImportRecord }>("/api/v1/imports/reimport", {
    method: "POST",
    body: JSON.stringify(params),
  });
  return r.data;
}

export async function fetchImportDecisions(
  limit = 50,
  offset = 0,
  mediaId?: string,
): Promise<ImportDecision[]> {
  let url = `/api/v1/imports/decisions?limit=${limit}&offset=${offset}`;
  if (mediaId) url += `&media_id=${encodeURIComponent(mediaId)}`;
  const r = await fetchJSON<{ data: ImportDecision[] }>(url);
  return r.data;
}

export async function triggerManualImport(path: string): Promise<void> {
  await fetchJSON<{ status: string }>("/api/v1/imports/manual", {
    method: "POST",
    body: JSON.stringify({ path }),
  });
}

// ---------- React Query hooks ----------

export function useImportHistory(limit = 50, offset = 0) {
  return useQuery({
    queryKey: ["imports", "history", limit, offset],
    queryFn: () => fetchImportHistory(limit, offset),
    staleTime: 10_000,
  });
}

export function useImportDecisions(limit = 50, offset = 0, mediaId?: string) {
  return useQuery({
    queryKey: ["imports", "decisions", limit, offset, mediaId],
    queryFn: () => fetchImportDecisions(limit, offset, mediaId),
    staleTime: 10_000,
  });
}

export function useScanFolder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => scanFolder(path),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["imports"] });
    },
  });
}

export function useReimportFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: reimportFile,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["imports"] });
    },
  });
}

export function useManualImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: triggerManualImport,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["imports"] });
    },
  });
}
