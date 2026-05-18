import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";
import { useState, useEffect, useCallback, useRef } from "react";

// ─── Types ──────────────────────────────────────────────────────────────

export interface LogEntry {
  id: string;
  timestamp: string;
  level: string;  // debug, info, warn, error
  message: string;
  source?: string;
  attrs?: string;  // JSON string
  workflow_id?: string;
}

export interface LogListResult {
  items: LogEntry[];
  total: number;
}

export interface LogListParams {
  level?: string;
  search?: string;
  workflow_id?: string;
  since?: string;
  until?: string;
  limit?: number;
  offset?: number;
}

export interface LogConfig {
  capture_level: string;
}

// ─── Fetch functions ────────────────────────────────────────────────────

export async function fetchSystemLogs(params: LogListParams = {}, signal?: AbortSignal): Promise<LogListResult> {
  const qs = new URLSearchParams();
  if (params.level) qs.set("level", params.level);
  if (params.search) qs.set("search", params.search);
  if (params.workflow_id) qs.set("workflow_id", params.workflow_id);
  if (params.since) qs.set("since", params.since);
  if (params.until) qs.set("until", params.until);
  if (params.limit != null) qs.set("limit", String(params.limit));
  if (params.offset != null) qs.set("offset", String(params.offset));
  const url = `/api/v1/system/logs${qs.toString() ? `?${qs}` : ""}`;
  const res = await apiFetch(url, { signal });
  if (!res.ok) throw new Error(`system logs: ${res.status} ${res.statusText}`);
  return (await res.json()) as LogListResult;
}

export async function fetchLogConfig(signal?: AbortSignal): Promise<LogConfig> {
  const res = await apiFetch("/api/v1/system/logs/config", { signal });
  if (!res.ok) throw new Error(`log config: ${res.status}`);
  return (await res.json()) as LogConfig;
}

export async function updateLogConfig(config: LogConfig): Promise<LogConfig> {
  const res = await apiFetch("/api/v1/system/logs/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(config),
  });
  if (!res.ok) throw new Error(`update log config: ${res.status}`);
  return (await res.json()) as LogConfig;
}

export async function clearSystemLogs(): Promise<void> {
  const res = await apiFetch("/api/v1/system/logs", { method: "DELETE" });
  if (!res.ok) throw new Error(`clear logs: ${res.status}`);
}

// ─── Hooks ──────────────────────────────────────────────────────────────

export function useSystemLogs(params: LogListParams = {}) {
  return useQuery({
    queryKey: ["system", "logs", params],
    queryFn: ({ signal }) => fetchSystemLogs(params, signal),
    refetchInterval: 10_000,
    staleTime: 5_000,
  });
}

export function useLogConfig() {
  return useQuery({
    queryKey: ["system", "logs", "config"],
    queryFn: ({ signal }) => fetchLogConfig(signal),
  });
}

export function useUpdateLogConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: updateLogConfig,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["system", "logs", "config"] }),
  });
}

export function useClearSystemLogs() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: clearSystemLogs,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["system", "logs"] }),
  });
}

// ─── SSE streaming hook ─────────────────────────────────────────────────

export function useLogStream(opts?: { workflowId?: string; enabled?: boolean }) {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);
  const enabled = opts?.enabled ?? true;

  const clear = useCallback(() => setEntries([]), []);

  useEffect(() => {
    if (!enabled) return;
    const params = new URLSearchParams();
    if (opts?.workflowId) params.set("workflow_id", opts.workflowId);
    const url = `/api/v1/system/logs/stream${params.toString() ? `?${params}` : ""}`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => setConnected(true);
    es.onmessage = (e) => {
      try {
        const entry = JSON.parse(e.data) as LogEntry;
        setEntries((prev) => [...prev.slice(-4999), entry]);
      } catch { /* ignore parse errors */ }
    };
    es.onerror = () => {
      setConnected(false);
      // EventSource auto-reconnects
    };

    return () => {
      es.close();
      esRef.current = null;
      setConnected(false);
    };
  }, [enabled, opts?.workflowId]);

  return { entries, connected, clear };
}
