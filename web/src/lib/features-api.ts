// Typed fetch wrappers + hooks for the Loom feature-flags REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

export interface Feature {
  key: string;
  label: string;
  description: string;
  category: string;
  default: boolean;
  enabled: boolean;
}

export const featureKeys = {
  all: ["features"] as const,
};

async function fetchFeatures(): Promise<Feature[]> {
  const res = await apiFetch("/api/v1/features");
  if (!res.ok) throw new Error(`fetch features failed: ${res.status}`);
  const body = (await res.json()) as { features: Feature[] };
  return body.features ?? [];
}

async function setFeature(key: string, enabled: boolean): Promise<void> {
  const res = await apiFetch(`/api/v1/features/${encodeURIComponent(key)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  if (!res.ok) {
    let msg = `set feature failed: ${res.status}`;
    try {
      const b = await res.json();
      if (b?.error) msg = b.error;
    } catch {
      // ignore
    }
    throw new Error(msg);
  }
}

export function useFeatures() {
  return useQuery<Feature[], Error>({
    queryKey: featureKeys.all,
    queryFn: fetchFeatures,
    staleTime: 30_000,
  });
}

// useFeatureEnabled returns whether a feature is enabled. While the query is
// loading it returns the supplied fallback (defaults to true) so UI gated on a
// flag does not flicker out on first paint.
export function useFeatureEnabled(key: string, fallback = true): boolean {
  const { data, isLoading } = useFeatures();
  if (isLoading || !data) return fallback;
  const f = data.find((x) => x.key === key);
  return f ? f.enabled : fallback;
}

export function useSetFeature() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean }) =>
      setFeature(key, enabled),
    onSuccess: () => qc.invalidateQueries({ queryKey: featureKeys.all }),
  });
}
