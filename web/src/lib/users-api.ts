// Typed fetch wrappers + react-query hooks for admin user management.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

export type UserRole = "admin" | "user";

export interface ManagedUser {
  id: number;
  username: string;
  email?: string;
  role: UserRole;
  protected: boolean;
  created_at?: string;
}

export interface CreateUserInput {
  username: string;
  password: string;
  email?: string;
  role: UserRole;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
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
    // Auth endpoints use {"error":"msg"}; others may use {error:{message}}.
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

export async function listUsers(signal?: AbortSignal): Promise<ManagedUser[]> {
  const data = await request<{ users: ManagedUser[] }>(
    "GET",
    "/api/v1/auth/users",
    undefined,
    signal,
  );
  return data?.users ?? [];
}

export async function createUser(body: CreateUserInput): Promise<ManagedUser> {
  const data = await request<{ user: ManagedUser }>(
    "POST",
    "/api/v1/auth/users",
    body,
  );
  return data.user;
}

export async function deleteUser(id: number): Promise<void> {
  await request<void>("DELETE", `/api/v1/auth/users/${id}`);
}

export async function setUserRole(
  id: number,
  role: UserRole,
): Promise<ManagedUser> {
  const data = await request<{ user: ManagedUser }>(
    "PATCH",
    `/api/v1/auth/users/${id}/role`,
    { role },
  );
  return data.user;
}

export async function resetUserPassword(
  id: number,
  password: string,
): Promise<void> {
  await request<void>("POST", `/api/v1/auth/users/${id}/password`, { password });
}

const USERS_KEY = ["auth", "users"] as const;

export function useUsers() {
  return useQuery({
    queryKey: USERS_KEY,
    queryFn: ({ signal }) => listUsers(signal),
  });
}

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: CreateUserInput) => createUser(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

export function useDeleteUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

export function useSetUserRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, role }: { id: number; role: UserRole }) =>
      setUserRole(id, role),
    onSuccess: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

export function useResetUserPassword() {
  return useMutation({
    mutationFn: ({ id, password }: { id: number; password: string }) =>
      resetUserPassword(id, password),
  });
}
