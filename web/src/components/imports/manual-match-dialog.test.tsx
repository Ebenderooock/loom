import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ManualMatchDialog } from "./manual-match-dialog";
import type { ScanResult } from "@/lib/imports-api";

vi.mock("sonner", () => ({
  toast: { success: vi.fn() },
}));

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
const fetchMock = vi.fn<typeof fetch>();
const musicFile: ScanResult = {
  file_path: "/downloads/song.flac",
  file_size: 1000,
  detected_title: "A Song",
  media_type: "music",
  confidence: 0,
  suggested_action: "no_match",
};
const albums = [
  {
    id: "album-1",
    title: "Shared Record",
    artists: [{ name: "First Artist" }, { name: "Second Artist" }],
  },
  {
    id: "album-2",
    title: "Other Record",
    artists: [{ name: "Another Artist" }],
  },
];
const tracks = [
  { id: "track-1", title: "A Song", disc_number: 1, track_number: 1 },
  { id: "track-2", title: "A Song (reprise)", disc_number: 2, track_number: 1 },
];

function renderDialog(scanResult = musicFile) {
  const onMatched = vi.fn();
  const onOpenChange = vi.fn();
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={queryClient}>
      <ManualMatchDialog
        open
        scanResult={scanResult}
        onMatched={onMatched}
        onOpenChange={onOpenChange}
      />
    </QueryClientProvider>,
  );
  return { onMatched, onOpenChange };
}

async function selectTrack() {
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", {
      name: /Shared Record.*First Artist, Second Artist/,
    }),
  );
  expect(screen.getByRole("button", { name: "Import Match" })).toBeDisabled();
  await user.click(
    await screen.findByRole("button", { name: /Disc 2 · Track 1.*reprise/ }),
  );
  return user;
}

describe("ManualMatchDialog", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/api/v1/albums") return json(albums);
      if (url === "/api/v1/albums/album-1")
        return json({ ...albums[0], tracks });
      if (url === "/api/v1/albums/album-2")
        return json({ ...albums[1], tracks: [] });
      if (url.includes("/movies/search"))
        return json({ data: [{ id: "movie-1", title: "A Movie" }] });
      if (url.includes("/series?"))
        return json({ data: [{ id: "series-1", title: "A Series" }] });
      if (url.includes("/episodes"))
        return json({
          data: [{ id: "episode-1", title: "Pilot", episodeNumber: 1 }],
        });
      if (url.endsWith("/manual-match"))
        return json({ status: "accepted" }, 202);
      throw new Error(`Unexpected request: ${url}`);
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("autochooses Music without a match, filters local titles/credits, and submits a specific track ID", async () => {
    const { onMatched, onOpenChange } = renderDialog();
    const user = userEvent.setup();
    expect(screen.getByRole("tab", { name: "Music" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const input = screen.getByRole("textbox", { name: "Search local albums" });
    await screen.findByRole("button", { name: /Other Record/ });
    await user.type(input, "SECOND ARTIST");
    expect(
      screen.queryByRole("button", { name: /Other Record/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Shared Record/ }),
    ).toBeInTheDocument();
    await user.clear(input);
    await user.type(input, "shared");
    expect(
      screen.queryByRole("button", { name: /Other Record/ }),
    ).not.toBeInTheDocument();
    await selectTrack();
    await user.click(screen.getByRole("button", { name: "Import Match" }));

    await waitFor(() => expect(onMatched).toHaveBeenCalledOnce());
    expect(onOpenChange).toHaveBeenCalledWith(false);
    const request = fetchMock.mock.calls.find(([url]) =>
      String(url).endsWith("/manual-match"),
    );
    expect(request?.[1]?.method).toBe("POST");
    expect(JSON.parse(String(request?.[1]?.body))).toEqual({
      path: musicFile.file_path,
      media_type: "music",
      media_id: "track-2",
    });
    expect(
      fetchMock.mock.calls.filter(([url]) => String(url) === "/api/v1/albums"),
    ).toHaveLength(1);
  });

  it("shows a manual import rejection and keeps the dialog open to retry", async () => {
    const successfulFetch = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation(async (input, init) =>
      String(input).endsWith("/manual-match")
        ? json({ error: "No music library configured" }, 400)
        : successfulFetch(input, init),
    );
    const { onMatched, onOpenChange } = renderDialog();
    const user = await selectTrack();
    await user.click(screen.getByRole("button", { name: "Import Match" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "No music library configured",
    );
    expect(onMatched).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Import Match" })).toBeEnabled();
  });

  it.each(["albums", "tracks"])(
    "shows %s loading errors without allowing import",
    async (failure) => {
      const successfulFetch = fetchMock.getMockImplementation()!;
      fetchMock.mockImplementation(async (input, init) =>
        String(input) ===
        (failure === "albums" ? "/api/v1/albums" : "/api/v1/albums/album-1")
          ? json({ error: "Unavailable" }, 503)
          : successfulFetch(input, init),
      );
      renderDialog();
      if (failure === "tracks") {
        await userEvent.click(
          await screen.findByRole("button", { name: /Shared Record/ }),
        );
      }
      expect(await screen.findByRole("alert")).toHaveTextContent(
        `Could not load ${failure}`,
      );
      expect(
        screen.getByRole("button", { name: "Import Match" }),
      ).toBeDisabled();
    },
  );

  it("clears track selection when changing albums and handles an empty track list", async () => {
    renderDialog();
    const user = await selectTrack();
    await user.click(screen.getByRole("button", { name: "Change album" }));
    expect(screen.getByRole("button", { name: "Import Match" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: /Other Record/ }));
    expect(
      await screen.findByText("No tracks found for this album."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import Match" })).toBeDisabled();
  });

  it.each([
    { type: "movie", title: "A Movie", id: "movie-1" },
    { type: "episode", title: "A Series", id: "episode-1" },
  ])("preserves $type matching", async ({ type, title, id }) => {
    renderDialog({
      ...musicFile,
      media_type: type,
      detected_title: title,
      detected_season: type === "episode" ? 1 : undefined,
      detected_episode: type === "episode" ? 1 : undefined,
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Search library" }));
    await user.click(await screen.findByRole("button", { name: title }));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Import Match" }),
      ).toBeEnabled(),
    );
    await user.click(screen.getByRole("button", { name: "Import Match" }));
    await waitFor(() => {
      const request = fetchMock.mock.calls.find(([url]) =>
        String(url).endsWith("/manual-match"),
      );
      expect(JSON.parse(String(request?.[1]?.body))).toEqual({
        path: musicFile.file_path,
        media_type: type,
        media_id: id,
      });
    });
  });
});
