import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type WorkflowState =
  | "searching"
  | "grabbed"
  | "downloading"
  | "importing"
  | "completed"
  | "failed"
  | "cancelled";

export type WorkflowType = "movie_search" | "episode_search" | "manual_import";

export interface WorkflowItem {
  workflowId: string;
  mediaType: string;
  mediaId: string;
}

export interface WorkflowEvent {
  id: number;
  workflowId: string;
  fromState: string;
  toState: string;
  message?: string;
  createdAt: string;
}

export interface Workflow {
  id: string;
  type: WorkflowType;
  state: WorkflowState;
  mediaType: string;
  grabTitle?: string;
  downloadClientId?: string;
  downloadId?: string;
  qualityProfileId?: string;
  retryCount: number;
  maxRetries: number;
  lastError?: string;
  metadata?: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
  items?: WorkflowItem[];
  history?: WorkflowEvent[];
}

// ---------- HTTP helpers ----------

async function request<T>(
  method: string,
  path: string,
  signal?: AbortSignal,
): Promise<T> {
  const res = await apiFetch(path, { method, signal });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  if (!res.ok) {
    throw new Error(text || `${method} ${path}: ${res.status}`);
  }
  return text ? JSON.parse(text) : (undefined as T);
}

// ---------- Endpoints ----------

export const workflowKeys = {
  all: ["workflows"] as const,
  list: () => [...workflowKeys.all, "list"] as const,
  detail: (id: string) => [...workflowKeys.all, "detail", id] as const,
};

export async function listWorkflows(signal?: AbortSignal): Promise<Workflow[]> {
  return request<Workflow[]>("GET", "/api/v1/workflows", signal);
}

export async function getWorkflow(
  id: string,
  signal?: AbortSignal,
): Promise<Workflow> {
  return request<Workflow>(
    "GET",
    `/api/v1/workflows/${encodeURIComponent(id)}`,
    signal,
  );
}

export async function cancelWorkflow(id: string): Promise<void> {
  await request<void>(
    "POST",
    `/api/v1/workflows/${encodeURIComponent(id)}/cancel`,
  );
}

export async function retryWorkflow(id: string): Promise<void> {
  await request<void>(
    "POST",
    `/api/v1/workflows/${encodeURIComponent(id)}/retry`,
  );
}

export async function deleteWorkflow(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/workflows/${encodeURIComponent(id)}`,
  );
}

// ---------- React Query hooks ----------

export function useWorkflows() {
  return useQuery<Workflow[], Error>({
    queryKey: workflowKeys.list(),
    queryFn: ({ signal }) => listWorkflows(signal),
    refetchInterval: 5000, // auto-refresh every 5s for live status
  });
}

export function useWorkflow(id: string) {
  return useQuery<Workflow, Error>({
    queryKey: workflowKeys.detail(id),
    queryFn: ({ signal }) => getWorkflow(id, signal),
    enabled: !!id,
    refetchInterval: 3000,
  });
}

export function useCancelWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: cancelWorkflow,
    onSuccess: () => qc.invalidateQueries({ queryKey: workflowKeys.all }),
  });
}

export function useRetryWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: retryWorkflow,
    onSuccess: () => qc.invalidateQueries({ queryKey: workflowKeys.all }),
  });
}

export function useDeleteWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteWorkflow,
    onSuccess: () => qc.invalidateQueries({ queryKey: workflowKeys.all }),
  });
}
