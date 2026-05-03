import { describe, it, expect } from "vitest";
import { mapHealth } from "@/components/downloads/health-badge";

describe("DownloadHealthBadge", () => {
  describe("mapHealth", () => {
    const now = new Date("2024-05-13T12:00:00Z");

    it("returns unknown when health is undefined", () => {
      const result = mapHealth(undefined, now);
      expect(result.status).toBe("unknown");
      expect(result.label).toBe("Unknown");
    });

    it("returns healthy when status is ok and recent", () => {
      const result = mapHealth(
        {
          client_id: "test",
          status: "ok",
          last_checked_at: "2024-05-13T11:00:00Z",
          consecutive_failures: 0,
        },
        now,
      );
      expect(result.status).toBe("healthy");
      expect(result.label).toBe("Healthy");
    });

    it("returns degraded when status is ok but stale (>24h)", () => {
      const result = mapHealth(
        {
          client_id: "test",
          status: "ok",
          last_checked_at: "2024-05-10T12:00:00Z",
          consecutive_failures: 0,
        },
        now,
      );
      expect(result.status).toBe("degraded");
      expect(result.label).toBe("Stale");
    });

    it("returns degraded when status is degraded", () => {
      const result = mapHealth(
        {
          client_id: "test",
          status: "degraded",
          last_checked_at: now.toISOString(),
          last_error: "Slow response",
          consecutive_failures: 2,
        },
        now,
      );
      expect(result.status).toBe("degraded");
      expect(result.label).toBe("Degraded");
    });

    it("returns down when status is failed", () => {
      const result = mapHealth(
        {
          client_id: "test",
          status: "failed",
          last_checked_at: now.toISOString(),
          last_error: "Connection refused",
          consecutive_failures: 3,
        },
        now,
      );
      expect(result.status).toBe("down");
      expect(result.label).toBe("Down");
    });
  });
});
