// Typed fetch wrappers + react-query hooks for account invites.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";
import { ApiError, type UserRole } from "@/lib/users-api";

export type InviteStatus = "pending" | "used" | "expired";

export interface Invite {
  id: string;
  token?: string;
  url?: string;
  email?: string;
  role: UserRole;
  status: InviteStatus;
  created_at?: string;
  expires_at?: string;
  used_at?: string;
  used_by_name?: string;
}

export interface CreateInviteInput {
  email?: string;
  role: UserRole;
  expires_in_hours?: number;
}

export interface PublicInvite {
  valid: boolean;
  email?: string;
  role?: UserRole;
}

export interface AcceptInviteInput {
  username: string;
  password: string;
}

export interface AcceptedUser {
  id: number;
  username: string;
  email?: string;
  role: UserRole;
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
      | { error?: string | { message?: string } }
      | undefined;
    let message = `${method} ${path} failed: ${res.status} ${res.statusText}`;
    if (typeof env?.error === "string") {
      message = env.error;
    } else if (env?.error?.message) {
      message = env.error.message;
    }
    throw new ApiError(res.status, message);
  }
  return parsed as T;
}

export async function listInvites(signal?: AbortSignal): Promise<Invite[]> {
  const data = await request<{ invites: Invite[] }>(
    "GET",
    "/api/v1/auth/invites",
    undefined,
    signal,
  );
  return data?.invites ?? [];
}

export async function createInvite(body: CreateInviteInput): Promise<Invite> {
  const data = await request<{ invite: Invite }>(
    "POST",
    "/api/v1/auth/invites",
    body,
  );
  return data.invite;
}

export async function revokeInvite(id: string): Promise<void> {
  await request<void>("DELETE", `/api/v1/auth/invites/${id}`);
}

export async function lookupInvite(token: string): Promise<PublicInvite> {
  return request<PublicInvite>(
    "GET",
    `/api/v1/auth/invites/redeem/${encodeURIComponent(token)}`,
  );
}

export async function acceptInvite(
  token: string,
  body: AcceptInviteInput,
): Promise<AcceptedUser> {
  return request<AcceptedUser>(
    "POST",
    `/api/v1/auth/invites/redeem/${encodeURIComponent(token)}/accept`,
    body,
  );
}

const INVITES_KEY = ["auth", "invites"] as const;

export function useInvites() {
  return useQuery({
    queryKey: INVITES_KEY,
    queryFn: ({ signal }) => listInvites(signal),
  });
}

export function useCreateInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateInviteInput) => createInvite(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: INVITES_KEY }),
  });
}

export function useRevokeInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => revokeInvite(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: INVITES_KEY }),
  });
}
