import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cancelImportScan,
  fetchImportScan,
  pollImportScan,
  queueImportFiles,
  reimportFile,
  startImportScan,
} from "./imports-api";
import type { ImportScan, ScanResult } from "./imports-api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

function session(overrides: Partial<ImportScan> = {}): ImportScan {
  return {
    id: "scan/1",
    path: "/downloads",
    status: "running",
    phase: "discovering",
    total_files: 0,
    processed_files: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    results: [],
    next_offset: 0,
    ...overrides,
  };
}

describe("async import scans", () => {
  it("cancels a session with DELETE and forwards the request abort signal", async () => {
    const cancelled = session({ status: "cancelled" });
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(Response.json({ data: cancelled }));
    const signal = new AbortController().signal;
    expect(await cancelImportScan("scan/1", signal)).toEqual(cancelled);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/imports/scans/scan%2F1",
      expect.objectContaining({
        method: "DELETE",
        signal,
        credentials: "include",
      }),
    );
  });

  it("surfaces rejected cancellation requests", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      Response.json({ error: "Cancellation unavailable" }, { status: 503 }),
    );
    await expect(
      cancelImportScan("scan-1", new AbortController().signal),
    ).rejects.toThrow("Cancellation unavailable");
  });

  it("surfaces start failure without polling or falling back to the legacy API", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        Response.json({ error: "Scan capacity reached" }, { status: 503 }),
      );
    const onProgress = vi.fn();
    await expect(
      pollImportScan({
        path: "/downloads",
        signal: new AbortController().signal,
        onProgress,
      }),
    ).rejects.toThrow("Scan capacity reached");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(onProgress).not.toHaveBeenCalled();
  });

  it("returns 202 acceptance immediately without waiting for results", async () => {
    const accepted = session();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        Response.json({ data: accepted }, { status: 202 }),
      );
    const signal = new AbortController().signal;
    expect(await startImportScan("/downloads", signal)).toEqual(accepted);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/imports/scans",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ path: "/downloads" }),
        signal,
      }),
    );
  });

  it("polls progress at one second intervals and stops when completed", async () => {
    vi.useFakeTimers();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        Response.json({ data: session() }, { status: 202 }),
      )
      .mockResolvedValueOnce(
        Response.json({ data: session({ total_files: 5 }) }),
      )
      .mockResolvedValueOnce(
        Response.json({ data: session({ status: "completed" }) }),
      );
    const onProgress = vi.fn();
    const signal = new AbortController().signal;
    const pending = pollImportScan({ path: "/downloads", signal, onProgress });
    await vi.advanceTimersByTimeAsync(0);
    expect(onProgress).toHaveBeenLastCalledWith(
      expect.objectContaining({
        status: "running",
        total_files: 5,
      }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(999);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    expect((await pending).status).toBe("completed");
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/v1/imports/scans/scan%2F1?offset=0&limit=1000",
      expect.objectContaining({ signal }),
    );
  });

  it.each(["completed", "failed", "cancelled"] as const)(
    "drains more than 1000 results immediately even when %s",
    async (status) => {
      vi.useFakeTimers();
      const results = Array.from(
        { length: 1001 },
        (_, index) =>
          ({
            file_path: `/downloads/${index}.flac`,
          }) as ScanResult,
      );
      const first = session({
        status,
        phase: "matching",
        total_files: 1001,
        processed_files: 1001,
        results: results.slice(0, 1000),
        next_offset: 1000,
        error: status === "failed" ? "Hashing failed" : undefined,
      });
      const last = {
        ...first,
        results: results.slice(1000),
        next_offset: 1001,
      };
      const fetchMock = vi
        .spyOn(globalThis, "fetch")
        .mockResolvedValueOnce(
          Response.json({ data: session() }, { status: 202 }),
        )
        .mockResolvedValueOnce(Response.json({ data: first }))
        .mockResolvedValueOnce(Response.json({ data: last }));
      const pages: ImportScan[] = [];
      expect(
        await pollImportScan({
          path: "/downloads",
          signal: new AbortController().signal,
          onProgress: (page) => pages.push(page),
        }),
      ).toEqual(last);
      expect(pages.flatMap((page) => page.results)).toEqual(results);
      expect(fetchMock).toHaveBeenLastCalledWith(
        "/api/v1/imports/scans/scan%2F1?offset=1000&limit=1000",
        expect.anything(),
      );
      expect(vi.getTimerCount()).toBe(0);
    },
  );

  it.each([404, 500])(
    "surfaces a %s poll failure without restarting or retrying",
    async (status) => {
      vi.useFakeTimers();
      const fetchMock = vi
        .spyOn(globalThis, "fetch")
        .mockResolvedValueOnce(
          Response.json({ data: session() }, { status: 202 }),
        )
        .mockResolvedValueOnce(
          Response.json({ data: session({ total_files: 1 }) }),
        )
        .mockResolvedValueOnce(
          Response.json({ error: "Scan unavailable" }, { status }),
        );
      const pending = pollImportScan({
        path: "/downloads",
        signal: new AbortController().signal,
        onProgress: vi.fn(),
      });
      const rejected = expect(pending).rejects.toThrow(
        status === 404 ? /not found or expired \(404\)/ : /Scan unavailable/,
      );
      await vi.advanceTimersByTimeAsync(1000);
      await rejected;
      await vi.advanceTimersByTimeAsync(5000);
      expect(fetchMock).toHaveBeenCalledTimes(3);
      expect(
        fetchMock.mock.calls.filter(([, init]) => init?.method === "POST"),
      ).toHaveLength(1);
    },
  );

  it("aborts the polling delay without another request", async () => {
    vi.useFakeTimers();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        Response.json({ data: session() }, { status: 202 }),
      )
      .mockResolvedValueOnce(Response.json({ data: session() }));
    const controller = new AbortController();
    const pending = pollImportScan({
      path: "/downloads",
      signal: controller.signal,
      onProgress: vi.fn(),
    });
    const rejected = expect(pending).rejects.toMatchObject({
      name: "AbortError",
    });
    await vi.advanceTimersByTimeAsync(0);
    controller.abort();
    await rejected;
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("stops rather than looping forever if a result cursor fails to advance", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        Response.json({ data: session() }, { status: 202 }),
      )
      .mockResolvedValueOnce(
        Response.json({
          data: session({
            status: "completed",
            processed_files: 1,
            next_offset: 0,
          }),
        }),
      );
    await expect(
      pollImportScan({
        path: "/downloads",
        signal: new AbortController().signal,
        onProgress: vi.fn(),
      }),
    ).rejects.toThrow("cursor did not advance");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("passes the abort signal to an in-flight page request", async () => {
    const controller = new AbortController();
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
      expect(init?.signal).toBe(controller.signal);
      return new Promise((_resolve, reject) => {
        init?.signal?.addEventListener(
          "abort",
          () => reject(init.signal?.reason),
          { once: true },
        );
      });
    });
    const pending = fetchImportScan("scan-1", 0, controller.signal);
    controller.abort();
    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
  });
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
