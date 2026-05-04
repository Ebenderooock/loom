// Typed fetch wrappers for the Loom RSS sources REST endpoints.
// The shapes mirror the /api/v1/rss/sources/* API; keep them in sync if the
// contract changes.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { UseQueryOptions } from "@tanstack/react-query";

// ---------- Types ----------

export type SourceType = "rss" | "scraper";

export interface RSSSourceConfig {
  url: string;
  refresh_interval_minutes?: number;
}

export interface PaginationConfig {
  type: "none" | "page_number" | "offset";
  page_param?: string;
  offset_param?: string;
  page_size?: number;
}

export interface ScraperConfig {
  url: string;
  selector_type: "css" | "xpath";
  item_selector: string;
  title_selector: string;
  link_selector?: string;
  published_selector?: string;
  auth_type?: "none" | "basic" | "apikey";
  username?: string;
  password?: string;
  api_key?: string;
  pagination?: PaginationConfig;
  refresh_interval_minutes?: number;
}

export type SourceConfig = RSSSourceConfig | ScraperConfig;

export interface UserSource {
  id: string;
  name: string;
  type: SourceType;
  config: SourceConfig;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface UserSourceCreate {
  name: string;
  type: SourceType;
  config: SourceConfig;
  enabled?: boolean;
}

export interface UserSourcePatch {
  name?: string;
  enabled?: boolean;
  config?: Partial<SourceConfig>;
}

export interface SourceItem {
  title: string;
  link: string;
  published?: string;
}

export interface TestSourceResult {
  success: boolean;
  items?: SourceItem[];
  error?: string;
}

// ---------- HTTP helpers ----------

export class ApiError extends Error {
  status: number;
  code?: string;
  details?: unknown;
  constructor(status: number, message: string, code?: string, details?: unknown) {
    super(message);
    this.status = status;
    this.code = code;
    this.details = details;
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
      | { error?: { code?: string; message?: string; details?: unknown } }
      | undefined;
    const message =
      env?.error?.message ??
      (typeof parsed === "string" ? parsed : undefined) ??
      `${method} ${path} failed: ${res.status} ${res.statusText}`;
    throw new ApiError(res.status, message, env?.error?.code, env?.error?.details);
  }
  return parsed as T;
}

// ---------- Sources endpoints ----------

export const sourcesKeys = {
  all: ["sources"] as const,
  list: () => [...sourcesKeys.all, "list"] as const,
  detail: (id: string) => [...sourcesKeys.all, "detail", id] as const,
};

export async function listSources(signal?: AbortSignal): Promise<UserSource[]> {
  const env = await request<{ sources: UserSource[] }>(
    "GET",
    "/api/v1/rss/sources/",
    undefined,
    signal,
  );
  return env.sources ?? [];
}

export async function getSource(id: string, signal?: AbortSignal): Promise<UserSource> {
  return request<UserSource>(
    "GET",
    `/api/v1/rss/sources/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createSource(body: UserSourceCreate): Promise<UserSource> {
  return request<UserSource>("POST", "/api/v1/rss/sources/", body);
}

export async function updateSource(
  id: string,
  body: UserSourcePatch,
): Promise<UserSource> {
  return request<UserSource>(
    "PATCH",
    `/api/v1/rss/sources/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteSource(id: string): Promise<void> {
  await request<void>("DELETE", `/api/v1/rss/sources/${encodeURIComponent(id)}`);
}

export async function testSource(id: string): Promise<TestSourceResult> {
  return request<TestSourceResult>(
    "POST",
    `/api/v1/rss/sources/${encodeURIComponent(id)}/test`,
  );
}

// ---------- React Query hooks ----------

export function useSources(
  options?: Omit<UseQueryOptions<UserSource[], Error>, "queryKey" | "queryFn">,
) {
  return useQuery<UserSource[], Error>({
    queryKey: sourcesKeys.list(),
    queryFn: ({ signal }) => listSources(signal),
    ...options,
  });
}

export function useSource(
  id: string,
  options?: Omit<UseQueryOptions<UserSource, Error>, "queryKey" | "queryFn">,
) {
  return useQuery<UserSource, Error>({
    queryKey: sourcesKeys.detail(id),
    queryFn: ({ signal }) => getSource(id, signal),
    ...options,
  });
}

export function useCreateSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createSource,
    onSuccess: () => qc.invalidateQueries({ queryKey: sourcesKeys.all }),
  });
}

export function useUpdateSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: UserSourcePatch }) =>
      updateSource(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: sourcesKeys.all }),
  });
}

export function useDeleteSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteSource,
    onSuccess: () => qc.invalidateQueries({ queryKey: sourcesKeys.all }),
  });
}

export function useTestSource() {
  return useMutation({
    mutationFn: testSource,
  });
}
