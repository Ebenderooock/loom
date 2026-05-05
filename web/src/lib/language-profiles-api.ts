// Typed fetch wrappers for the Loom language-profile REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

// ---------- Types ----------

export interface Language {
  code: string;
  name: string;
  native_name: string;
}

export interface LanguagePriority {
  language: Language;
  allowed: boolean;
  priority: number;
}

export interface LanguageProfile {
  id: string;
  name: string;
  languages: LanguagePriority[];
  cutoff_language: string;
  upgrade_allowed: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateProfileRequest {
  name: string;
  languages: LanguagePriority[];
  cutoff_language: string;
  upgrade_allowed: boolean;
}

export interface UpdateProfileRequest {
  name?: string;
  languages?: LanguagePriority[];
  cutoff_language?: string;
  upgrade_allowed?: boolean;
}

// ---------- Fetch helpers ----------

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    const msg =
      (body as { error?: { message?: string } })?.error?.message ??
      res.statusText;
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as unknown as T;
  return res.json() as Promise<T>;
}

// ---------- React-Query hooks ----------

export function useLanguages() {
  return useQuery<Language[]>({
    queryKey: ["languages"],
    queryFn: () =>
      fetchJSON<{ data: Language[] }>("/api/v1/languages").then((r) => r.data),
    staleTime: Infinity,
  });
}

export function useLanguageProfiles() {
  return useQuery<LanguageProfile[]>({
    queryKey: ["language-profiles"],
    queryFn: () =>
      fetchJSON<{ data: LanguageProfile[] }>("/api/v1/language-profiles").then(
        (r) => r.data,
      ),
  });
}

export function useCreateLanguageProfile() {
  const qc = useQueryClient();
  return useMutation<LanguageProfile, Error, CreateProfileRequest>({
    mutationFn: (req) =>
      fetchJSON<LanguageProfile>("/api/v1/language-profiles", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["language-profiles"] }),
  });
}

export function useUpdateLanguageProfile() {
  const qc = useQueryClient();
  return useMutation<
    LanguageProfile,
    Error,
    { id: string; req: UpdateProfileRequest }
  >({
    mutationFn: ({ id, req }) =>
      fetchJSON<LanguageProfile>(`/api/v1/language-profiles/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(req),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["language-profiles"] }),
  });
}

export function useDeleteLanguageProfile() {
  const qc = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: (id) =>
      fetchJSON<void>(`/api/v1/language-profiles/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["language-profiles"] }),
  });
}
