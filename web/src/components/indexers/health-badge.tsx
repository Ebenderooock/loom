// HealthBadge maps an IndexerHealth row to a small status pill so
// operators can scan a list of indexers and immediately see which
// ones need attention. The mapping rules live in `mapHealth` and
// are exported for unit tests.

import type { IndexerHealth } from "@/lib/indexers-api";
import { cn } from "@/lib/utils";

export type HealthStatus = "healthy" | "degraded" | "down" | "unknown";

export interface HealthSummary {
  status: HealthStatus;
  label: string;
  reason: string;
}

const STALE_AFTER_MS = 24 * 60 * 60 * 1000;

export function mapHealth(
  health: IndexerHealth | undefined,
  now: Date = new Date(),
): HealthSummary {
  if (!health) {
    return {
      status: "unknown",
      label: "Unknown",
      reason: "No health checks have run yet for this indexer.",
    };
  }
  switch (health.status) {
    case "ok": {
      const checkedAt = Date.parse(health.last_checked_at);
      const stale =
        Number.isFinite(checkedAt) && now.getTime() - checkedAt > STALE_AFTER_MS;
      if (stale) {
        return {
          status: "degraded",
          label: "Stale",
          reason: `Last successful check was more than 24 hours ago (${health.last_checked_at}).`,
        };
      }
      return {
        status: "healthy",
        label: "Healthy",
        reason:
          typeof health.latency_ms === "number"
            ? `Last check OK in ${health.latency_ms} ms.`
            : "Last check OK.",
      };
    }
    case "degraded":
      return {
        status: "degraded",
        label: "Degraded",
        reason:
          health.last_error ??
          "The last check returned a partial or slow response.",
      };
    case "failed":
      return {
        status: "down",
        label: "Down",
        reason: health.last_error ?? "Last check failed.",
      };
    case "unknown":
    default:
      return {
        status: "unknown",
        label: "Unknown",
        reason: "The indexer health status is unknown.",
      };
  }
}

const STYLES: Record<HealthStatus, string> = {
  healthy:
    "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 ring-emerald-500/30",
  degraded:
    "bg-amber-500/15 text-amber-700 dark:text-amber-300 ring-amber-500/30",
  down: "bg-red-500/15 text-red-700 dark:text-red-300 ring-red-500/30",
  unknown: "bg-muted text-muted-foreground ring-border",
};

export function HealthBadge({
  health,
  className,
}: {
  health: IndexerHealth | undefined;
  className?: string;
}) {
  const summary = mapHealth(health);
  return (
    <span
      role="status"
      aria-label={`Indexer health: ${summary.label}. ${summary.reason}`}
      title={summary.reason}
      data-status={summary.status}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset",
        STYLES[summary.status],
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "inline-block h-1.5 w-1.5 rounded-full",
          summary.status === "healthy" && "bg-emerald-500",
          summary.status === "degraded" && "bg-amber-500",
          summary.status === "down" && "bg-red-500",
          summary.status === "unknown" && "bg-muted-foreground",
        )}
      />
      {summary.label}
    </span>
  );
}
