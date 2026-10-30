import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TitleContent, TitleSearchDialog } from "./title-experience";
import { titleKey, type TitleMedia } from "@/lib/title-api";

const api = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock("@/lib/fetch", () => ({ apiFetch: api.fetch }));

let writes: { url: string; body: Record<string, unknown> }[];
let failAdd = false;
let failTracking = false;
let members: Record<string, unknown>[];

const movie = {
  tmdb_id: "42",
  title: "Test title",
  overview: "A complete synopsis",
  rating: 8.5,
  runtime: 120,
  genres: ["Drama"],
};
const libraries = [
  {
    id: "movies",
    name: "Movies",
    path: "/movies",
    media_type: "movie",
    quality_profile_id: "library-profile",
    monitor_on_add: true,
  },
  {
    id: "series",
    name: "TV",
    path: "/tv",
    media_type: "series",
    quality_profile_id: "library-profile",
    monitor_on_add: true,
  },
  {
    id: "music",
    name: "Music",
    path: "/music",
    media_type: "music",
    quality_profile_id: "audio",
    monitor_on_add: true,
  },
  {
    id: "source",
    name: "Source movies",
    path: "/source",
    media_type: "movie",
    quality_profile_id: "library-profile",
    monitor_on_add: true,
  },
];
const profiles = [
  { id: "library-profile", name: "Library quality" },
  { id: "source-profile", name: "Source quality" },
  { id: "global", name: "Global quality" },
];

beforeEach(() => {
  writes = [];
  members = [];
  failAdd = false;
  failTracking = false;
  api.fetch.mockReset();
  api.fetch.mockImplementation(async (url: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      writes.push({ url, body: JSON.parse(String(init.body)) });
      if (url.endsWith("/track"))
        return new Response(failTracking ? "Tracking unavailable" : "{}", {
          status: failTracking ? 503 : 200,
        });
      if (failAdd) return new Response("Unable to add", { status: 500 });
      return Response.json({ id: "added" });
    }
    if (url === "/api/v1/libraries") return Response.json({ data: libraries });
    if (url === "/api/v1/quality-profiles")
      return Response.json({ data: profiles });
    if (url === "/api/v1/media-info/preferences")
      return Response.json({ default_quality_profile_id: "global" });
    if (url === "/api/v1/music/audio-quality-profiles")
      return Response.json([{ id: "audio", name: "Lossless" }]);
    if (url === "/api/v1/music/metadata-profiles")
      return Response.json([{ id: "metadata", name: "Studio albums" }]);
    if (url.startsWith("/api/v1/discover/artist/"))
      return Response.json({
        title: {
          mbid: "artist-id",
          name: "Test artist",
          country: "NZ",
          genres: ["Jazz"],
        },
        albums: [
          {
            mbid: "album-id",
            title: "Test album",
            type: "Album",
            release_date: "2020-01-01",
          },
        ],
      });
    if (url === "/api/v1/discover/album/album-id")
      return Response.json({
        album: { mbid: "album-id", title: "Test album", genres: ["Jazz"] },
        releases: [{ mbid: "release-id", title: "Edition" }],
      });
    if (url === "/api/v1/discover/release/release-id")
      return Response.json({
        mbid: "release-id",
        tracks: [
          {
            track_id: "track",
            title: "Test track",
            track_number: 1,
            disc_number: 1,
          },
        ],
      });
    if (url.startsWith("/api/v1/discover/"))
      return Response.json({
        title: movie,
        credits: {
          cast: [
            { id: 1, name: "Cast member", role: "Lead", profile_path: "" },
          ],
          crew: [
            {
              id: 2,
              name: "Director",
              role: "Director",
              department: "Directing",
            },
          ],
        },
      });
    if (url.includes("/lookup?") || url.includes("/search?"))
      return Response.json([
        { tmdb_id: "42", title: "Test title", poster_path: "/poster.jpg" },
      ]);
    if (url.startsWith("/api/v1/movies?"))
      return Response.json({ data: members });
    if (url === "/api/v1/series" || url === "/api/v1/artists")
      return Response.json(members);
    throw new Error(`Unexpected request: ${url}`);
  });
});
afterEach(cleanup);

function mount(content: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const rendered = render(
    <QueryClientProvider client={client}>{content}</QueryClientProvider>,
  );
  return { ...rendered, client };
}
function identity(media: TitleMedia = "movie") {
  return { media, id: "42", title: "Test title" };
}

describe("shared title details and settings", () => {
  it("loads synopsis, score, cast and crew without a library write", async () => {
    mount(<TitleContent identity={identity()} />);
    expect(await screen.findByText("A complete synopsis")).toBeInTheDocument();
    expect(screen.getByText("8.5")).toBeInTheDocument();
    expect(screen.getByText("Cast member")).toBeInTheDocument();
    expect(
      screen.getByText("Director", { selector: "p.text-xs" }),
    ).toBeInTheDocument();
    expect(writes).toEqual([]);
    expect(screen.queryByLabelText("Library")).not.toBeInTheDocument();
  });

  it("uses editable visible source defaults and tracks only after persisting", async () => {
    const onAdded = vi.fn();
    mount(
      <TitleContent
        identity={identity()}
        initialView="add"
        defaults={{
          library_path: "/source",
          quality_profile_id: "source-profile",
          monitor_type: "none",
          search_on_add: false,
        }}
        sourceName="Watchlist"
        sourceItemId="source-item"
        onAdded={onAdded}
      />,
    );
    expect(await screen.findByLabelText("Library")).toHaveValue("source");
    await waitFor(() =>
      expect(screen.getByLabelText("Quality profile")).toHaveValue(
        "source-profile",
      ),
    );
    expect(screen.getByLabelText("Monitor for downloads")).not.toBeChecked();
    expect(screen.getByLabelText("Search after adding")).not.toBeChecked();
    fireEvent.change(screen.getByLabelText("Library"), {
      target: { value: "movies" },
    });
    expect(screen.getByLabelText("Quality profile")).toHaveValue(
      "library-profile",
    );
    fireEvent.click(screen.getByRole("button", { name: "View full details" }));
    fireEvent.click(screen.getByRole("button", { name: "Add to library" }));
    expect(screen.getByLabelText("Library")).toHaveValue("movies");
    fireEvent.click(screen.getByRole("button", { name: "Add to library" }));
    await waitFor(() => expect(onAdded).toHaveBeenCalledOnce());
    expect(writes.map((w) => w.url)).toEqual([
      "/api/v1/movies",
      "/api/v1/import-lists/discover/track",
    ]);
    expect(writes[0]!.body).toMatchObject({
      tmdb_id: "42",
      library_id: "movies",
      quality_profile_id: "library-profile",
      monitoring_status: "unmonitored",
      search: false,
      overview: movie.overview,
    });
  });

  it("keeps errors editable, allows retry, and prevents duplicate submissions", async () => {
    failAdd = true;
    const onAdded = vi.fn();
    mount(
      <TitleContent
        identity={identity()}
        initialView="add"
        onAdded={onAdded}
      />,
    );
    const add = await screen.findByRole("button", { name: "Add to library" });
    await waitFor(() => expect(add).toBeEnabled());
    fireEvent.click(add);
    fireEvent.click(add);
    expect(await screen.findByRole("alert")).toHaveTextContent("Unable to add");
    expect(writes).toHaveLength(1);
    expect(onAdded).not.toHaveBeenCalled();
    failAdd = false;
    fireEvent.click(add);
    await waitFor(() => expect(onAdded).toHaveBeenCalledOnce());
    expect(writes).toHaveLength(2);
  });

  it("retries failed source tracking without posting another library add", async () => {
    failTracking = true;
    mount(
      <TitleContent
        identity={identity()}
        initialView="add"
        sourceItemId="item"
      />,
    );
    const add = await screen.findByRole("button", { name: "Add to library" });
    await waitFor(() => expect(add).toBeEnabled());
    fireEvent.click(add);
    const retry = await screen.findByRole("button", {
      name: "Retry source tracking",
    });
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Title is in your library",
    );
    failTracking = false;
    fireEvent.click(retry);
    await waitFor(() => expect(writes).toHaveLength(3));
    expect(writes.map((w) => w.url)).toEqual([
      "/api/v1/movies",
      "/api/v1/import-lists/discover/track",
      "/api/v1/import-lists/discover/track",
    ]);
  });

  it("prevents a second add if another entry point added the provider identity", async () => {
    mount(<TitleContent identity={identity()} initialView="add" />);
    const add = await screen.findByRole("button", { name: "Add to library" });
    await waitFor(() => expect(add).toBeEnabled());
    members = [{ id: "existing", tmdbId: "42" }];
    fireEvent.click(add);
    await screen.findByRole("button", { name: "In library" });
    expect(writes).toEqual([]);
  });

  it("preserves series monitoring, type and season-folder settings", async () => {
    mount(<TitleContent identity={identity("series")} initialView="add" />);
    const monitor = await screen.findByLabelText("Monitor");
    fireEvent.change(monitor, { target: { value: "future" } });
    fireEvent.change(screen.getByLabelText("Series type"), {
      target: { value: "anime" },
    });
    fireEvent.click(screen.getByLabelText("Season folders"));
    const add = screen.getByRole("button", { name: "Add to library" });
    await waitFor(() => expect(add).toBeEnabled());
    fireEvent.click(add);
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      url: "/api/v1/series",
      body: {
        tmdbId: "42",
        libraryId: "series",
        monitoringStatus: "future",
        seriesType: "anime",
        seasonFolder: false,
      },
    });
  });

  it("provides artist albums, editions and tracks read-only, with music settings on add", async () => {
    mount(
      <TitleContent
        identity={{ media: "artist", id: "artist-id", title: "Test artist" }}
      />,
    );
    expect(await screen.findByText("Test album")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: /View editions and tracks/ }),
    );
    expect(await screen.findByText("Test track")).toBeInTheDocument();
    expect(writes).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Add to library" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Metadata profile")).toHaveValue("metadata"),
    );
    const add = screen.getByRole("button", { name: "Add to library" });
    await waitFor(() => expect(add).toBeEnabled());
    fireEvent.click(add);
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      url: "/api/v1/artists",
      body: {
        mbid: "artist-id",
        libraryId: "music",
        qualityProfileId: "audio",
        metadataProfileId: "metadata",
      },
    });
  });

  it("shows a request action, never direct-add controls, in the request context", async () => {
    const request = vi.fn();
    mount(
      <TitleContent
        identity={identity()}
        requestAction={<button onClick={request}>Request</button>}
      />,
    );
    await screen.findByText("A complete synopsis");
    expect(
      screen.queryByRole("button", { name: "Add to library" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Library")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Request" }));
    expect(request).toHaveBeenCalledOnce();
    expect(writes).toEqual([]);
    expect(
      api.fetch.mock.calls.some(([url]) => url === "/api/v1/libraries"),
    ).toBe(false);
  });

  it("scopes detail cache keys by media and provider", () => {
    expect(titleKey(identity())).not.toEqual(titleKey(identity("series")));
    expect(titleKey(identity())).not.toEqual(
      titleKey({ ...identity(), provider: "tvdb" }),
    );
  });

  it("adds the resolved canonical identity instead of the source provider ID", async () => {
    mount(
      <TitleContent
        identity={{
          media: "movie",
          provider: "imdb",
          id: "tt123",
          title: "Test title",
        }}
        initialView="add"
      />,
    );
    const add = await screen.findByRole("button", { name: "Add to library" });
    await waitFor(() => expect(add).toBeEnabled());
    fireEvent.click(add);
    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]?.body.tmdb_id).toBe("42");
    expect(api.fetch).toHaveBeenCalledWith(
      "/api/v1/discover/movie/tt123?provider=imdb",
      expect.anything(),
    );
  });

  it("requires a matching library rather than silently saving invalid settings", async () => {
    const original = api.fetch.getMockImplementation()!;
    api.fetch.mockImplementation((url, init) =>
      url === "/api/v1/libraries"
        ? Promise.resolve(Response.json({ data: [] }))
        : original(url, init),
    );
    mount(<TitleContent identity={identity()} initialView="add" />);
    const add = await screen.findByRole("button", { name: "Add to library" });
    await screen.findByText(/Configure a movie library/);
    expect(add).toBeDisabled();
    expect(writes).toEqual([]);
  });

  it("falls back to the library profile when a source profile no longer exists", async () => {
    const original = api.fetch.getMockImplementation()!;
    api.fetch.mockImplementation((url, init) =>
      url === "/api/v1/quality-profiles"
        ? Promise.resolve(Response.json({ data: [profiles[2], profiles[0]] }))
        : original(url, init),
    );
    mount(
      <TitleContent
        identity={identity()}
        initialView="add"
        defaults={{ quality_profile_id: "deleted-profile" }}
      />,
    );
    await waitFor(() =>
      expect(screen.getByLabelText("Quality profile")).toHaveValue(
        "library-profile",
      ),
    );
    expect(writes).toEqual([]);
  });

  it("opens details from search title/artwork and the same settings directly from Add", async () => {
    mount(<TitleSearchDialog media="movie" open onOpenChange={vi.fn()} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Search titles" }), {
      target: { value: "Test" },
    });
    const title = await screen.findByRole("button", { name: "Test title" });
    fireEvent.click(title);
    await screen.findByText("A complete synopsis");
    expect(writes).toEqual([]);
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Back to results" }));
    fireEvent.click(await screen.findByRole("button", { name: "Add" }));
    await screen.findByLabelText("Library");
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(writes).toEqual([]);
  });
});
