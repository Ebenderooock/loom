// Typed client + hooks for the Downloads Cleanup API
// (internal/cleanup/handlers.go). Keep shapes in sync with internal/cleanup.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

export type OrphanStatus = "pending" | "ignored" | "deleted" | "delete_failed";

export interface Orphan {
  id: string;
  path: string;
  client_id: string;
  client_name?: string;
  root: string;
  size_bytes: number;
  status: OrphanStatus;
  error?: string;
  first_seen_at: string;
  last_seen_at: string;
  deleted_at?: string;
}

export interface CleanupSettings {
  auto_delete_enabled: boolean;
  retention_days: number;
}

async function req<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const init: RequestInit = { method };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  const res = await apiFetch(path, init);
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
    throw new Error(
      env?.error?.message ?? `${method} ${path} failed (${res.status})`,
    );
  }
  return parsed as T;
}

export async function listOrphans(
  status?: OrphanStatus | "all",
): Promise<Orphan[]> {
  const q = status ? `?status=${encodeURIComponent(status)}` : "";
  const env = await req<{ data: Orphan[] }>(
    "GET",
    `/api/v1/cleanup/orphans${q}`,
  );
  return env.data ?? [];
}

export async function scanCleanup(): Promise<{ found: number }> {
  return req<{ found: number }>("POST", "/api/v1/cleanup/scan");
}

export async function getCleanupSettings(): Promise<CleanupSettings> {
  return req<CleanupSettings>("GET", "/api/v1/cleanup/settings");
}

export async function saveCleanupSettings(
  s: CleanupSettings,
): Promise<CleanupSettings> {
  return req<CleanupSettings>("PUT", "/api/v1/cleanup/settings", s);
}

export async function approveOrphan(id: string): Promise<void> {
  await req<unknown>(
    "POST",
    `/api/v1/cleanup/orphans/${encodeURIComponent(id)}/approve`,
  );
}

export async function ignoreOrphan(id: string): Promise<void> {
  await req<unknown>(
    "POST",
    `/api/v1/cleanup/orphans/${encodeURIComponent(id)}/ignore`,
  );
}

// ---------- Hooks ----------

export const cleanupKeys = {
  all: ["cleanup"] as const,
  orphans: (status?: string) =>
    [...cleanupKeys.all, "orphans", status ?? "pending"] as const,
  settings: () => [...cleanupKeys.all, "settings"] as const,
};

export function useOrphans(status: OrphanStatus | "all" = "pending") {
  return useQuery<Orphan[], Error>({
    queryKey: cleanupKeys.orphans(status),
    queryFn: () => listOrphans(status),
  });
}

export function useCleanupSettings() {
  return useQuery<CleanupSettings, Error>({
    queryKey: cleanupKeys.settings(),
    queryFn: getCleanupSettings,
  });
}

export function useScanCleanup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: scanCleanup,
    onSuccess: () => qc.invalidateQueries({ queryKey: cleanupKeys.all }),
  });
}

export function useSaveCleanupSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: saveCleanupSettings,
    onSuccess: (data) => {
      qc.setQueryData(cleanupKeys.settings(), data);
      qc.invalidateQueries({ queryKey: cleanupKeys.settings() });
    },
  });
}

export function useApproveOrphan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: approveOrphan,
    onSuccess: () => qc.invalidateQueries({ queryKey: cleanupKeys.all }),
  });
}

export function useIgnoreOrphan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ignoreOrphan,
    onSuccess: () => qc.invalidateQueries({ queryKey: cleanupKeys.all }),
  });
}
