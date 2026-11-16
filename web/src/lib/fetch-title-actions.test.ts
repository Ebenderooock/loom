import { afterEach, expect, it, vi } from "vitest";
import { apiFetch, registerAcquisitionRecovery } from "./fetch";

afterEach(() => vi.unstubAllGlobals());

it.each(["refresh-metadata", "rescan-library", "sync-plex"])(
  "never invokes acquisition recovery for title maintenance (%s)",
  async (endpoint) => {
    const recovery = vi.fn().mockResolvedValue(true);
    const unregister = registerAcquisitionRecovery(recovery);
    const response = Response.json(
      {
        error: {
          code: "acquisition_deferred",
          details: {
            history_id: "history",
            title: "Test",
            status: "failed",
            retry_allowed: true,
          },
        },
      },
      { status: 409 },
    );
    const fetch = vi.fn().mockResolvedValue(response);
    vi.stubGlobal("fetch", fetch);
    try {
      expect(
        await apiFetch(
          `/api/v1/movies/title/${endpoint}`,
          { method: "POST" },
          { acquisitionRecovery: false },
        ),
      ).toBe(response);
      expect(fetch).toHaveBeenCalledTimes(1);
      expect(recovery).not.toHaveBeenCalled();
      expect(fetch).toHaveBeenCalledWith(`/api/v1/movies/title/${endpoint}`, {
        method: "POST",
        credentials: "include",
      });
    } finally {
      unregister();
    }
  },
);
