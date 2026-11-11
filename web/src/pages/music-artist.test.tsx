import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MusicArtistPage } from "@/pages/music-artist";
import type { Album, Artist } from "@/lib/music-api";

vi.mock("@tanstack/react-router", () => ({
  useParams: () => ({ artistId: "artist-1" }),
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/music">{children}</a>
  ),
}));
vi.mock("@/hooks/use-page-header", () => ({ useSetPageHeader: vi.fn() }));
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

const album: Album = {
  id: "album-1",
  mbid: "group-1",
  artist_id: "artist-1",
  title: "Test Record",
  monitored: true,
};
const overview = `${"An artist biography with a long history. ".repeat(20)}The end.`;
const artist: Artist = {
  id: "artist-1",
  mbid: "artist-mbid",
  name: "Test Artist",
  overview,
  monitoring_status: "monitored",
  albums: [album],
  created_at: "",
  updated_at: "",
};
const fullAlbum: Album = {
  ...album,
  tracks: [
    {
      id: "track-1",
      album_id: album.id,
      title: "First Track",
      disc_number: 2,
      track_number: 1,
      duration_ms: 125000,
      has_file: true,
      monitored: true,
    },
  ],
};
const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
const fetchMock = vi.fn<typeof fetch>();
function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MusicArtistPage />
    </QueryClientProvider>,
  );
}

describe("MusicArtistPage", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith("/artists/artist-1")) return json(artist);
      if (url.endsWith("/albums/album-1")) return json(fullAlbum);
      if (
        url.endsWith("/tracks/track-1/monitoring") &&
        init?.method === "PUT"
      ) {
        return json({ ...fullAlbum.tracks![0], monitored: false });
      }
      return json([]);
    });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("shows a compact biography and lazily loads the same track controls as the artist drawer", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Test Record");
    expect(screen.queryByText(overview)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Read more" }));
    expect(screen.getByText(overview)).toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).endsWith("/albums/album-1"),
      ),
    ).toBe(false);
    await user.click(screen.getByRole("button", { name: "Show tracks" }));
    expect(await screen.findByText("First Track")).toBeInTheDocument();
    expect(screen.getByText("2.1")).toBeInTheDocument();
    expect(screen.getByText("2:05")).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Unmonitor First Track" }),
    );
    await waitFor(() => {
      const request = fetchMock.mock.calls.find(
        ([url, init]) =>
          String(url).endsWith("/tracks/track-1/monitoring") &&
          init?.method === "PUT",
      );
      expect(JSON.parse(String(request?.[1]?.body))).toEqual({
        monitored: false,
      });
    });
  });

  it("keeps album details and editions accessible separately from track expansion", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Test Record");
    await user.click(
      screen.getByRole("button", { name: "Album details for Test Record" }),
    );
    expect(await screen.findByText("Edition and tracks")).toBeInTheDocument();
    expect(await screen.findByText("First Track")).toBeInTheDocument();
  });

  it("uses the movie/TV action hierarchy on the standalone page", async () => {
    renderPage();
    await screen.findByText("Test Record");
    const actions = screen.getByRole("group", { name: "Artist actions" });
    expect(within(actions).getByRole("button", { name: "Search" })).toHaveClass(
      "h-8",
      "text-xs",
    );
    expect(
      within(actions).getByRole("button", { name: "Refresh metadata" }),
    ).toHaveClass("h-8", "w-8");
    expect(
      within(actions).getByRole("button", { name: "Remove artist" }),
    ).toHaveClass("ml-auto");
  });

  it("does not leave an empty track expansion after successful no-editions metadata", async () => {
    const user = userEvent.setup();
    const normalFetch = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input, init) =>
      String(input).endsWith("/albums/album-1")
        ? Promise.resolve(
            json({
              ...album,
              releases: [],
              tracks: [],
              releases_fetched_at: "2024-01-01",
            }),
          )
        : normalFetch(input, init),
    );
    renderPage();
    await screen.findByText("Test Record");
    await user.click(screen.getByRole("button", { name: "Show tracks" }));
    expect(
      await screen.findByRole("button", { name: "No tracks available" }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Hide tracks" }),
    ).not.toBeInTheDocument();
  });
});
