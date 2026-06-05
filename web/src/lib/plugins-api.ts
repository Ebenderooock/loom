// Typed fetch wrappers + react-query hooks for custom-script plugins.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

export interface Plugin {
  id: string;
  name: string;
  enabled: boolean;
  command: string[];
  events: string[];
  env: Record<string, string>;
  timeout_secs: number;
  working_dir: string;
  created_at: string;
  updated_at: string;
}

export interface PluginEvent {
  key: string;
  label: string;
  topic: string;
}

export interface PluginRun {
  id: string;
  plugin_id: string;
  plugin_name: string;
  topic: string;
  success: boolean;
  exit_code: number;
  duration_ms: number;
  stdout: string;
  stderr: string;
  error_msg: string;
  started_at: string;
}

export interface PluginInput {
  name: string;
  enabled: boolean;
  command: string[];
  events: string[];
  env: Record<string, string>;
  timeout_secs: number;
  working_dir: string;
}

const BASE = "/api/v1/plugins";

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method };
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
    throw new Error(env?.error?.message ?? `${method} ${path} failed: ${res.status}`);
  }
  return parsed as T;
}

export function usePlugins() {
  return useQuery({
    queryKey: ["plugins"],
    queryFn: () => request<Plugin[]>("GET", BASE),
  });
}

export function usePluginEvents() {
  return useQuery({
    queryKey: ["plugin-events"],
    queryFn: () => request<PluginEvent[]>("GET", `${BASE}/events`),
    staleTime: 60 * 60 * 1000,
  });
}

export function usePluginRuns(pluginId: string | null) {
  return useQuery({
    queryKey: ["plugin-runs", pluginId],
    queryFn: () => request<PluginRun[]>("GET", `${BASE}/${pluginId}/runs?limit=50`),
    enabled: !!pluginId,
  });
}

export function useCreatePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PluginInput) => request<Plugin>("POST", BASE, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plugins"] }),
  });
}

export function useUpdatePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: PluginInput }) =>
      request<Plugin>("PUT", `${BASE}/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plugins"] }),
  });
}

export function useDeletePlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => request<void>("DELETE", `${BASE}/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plugins"] }),
  });
}

export function useTestPlugin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => request<PluginRun>("POST", `${BASE}/${id}/test`),
    onSuccess: (_data, id) => qc.invalidateQueries({ queryKey: ["plugin-runs", id] }),
  });
}
