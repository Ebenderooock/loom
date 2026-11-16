// Typed fetch wrappers for import management endpoints.
// Mirrors internal/imports/ types; keep in sync if the contract changes.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export interface ImportRecord {
  id: string;
  media_type: string;
  media_id: string;
  /** Parent title identity supplied on scoped series history responses. */
  series_id?: string;
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
  detected_artist?: string;
  detected_album?: string;
  detected_track?: number;
  detected_disc?: number;
  matched_media?: string;
  matched_media_id?: string;
  media_type?: string;
  confidence: number;
  suggested_action: string;
  quality?: string;
  import_status?: string;
  import_error?: string;
}

export interface ImportScan {
  id: string;
  path: string;
  status: "running" | "completed" | "failed" | "cancelled";
  phase: "discovering" | "matching";
  total_files: number;
  processed_files: number;
  current_file?: string;
  error?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  results: ScanResult[];
  next_offset: number;
}

export interface ImportJob {
  id: string;
  source_path: string;
  media_type?: string;
  media_id?: string;
  status:
    | "queued"
    | "importing"
    | "retry_pending"
    | "imported"
    | "failed"
    | "pending_review";
  attempts: number;
  total_files: number;
  processed_files: number;
  imported_files: number;
  failed_files: number;
  skipped_files?: number;
  pending_review_files?: number;
  review_id?: string;
  error?: string;
  next_retry_at?: string;
  created_at: string;
  updated_at: string;
  files?: ImportJobFile[];
}

export interface ImportJobFile {
  id: string;
  job_id: string;
  source_path: string;
  dest_path?: string;
  status:
    | "queued"
    | "importing"
    | "imported"
    | "failed"
    | "skipped"
    | "pending_review";
  attempts: number;
  error?: string;
  updated_at: string;
}

export interface QueueImportResult {
  data: ImportJob[];
  errors: { path: string; error: string }[];
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

export interface ImportAccepted {
  status: "accepted";
}

// ---------- API calls ----------

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(url, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const message =
      typeof body?.error === "string" ? body.error : body?.error?.message;
    throw new Error(
      res.status === 404 && url.startsWith("/api/v1/imports/scans/")
        ? "Scan session not found or expired (404)"
        : message || `${res.status} ${res.statusText}`,
    );
  }
  return (await res.json()) as T;
}

export async function fetchImportHistory(
  limit = 50,
  offset = 0,
  title?: { mediaType: "movie" | "series"; mediaId: string },
  signal?: AbortSignal,
): Promise<ImportRecord[]> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (title) {
    params.set("media_type", title.mediaType);
    params.set("media_id", title.mediaId);
  }
  const r = await fetchJSON<{ data: ImportRecord[] }>(
    `/api/v1/imports/history?${params}`,
    { signal },
  );
  if (!Array.isArray(r.data))
    throw new Error("Unexpected import history response");
  if (
    title &&
    r.data.some((row) =>
      title.mediaType === "movie"
        ? row.media_type !== "movie" || row.media_id !== title.mediaId
        : row.media_type !== "episode" || row.series_id !== title.mediaId,
    )
  ) {
    throw new Error("Import history returned records for another title");
  }
  return r.data;
}

export async function scanFolder(path: string): Promise<ScanResult[]> {
  const r = await fetchJSON<{ data: ScanResult[] }>("/api/v1/imports/scan", {
    method: "POST",
    body: JSON.stringify({ path }),
  });
  return r.data;
}

export async function startImportScan(
  path: string,
  signal: AbortSignal,
): Promise<ImportScan> {
  const r = await fetchJSON<{ data: ImportScan }>("/api/v1/imports/scans", {
    method: "POST",
    body: JSON.stringify({ path }),
    signal,
  });
  return r.data;
}

export async function fetchImportScan(
  id: string,
  offset: number,
  signal: AbortSignal,
): Promise<ImportScan> {
  const r = await fetchJSON<{ data: ImportScan }>(
    `/api/v1/imports/scans/${encodeURIComponent(id)}?offset=${offset}&limit=1000`,
    { signal },
  );
  return r.data;
}

export async function cancelImportScan(
  id: string,
  signal: AbortSignal,
): Promise<ImportScan> {
  const r = await fetchJSON<{ data: ImportScan }>(
    `/api/v1/imports/scans/${encodeURIComponent(id)}`,
    { method: "DELETE", signal },
  );
  return r.data;
}

function waitForScanPoll(signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    signal.throwIfAborted();
    const abort = () => {
      clearTimeout(timer);
      reject(signal.reason);
    };
    const timer = setTimeout(() => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, 1000);
    signal.addEventListener("abort", abort, { once: true });
  });
}

// Each callback contains only the newly fetched page. Never retry a failed
// request by starting another session: sessions may expire and POST is not
// idempotent. The caller retains partial results if polling fails.
export async function pollImportScan({
  path,
  signal,
  onProgress,
  resume,
}: {
  path: string;
  signal: AbortSignal;
  onProgress: (scan: ImportScan) => void;
  resume?: ImportScan;
}): Promise<ImportScan> {
  let scan = resume ?? (await startImportScan(path, signal));
  signal.throwIfAborted();
  if (!resume) onProgress(scan);
  let offset = resume?.next_offset ?? 0;
  for (;;) {
    signal.throwIfAborted();
    scan = await fetchImportScan(scan.id, offset, signal);
    signal.throwIfAborted();
    onProgress(scan);
    if (scan.next_offset < scan.processed_files) {
      if (scan.next_offset <= offset) {
        throw new Error(
          "Scan results could not be loaded: cursor did not advance.",
        );
      }
      offset = scan.next_offset;
      continue;
    }
    if (scan.status !== "running") return scan;
    offset = scan.next_offset;
    await waitForScanPoll(signal);
  }
}

export async function reimportFile(params: {
  media_type: string;
  media_id: string;
  source_path: string;
  conflict_policy?: string;
}): Promise<ImportAccepted> {
  return fetchJSON<ImportAccepted>("/api/v1/imports/reimport", {
    method: "POST",
    body: JSON.stringify(params),
  });
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

export async function fetchImportJobs(): Promise<ImportJob[]> {
  const r = await fetchJSON<{ data: ImportJob[] }>("/api/v1/imports/jobs");
  return r.data;
}

export async function fetchImportJob(id: string): Promise<ImportJob> {
  const result = await fetchJSON<{ data: ImportJob }>(
    `/api/v1/imports/jobs/${encodeURIComponent(id)}`,
  );
  return result.data;
}

export async function queueImportFiles(
  paths: string[],
): Promise<QueueImportResult> {
  const result: QueueImportResult = { data: [], errors: [] };
  for (let offset = 0; offset < paths.length; offset += 1000) {
    const batch = paths.slice(offset, offset + 1000);
    try {
      const response = await apiFetch("/api/v1/imports/batch", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ paths: batch }),
      });
      const body = (await response.json()) as Partial<QueueImportResult> & {
        error?: string;
      };
      if (response.ok || body.errors?.length) {
        result.data.push(...(body.data ?? []));
        result.errors.push(...(body.errors ?? []));
        const accounted = new Set([
          ...(body.data ?? []).map((job) => job.source_path),
          ...(body.errors ?? []).map((failure) => failure.path),
        ]);
        result.errors.push(
          ...batch
            .filter((path) => !accounted.has(path))
            .map((path) => ({
              path,
              error:
                "Queue outcome unknown; the server did not return this file. Refresh status before retrying.",
            })),
        );
      } else {
        result.errors.push(
          ...batch.map((path) => ({
            path,
            error: body.error ?? `Queue request failed (${response.status})`,
          })),
        );
      }
    } catch (error) {
      // Earlier chunks may already be durable. Preserve those outcomes and
      // surface uncertain requests instead of claiming the entire batch failed.
      result.errors.push(
        ...batch.map((path) => ({
          path,
          error: `Queue outcome unknown; refresh status before retrying. ${error instanceof Error ? error.message : "Request failed"}`,
        })),
      );
    }
  }
  return result;
}

export async function retryImportJob(id: string): Promise<void> {
  await fetchJSON(`/api/v1/imports/jobs/${encodeURIComponent(id)}/retry`, {
    method: "POST",
  });
}

export async function manualMatch(params: {
  path: string;
  media_type: string;
  media_id: string;
}): Promise<void> {
  await fetchJSON<{ status: string }>("/api/v1/imports/manual-match", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

// Local library search helpers

export interface LocalMovie {
  id: string;
  title: string;
  year: number;
  tmdbId?: string;
  imdbId?: string;
}

export interface LocalSeriesSeason {
  id: string;
  seasonNumber: number;
  episodeCount: number;
}

export interface LocalSeries {
  id: string;
  title: string;
  year: number;
  tmdbId?: string;
  imdbId?: string;
  seasons?: LocalSeriesSeason[];
}

export interface SeriesEpisode {
  id: string;
  title: string;
  seasonId: string;
  episodeNumber: number;
}

export async function searchLocalMovies(query: string): Promise<LocalMovie[]> {
  const r = await fetchJSON<{ data: LocalMovie[] }>(
    `/api/v1/movies/search?q=${encodeURIComponent(query)}`,
  );
  return r.data;
}

export async function searchLocalSeries(query: string): Promise<LocalSeries[]> {
  const r = await fetchJSON<{ data: LocalSeries[] }>(
    `/api/v1/series?search=${encodeURIComponent(query)}`,
  );
  return r.data;
}

export async function fetchEpisodes(
  seriesId: string,
  seasonNum: number,
): Promise<SeriesEpisode[]> {
  const r = await fetchJSON<{ data: SeriesEpisode[] }>(
    `/api/v1/series/${encodeURIComponent(seriesId)}/seasons/${seasonNum}/episodes`,
  );
  return r.data;
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
    mutationFn: pollImportScan,
    retry: false,
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

export function useManualMatch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: manualMatch,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["imports"] });
    },
  });
}

export function useImportJobs() {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["imports", "jobs"],
    queryFn: fetchImportJobs,
    refetchInterval: 2_000,
  });
  const completedVersion = query.data
    ?.filter((job) => job.status === "imported")
    .map((job) => `${job.id}:${job.updated_at}`)
    .join(",");
  // Library pages must not keep showing missing files after an async import.
  useEffect(() => {
    if (!completedVersion) return;
    for (const key of [
      "movies",
      "series",
      "episodes",
      "workflows",
      "library-media",
      "media-files",
      "title-membership",
      "music",
    ]) {
      void qc.invalidateQueries({ queryKey: [key] });
    }
    void qc.invalidateQueries({ queryKey: ["imports", "history"] });
  }, [completedVersion, qc]);
  return query;
}

export function useQueueImportFiles() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: queueImportFiles,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["imports"] }),
  });
}

export function useImportJob(id: string) {
  return useQuery({
    queryKey: ["imports", "jobs", id],
    queryFn: () => fetchImportJob(id),
    refetchInterval: 2_000,
  });
}

export function useRetryImportJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: retryImportJob,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["imports"] }),
  });
}
