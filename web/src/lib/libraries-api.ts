// Typed fetch wrappers for the Loom library REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type MediaType = "movie" | "series" | "music";

export interface DiskSpace {
  total_bytes: number;
  free_bytes: number;
  used_bytes: number;
}

export interface Library {
  id: string;
  name: string;
  path: string;
  media_type: MediaType;
  monitor_on_add: boolean;
  quality_profile_id: string;
  unmonitor_on_delete: boolean;
  auto_archive_watched: boolean;
  auto_archive_days_after_watch: number;
  created_at: string;
  updated_at: string;
  accessible: boolean;
  disk_space: DiskSpace;
  file_count: number;
  unmapped_count: number;
}

export interface LibraryFile {
  id: string;
  library_id: string;
  path: string;
  size_bytes: number;
  media_id?: string;
  last_scanned?: string;
  created_at: string;
}

export interface UnmappedFolder {
  name: string;
  path: string;
}

export interface CreateLibraryRequest {
  name: string;
  path: string;
  media_type: MediaType;
  monitor_on_add?: boolean;
  quality_profile_id?: string;
  unmonitor_on_delete?: boolean;
  auto_archive_watched?: boolean;
  auto_archive_days_after_watch?: number;
}

export interface UpdateLibraryRequest {
  name?: string;
  path?: string;
  media_type?: MediaType;
  monitor_on_add?: boolean;
  quality_profile_id?: string;
  unmonitor_on_delete?: boolean;
  auto_archive_watched?: boolean;
  auto_archive_days_after_watch?: number;
}

export interface DirectoryEntry {
  name: string;
  path: string;
}

export interface FilesystemResponse {
  parent: string;
  current: string;
  directories: DirectoryEntry[];
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

export async function listLibraries(
  signal?: AbortSignal,
): Promise<Library[]> {
  const data = await request<{ data: Library[] }>(
    "GET",
    "/api/v1/libraries",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function getLibrary(
  id: string,
  signal?: AbortSignal,
): Promise<{ library: Library; files: LibraryFile[] }> {
  return request(
    "GET",
    `/api/v1/libraries/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createLibrary(
  body: CreateLibraryRequest,
): Promise<Library> {
  return request("POST", "/api/v1/libraries", body);
}

export async function updateLibrary(
  id: string,
  body: UpdateLibraryRequest,
): Promise<Library> {
  return request(
    "PUT",
    `/api/v1/libraries/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteLibrary(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/libraries/${encodeURIComponent(id)}`,
  );
}

export async function scanLibrary(
  id: string,
): Promise<{ message: string }> {
  return request(
    "POST",
    `/api/v1/libraries/${encodeURIComponent(id)}/scan`,
  );
}

export async function listUnmappedFolders(
  id: string,
  signal?: AbortSignal,
): Promise<UnmappedFolder[]> {
  const data = await request<{ data: UnmappedFolder[] }>(
    "GET",
    `/api/v1/libraries/${encodeURIComponent(id)}/unmapped`,
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function browseFilesystem(
  path?: string,
  signal?: AbortSignal,
): Promise<FilesystemResponse> {
  const params = path ? `?path=${encodeURIComponent(path)}` : "";
  return request("GET", `/api/v1/filesystem${params}`, undefined, signal);
}

// ---------- Query keys ----------

export const libraryKeys = {
  all: ["libraries"] as const,
  list: () => [...libraryKeys.all, "list"] as const,
  detail: (id: string) => [...libraryKeys.all, "detail", id] as const,
  unmapped: (id: string) => [...libraryKeys.all, "unmapped", id] as const,
  filesystem: (path?: string) => ["filesystem", path ?? ""] as const,
};

// ---------- React Query hooks ----------

export function useLibraries() {
  return useQuery<Library[], Error>({
    queryKey: libraryKeys.list(),
    queryFn: ({ signal }) => listLibraries(signal),
  });
}

export function useLibraryDetail(id: string) {
  return useQuery({
    queryKey: libraryKeys.detail(id),
    queryFn: ({ signal }) => getLibrary(id, signal),
    enabled: !!id,
  });
}

export function useUnmappedFolders(id: string) {
  return useQuery({
    queryKey: libraryKeys.unmapped(id),
    queryFn: ({ signal }) => listUnmappedFolders(id, signal),
    enabled: !!id,
  });
}

export function useFilesystem(path?: string) {
  return useQuery({
    queryKey: libraryKeys.filesystem(path),
    queryFn: ({ signal }) => browseFilesystem(path, signal),
  });
}

export function useCreateLibrary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createLibrary,
    onSuccess: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

export function useUpdateLibrary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: UpdateLibraryRequest;
    }) => updateLibrary(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

export function useDeleteLibrary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteLibrary,
    onSuccess: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

export function useScanLibrary() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: scanLibrary,
    onSuccess: () => qc.invalidateQueries({ queryKey: libraryKeys.all }),
  });
}

// ---------- Helpers ----------

export const MEDIA_TYPES: { value: MediaType; label: string }[] = [
  { value: "movie", label: "Movies" },
  { value: "series", label: "TV Series" },
  { value: "music", label: "Music" },
];

// Re-export formatBytes from the centralized utils module for backwards compat
export { formatBytes } from "@/lib/utils";

export function getMediaTypeLabel(type: MediaType): string {
  return MEDIA_TYPES.find((t) => t.value === type)?.label ?? type;
}
