import { afterEach, describe, expect, it, vi } from "vitest";
import { queueImportFiles, reimportFile } from "./imports-api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

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

it("queues all files beyond the server's per-request limit", async () => {
  const paths = Array.from({ length: 1001 }, (_, i) => `/downloads/${i}.mkv`);
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async (_url, init) => {
      const body = JSON.parse(String(init?.body)) as { paths: string[] };
      return Response.json({
        data: body.paths.map((source_path) => ({
          source_path,
          status: "queued",
        })),
        errors: [],
      });
    });
  const outcome = await queueImportFiles(paths);
  expect(fetchSpy).toHaveBeenCalledTimes(2);
  expect(outcome.data.map((job) => job.source_path)).toEqual(paths);
  expect(outcome.errors).toEqual([]);
});

it("preserves queued files when a later chunk fails and surfaces uncertain outcomes", async () => {
  const paths = Array.from({ length: 1001 }, (_, i) => `/downloads/${i}.mkv`);
  vi.spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(
      Response.json({
        data: paths
          .slice(0, 1000)
          .map((source_path) => ({ source_path, status: "queued" })),
        errors: [],
      }),
    )
    .mockRejectedValueOnce(new Error("Connection interrupted"));
  const outcome = await queueImportFiles(paths);
  expect(outcome.data).toHaveLength(1000);
  expect(outcome.errors).toEqual([
    {
      path: paths[1000],
      error: expect.stringContaining("Queue outcome unknown"),
    },
  ]);
});

it("retains per-file errors from a completely rejected batch", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
    Response.json(
      {
        data: [],
        errors: [{ path: "/missing.mkv", error: "Source path does not exist" }],
      },
      { status: 422 },
    ),
  );
  const outcome = await queueImportFiles(["/missing.mkv"]);
  expect(outcome.data).toEqual([]);
  expect(outcome.errors[0]?.error).toBe("Source path does not exist");
});
