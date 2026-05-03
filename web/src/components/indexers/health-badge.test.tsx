import { describe, expect, it } from "vitest";
import { mapHealth } from "@/components/indexers/health-badge";
import type { IndexerHealth } from "@/lib/indexers-api";

const NOW = new Date("2025-01-15T12:00:00Z");

function h(partial: Partial<IndexerHealth>): IndexerHealth {
  return {
    indexer_id: "x",
    status: "ok",
    last_checked_at: NOW.toISOString(),
    ...partial,
  };
}

describe("mapHealth", () => {
  it("returns unknown when health is missing", () => {
    expect(mapHealth(undefined, NOW).status).toBe("unknown");
  });

  it("maps a recent ok status to healthy", () => {
    const summary = mapHealth(h({ status: "ok", latency_ms: 42 }), NOW);
    expect(summary.status).toBe("healthy");
    expect(summary.label).toBe("Healthy");
    expect(summary.reason).toContain("42 ms");
  });

  it("downgrades a stale ok status to degraded", () => {
    const old = new Date(NOW.getTime() - 36 * 60 * 60 * 1000).toISOString();
    const summary = mapHealth(
      h({ status: "ok", last_checked_at: old }),
      NOW,
    );
    expect(summary.status).toBe("degraded");
    expect(summary.label).toBe("Stale");
  });

  it("maps degraded to degraded with the last error", () => {
    const summary = mapHealth(
      h({ status: "degraded", last_error: "slow upstream" }),
      NOW,
    );
    expect(summary.status).toBe("degraded");
    expect(summary.reason).toBe("slow upstream");
  });

  it("maps failed to down", () => {
    const summary = mapHealth(
      h({ status: "failed", last_error: "401 Unauthorized" }),
      NOW,
    );
    expect(summary.status).toBe("down");
    expect(summary.reason).toBe("401 Unauthorized");
  });

  it("maps explicit unknown status", () => {
    const summary = mapHealth(h({ status: "unknown" }), NOW);
    expect(summary.status).toBe("unknown");
  });
});
