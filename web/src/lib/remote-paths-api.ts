// Typed fetch wrappers for remote path mappings REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// ---------- Types ----------

export interface RemotePathMapping {
  id: string;
  client_id: string;
  remote_path: string;
  local_path: string;
  created_at: string;
}

export interface CreateRemotePathMappingRequest {
  client_id: string;
  remote_path: string;
  local_path: string;
}

// ---------- API Functions ----------

const BASE = "/api/v1/download-clients/remote-path-mappings";

export async function listRemotePathMappings(): Promise<RemotePathMapping[]> {
  const res = await fetch(BASE, { credentials: "include" });
  if (!res.ok) throw new Error("Failed to fetch remote path mappings");
  return res.json();
}

export async function createRemotePathMapping(
  req: CreateRemotePathMappingRequest
): Promise<RemotePathMapping> {
  const res = await fetch(BASE, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body?.error?.message || "Failed to create mapping");
  }
  return res.json();
}

export async function deleteRemotePathMapping(id: string): Promise<void> {
  const res = await fetch(`${BASE}/${id}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) throw new Error("Failed to delete mapping");
}

// ---------- React Query Hooks ----------

export const remotePathMappingsKey = ["remote-path-mappings"] as const;

export function useRemotePathMappings() {
  return useQuery({
    queryKey: remotePathMappingsKey,
    queryFn: listRemotePathMappings,
  });
}

export function useCreateRemotePathMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createRemotePathMapping,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: remotePathMappingsKey });
    },
  });
}

export function useDeleteRemotePathMapping() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteRemotePathMapping,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: remotePathMappingsKey });
    },
  });
}
