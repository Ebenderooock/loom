// Typed fetch wrappers for the Loom notification REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type ConnectionType =
  | "discord"
  | "slack"
  | "telegram"
  | "email"
  | "webhook"
  | "gotify"
  | "pushover"
  | "apprise"
  | "ntfy";

export interface ConnectionSettings {
  webhook_url?: string;
  api_key?: string;
  channel?: string;
  bot_token?: string;
  chat_id?: string;
  host?: string;
  port?: number;
  username?: string;
  password?: string;
  from?: string;
  to?: string;
  tls?: boolean;
  user_key?: string;
  server_url?: string;
  topic?: string;
  template_override?: string;
}

export interface NotificationConnection {
  id: string;
  name: string;
  type: ConnectionType;
  enabled: boolean;
  settings: ConnectionSettings;
  on_grab: boolean;
  on_download: boolean;
  on_upgrade: boolean;
  on_rename: boolean;
  on_delete: boolean;
  on_health_issue: boolean;
  on_application_update: boolean;
  on_playback: boolean;
  tags: string[];
  created_at: string;
  updated_at: string;
}

export interface CreateConnectionRequest {
  name: string;
  type: ConnectionType;
  enabled?: boolean;
  settings: ConnectionSettings;
  on_grab?: boolean;
  on_download?: boolean;
  on_upgrade?: boolean;
  on_rename?: boolean;
  on_delete?: boolean;
  on_health_issue?: boolean;
  on_application_update?: boolean;
  on_playback?: boolean;
  tags?: string[];
}

export interface UpdateConnectionRequest {
  name?: string;
  type?: ConnectionType;
  enabled?: boolean;
  settings?: ConnectionSettings;
  on_grab?: boolean;
  on_download?: boolean;
  on_upgrade?: boolean;
  on_rename?: boolean;
  on_delete?: boolean;
  on_health_issue?: boolean;
  on_application_update?: boolean;
  on_playback?: boolean;
  tags?: string[];
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

export async function listConnections(
  signal?: AbortSignal,
): Promise<NotificationConnection[]> {
  const data = await request<NotificationConnection[]>(
    "GET",
    "/api/v1/notifications",
    undefined,
    signal,
  );
  return data ?? [];
}

export async function getConnection(
  id: string,
  signal?: AbortSignal,
): Promise<NotificationConnection> {
  return request<NotificationConnection>(
    "GET",
    `/api/v1/notifications/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createConnection(
  body: CreateConnectionRequest,
): Promise<NotificationConnection> {
  return request<NotificationConnection>(
    "POST",
    "/api/v1/notifications",
    body,
  );
}

export async function updateConnection(
  id: string,
  body: UpdateConnectionRequest,
): Promise<NotificationConnection> {
  return request<NotificationConnection>(
    "PUT",
    `/api/v1/notifications/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteConnection(id: string): Promise<void> {
  await request<void>(
    "DELETE",
    `/api/v1/notifications/${encodeURIComponent(id)}`,
  );
}

export async function testConnection(
  id: string,
): Promise<{ message: string }> {
  return request<{ message: string }>(
    "POST",
    `/api/v1/notifications/${encodeURIComponent(id)}/test`,
  );
}

export async function testConnectionConfig(
  body: CreateConnectionRequest,
): Promise<{ ok: boolean; message?: string; error?: string }> {
  return request<{ ok: boolean; message?: string; error?: string }>(
    "POST",
    "/api/v1/notifications/test",
    body,
  );
}

// ---------- History types ----------

export interface HistoryEntry {
  id: number;
  connection_id?: string;
  event_type: string;
  title: string;
  message: string;
  success: boolean;
  error_message?: string;
  sent_at: string;
}

export async function listHistory(
  limit?: number,
  signal?: AbortSignal,
): Promise<HistoryEntry[]> {
  const params = limit ? `?limit=${limit}` : "";
  const data = await request<HistoryEntry[]>(
    "GET",
    `/api/v1/notifications/history${params}`,
    undefined,
    signal,
  );
  return data ?? [];
}

// ---------- Query keys ----------

export const notificationKeys = {
  all: ["notifications"] as const,
  list: () => [...notificationKeys.all, "list"] as const,
  detail: (id: string) => [...notificationKeys.all, "detail", id] as const,
  history: (limit?: number) =>
    [...notificationKeys.all, "history", limit] as const,
};

// ---------- React Query hooks ----------

export function useNotifications() {
  return useQuery<NotificationConnection[], Error>({
    queryKey: notificationKeys.list(),
    queryFn: ({ signal }) => listConnections(signal),
  });
}

export function useCreateNotification() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createConnection,
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.all }),
  });
}

export function useUpdateNotification() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: UpdateConnectionRequest;
    }) => updateConnection(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.all }),
  });
}

export function useDeleteNotification() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteConnection,
    onSuccess: () => qc.invalidateQueries({ queryKey: notificationKeys.all }),
  });
}

export function useTestNotification() {
  return useMutation({ mutationFn: testConnection });
}

export function useTestNotificationConfig() {
  return useMutation({ mutationFn: testConnectionConfig });
}

export function useNotificationHistory(limit?: number) {
  return useQuery<HistoryEntry[], Error>({
    queryKey: notificationKeys.history(limit),
    queryFn: ({ signal }) => listHistory(limit, signal),
  });
}

// ---------- Connection type metadata ----------

export const CONNECTION_TYPES: {
  value: ConnectionType;
  label: string;
  fields: string[];
}[] = [
  {
    value: "discord",
    label: "Discord",
    fields: ["webhook_url"],
  },
  {
    value: "slack",
    label: "Slack",
    fields: ["webhook_url"],
  },
  {
    value: "webhook",
    label: "Webhook",
    fields: ["webhook_url"],
  },
  {
    value: "gotify",
    label: "Gotify",
    fields: ["server_url", "api_key"],
  },
  {
    value: "ntfy",
    label: "ntfy",
    fields: ["server_url", "topic"],
  },
  {
    value: "telegram",
    label: "Telegram",
    fields: ["bot_token", "chat_id"],
  },
  {
    value: "pushover",
    label: "Pushover",
    fields: ["api_key", "user_key"],
  },
  {
    value: "email",
    label: "Email",
    fields: ["host", "port", "username", "password", "from", "to", "tls"],
  },
  {
    value: "apprise",
    label: "Apprise",
    fields: ["server_url"],
  },
];

export const EVENT_TYPES = [
  { key: "on_grab" as const, label: "On Grab" },
  { key: "on_download" as const, label: "On Download" },
  { key: "on_upgrade" as const, label: "On Upgrade" },
  { key: "on_rename" as const, label: "On Rename" },
  { key: "on_delete" as const, label: "On Delete" },
  { key: "on_health_issue" as const, label: "On Health Issue" },
  { key: "on_application_update" as const, label: "On App Update" },
  { key: "on_playback" as const, label: "On Playback" },
];

export const TEMPLATE_VARIABLES = [
  "{{.Title}}",
  "{{.Year}}",
  "{{.Quality}}",
  "{{.Indexer}}",
  "{{.Size}}",
  "{{.EventType}}",
  "{{.MediaType}}",
];
