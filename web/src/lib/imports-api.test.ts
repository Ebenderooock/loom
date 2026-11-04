import { afterEach, describe, expect, it, vi } from "vitest";
import { reimportFile } from "./imports-api";

afterEach(() => vi.unstubAllGlobals());

describe("reimportFile", () => {
  it("returns the 202 acceptance payload, not a completed import record", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ status: "accepted" }), { status: 202 }),
      );
    vi.stubGlobal("fetch", fetchMock);
    const params = {
      media_type: "music",
      media_id: "track-1",
      source_path: "/downloads/song.flac",
      conflict_policy: "replace_if_better",
    };
    expect(await reimportFile(params)).toEqual({ status: "accepted" });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/imports/reimport",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify(params),
        credentials: "include",
      }),
    );
  });
});
