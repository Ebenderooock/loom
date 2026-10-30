import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DiscoverPage } from "../discover";
import type {
  CollectionResponse,
  CollectionsResponse,
  RecommendationsResponse,
} from "@/lib/discovery-api";

const auth = vi.hoisted(() => ({ user: { id: 1 } }));
vi.mock("@/hooks/use-auth", () => ({ useAuth: () => auth }));

const generated = {
  warnings: [],
  generated_at: "2026-10-28T12:00:00Z",
  stale: false,
};
const movie = {
  tmdb_id: 2,
  media_type: "movie" as const,
  title: "Another adventure",
  year: 2020,
  poster_path: "https://image.tmdb.org/t/p/w342/poster.jpg",
  rating: 7.5,
  popularity: 30,
  reasons: ["You have 2 movies starring Ryan Reynolds"],
  source: "tmdb",
  collection_id: 10,
};
const collection = {
  id: 10,
  name: "Adventure collection",
  total: 4,
  in_library: 2,
  missing: 1,
  upcoming: 1,
};

let recommendations: RecommendationsResponse;
let collections: CollectionsResponse;
let detail: CollectionResponse;
let providerError = false;
let dismissalError = false;
let librariesEmpty = false;
let addError = false;
let savedMovie: unknown;
let savedSeries: unknown;
let requests: string[];
let writes: string[];
let clients: QueryClient[];

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return {
    ...render(
      <QueryClientProvider client={client}>
        <DiscoverPage />
      </QueryClientProvider>,
    ),
    client,
  };
}

beforeEach(() => {
  auth.user = { id: 1 };
  recommendations = { ...generated, items: [movie] };
  collections = { ...generated, items: [collection] };
  detail = {
    ...generated,
    collection,
    items: [
      {
        ...movie,
        tmdb_id: 1,
        title: "Original adventure",
        in_library: true,
        has_file: true,
        monitored: true,
      },
      {
        ...movie,
        tmdb_id: 3,
        title: "Missing file",
        in_library: true,
        has_file: false,
        monitored: true,
      },
      {
        ...movie,
        tmdb_id: 5,
        title: "Missing adventure",
        release_date: "2020-01-01",
        in_library: false,
        has_file: false,
        monitored: false,
      },
      {
        ...movie,
        tmdb_id: 4,
        title: "Future adventure",
        release_date: "2099-01-01",
        in_library: false,
        has_file: false,
        monitored: false,
      },
    ],
  };
  providerError = false;
  dismissalError = false;
  librariesEmpty = false;
  addError = false;
  savedMovie = undefined;
  savedSeries = undefined;
  requests = [];
  writes = [];
  clients = [];

  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    requests.push(url);
    expect(init?.credentials).toBe("include");
    if (init?.method === "POST") writes.push(url);
    if (url.startsWith("/api/v1/discover/recommendations")) {
      if (providerError)
        return new Response("TMDB access is not configured", { status: 503 });
      if (url.includes("media_type=series")) {
        return Response.json({
          ...generated,
          items: [
            {
              ...movie,
              media_type: "series",
              title: "Another show",
              collection_id: undefined,
            },
          ],
        });
      }
      return Response.json(recommendations);
    }
    if (url === "/api/v1/discover/collections")
      return Response.json(collections);
    if (url === "/api/v1/discover/collections/10") return Response.json(detail);
    if (url === "/api/v1/discover/dismissals") {
      if (dismissalError)
        return new Response("Could not save dismissal", { status: 500 });
      expect(JSON.parse(String(init?.body))).toEqual({
        tmdb_id: 2,
        media_type: "movie",
      });
      recommendations = { ...recommendations, items: [] };
      return new Response(null, { status: 204 });
    }
    if (url === "/api/v1/libraries") {
      return Response.json({
        data: librariesEmpty
          ? []
          : [
              {
                id: "movies",
                name: "Movies",
                media_type: "movie",
                quality_profile_id: "hd",
              },
              {
                id: "shows",
                name: "Shows",
                media_type: "series",
                quality_profile_id: "hd",
              },
            ],
      });
    }
    if (url === "/api/v1/quality-profiles")
      return Response.json({ data: [{ id: "hd", name: "HD" }] });
    if (url === "/api/v1/media-info/preferences")
      return Response.json({ default_quality_profile_id: "hd" });
    if (url.startsWith("/api/v1/discover/movie/")) {
      const id = url.split("/").at(-1);
      return Response.json({
        title: {
          tmdb_id: id,
          title: id === "2" ? "Another adventure" : "Another show",
          overview: "Full synopsis",
          poster_path: "/poster.jpg",
          rating: 7.5,
          release_date: "2020-01-01",
          first_air_date: "2020-01-01",
          genres: ["Adventure"],
        },
        credits: { cast: [], crew: [] },
        credits_error: "",
      });
    }
    if (url.startsWith("/api/v1/discover/series/")) {
      const id = url.split("/").at(-1);
      return Response.json({
        title: {
          tmdb_id: id,
          title: "Another show",
          overview: "Full synopsis",
          poster_path: "/poster.jpg",
          rating: 7.5,
          first_air_date: "2020-01-01",
          genres: ["Adventure"],
        },
        credits: { cast: [], crew: [] },
        credits_error: "",
      });
    }
    if (url.startsWith("/api/v1/movies?")) return Response.json([]);
    if (url === "/api/v1/series" && init?.method !== "POST")
      return Response.json([]);
    if (url === "/api/v1/movies" && init?.method === "POST") {
      if (addError) return new Response("Movie already added", { status: 409 });
      savedMovie = JSON.parse(String(init.body));
      recommendations = { ...recommendations, items: [] };
      return Response.json({ id: "movie-2" }, { status: 201 });
    }
    if (url === "/api/v1/series" && init?.method === "POST") {
      savedSeries = JSON.parse(String(init.body));
      return Response.json({ id: "show-2" }, { status: 201 });
    }
    if (url.startsWith("/api/v1/import-lists/discover"))
      return Response.json({ data: [] });
    throw new Error(`Unexpected request: ${url}`);
  });
});

afterEach(() => {
  clients.forEach((client) => client.clear());
  vi.restoreAllMocks();
});

describe("Discover library expansion", () => {
  it.each(["Movies", "TV shows"])(
    "can cancel a suggested %s addition without a write",
    async (mediaType) => {
      const user = userEvent.setup();
      renderPage();
      await screen.findByRole("heading", { name: movie.title });
      if (mediaType === "TV shows") {
        await user.click(screen.getByRole("tab", { name: mediaType }));
        await screen.findByRole("heading", { name: "Another show" });
      }
      await user.click(screen.getByRole("button", { name: "Add to library" }));
      const dialog = await screen.findByRole("dialog");
      await user.click(within(dialog).getByRole("button", { name: "Close" }));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(writes).toEqual([]);
    },
  );

  it("does not reuse another user's personalized recommendation cache", async () => {
    const view = renderPage();
    await screen.findByRole("heading", { name: movie.title });
    auth.user = { id: 2 };
    recommendations = {
      ...generated,
      items: [
        { ...movie, tmdb_id: 99, title: "A different user's suggestion" },
      ],
    };
    view.rerender(
      <QueryClientProvider client={view.client}>
        <DiscoverPage />
      </QueryClientProvider>,
    );
    await screen.findByRole("heading", {
      name: "A different user's suggestion",
    });
    expect(
      screen.queryByRole("heading", { name: movie.title }),
    ).not.toBeInTheDocument();
    expect(
      requests.filter((url) =>
        url.startsWith("/api/v1/discover/recommendations"),
      ),
    ).toHaveLength(2);
  });

  it("shows explainable suggestions without fetching Import Lists or writing to the library", async () => {
    renderPage();
    expect(await screen.findByText("Another adventure")).toBeInTheDocument();
    expect(
      screen.getByText("You have 2 movies starring Ryan Reynolds"),
    ).toBeInTheDocument();
    expect(requests.some((url) => url.includes("import-lists"))).toBe(false);
    expect(writes).toEqual([]);
  });

  it("switches media types and explicitly opts into upcoming suggestions", async () => {
    renderPage();
    const user = userEvent.setup();
    await screen.findByText("Another adventure");
    await user.click(screen.getByRole("tab", { name: "TV shows" }));
    expect(await screen.findByText("Another show")).toBeInTheDocument();
    await user.click(
      screen.getByRole("checkbox", { name: "Include upcoming titles" }),
    );
    await waitFor(() =>
      expect(requests).toContain(
        "/api/v1/discover/recommendations?media_type=series&include_upcoming=true",
      ),
    );
    expect(writes).toEqual([]);
  });

  it("adds a suggested movie through the existing dialog with immediate search off by default", async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Add to library" }),
    );
    const dialog = await screen.findByRole("dialog");
    const search = await within(dialog).findByRole("checkbox", {
      name: "Search after adding",
    });
    expect(search).not.toBeChecked();
    const poster = within(dialog).getByRole("presentation");
    expect(poster.getAttribute("src")).toBe(
      "https://image.tmdb.org/t/p/w342/poster.jpg",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Add to library" }),
    );
    await waitFor(() =>
      expect(savedMovie).toMatchObject({
        tmdb_id: "2",
        title: "Another adventure",
        library_id: "movies",
        quality_profile_id: "hd",
        search: false,
      }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("adds TV suggestions through the series API, not the movie API", async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "TV shows" }));
    await user.click(
      await screen.findByRole("button", { name: "Add to library" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      await within(dialog).findByRole("button", { name: "Add to library" }),
    );
    await waitFor(() =>
      expect(savedSeries).toMatchObject({
        tmdbId: "2",
        libraryId: "shows",
        qualityProfileId: "hd",
        search: false,
      }),
    );
    expect(savedMovie).toBeUndefined();
  });

  it("keeps an add failure visible without claiming success", async () => {
    addError = true;
    renderPage();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Add to library" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      await within(dialog).findByRole("button", { name: "Add to library" }),
    );
    expect(
      await within(dialog).findByText("Movie already added"),
    ).toBeInTheDocument();
    expect(savedMovie).toBeUndefined();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("explains missing library setup in the dialog", async () => {
    librariesEmpty = true;
    renderPage();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Add to library" }),
    );
    expect(
      await screen.findByText(/Create a movie library in Settings/),
    ).toBeInTheDocument();
    expect(writes).toEqual([]);
  });

  it("dismisses a suggestion only after its dismissal is saved", async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Dismiss Another adventure" }),
    );
    expect(
      await screen.findByText("No new suggestions yet"),
    ).toBeInTheDocument();
    expect(writes).toEqual(["/api/v1/discover/dismissals"]);
  });

  it("preserves suggestions when dismissal fails", async () => {
    dismissalError = true;
    renderPage();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Dismiss Another adventure" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not save dismissal",
    );
    expect(screen.getByText("Another adventure")).toBeInTheDocument();
  });

  it("distinguishes collection membership, missing files, and upcoming entries", async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "Collections" }));
    const open = await screen.findByRole("button", {
      name: "View Adventure collection",
    });
    expect(within(open).getByText("2 of 4 added to Loom")).toBeInTheDocument();
    expect(
      within(open).getByText("1 missing / 1 upcoming"),
    ).toBeInTheDocument();
    await user.click(open);
    expect(await screen.findByText("Original adventure")).toBeInTheDocument();
    expect(screen.getByText(/No imported file recorded/)).toBeInTheDocument();
    expect(screen.getByText("Upcoming")).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: "Add to library" }),
    ).toHaveLength(2);
    expect(writes).toEqual([]);
    await user.click(screen.getByRole("button", { name: "Back" }));
    expect(
      await screen.findByRole("button", { name: "View Adventure collection" }),
    ).toBeInTheDocument();
  });

  it("opens a recommendation's related collection", async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "View collection" }),
    );
    expect(await screen.findByText("Original adventure")).toBeInTheDocument();
  });

  it("shows provider errors with retry rather than an empty success state", async () => {
    providerError = true;
    renderPage();
    const user = userEvent.setup();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "TMDB access is not configured",
    );
    expect(
      screen.queryByText("No new suggestions yet"),
    ).not.toBeInTheDocument();
    providerError = false;
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("Another adventure")).toBeInTheDocument();
  });

  it("labels cached data and incomplete sources", async () => {
    recommendations = {
      ...recommendations,
      stale: true,
      warnings: ["Actor source unavailable"],
    };
    renderPage();
    expect(
      await screen.findByText(/Showing cached TMDB data/),
    ).toBeInTheDocument();
    expect(screen.getByText("Actor source unavailable")).toBeInTheDocument();
  });

  it("preserves existing Import List discovery as an optional section", async () => {
    renderPage();
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "Import Lists" }));
    expect(
      await screen.findByText("Nothing to discover yet"),
    ).toBeInTheDocument();
    expect(requests).toContain(
      "/api/v1/import-lists/discover?media_type=movie",
    );
    expect(writes).toEqual([]);
  });
});
