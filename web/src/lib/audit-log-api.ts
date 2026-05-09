import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ─── Types ──────────────────────────────────────────────────────────────

export interface AuditLogEntry {
  id: string;
  timestamp: string;
  occurred_at?: string;
  category: string;
  event_type: string;
  message: string;
  detail?: string;
  entity_type?: string;
  entity_id?: string;
  entity_name?: string;
  level: string;
  source?: string;
}

export interface AuditLogResult {
  entries: AuditLogEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditLogParams {
  category?: string;
  event_type?: string;
  level?: string;
  limit?: number;
  offset?: number;
  since?: string;
  until?: string;
}

// ─── Fetch ──────────────────────────────────────────────────────────────

export async function fetchAuditLog(
  params: AuditLogParams = {},
  signal?: AbortSignal,
): Promise<AuditLogResult> {
  const qs = new URLSearchParams();
  if (params.category) qs.set("category", params.category);
  if (params.event_type) qs.set("event_type", params.event_type);
  if (params.level) qs.set("level", params.level);
  if (params.limit != null) qs.set("limit", String(params.limit));
  if (params.offset != null) qs.set("offset", String(params.offset));
  if (params.since) qs.set("since", params.since);
  if (params.until) qs.set("until", params.until);

  const url = `/api/v1/system/audit-log${qs.toString() ? `?${qs}` : ""}`;
  const res = await apiFetch(url, { signal });
  if (!res.ok) {
    throw new Error(`audit log: ${res.status} ${res.statusText}`);
  }
  return (await res.json()) as AuditLogResult;
}

// ─── Hook ───────────────────────────────────────────────────────────────

export function useAuditLog(params: AuditLogParams = {}) {
  return useQuery({
    queryKey: ["system", "audit-log", params],
    queryFn: ({ signal }) => fetchAuditLog(params, signal),
    refetchInterval: 15_000,
    staleTime: 10_000,
  });
}
