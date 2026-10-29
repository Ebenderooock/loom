import { expect, test, type Page } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockMovies,
  mockRoute,
  SAMPLE_LIBRARY,
  SAMPLE_MOVIE,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";

const externalMovie = {
  tmdb_id: "42",
  title: "Shared title",
  overview: "Full shared synopsis.",
  rating: 8.5,
  runtime: 120,
  genres: ["Drama"],
  poster_path: "/poster.jpg",
  release_date: "2025-01-01",
};
async function setup(page: Page) {
  await mockBaseApp(page);
  await mockMovies(page, []);
  await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
  await mockGet(page, "quality-profiles", { data: [SAMPLE_QUALITY_PROFILE] });
  await mockGet(page, "media-info/preferences", {});
  await mockGet(page, "features", {
    features: [{ key: "music", enabled: true }],
  });
  await mockGet(page, "movies/lookup*", [externalMovie]);
  await mockGet(page, "discover/movie/42*", {
    title: externalMovie,
    credits: {
      cast: [{ id: 7, name: "Cast member", role: "Lead", profile_path: "" }],
      crew: [],
    },
  });
  await page.route("https://image.tmdb.org/**", (route) =>
    route.fulfill({
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="100" height="150"><rect width="100" height="150" fill="#334155"/></svg>',
    }),
  );
}

test("search details and direct Add use one dialog, with keyboard focus restored", async ({
  page,
}) => {
  await setup(page);
  const writes: unknown[] = [];
  await mockRoute(page, "POST", "movies", async (route) => {
    writes.push(route.request().postDataJSON());
    await route.fulfill({ status: 201, json: { id: "shared-title" } });
  });
  await page.goto("/movies");
  const launch = page.getByRole("button", { name: "Add Movie" }).first();
  await launch.click();
  await page.getByRole("textbox", { name: "Search titles" }).fill("Shared");
  await page.getByRole("button", { name: "Shared title" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toHaveCount(1);
  await expect(dialog.getByText("Full shared synopsis.")).toBeVisible();
  await expect(dialog.getByText("Cast member")).toBeVisible();
  expect(writes).toEqual([]);
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(dialog.getByLabel("Library", { exact: true })).toHaveValue(
    "lib-1",
  );
  await dialog.getByRole("button", { name: "View full details" }).click();
  await expect(dialog.getByText("Full shared synopsis.")).toBeVisible();
  await dialog.getByRole("button", { name: "Back to results" }).click();
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Add to library", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatchObject({
    tmdb_id: "42",
    library_id: "lib-1",
    quality_profile_id: "qp-1",
    search: true,
  });
  await expect(launch).toBeFocused();
});

test("Discover artwork is read-only and Add shows source settings before tracking", async ({
  page,
}) => {
  await setup(page);
  const writes: string[] = [];
  await mockGet(page, "import-lists/discover*", {
    data: [
      {
        id: "source-item",
        list_id: "list",
        list_name: "Watchlist",
        tmdb_id: "42",
        title: "Shared title",
        media_type: "movie",
        in_library: false,
        defaults: {
          library_path: SAMPLE_LIBRARY.path,
          quality_profile_id: "qp-1",
          monitor_type: "none",
          search_on_add: false,
        },
      },
    ],
  });
  for (const endpoint of ["movies", "import-lists/discover/track"]) {
    await mockRoute(page, "POST", endpoint, async (route) => {
      writes.push(endpoint);
      await route.fulfill({ status: 200, json: {} });
    });
  }
  await page.goto("/discover");
  await page
    .getByRole("button", { name: "View details for Shared title" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Full shared synopsis.")).toBeVisible();
  expect(writes).toEqual([]);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(dialog.getByLabel("Monitor for downloads")).not.toBeChecked();
  await expect(dialog.getByLabel("Search after adding")).not.toBeChecked();
  await expect(dialog.getByText(/Settings from Watchlist/)).toBeVisible();
  await dialog
    .getByRole("button", { name: "Add to library", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  expect(writes).toEqual(["movies", "import-lists/discover/track"]);
  await expect(
    page.getByRole("button", { name: "Added", exact: true }),
  ).toBeVisible();
});

test("request details retain request semantics and never expose library add", async ({
  page,
}) => {
  await setup(page);
  const unlimited = { limit: 0, used: 0, remaining: -1, unlimited: true };
  await mockGet(page, "requests*", { data: [] });
  await mockGet(page, "requests/mine", { data: [] });
  await mockGet(page, "music/audio-quality-profiles", []);
  await mockGet(page, "requests/quota", {
    movie: unlimited,
    series: unlimited,
    music: unlimited,
    window_days: 7,
  });
  await mockGet(page, "requests/quota/config", {
    movie_limit: 0,
    series_limit: 0,
    music_limit: 0,
    window_days: 7,
  });
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST")
      writes.push(new URL(request.url()).pathname);
  });
  await mockRoute(page, "POST", "requests", async (route) =>
    route.fulfill({ status: 201, json: { id: "request", status: "pending" } }),
  );
  await page.goto("/requests");
  await page.getByPlaceholder("Search for a movie to request…").fill("Shared");
  await page
    .getByRole("button", { name: "View details for Shared title" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Full shared synopsis.")).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Add to library" }),
  ).toHaveCount(0);
  await expect(dialog.getByLabel("Library", { exact: true })).toHaveCount(0);
  expect(writes).toEqual([]);
  await dialog.getByRole("button", { name: "Request", exact: true }).click();
  await expect(
    dialog.getByRole("button", { name: "Requested", exact: true }),
  ).toBeDisabled();
  expect(writes).toEqual(["/api/v1/requests"]);
});

test("music uses shared details and preserves audio and metadata profiles", async ({
  page,
}) => {
  await setup(page);
  await mockGet(page, "libraries", {
    data: [
      {
        ...SAMPLE_LIBRARY,
        id: "music-lib",
        media_type: "music",
        quality_profile_id: "audio",
      },
    ],
  });
  await mockGet(page, "artists", []);
  await mockGet(page, "artists/lookup*", [
    { mbid: "artist-id", name: "Shared artist", already_added: false },
  ]);
  await mockGet(page, "discover/artist/artist-id", {
    title: { mbid: "artist-id", name: "Shared artist", country: "NZ" },
    albums: [
      { mbid: "album-id", title: "Album title", release_date: "2020-01-01" },
    ],
  });
  await mockGet(page, "music/audio-quality-profiles", [
    { id: "audio", name: "Lossless" },
  ]);
  await mockGet(page, "music/metadata-profiles", [
    { id: "metadata", name: "Albums" },
  ]);
  let payload: unknown;
  await mockRoute(page, "POST", "artists", async (route) => {
    payload = route.request().postDataJSON();
    await route.fulfill({ status: 201, json: { id: "artist" } });
  });
  await page.goto("/music");
  await page.getByRole("button", { name: "Add Artist" }).first().click();
  await page.getByRole("textbox", { name: "Search titles" }).fill("Shared");
  await page
    .getByRole("button", { name: "Shared artist", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Album title")).toBeVisible();
  expect(payload).toBeUndefined();
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(dialog.getByLabel("Quality profile")).toHaveValue("audio");
  await expect(dialog.getByLabel("Metadata profile")).toHaveValue("metadata");
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(dialog).toHaveCount(0);
  expect(payload).toMatchObject({
    mbid: "artist-id",
    qualityProfileId: "audio",
    metadataProfileId: "metadata",
    libraryId: "music-lib",
  });
});

test("filmography replaces the library sheet and preserves the selected tab and focus", async ({
  page,
}) => {
  await setup(page);
  await mockMovies(page, [SAMPLE_MOVIE]);
  await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
  await mockGet(page, "movies/mov-1/credits", {
    cast: [{ id: 7, name: "Cast member", role: "Lead" }],
    crew: [],
  });
  await mockGet(page, "movies/files/mov-1", []);
  await mockGet(page, "movies/mov-1/history", []);
  await mockGet(page, "discover/people/7", {
    person: { name: "Cast member", biography: "A performer biography." },
    credits: [
      { tmdb_id: 42, media_type: "tv", title: "Shared show", year: 2025 },
    ],
  });
  await mockGet(page, "series", { data: [] });
  await mockGet(page, "discover/series/42", {
    title: {
      tmdb_id: "42",
      title: "Shared show",
      overview: "Full show synopsis.",
      seasons: 3,
    },
    credits: { cast: [], crew: [] },
  });
  await page.goto("/movies");
  await page.locator("main").getByText("Test Movie").first().click();
  await page.getByRole("button", { name: /Cast member/ }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toHaveCount(1);
  await page.getByRole("tab", { name: "TV shows" }).click();
  const title = page.getByRole("button", {
    name: "View details for Shared show",
  });
  await title.click();
  await expect(dialog.getByText("Full show synopsis.")).toBeVisible();
  await expect(dialog).toHaveCount(1);
  await dialog.getByRole("button", { name: "Back to results" }).click();
  await expect(page.getByRole("tab", { name: "TV shows" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(title).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(1);
  await expect(
    dialog.getByText("Test Movie", { exact: true }).first(),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
});
