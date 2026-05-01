import { useQuery } from "@tanstack/react-query";

export interface SystemStatus {
  version: string;
  commit: string;
  buildDate: string;
  engine: string;
}

export async function fetchSystemStatus(
  signal?: AbortSignal,
): Promise<SystemStatus> {
  const res = await fetch("/api/v1/system/status", { signal });
  if (!res.ok) {
    throw new Error(`system status: ${res.status} ${res.statusText}`);
  }
  return (await res.json()) as SystemStatus;
}

export function useSystemStatus() {
  return useQuery({
    queryKey: ["system", "status"],
    queryFn: ({ signal }) => fetchSystemStatus(signal),
    staleTime: 30_000,
    retry: 1,
  });
}
