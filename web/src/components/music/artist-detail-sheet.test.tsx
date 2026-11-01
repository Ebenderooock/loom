import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ArtistDetailSheet } from "@/components/music/artist-detail-sheet";
import type { Artist } from "@/lib/music-api";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    ...props
  }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a {...props}>{children}</a>
  ),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const artist: Artist = {
  id: "artist-1",
  mbid: "mbid-1",
  name: "Linkin Park",
  monitoring_status: "monitored",
  created_at: "",
  updated_at: "",
};

const album = {
  id: "album-1",
  mbid: "release-group-1",
  artist_id: "artist-1",
  title: "Hybrid Theory",
  album_type: "Album",
  release_date: "2000-10-24",
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
      <ArtistDetailSheet
        artist={artist}
        open
        onOpenChange={vi.fn()}
        onDelete={vi.fn()}
      />
    </QueryClientProvider>,
  );
}

describe("ArtistDetailSheet", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/artists/artist-1") && !init?.method) {
        return json({
          ...artist,
          stats: {
            albumCount: 1,
            monitoredAlbumCount: 1,
            trackCount: 2,
            trackFileCount: 1,
            missingTrackCount: 1,
          },
          albums: [album],
        });
      }
      if (url.endsWith("/api/v1/albums/album-1") && !init?.method) {
        return json({
          ...album,
          tracks: [
            {
              id: "track-1",
              album_id: "album-1",
              title: "Papercut",
              track_number: 1,
              disc_number: 1,
              monitored: true,
              has_file: true,
            },
            {
              id: "track-2",
              album_id: "album-1",
              title: "One Step Closer",
              track_number: 2,
              disc_number: 1,
              monitored: true,
              has_file: false,
            },
          ],
        });
      }
      if (url.endsWith("/api/v1/artists/artist-1/files")) {
        return json([
          {
            id: "file-1",
            track_id: "track-1",
            album_id: "album-1",
            file_path: "/music/Linkin Park/Hybrid Theory/01 Papercut.flac",
            size: 2048,
            quality: "FLAC",
            date_added: "2024-01-01T00:00:00Z",
            album_title: "Hybrid Theory",
            track_title: "Papercut",
          },
        ]);
      }
      if (url.endsWith("/api/v1/artists/artist-1/history")) {
        return json([
          {
            id: "h-1",
            type: "import",
            status: "imported",
            album_id: "album-1",
            album_title: "Hybrid Theory",
            dest_path: "/music/Linkin Park/Hybrid Theory/01 Papercut.flac",
            date: "2024-01-01T00:00:00Z",
          },
        ]);
      }
      if (
        url.endsWith("/api/v1/music/audio-quality-profiles") ||
        url.endsWith("/api/v1/music/metadata-profiles")
      ) {
        return json([{ id: "p-1", name: "Standard" }]);
      }
      if (url.includes("/tracks/") && init?.method === "PUT") {
        return json({ id: "track-2", album_id: "album-1", monitored: false });
      }
      return json({ ...album, monitored: false });
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("shows artist catalogue and per-track availability on demand", async () => {
    const user = userEvent.setup();
    renderSheet();

    expect(await screen.findByText("Albums and tracks")).toBeInTheDocument();
    expect(screen.getAllByText("Linkin Park")).toHaveLength(2);
    expect(await screen.findByText("Hybrid Theory")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Show tracks" }));

    expect(await screen.findByText("Papercut")).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();
    expect(screen.getAllByText("Missing")).toHaveLength(2);
    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).endsWith("/api/v1/albums/album-1"),
      ),
    ).toBe(true);
  });

  it("updates album monitoring through the existing endpoint", async () => {
    const user = userEvent.setup();
    renderSheet();

    await screen.findByText("Hybrid Theory");
    await user.click(screen.getByRole("button", { name: "Unmonitor album" }));

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([url, init]) =>
            String(url).endsWith("/api/v1/albums/album-1/monitoring") &&
            init?.method === "PUT",
        ),
      ).toBe(true),
    );
    const request = fetchMock.mock.calls.find(
      ([url, init]) =>
        String(url).endsWith("/api/v1/albums/album-1/monitoring") &&
        init?.method === "PUT",
    );
    expect(JSON.parse(String(request?.[1]?.body))).toEqual({
      monitored: false,
    });
  });

  it("toggles monitoring for a single track", async () => {
    const user = userEvent.setup();
    renderSheet();

    await screen.findByText("Hybrid Theory");
    await user.click(screen.getByRole("button", { name: "Show tracks" }));
    await user.click(
      await screen.findByRole("button", { name: "Unmonitor One Step Closer" }),
    );

    await waitFor(() => {
      const request = fetchMock.mock.calls.find(
        ([url, init]) =>
          String(url).endsWith(
            "/api/v1/albums/album-1/tracks/track-2/monitoring",
          ) && init?.method === "PUT",
      );
      expect(JSON.parse(String(request?.[1]?.body))).toEqual({
        monitored: false,
      });
    });
  });

  it("loads files and history only when their sections are opened", async () => {
    const user = userEvent.setup();
    renderSheet();

    await screen.findByText("Hybrid Theory");
    const requested = (suffix: string) =>
      fetchMock.mock.calls.some(([url]) => String(url).endsWith(suffix));
    expect(requested("/artists/artist-1/files")).toBe(false);
    expect(requested("/artists/artist-1/history")).toBe(false);

    await user.click(screen.getByRole("button", { name: /Files/ }));
    expect(await screen.findByText("Papercut")).toBeInTheDocument();
    expect(screen.getByText("FLAC")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "History" }));
    expect(await screen.findByText("imported")).toBeInTheDocument();
    expect(requested("/artists/artist-1/history")).toBe(true);
  });

  it("opens the edit dialog", async () => {
    const user = userEvent.setup();
    renderSheet();

    await screen.findByText("Hybrid Theory");
    await user.click(screen.getByRole("button", { name: "Edit" }));
    expect(
      await screen.findByRole("heading", { name: "Edit Linkin Park" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Monitored")).toBeInTheDocument();
  });
});
