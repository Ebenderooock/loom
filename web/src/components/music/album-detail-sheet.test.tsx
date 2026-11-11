import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AlbumDetailSheet } from "@/components/music/album-detail-sheet";
import type { Album } from "@/lib/music-api";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const album: Album = {
  id: "album-1",
  mbid: "group-1",
  title: "A Shared Record",
  album_type: "Album",
  release_date: "2024-06-01",
  artists: [{ name: "First Artist" }, { name: "Second Artist" }],
  monitored: true,
};

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
const fetchMock = vi.fn<typeof fetch>();

function renderSheet() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AlbumDetailSheet album={album} open onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  );
}

describe("AlbumDetailSheet", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/albums/album-1") && !init?.method) {
        return json({
          ...album,
          releases: [
            {
              id: "edition-1",
              mbid: "release-1",
              album_id: "album-1",
              title: "A Shared Record",
              country: "GB",
              format: "CD",
              track_count: 2,
            },
          ],
          selected_release_id: "edition-1",
          tracks: [
            {
              id: "track-1",
              album_id: "album-1",
              title: "Opening Theme",
              track_number: 1,
              disc_number: 1,
              duration_ms: 205000,
              monitored: true,
              has_file: true,
            },
            {
              id: "track-2",
              album_id: "album-1",
              title: "Closing Theme",
              track_number: 2,
              disc_number: 1,
              monitored: true,
              has_file: false,
            },
          ],
        });
      }
      if (url.endsWith("/api/v1/albums/album-1/monitoring")) {
        return json({ ...album, monitored: false });
      }
      return json({});
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("shows credited artists, edition and track availability", async () => {
    renderSheet();
    expect(await screen.findAllByText("A Shared Record")).not.toHaveLength(0);
    expect(screen.getByText("First Artist, Second Artist")).toBeInTheDocument();
    expect(await screen.findByText("Opening Theme")).toBeInTheDocument();
    expect(
      screen.getByText("1 of 2 tracks available · 1 missing"),
    ).toBeInTheDocument();
    expect(screen.getByText("3:25")).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: "Album edition" }),
    ).toBeInTheDocument();
  });

  it("uses the album monitoring endpoint for standalone albums", async () => {
    const user = userEvent.setup();
    renderSheet();
    await screen.findAllByText("A Shared Record");
    await user.click(screen.getByRole("button", { name: "Unmonitor album" }));
    await waitFor(() => {
      const request = fetchMock.mock.calls.find(
        ([url, init]) =>
          String(url).endsWith("/api/v1/albums/album-1/monitoring") &&
          init?.method === "PUT",
      );
      expect(JSON.parse(String(request?.[1]?.body))).toEqual({
        monitored: false,
      });
    });
  });

  it("matches the movie/TV compact action hierarchy", async () => {
    renderSheet();
    const actions = screen.getByRole("group", { name: "Album actions" });
    expect(within(actions).getByRole("button", { name: "Search" })).toHaveClass(
      "h-8",
      "text-xs",
    );
    expect(
      within(actions).getByRole("button", { name: "Manual Search" }),
    ).toHaveClass("h-8", "text-xs");
    expect(
      within(actions).getByRole("button", { name: "Unmonitor album" }),
    ).toHaveClass("h-8", "w-8");
    await screen.findByText("Opening Theme");
  });

  it("retries metadata failures even when the API returns an empty stored album", async () => {
    const user = userEvent.setup();
    const normalFetch = fetchMock.getMockImplementation()!;
    let attempts = 0;
    fetchMock.mockImplementation((input, init) => {
      if (
        String(input).endsWith("/api/v1/albums/album-1") &&
        attempts++ === 0
      ) {
        return Promise.resolve(json({ ...album, tracks: [] }));
      }
      return normalFetch(input, init);
    });
    renderSheet();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Track metadata could not be loaded.",
    );
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Opening Theme")).toBeInTheDocument();
  });

  it("shows a definite empty state without asking users to repeatedly reopen the album", async () => {
    fetchMock.mockResolvedValue(
      json({
        ...album,
        releases_fetched_at: "2024-01-01",
      }),
    );
    renderSheet();
    expect(
      await screen.findByText(/No track list is available for this album/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Retry" }),
    ).not.toBeInTheDocument();
  });
});
