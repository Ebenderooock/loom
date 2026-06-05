// Typed fetch wrappers for the Loom custom formats REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type SpecImplementation =
  | "ReleaseTitleSpec"
  | "QualitySpec"
  | "SizeSpec"
  | "IndexerFlagSpec"
  | "SourceSpec"
  | "ResolutionSpec"
  | "CodecSpec"
  | "AudioSpec"
  | "ReleaseGroupSpec"
  | "LanguageSpec";

export interface Specification {
  name: string;
  implementation: SpecImplementation;
  negate: boolean;
  required: boolean;
  fields: Record<string, unknown>;
}

export interface CustomFormat {
  id: string;
  name: string;
  include_when_renaming: boolean;
  specifications: Specification[];
  created_at?: string;
  updated_at?: string;
}

export interface ReleaseInfo {
  title: string;
  quality: string;
  size: number;
  indexer: string;
  source: string;
  resolution: string;
  codec: string;
  audio: string;
  group: string;
  languages: string[];
  indexer_flags: string[];
}

export interface FormatMatch {
  custom_format_id: string;
  custom_format_name: string;
  score: number;
}

export interface TestResult {
  release: ReleaseInfo;
  matches: FormatMatch[];
  score: number;
}

// ---------- Constants ----------

export const IMPLEMENTATIONS: {
  value: SpecImplementation;
  label: string;
  fieldKey: string;
  placeholder: string;
}[] = [
  {
    value: "ReleaseTitleSpec",
    label: "Release Title (regex)",
    fieldKey: "value",
    placeholder: "e.g. \\bHEVC\\b",
  },
  {
    value: "QualitySpec",
    label: "Quality",
    fieldKey: "value",
    placeholder: "e.g. Bluray-1080p",
  },
  {
    value: "SourceSpec",
    label: "Source",
    fieldKey: "value",
    placeholder: "e.g. BluRay, WEB-DL",
  },
  {
    value: "ResolutionSpec",
    label: "Resolution",
    fieldKey: "value",
    placeholder: "e.g. 2160p, 1080p",
  },
  {
    value: "CodecSpec",
    label: "Codec",
    fieldKey: "value",
    placeholder: "e.g. x265, AV1",
  },
  {
    value: "AudioSpec",
    label: "Audio",
    fieldKey: "value",
    placeholder: "e.g. Atmos, TrueHD",
  },
  {
    value: "ReleaseGroupSpec",
    label: "Release Group",
    fieldKey: "value",
    placeholder: "e.g. FraMeSToR",
  },
  {
    value: "LanguageSpec",
    label: "Language",
    fieldKey: "value",
    placeholder: "e.g. English, MULTi",
  },
  {
    value: "SizeSpec",
    label: "Size (GB)",
    fieldKey: "min",
    placeholder: "min GB",
  },
  {
    value: "IndexerFlagSpec",
    label: "Indexer Flag",
    fieldKey: "value",
    placeholder: "e.g. freeleech",
  },
];

// ---------- HTTP helpers ----------

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
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
    throw new ApiError(
      res.status,
      env?.error?.message ?? `${method} ${path} failed: ${res.status}`,
    );
  }
  return parsed as T;
}

// ---------- API functions ----------

export async function listCustomFormats(
  signal?: AbortSignal,
): Promise<CustomFormat[]> {
  const data = await request<{ data: CustomFormat[] }>(
    "GET",
    "/api/v1/custom-formats",
    undefined,
    signal,
  );
  return data?.data ?? [];
}

export async function getCustomFormat(
  id: string,
  signal?: AbortSignal,
): Promise<CustomFormat> {
  return request<CustomFormat>(
    "GET",
    `/api/v1/custom-formats/${encodeURIComponent(id)}`,
    undefined,
    signal,
  );
}

export async function createCustomFormat(
  body: CustomFormat,
): Promise<CustomFormat> {
  return request<CustomFormat>("POST", "/api/v1/custom-formats", body);
}

export async function updateCustomFormat(
  id: string,
  body: CustomFormat,
): Promise<CustomFormat> {
  return request<CustomFormat>(
    "PUT",
    `/api/v1/custom-formats/${encodeURIComponent(id)}`,
    body,
  );
}

export async function deleteCustomFormat(id: string): Promise<void> {
  return request<void>(
    "DELETE",
    `/api/v1/custom-formats/${encodeURIComponent(id)}`,
  );
}

export async function testCustomFormat(title: string): Promise<TestResult> {
  return request<TestResult>("POST", "/api/v1/custom-formats/test", { title });
}

// ---------- React Query hooks ----------

export function useCustomFormats() {
  return useQuery({
    queryKey: ["custom-formats"],
    queryFn: ({ signal }) => listCustomFormats(signal),
  });
}

export function useCreateCustomFormat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createCustomFormat,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["custom-formats"] }),
  });
}

export function useUpdateCustomFormat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: CustomFormat }) =>
      updateCustomFormat(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["custom-formats"] }),
  });
}

export function useDeleteCustomFormat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteCustomFormat,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["custom-formats"] }),
  });
}

export function useTestCustomFormat() {
  return useMutation({
    mutationFn: testCustomFormat,
  });
}
