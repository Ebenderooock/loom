import { afterEach, describe, expect, it, vi } from "vitest";
import { deleteMediaFiles } from "./media-files-api";

afterEach(() => vi.unstubAllGlobals());

describe("media file deletion request bounds", () => {
  it.each([
    { paths: [] },
    { paths: Array.from({ length: 1001 }, (_, index) => `file-${index}`) },
  ])(
    "rejects selections outside the 1..1000 unique path limit before POST",
    async ({ paths }) => {
      const fetchMock = vi.fn();
      vi.stubGlobal("fetch", fetchMock);
      await expect(
        deleteMediaFiles({ mediaType: "movie", id: "m1" }, paths, "recycle"),
      ).rejects.toThrow("Select between 1 and 1,000 files");
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it("counts encoded bytes and rejects oversized request bodies before POST", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(
      deleteMediaFiles(
        { mediaType: "movie", id: "m1" },
        ["é".repeat(66_000)],
        "permanent",
      ),
    ).rejects.toThrow("128 KiB request limit");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("deduplicates opaque paths without normalizing them", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(Response.json({ deleted: ["Extras/file.mkv"] }));
    vi.stubGlobal("fetch", fetchMock);
    await deleteMediaFiles(
      { mediaType: "series", id: "s1", season: 2 },
      ["Extras/file.mkv", "Extras/file.mkv"],
      "recycle",
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/media-files/series/s1/delete?season=2",
      expect.objectContaining({
        body: JSON.stringify({
          paths: ["Extras/file.mkv"],
          deletionMode: "recycle",
        }),
      }),
    );
  });
});
