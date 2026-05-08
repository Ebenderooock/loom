// Typed fetch wrappers for the Loom indexer + proxy REST endpoints.
// The shapes mirror api/openapi/loom.yaml; keep them in sync if the
// contract changes. We hand-write rather than codegen because the
// surface is small and the codegen toolchain is not yet wired up.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { UseQueryOptions } from "@tanstack/react-query";

// ---------- Types ----------

export type IndexerKind = "newznab" | "torznab" | "builtin/null";
export type ProxyKind = "http" | "https" | "socks5" | "flaresolverr";

export interface IndexerHealth {
  indexer_id: string;
  status: "ok" | "degraded" | "failed" | "unknown";
  last_checked_at: string;
  last_success_at?: string;
  latency_ms?: number;
  last_error?: string;
}

export interface NewznabConfig {
  url: string;
  api_key: string;
  user_agent?: string;
  timeout?: string;
}

export interface Indexer {
  id: string;
  kind: IndexerKind | string;
  name: string;
  enabled: boolean;
  priority: number;
  config?: NewznabConfig | Record<string, unknown>;
  categories: number[];
  tags: string[];
  proxy_id?: string;
  created_at?: string;
  updated_at?: string;
  health?: IndexerHealth;
}

export interface IndexerCreate {
  id?: string;
  kind: IndexerKind | string;
  name: string;
  enabled?: boolean;
  priority?: number;
  config?: Record<string, unknown>;
  categories?: number[];
  tags?: string[];
  proxy_id?: string;
}

// IndexerPatch encodes the "null vs unset" distinction for proxy_id:
//   - undefined  → field omitted from request, proxy unchanged
//   - null       → field sent as JSON null, proxy detached
//   - string     → field sent as that ID, proxy attached
export interface IndexerPatch {
  name?: string;
  enabled?: boolean;
  priority?: number;
  config?: Record<string, unknown>;
  categories?: number[];
  tags?: string[];
  proxy_id?: string | null;
}

export interface ProxyHTTPConfig {
  url: string;
  username?: string;
  password?: string;
}
export interface ProxySOCKS5Config {
  address: string;
  username?: string;
  password?: string;
}
export interface ProxyFlareSolverrConfig {
  url: string;
  max_timeout_sec?: number;
  session_mode?: "" | "none" | "shared";
}
export type ProxyConfig =
  | ProxyHTTPConfig
  | ProxySOCKS5Config
  | ProxyFlareSolverrConfig;

export interface Proxy {
  id: string;
  kind: ProxyKind;
  name: string;
  enabled: boolean;
  config: ProxyConfig;
  created_at?: string;
  updated_at?: string;
}

export interface ProxyCreate {
  id?: string;
  kind: ProxyKind;
  name: string;
  enabled?: boolean;
  config: ProxyConfig;
}

export interface ProxyPatch {
  name?: string;
  enabled?: boolean;
  kind?: ProxyKind;
  config?: ProxyConfig;
}

export interface TestResult {
  ok: boolean;
  latency_ms: number;
  error?: string;
  status_code?: number;
}

export interface SearchResult {
  indexer_id: string;
  title: string;
  guid?: string;
  link: string;
  info_url?: string;
  size_bytes?: number;
  seeders?: number;
  leechers?: number;
  publish_date?: string;
  categories?: number[];
  quality?: string;
  magnet_uri?: string;
  infohash?: string;
  nzb_url?: string;
  score?: number;
  freeleech?: boolean;
  internal?: boolean;
  scene?: boolean;
}

export interface IndexerDiagnostic {
  name: string;
  status: "ok" | "error" | "timeout";
  response_time_ms: number;
  result_count: number;
  error_message?: string;
}

export interface SearchDiagnostics {
  indexers: IndexerDiagnostic[];
  total_results: number;
  search_duration_ms: number;
}

export interface AggregatedResults {
  results: SearchResult[];
  errors: Record<string, string>;
  diagnostics?: SearchDiagnostics;
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
    // Loom returns {error: {code, message, details?}} envelopes.
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

// ---------- Indexer definitions (Cardigann catalogue) ----------

export interface IndexerDefinitionSetting {
  name: string;
  type?: string;
  label?: string;
  default?: string;
}

export interface IndexerDefinition {
  id: string;
  name: string;
  description?: string;
  type?: string;
  language?: string;
  links?: string[];
  settings?: IndexerDefinitionSetting[];
  categories?: string[];
}

export async function listDefinitions(signal?: AbortSignal): Promise<IndexerDefinition[]> {
  const env = await request<{ data: IndexerDefinition[] }>(
    "GET",
    "/api/v1/indexers/definitions",
    undefined,
    signal,
  );
  return env.data ?? [];
}

export const definitionKeys = {
  all: ["indexer-definitions"] as const,
  list: () => [...definitionKeys.all, "list"] as const,
};

export function useDefinitions(
  options?: Omit<UseQueryOptions<IndexerDefinition[], Error>, "queryKey" | "queryFn">,
) {
  return useQuery<IndexerDefinition[], Error>({
    queryKey: definitionKeys.list(),
    queryFn: ({ signal }) => listDefinitions(signal),
    staleTime: Infinity,
    ...options,
  });
}

// ---------- Indexer endpoints ----------

export const indexerKeys = {
  all: ["indexers"] as const,
  list: () => [...indexerKeys.all, "list"] as const,
  detail: (id: string) => [...indexerKeys.all, "detail", id] as const,
};

export async function listIndexers(signal?: AbortSignal): Promise<Indexer[]> {
  const env = await request<{ indexers: Indexer[] }>(
    "GET",
    "/api/v1/indexers/",
    undefined,
    signal,
  );
  return env.indexers ?? [];
}

export async function createIndexer(body: IndexerCreate): Promise<Indexer> {
  return request<Indexer>("POST", "/api/v1/indexers/", body);
}

export async function patchIndexer(
  id: string,
  body: IndexerPatch,
): Promise<Indexer> {
  return request<Indexer>("PATCH", `/api/v1/indexers/${encodeURIComponent(id)}`, body);
}

export async function deleteIndexer(id: string): Promise<void> {
  await request<void>("DELETE", `/api/v1/indexers/${encodeURIComponent(id)}`);
}

export async function testIndexer(id: string): Promise<TestResult> {
  return request<TestResult>(
    "POST",
    `/api/v1/indexers/${encodeURIComponent(id)}/test`,
  );
}

export interface TestConfigPayload {
  kind: string;
  name: string;
  config: Record<string, unknown>;
  proxy_id?: string;
}

export async function testIndexerConfig(
  payload: TestConfigPayload,
): Promise<TestResult> {
  return request<TestResult>("POST", "/api/v1/indexers/test", payload);
}

export interface SearchParams {
  q: string;
  indexer_ids?: string[];
  categories?: number[];
  timeout_ms?: number;
}

export async function searchIndexers(
  params: SearchParams,
): Promise<AggregatedResults> {
  return request<AggregatedResults>("POST", "/api/v1/indexers/search", {
    query: params.q,
    indexer_ids: params.indexer_ids,
    categories: params.categories,
    timeout_ms: params.timeout_ms,
  });
}

// ---------- Streaming search (SSE) ----------

export type IndexerStatus = "pending" | "searching" | "done" | "error" | "timeout";

export interface IndexerStreamState {
  id: string;
  name: string;
  status: IndexerStatus;
  resultCount: number;
  elapsedMs: number;
  error?: string;
}

export interface StreamSearchEvent {
  type: "search-start" | "indexer-start" | "indexer-result" | "indexer-error" | "done";
  indexer_id?: string;
  indexer_name?: string;
  results?: SearchResult[];
  result_count?: number;
  elapsed_ms?: number;
  error?: string;
  status?: string;
  indexers?: { id: string; name: string }[];
  total_results?: number;
  total_errors?: number;
  search_duration_ms?: number;
}

export interface StreamSearchCallbacks {
  onSearchStart?: (indexers: { id: string; name: string }[]) => void;
  onIndexerStart?: (id: string, name: string) => void;
  onIndexerResult?: (id: string, name: string, results: SearchResult[], count: number, elapsedMs: number) => void;
  onIndexerError?: (id: string, name: string, error: string, status: string, elapsedMs: number) => void;
  onDone?: (totalResults: number, totalErrors: number, durationMs: number) => void;
  onError?: (error: Error) => void;
}

/**
 * Stream search results via SSE. Results arrive incrementally as each
 * indexer completes. Returns an AbortController to cancel the search.
 */
export function streamSearch(
  params: SearchParams,
  callbacks: StreamSearchCallbacks,
): AbortController {
  const controller = new AbortController();

  (async () => {
    try {
      const res = await fetch("/api/v1/indexers/search/stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          query: params.q,
          indexer_ids: params.indexer_ids,
          categories: params.categories,
          timeout_ms: params.timeout_ms,
        }),
        signal: controller.signal,
      });

      if (!res.ok) {
        const text = await res.text();
        let msg = `Search stream failed: ${res.status}`;
        try {
          const env = JSON.parse(text);
          if (env?.error?.message) msg = env.error.message;
        } catch { /* ignore */ }
        callbacks.onError?.(new Error(msg));
        return;
      }

      const reader = res.body?.getReader();
      if (!reader) {
        callbacks.onError?.(new Error("Streaming not supported by browser"));
        return;
      }

      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // Parse SSE events separated by double newline
        const parts = buffer.split("\n\n");
        buffer = parts.pop() ?? "";

        for (const part of parts) {
          if (!part.trim()) continue;

          // Skip heartbeat comments
          const lines = part.split("\n");
          let eventType = "";
          let dataStr = "";

          for (const line of lines) {
            if (line.startsWith(":")) continue; // comment/heartbeat
            if (line.startsWith("event: ")) {
              eventType = line.slice(7).trim();
            } else if (line.startsWith("data: ")) {
              dataStr += (dataStr ? "\n" : "") + line.slice(6);
            }
          }

          if (!eventType || !dataStr) continue;

          let evt: StreamSearchEvent;
          try {
            evt = JSON.parse(dataStr);
          } catch {
            continue;
          }

          switch (eventType) {
            case "search-start":
              callbacks.onSearchStart?.(evt.indexers ?? []);
              break;
            case "indexer-start":
              callbacks.onIndexerStart?.(evt.indexer_id ?? "", evt.indexer_name ?? "");
              break;
            case "indexer-result":
              callbacks.onIndexerResult?.(
                evt.indexer_id ?? "",
                evt.indexer_name ?? "",
                evt.results ?? [],
                evt.result_count ?? 0,
                evt.elapsed_ms ?? 0,
              );
              break;
            case "indexer-error":
              callbacks.onIndexerError?.(
                evt.indexer_id ?? "",
                evt.indexer_name ?? "",
                evt.error ?? "Unknown error",
                evt.status ?? "error",
                evt.elapsed_ms ?? 0,
              );
              break;
            case "done":
              callbacks.onDone?.(
                evt.total_results ?? 0,
                evt.total_errors ?? 0,
                evt.search_duration_ms ?? 0,
              );
              return; // stream complete
          }
        }
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      callbacks.onError?.(err instanceof Error ? err : new Error(String(err)));
    }
  })();

  return controller;
}

// ---------- Proxy endpoints ----------

export const proxyKeys = {
  all: ["proxies"] as const,
  list: () => [...proxyKeys.all, "list"] as const,
  detail: (id: string) => [...proxyKeys.all, "detail", id] as const,
};

export async function listProxies(signal?: AbortSignal): Promise<Proxy[]> {
  const env = await request<{ proxies: Proxy[] }>(
    "GET",
    "/api/v1/proxies/",
    undefined,
    signal,
  );
  return env.proxies ?? [];
}

export async function createProxy(body: ProxyCreate): Promise<Proxy> {
  return request<Proxy>("POST", "/api/v1/proxies/", body);
}

export async function patchProxy(id: string, body: ProxyPatch): Promise<Proxy> {
  return request<Proxy>("PATCH", `/api/v1/proxies/${encodeURIComponent(id)}`, body);
}

export async function deleteProxy(id: string): Promise<void> {
  await request<void>("DELETE", `/api/v1/proxies/${encodeURIComponent(id)}`);
}

export async function testProxy(id: string): Promise<TestResult> {
  return request<TestResult>(
    "POST",
    `/api/v1/proxies/${encodeURIComponent(id)}/test`,
  );
}

// ---------- React Query hooks ----------

export function useIndexers(
  options?: Omit<UseQueryOptions<Indexer[], Error>, "queryKey" | "queryFn">,
) {
  return useQuery<Indexer[], Error>({
    queryKey: indexerKeys.list(),
    queryFn: ({ signal }) => listIndexers(signal),
    ...options,
  });
}

export function useProxies(
  options?: Omit<UseQueryOptions<Proxy[], Error>, "queryKey" | "queryFn">,
) {
  return useQuery<Proxy[], Error>({
    queryKey: proxyKeys.list(),
    queryFn: ({ signal }) => listProxies(signal),
    ...options,
  });
}

export function useCreateIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createIndexer,
    onSuccess: () => qc.invalidateQueries({ queryKey: indexerKeys.all }),
  });
}

export function usePatchIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: IndexerPatch }) =>
      patchIndexer(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: indexerKeys.all }),
  });
}

export function useDeleteIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteIndexer,
    onSuccess: () => qc.invalidateQueries({ queryKey: indexerKeys.all }),
  });
}

export function useTestIndexer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: testIndexer,
    onSuccess: () => qc.invalidateQueries({ queryKey: indexerKeys.all }),
  });
}

export function useTestIndexerConfig() {
  return useMutation({ mutationFn: testIndexerConfig });
}

export function useCreateProxy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createProxy,
    onSuccess: () => qc.invalidateQueries({ queryKey: proxyKeys.all }),
  });
}

export function usePatchProxy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: ProxyPatch }) =>
      patchProxy(id, patch),
    onSuccess: () => qc.invalidateQueries({ queryKey: proxyKeys.all }),
  });
}

export function useDeleteProxy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteProxy,
    onSuccess: () => qc.invalidateQueries({ queryKey: proxyKeys.all }),
  });
}

export function useTestProxy() {
  return useMutation({ mutationFn: testProxy });
}
