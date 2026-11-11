import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ImportManager } from "./import-manager";
import type { ImportJob, ScanResult } from "@/lib/imports-api";

afterEach(() => vi.restoreAllMocks());

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
    if (url === "/api/v1/imports/scan") {
      return Response.json({ data: results });
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
  render(
    <QueryClientProvider client={client}>
      <ImportManager />
    </QueryClientProvider>,
  );
  return { batches, client };
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
