import * as React from "react";
import { RotateCcw, ChevronDown, ChevronUp, HeartPulse } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useApiClient } from "@/lib/api-client";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty-state";

// ---------- Types ----------

interface IndexerSearchHealth {
  indexer_id: string;
  indexer_name: string;
  total_searches: number;
  success_count: number;
  fail_count: number;
  success_rate: number;
  avg_response_ms: number;
  last_search_at?: string;
  last_error_at?: string;
  last_error?: string;
  api_calls_today: number;
  status: "healthy" | "degraded" | "failing" | "unknown";
}

// ---------- Hooks ----------

function useIndexerSearchHealth() {
  const api = useApiClient();
  return useQuery({
    queryKey: ["indexer-search-health"],
    queryFn: () =>
      api.get<{ data: IndexerSearchHealth[] }>("/indexers/health"),
    refetchInterval: 30_000,
  });
}

function useResetAllHealth() {
  const api = useApiClient();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<{ ok: true }>("/indexers/health/reset"),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ["indexer-search-health"] }),
  });
}

// ---------- Helpers ----------

const STATUS_STYLES: Record<
  IndexerSearchHealth["status"],
  { bg: string; text: string; label: string }
> = {
  healthy: {
    bg: "bg-green-500/10",
    text: "text-green-500",
    label: "Healthy",
  },
  degraded: {
    bg: "bg-yellow-500/10",
    text: "text-yellow-500",
    label: "Degraded",
  },
  failing: {
    bg: "bg-red-500/10",
    text: "text-red-500",
    label: "Failing",
  },
  unknown: {
    bg: "bg-gray-500/10",
    text: "text-gray-500",
    label: "Unknown",
  },
};

function StatusBadge({ status }: { status: IndexerSearchHealth["status"] }) {
  const s = STATUS_STYLES[status];
  return (
    <Badge variant="outline" className={`${s.bg} ${s.text} border-0`}>
      {s.label}
    </Badge>
  );
}

// ---------- Components ----------

function SummaryBar({ items }: { items: IndexerSearchHealth[] }) {
  const counts = {
    total: items.length,
    healthy: items.filter((i) => i.status === "healthy").length,
    degraded: items.filter((i) => i.status === "degraded").length,
    failing: items.filter((i) => i.status === "failing").length,
    unknown: items.filter((i) => i.status === "unknown").length,
  };

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-5">
      <SummaryCard label="Total" count={counts.total} color="text-foreground" />
      <SummaryCard label="Healthy" count={counts.healthy} color="text-green-500" />
      <SummaryCard label="Degraded" count={counts.degraded} color="text-yellow-500" />
      <SummaryCard label="Failing" count={counts.failing} color="text-red-500" />
      <SummaryCard label="Unknown" count={counts.unknown} color="text-gray-500" />
    </div>
  );
}

function SummaryCard({
  label,
  count,
  color,
}: {
  label: string;
  count: number;
  color: string;
}) {
  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className={`text-2xl font-bold ${color}`}>{count}</p>
      </CardContent>
    </Card>
  );
}

function HealthCard({ item }: { item: IndexerSearchHealth }) {
  const [errorExpanded, setErrorExpanded] = React.useState(false);
  const pct = Math.round(item.success_rate * 100);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">
          {item.indexer_name}
        </CardTitle>
        <StatusBadge status={item.status} />
      </CardHeader>
      <CardContent className="space-y-3">
        {/* Success rate */}
        <div>
          <div className="mb-1 flex items-center justify-between text-xs text-muted-foreground">
            <span>Success rate</span>
            <span className="font-medium text-foreground">{pct}%</span>
          </div>
          <Progress value={pct} />
        </div>

        {/* Stats grid */}
        <div className="grid grid-cols-2 gap-2 text-xs">
          <div>
            <p className="text-muted-foreground">Avg response</p>
            <p className="font-medium">{item.avg_response_ms.toFixed(0)} ms</p>
          </div>
          <div>
            <p className="text-muted-foreground">Total searches</p>
            <p className="font-medium">{item.total_searches.toLocaleString()}</p>
          </div>
          <div>
            <p className="text-muted-foreground">API calls today</p>
            <p className="font-medium">{item.api_calls_today.toLocaleString()}</p>
          </div>
          <div>
            <p className="text-muted-foreground">Failures</p>
            <p className="font-medium">{item.fail_count.toLocaleString()}</p>
          </div>
        </div>

        {/* Last error (collapsible) */}
        {item.last_error && (
          <div className="rounded-md border border-border p-2">
            <button
              type="button"
              className="flex w-full items-center justify-between text-xs text-muted-foreground hover:text-foreground"
              onClick={() => setErrorExpanded((v) => !v)}
            >
              <span className="font-medium text-red-500">Last error</span>
              {errorExpanded ? (
                <ChevronUp className="h-3 w-3" />
              ) : (
                <ChevronDown className="h-3 w-3" />
              )}
            </button>
            <p
              className={`mt-1 text-xs text-muted-foreground break-all ${
                errorExpanded ? "" : "line-clamp-1"
              }`}
            >
              {item.last_error}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-5">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-[72px] rounded-xl" />
        ))}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-[220px] rounded-xl" />
        ))}
      </div>
    </div>
  );
}

// ---------- Page ----------

export function IndexerHealthPage() {
  useSetPageHeader("Indexer Health");

  const { data, isLoading, isError } = useIndexerSearchHealth();
  const resetAll = useResetAllHealth();

  const items = data?.data ?? [];

  const handleReset = () => {
    if (window.confirm("Reset all indexer health stats? This cannot be undone.")) {
      resetAll.mutate();
    }
  };

  return (
    <div className="space-y-6">
      {/* Header row */}
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Indexer Health</h1>
        <Button
          variant="outline"
          size="sm"
          className="gap-2"
          onClick={handleReset}
          disabled={resetAll.isPending}
        >
          <RotateCcw className="h-4 w-4" />
          Reset Stats
        </Button>
      </div>

      {isLoading && <LoadingSkeleton />}

      {isError && (
        <p className="text-sm text-destructive">
          Failed to load health data. Will retry automatically.
        </p>
      )}

      {!isLoading && !isError && (
        <>
          <SummaryBar items={items} />
          {items.length === 0 ? (
            <EmptyState
              icon={<HeartPulse />}
              title="No indexer health data yet"
              description="Health metrics appear here once your indexers have run some searches."
            />
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((item) => (
                <HealthCard key={item.indexer_id} item={item} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}
