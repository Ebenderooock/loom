import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ImportManager } from "./import-manager";
import type { ScanResult } from "@/lib/imports-api";

vi.mock("sonner", () => ({ toast: { success: vi.fn() } }));
const fetchMock = vi.fn<typeof fetch>();
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status });
const file: ScanResult = {
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

async function scan(result = file) {
  fetchMock.mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/scan")) return json({ data: [result] });
    if (url === "/api/v1/albums")
      return json([{ id: "album-1", title: "An Album" }]);
    if (url === "/api/v1/albums/album-1")
      return json({
        id: "album-1",
        tracks: [
          { id: "track-3", title: "A Song", disc_number: 2, track_number: 3 },
        ],
      });
    return json({ status: "accepted" }, 202);
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ImportManager />
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.type(
    screen.getByRole("textbox", { name: "Folder to scan" }),
    "/downloads",
  );
  await user.click(screen.getByRole("button", { name: "Scan" }));
  await screen.findByText("song.flac");
  return user;
}

describe("ImportManager", () => {
  beforeEach(() => vi.stubGlobal("fetch", fetchMock));
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("renders audio tags and queues a quality-safe import with the track ID", async () => {
    const user = await scan();
    expect(screen.getByLabelText("Audio file")).toBeInTheDocument();
    expect(
      screen.getByText("An Artist · An Album · Disc 2 · Track 3"),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Queue import for song.flac" }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "File import queued, not completed",
    );
    expect(screen.getByText("Queued (not completed)")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Import All Matched" }),
    ).toBeDisabled();
    const request = fetchMock.mock.calls.find(([url]) =>
      String(url).endsWith("/reimport"),
    );
    expect(JSON.parse(String(request?.[1]?.body))).toEqual({
      media_type: "music",
      media_id: "track-3",
      source_path: file.file_path,
      conflict_policy: "replace_if_better",
    });
  });

  it("does not turn a queued manual match into an importable match", async () => {
    const user = await scan({
      ...file,
      suggested_action: "no_match",
      matched_media_id: undefined,
      matched_media: undefined,
    });
    await user.click(screen.getByRole("button", { name: "Match" }));
    await user.click(await screen.findByRole("button", { name: /An Album/ }));
    await user.click(
      await screen.findByRole("button", { name: /Disc 2 · Track 3.*A Song/ }),
    );
    await user.click(screen.getByRole("button", { name: "Import Match" }));
    expect(
      await screen.findByText("Queued (not completed)"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Import All Matched" }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Queue import for song.flac" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Match" }),
    ).not.toBeInTheDocument();
  });

  it("reports folder import acceptance as queued rather than completed", async () => {
    const user = await scan();
    await user.click(
      screen.getByRole("button", { name: "Import All Matched" }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Folder import queued, not completed",
    );
  });

  it.each([
    ["Import All Matched", "folder"],
    ["Queue import for song.flac", "file"],
  ])("shows %s request errors", async (button, kind) => {
    const user = await scan();
    fetchMock.mockResolvedValue(json({ error: "Library is unavailable" }, 400));
    await user.click(screen.getByRole("button", { name: button }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      `Could not queue ${kind} import: Library is unavailable`,
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: button })).toBeEnabled(),
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Queued (not completed)"),
    ).not.toBeInTheDocument();
  });
});
