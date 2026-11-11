import { afterEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ImportManager } from "./import-manager";
import type { ImportJob, ImportScan, ScanResult } from "@/lib/imports-api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function scanSession(overrides: Partial<ImportScan> = {}): ImportScan {
  return {
    id: "scan-1",
    path: "/downloads",
    status: "running",
    phase: "discovering",
    total_files: 0,
    processed_files: 0,
    results: [],
    next_offset: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const audioFile: ScanResult = {
  file_path: "/downloads/song.flac",
  file_size: 1000,
  media_type: "music",
  detected_title: "A Song",
  detected_artist: "An Artist",
  detected_album: "An Album",
  detected_track: 3,
  detected_disc: 2,
  matched_media: "A Song",
  matched_media_id: "track-3",
  suggested_action: "import",
  confidence: 1,
};

function scanResult(index: number, action = "import"): ScanResult {
  return {
    file_path: `/downloads/Season/Show.S01E${index}.mkv`,
    file_size: 1_000,
    detected_title: "Show",
    detected_season: 1,
    detected_episode: index,
    matched_media: "Show",
    matched_media_id: `episode-${index}`,
    media_type: "episode",
    confidence: 1,
    suggested_action: action,
  };
}

function setup(
  results: ScanResult[],
  initialJobs: ImportJob[] = [],
  rejectedPath?: string,
  getPage?: (offset: number, signal: AbortSignal) => Promise<Response>,
  cancelScan?: () => Promise<Response>,
) {
  let jobs = initialJobs;
  const batches: string[][] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (url === "/api/v1/imports/jobs") {
      return Response.json({ data: jobs });
    }
    if (url.endsWith("/retry")) {
      return Response.json(
        { error: "awaiting safety review approval" },
        { status: 409 },
      );
    }
    if (url.startsWith("/api/v1/imports/jobs/")) {
      return Response.json({
        data: jobs.find((job) => url.endsWith(`/${job.id}`)),
      });
    }
    if (url === "/api/v1/imports/scans") {
      return Response.json({ data: scanSession() }, { status: 202 });
    }
    if (url === "/api/v1/imports/scans/scan-1" && init?.method === "DELETE") {
      return cancelScan
        ? cancelScan()
        : Response.json({ data: scanSession({ status: "cancelled" }) });
    }
    if (url.startsWith("/api/v1/imports/scans/scan-1?")) {
      const offset = Number(
        new URL(url, "http://localhost").searchParams.get("offset"),
      );
      if (getPage) return getPage(offset, init?.signal as AbortSignal);
      const page = results.slice(offset, offset + 1000);
      return Response.json({
        data: scanSession({
          status: "completed",
          phase: "matching",
          total_files: results.length,
          processed_files: results.length,
          results: page,
          next_offset: offset + page.length,
        }),
      });
    }
    if (url === "/api/v1/albums") {
      return Response.json([{ id: "album-1", title: "An Album" }]);
    }
    if (url === "/api/v1/albums/album-1") {
      return Response.json({
        id: "album-1",
        tracks: [
          { id: "track-3", title: "A Song", disc_number: 2, track_number: 3 },
        ],
      });
    }
    if (url === "/api/v1/imports/manual-match") {
      return Response.json({ status: "accepted" }, { status: 202 });
    }
    if (url === "/api/v1/imports/batch") {
      const body = JSON.parse(String(init?.body)) as { paths: string[] };
      batches.push(body.paths);
      jobs = body.paths
        .filter((path) => path !== rejectedPath)
        .map((path, index) => ({
          id: `job-${index}`,
          source_path: path,
          status: "queued",
          attempts: 0,
          total_files: 1,
          processed_files: 0,
          imported_files: 0,
          failed_files: 0,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        }));
      return Response.json({
        data: jobs,
        errors: rejectedPath
          ? [{ path: rejectedPath, error: "Source file is unavailable" }]
          : [],
      });
    }
    throw new Error(`Unexpected request: ${url}`);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <ImportManager />
    </QueryClientProvider>,
  );
  return { batches, client, ...view };
}

async function scan() {
  const user = userEvent.setup();
  await user.type(
    screen.getByRole("textbox", { name: "Download folder" }),
    "/downloads",
  );
  await user.click(screen.getByRole("button", { name: /^Scan$/ }));
  await screen.findByText(/Scan Results/);
  return user;
}

describe("ImportManager", () => {
  it("cancels the scan, drains unread cancelled pages, and preserves queued imports and selection", async () => {
    let cancelled = false;
    const getPage = vi.fn(async (offset: number) => {
      if (!cancelled)
        return Response.json({
          data: scanSession({
            phase: "matching",
            total_files: 4,
            processed_files: 2,
            results: offset === 0 ? [audioFile, scanResult(1)] : [],
            next_offset: 2,
          }),
        });
      return Response.json({
        data: scanSession({
          status: "cancelled",
          phase: "matching",
          total_files: 4,
          processed_files: 4,
          results: [scanResult(offset)],
          next_offset: offset + 1,
        }),
      });
    });
    const { batches } = setup([], [], undefined, getPage, async () => {
      cancelled = true;
      // DELETE metadata is not the cursor page the UI needs to drain.
      return Response.json({
        data: scanSession({ status: "cancelled", processed_files: 4 }),
      });
    });
    const user = await scan();
    await user.click(
      screen.getByRole("checkbox", { name: "Select Show.S01E1.mkv" }),
    );
    await user.click(screen.getByRole("button", { name: "Import song.flac" }));
    expect(
      await screen.findByText(/1 file queued, not completed/),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel scan" }));
    expect(
      await screen.findByText(
        "Scan cancelled. Partial results are still available.",
        {},
        { timeout: 3000 },
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("Show.S01E2.mkv")).toBeInTheDocument();
    expect(screen.getByText("Show.S01E3.mkv")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Select song.flac" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Import selected (1)" }),
    ).toBeEnabled();
    expect(screen.getByRole("button", { name: /^Scan$/ })).toBeEnabled();
    expect(
      screen.queryByRole("button", { name: "Cancel scan" }),
    ).not.toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/imports/scans/scan-1",
      expect.objectContaining({
        method: "DELETE",
        signal: expect.any(AbortSignal),
      }),
    );
    expect(getPage.mock.calls.slice(-2).map(([offset]) => offset)).toEqual([
      2, 3,
    ]);
    expect(batches).toEqual([[audioFile.file_path]]);
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.filter(([url]) => url === "/api/v1/imports/scans"),
    ).toHaveLength(1);
  });

  it("shows cancellation request errors and allows retry without discarding partial results", async () => {
    let cancelled = false;
    const cancelScan = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({ error: "Cancellation unavailable" }, { status: 503 }),
      )
      .mockImplementationOnce(async () => {
        cancelled = true;
        return Response.json({ data: scanSession({ status: "cancelled" }) });
      });
    setup(
      [],
      [],
      undefined,
      async (offset) =>
        Response.json({
          data: scanSession({
            status: cancelled ? "cancelled" : "running",
            phase: "matching",
            total_files: 2,
            processed_files: 1,
            next_offset: 1,
            results: offset === 0 ? [audioFile] : [],
          }),
        }),
      cancelScan,
    );
    const user = await scan();
    await user.click(screen.getByRole("button", { name: "Cancel scan" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not cancel scan: Cancellation unavailable",
    );
    expect(screen.getByRole("button", { name: "Cancel scan" })).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "Import song.flac" }),
    ).toBeEnabled();
    expect(
      screen.queryByText(
        "Scan cancelled. Partial results are still available.",
      ),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel scan" }));
    expect(
      await screen.findByText(
        "Scan cancelled. Partial results are still available.",
        {},
        { timeout: 3000 },
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(cancelScan).toHaveBeenCalledTimes(2);
  });

  it("can cancel after polling failed and resumes only the existing session cursor", async () => {
    const getPage = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          data: scanSession({
            phase: "matching",
            total_files: 3,
            processed_files: 1,
            results: [audioFile],
            next_offset: 1,
          }),
        }),
      )
      .mockResolvedValueOnce(
        Response.json({ error: "Poll interrupted" }, { status: 500 }),
      )
      .mockResolvedValueOnce(
        Response.json({
          data: scanSession({
            status: "cancelled",
            phase: "matching",
            total_files: 3,
            processed_files: 2,
            results: [scanResult(2)],
            next_offset: 2,
          }),
        }),
      );
    setup([], [], undefined, getPage);
    const user = await scan();
    expect(
      await screen.findByRole("alert", {}, { timeout: 3000 }),
    ).toHaveTextContent("Poll interrupted");
    await user.click(screen.getByRole("button", { name: "Cancel scan" }));
    expect(
      await screen.findByText(
        "Scan cancelled. Partial results are still available.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText("song.flac")).toBeInTheDocument();
    expect(screen.getByText("Show.S01E2.mkv")).toBeInTheDocument();
    expect(getPage.mock.calls.map(([offset]) => offset)).toEqual([0, 1, 1]);
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.filter(([url]) => url === "/api/v1/imports/scans"),
    ).toHaveLength(1);
  });

  it("keeps partial results queueable after a poll error and rescans only on user request", async () => {
    const getPage = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          data: scanSession({
            phase: "matching",
            total_files: 2,
            processed_files: 1,
            results: [audioFile],
            next_offset: 1,
          }),
        }),
      )
      .mockResolvedValueOnce(
        Response.json({ error: "Polling interrupted" }, { status: 500 }),
      )
      .mockResolvedValueOnce(
        Response.json({
          data: scanSession({
            status: "completed",
            phase: "matching",
            total_files: 1,
            processed_files: 1,
            results: [scanResult(1)],
            next_offset: 1,
          }),
        }),
      );
    const { batches } = setup([], [], undefined, getPage);
    const user = await scan();
    await user.click(
      screen.getByRole("checkbox", { name: "Select song.flac" }),
    );
    expect(
      await screen.findByRole("alert", {}, { timeout: 3000 }),
    ).toHaveTextContent("Polling interrupted");
    expect(
      screen.getByRole("button", { name: "Import selected (1)" }),
    ).toBeEnabled();
    expect(getPage).toHaveBeenCalledTimes(2);
    await user.click(
      screen.getByRole("button", { name: "Import selected (1)" }),
    );
    await waitFor(() => expect(batches).toEqual([[audioFile.file_path]]));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /^Scan$/ })).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: /^Scan$/ }));
    expect(await screen.findByText("Show.S01E1.mkv")).toBeInTheDocument();
    expect(screen.queryByText("song.flac")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Import selected (0)" }),
    ).toBeDisabled();
    expect(getPage).toHaveBeenCalledTimes(3);
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.filter(([url]) => url === "/api/v1/imports/scans"),
    ).toHaveLength(2);
  });

  it("shows discovery progress, prevents Enter duplicates, and surfaces an expired poll without restarting", async () => {
    let rejectPoll: ((response: Response) => void) | undefined;
    const laterPoll = new Promise<Response>((resolve) => {
      rejectPoll = resolve;
    });
    const getPage = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          data: scanSession({
            total_files: 12,
            current_file: "/downloads/new.flac",
          }),
        }),
      )
      .mockReturnValue(laterPoll);
    setup([], [], undefined, getPage);
    const user = userEvent.setup();
    await user.type(
      screen.getByRole("textbox", { name: "Download folder" }),
      "/downloads{Enter}{Enter}",
    );
    expect(
      await screen.findByText("Discovering files: 12 found · 0 processed"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Current file: /downloads/new.flac"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Scan$/ })).toBeDisabled();
    await user.type(
      screen.getByRole("textbox", { name: "Download folder" }),
      "{Enter}",
    );
    await act(async () =>
      rejectPoll?.(Response.json({ error: "expired" }, { status: 404 })),
    );
    expect(
      await screen.findByRole("alert", {}, { timeout: 3000 }),
    ).toHaveTextContent("not found or expired (404)");
    expect(screen.getByRole("button", { name: /^Scan$/ })).toBeEnabled();
    expect(globalThis.fetch).toHaveBeenCalledTimes(4); // jobs, one start, two GETs
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.filter(([url]) => url === "/api/v1/imports/scans"),
    ).toHaveLength(1);
    expect(
      vi
        .mocked(globalThis.fetch)
        .mock.calls.some(([url]) => url === "/api/v1/imports/scan"),
    ).toBe(false);
  });

  it("keeps queued/manual-match edits and selection as partial results arrive", async () => {
    const unmatched = {
      ...audioFile,
      suggested_action: "no_match",
      matched_media_id: undefined,
      matched_media: undefined,
    };
    const firstResults = [unmatched, scanResult(1), scanResult(2)];
    let finish: ((response: Response) => void) | undefined;
    const finalPage = new Promise<Response>((resolve) => {
      finish = resolve;
    });
    const getPage = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          data: scanSession({
            phase: "matching",
            total_files: 4,
            processed_files: 3,
            current_file: "/downloads/last.mkv",
            results: firstResults,
            next_offset: 3,
          }),
        }),
      )
      .mockReturnValue(finalPage);
    const { batches } = setup([], [], undefined, getPage);
    const user = await scan();
    expect(
      screen.getByText("Matching files: 3 / 4 processed"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Scan completed/)).not.toBeInTheDocument();
    await user.click(
      screen.getByRole("checkbox", { name: "Select Show.S01E2.mkv" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Import Show.S01E1.mkv" }),
    );
    expect(
      await screen.findByText(/1 file queued, not completed/),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Match" }));
    await user.click(await screen.findByRole("button", { name: /An Album/ }));
    await user.click(
      await screen.findByRole("button", { name: /Disc 2 · Track 3.*A Song/ }),
    );
    await user.click(screen.getByRole("button", { name: "Import Match" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Match" }),
      ).not.toBeInTheDocument(),
    );
    await act(async () =>
      finish?.(
        Response.json({
          data: scanSession({
            status: "completed",
            phase: "matching",
            total_files: 4,
            processed_files: 4,
            results: [scanResult(3)],
            next_offset: 4,
          }),
        }),
      ),
    );
    expect(
      await screen.findByText(/Scan completed: 4 \/ 4/, {}, { timeout: 3000 }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Select Show.S01E1.mkv" }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Match" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Import all matched (2)" }),
    ).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "Import selected (1)" }),
    ).toBeEnabled();
    await user.click(
      screen.getByRole("checkbox", { name: "Select Show.S01E3.mkv" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Import selected (2)" }),
    );
    await waitFor(() =>
      expect(batches).toEqual([
        [scanResult(1).file_path],
        [scanResult(2).file_path, scanResult(3).file_path],
      ]),
    );
  });

  it("drains more than 1000 results before declaring completion and selects across display pages", async () => {
    const results = Array.from({ length: 1001 }, (_, index) =>
      scanResult(index + 1),
    );
    let finish: ((response: Response) => void) | undefined;
    const lastPage = new Promise<Response>((resolve) => {
      finish = resolve;
    });
    const getPage = vi
      .fn()
      .mockImplementationOnce(async () =>
        Response.json({
          data: scanSession({
            status: "completed",
            phase: "matching",
            total_files: 1001,
            processed_files: 1001,
            results: results.slice(0, 1000),
            next_offset: 1000,
          }),
        }),
      )
      .mockImplementation((offset) => {
        expect(offset).toBe(1000);
        return lastPage;
      });
    const { batches } = setup([], [], undefined, getPage);
    const user = await scan();
    expect(
      screen.getByText("Loading remaining scan results..."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Scan completed/)).not.toBeInTheDocument();
    await user.click(
      screen.getByRole("checkbox", { name: "Select Show.S01E1.mkv" }),
    );
    await user.click(screen.getByRole("button", { name: /^Next$/ }));
    await user.click(
      screen.getByRole("checkbox", { name: "Select Show.S01E51.mkv" }),
    );
    await act(async () =>
      finish?.(
        Response.json({
          data: scanSession({
            status: "completed",
            phase: "matching",
            total_files: 1001,
            processed_files: 1001,
            results: results.slice(1000),
            next_offset: 1001,
          }),
        }),
      ),
    );
    expect(
      await screen.findByText(/Scan completed: 1001 \/ 1001/),
    ).toBeInTheDocument();
    expect(screen.getByText("1001 files · Page 2 of 21")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Import selected (2)" }),
    );
    await waitFor(() =>
      expect(batches).toEqual([
        [results[0]?.file_path, results[50]?.file_path],
      ]),
    );
    expect(getPage).toHaveBeenCalledTimes(2);
  });

  it.each(["failed", "cancelled"] as const)(
    "retains queueable partial results when a scan is %s",
    async (status) => {
      setup([], [], undefined, async () =>
        Response.json({
          data: scanSession({
            status,
            phase: "matching",
            total_files: 5,
            processed_files: 1,
            results: [audioFile],
            next_offset: 1,
            error: status === "failed" ? "Hashing failed" : undefined,
          }),
        }),
      );
      const user = await scan();
      expect(
        await screen.findByText(
          status === "failed"
            ? "Scan failed. Partial results are still available."
            : "Scan cancelled. Partial results are still available.",
        ),
      ).toBeInTheDocument();
      if (status === "failed")
        expect(screen.getByRole("alert")).toHaveTextContent("Hashing failed");
      await user.click(
        screen.getByRole("button", { name: "Import song.flac" }),
      );
      expect(
        await screen.findByText(/1 file queued, not completed/),
      ).toBeInTheDocument();
    },
  );

  it("aborts an in-flight poll on unmount", async () => {
    let signal: AbortSignal | undefined;
    const getPage = vi.fn(async (_offset, requestSignal: AbortSignal) => {
      signal = requestSignal;
      return new Promise<Response>((_resolve, reject) => {
        requestSignal.addEventListener(
          "abort",
          () => reject(requestSignal.reason),
          { once: true },
        );
      });
    });
    const { unmount } = setup([], [], undefined, getPage);
    const user = userEvent.setup();
    await user.type(
      screen.getByRole("textbox", { name: "Download folder" }),
      "/downloads",
    );
    await user.click(screen.getByRole("button", { name: /^Scan$/ }));
    await waitFor(() => expect(getPage).toHaveBeenCalledTimes(1));
    await act(async () => unmount());
    expect(signal?.aborted).toBe(true);
    expect(getPage).toHaveBeenCalledTimes(1);
  });

  it("preserves audio tag rendering and queues the track source without claiming completion", async () => {
    const { batches } = setup([audioFile]);
    const user = await scan();
    expect(screen.getByLabelText("Audio file")).toBeInTheDocument();
    expect(
      screen.getByText("An Artist · An Album · Disc 2 · Track 3"),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Import song.flac" }));
    await waitFor(() => expect(batches).toEqual([[audioFile.file_path]]));
    expect(
      await screen.findByText(/1 file queued, not completed/),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Import all matched (0)" }),
    ).toBeDisabled();
  });

  it("keeps a manually matched music track queued instead of offering a duplicate import", async () => {
    setup([
      {
        ...audioFile,
        suggested_action: "no_match",
        matched_media_id: undefined,
        matched_media: undefined,
      },
    ]);
    const user = await scan();
    await user.click(screen.getByRole("button", { name: "Match" }));
    await user.click(await screen.findByRole("button", { name: /An Album/ }));
    await user.click(
      await screen.findByRole("button", { name: /Disc 2 · Track 3.*A Song/ }),
    );
    await user.click(screen.getByRole("button", { name: "Import Match" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Match" }),
      ).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: "Import all matched (0)" }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Import song.flac" }),
    ).not.toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/v1/imports/manual-match",
      expect.objectContaining({
        body: JSON.stringify({
          path: audioFile.file_path,
          media_type: "music",
          media_id: "track-3",
        }),
      }),
    );
  });

  it("explains safety review and surfaces a refused retry without marking it imported", async () => {
    setup(
      [],
      [
        {
          id: "review-job",
          source_path: "/downloads/Flagged",
          status: "pending_review",
          attempts: 0,
          total_files: 1,
          processed_files: 0,
          imported_files: 0,
          failed_files: 0,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
    );
    const retry = await screen.findByRole("button", {
      name: "Retry /downloads/Flagged after approval",
    });
    expect(
      screen.getByText(/Approve this import in Activity/),
    ).toBeInTheDocument();
    await userEvent.setup().click(retry);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "awaiting safety review approval",
    );
    expect(screen.getByText("Needs review")).toBeInTheDocument();
  });

  it("only marks accepted files queued when a bulk submission partially fails", async () => {
    const results = [1, 2, 3].map((n) => scanResult(n));
    setup(results, [], results[2]?.file_path);
    const user = await scan();
    await user.click(
      screen.getByRole("checkbox", {
        name: "Select all eligible files on this page",
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: "Import selected (3)",
      }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Source file is unavailable",
    );
    expect(
      screen.getByRole("checkbox", {
        name: "Select Show.S01E3.mkv",
      }),
    ).toBeEnabled();
    expect(
      screen.getByRole("button", {
        name: "Import selected (1)",
      }),
    ).toBeEnabled();
    expect(
      screen.getByRole("checkbox", {
        name: "Select Show.S01E1.mkv",
      }),
    ).toBeDisabled();
  });

  it("queues multiple selected files in one request and prevents duplicate queueing", async () => {
    const { batches } = setup([1, 2, 3, 4].map((n) => scanResult(n)));
    const user = await scan();
    for (const index of [1, 2, 3]) {
      await user.click(
        screen.getByRole("checkbox", {
          name: `Select Show.S01E${index}.mkv`,
        }),
      );
    }
    await user.click(
      screen.getByRole("button", { name: "Import selected (3)" }),
    );
    await waitFor(() =>
      expect(batches).toEqual([[1, 2, 3].map((n) => scanResult(n).file_path)]),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("checkbox", { name: "Select Show.S01E1.mkv" }),
      ).toBeDisabled(),
    );
    expect(
      screen.getByRole("button", { name: "Import all matched (1)" }),
    ).toBeEnabled();
  });

  it("imports all matched files across pages, excluding successful and unmatched files", async () => {
    const eligible = Array.from({ length: 55 }, (_, i) => scanResult(i + 1));
    setup([
      ...eligible,
      scanResult(56, "already_imported"),
      { ...scanResult(57, "no_match"), matched_media_id: undefined },
    ]);
    const user = await scan();
    expect(screen.queryByText("Show.S01E56.mkv")).not.toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Import all matched (55)" }),
    );
    await waitFor(() =>
      expect(globalThis.fetch).toHaveBeenCalledWith(
        "/api/v1/imports/batch",
        expect.objectContaining({
          body: JSON.stringify({ paths: eligible.map((r) => r.file_path) }),
        }),
      ),
    );
  });

  it("labels metadata-only history separately and only queues it when explicitly shown", async () => {
    const result = scanResult(1, "previously_imported");
    const { batches } = setup([result, scanResult(2)]);
    const user = await scan();
    expect(screen.queryByText("Show.S01E1.mkv")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Import all matched (1)" }),
    ).toBeEnabled();
    await user.click(
      screen.getByRole("checkbox", { name: "Show already imported files" }),
    );
    expect(screen.getByText("Previously imported")).toHaveAttribute(
      "title",
      expect.stringContaining("Contents will be verified"),
    );
    await user.click(
      screen.getByRole("button", { name: "Import Show.S01E1.mkv" }),
    );
    await waitFor(() => expect(batches).toEqual([[result.file_path]]));
  });

  it("shows failed file progress and scheduled retry reasons after reopening", async () => {
    setup(
      [],
      [
        {
          id: "retry-job",
          source_path: "/downloads/Season",
          status: "retry_pending",
          attempts: 1,
          total_files: 22,
          processed_files: 22,
          imported_files: 20,
          failed_files: 2,
          error: "Waiting for torrent client to release files",
          next_retry_at: "2026-01-01T00:01:00Z",
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:30Z",
          files: [
            {
              id: "failed-episode",
              job_id: "retry-job",
              source_path: "/downloads/Season/Show.S01E21.mkv",
              status: "failed",
              attempts: 1,
              error: "File is held by torrent client",
              updated_at: "2026-01-01T00:00:30Z",
            },
          ],
        },
      ],
    );
    expect(await screen.findByText("Retry scheduled")).toBeInTheDocument();
    expect(
      screen.getByText("Waiting for torrent client to release files"),
    ).toBeInTheDocument();
    expect(screen.getByText(/22 \/ 22 files processed/)).toHaveTextContent(
      "20 imported",
    );
    expect(screen.getByText(/Next check:/)).toBeInTheDocument();
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "100",
    );
    await userEvent.setup().click(
      screen.getByRole("button", {
        name: "Show file outcomes",
      }),
    );
    expect(
      await screen.findByText("File is held by torrent client"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("/downloads/Season/Show.S01E21.mkv"),
    ).toBeInTheDocument();
  });

  it("does not hide a changed or missing file behind an older successful job", async () => {
    const result = scanResult(1);
    setup(
      [result],
      [
        {
          id: "old-success",
          source_path: result.file_path,
          status: "imported",
          attempts: 1,
          total_files: 1,
          processed_files: 1,
          imported_files: 1,
          failed_files: 0,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
    );
    await scan();
    expect(
      screen.getByRole("checkbox", {
        name: "Select Show.S01E1.mkv",
      }),
    ).toBeEnabled();
    expect(
      screen.getByRole("button", {
        name: "Import all matched (1)",
      }),
    ).toBeEnabled();
  });
});
