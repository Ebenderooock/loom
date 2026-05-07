import { useQuery } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-client";

export function useReviewCount() {
  const api = useApiClient();
  const { data } = useQuery({
    queryKey: ["reviews", "count"],
    queryFn: () => api.get<{ count: number }>("/reviews/count"),
    refetchInterval: 30_000,
  });
  return data?.count ?? 0;
}
