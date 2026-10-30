import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  SAMPLE_LIBRARY,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";

const source = {
  warnings: [],
  stale: false,
  generated_at: "2026-10-28T12:00:00Z",
};
const suggestion = {
  tmdb_id: 107,
  media_type: "movie",
  title: "A new adventure",
  year: 2024,
  rating: 7.7,
  popularity: 50,
  source: "tmdb",
  reasons: ["You have 2 movies starring Ryan Reynolds"],
  collection_id: 10,
};
const collection = {
  id: "loom-collection",
  name: "Adventure collection",
  overview: "A connected set of adventures.",
  revision: 1,
  total: 3,
  in_library: 1,
  sources: [{ provider: "tmdb", kind: "collection", external_id: "10" }],
  members: [
    {
      id: "member-1",
      media_type: "movie",
      title: "Original adventure",
      external_ids: { tmdb: "101" },
      sources: ["tmdb:collection:10"],
      provenance: ["tmdb:collection:10"],
      local: false,
      excluded: false,
      in_library: true,
      media_id: "actual-movie",
      library_id: "lib-1",
      has_file: false,
      monitored: true,
    },
    {
      id: "member-2",
      media_type: "series",
      title: "Connected show",
      external_ids: { tmdb: "101" },
      sources: ["tmdb:collection:10"],
      provenance: ["tmdb:collection:10"],
      local: false,
      excluded: false,
      in_library: false,
      monitored: false,
    },
    {
      id: "member-3",
      media_type: "movie",
      title: "Future adventure",
      external_ids: { tmdb: "103" },
      sources: ["tmdb:collection:10"],
      provenance: ["tmdb:collection:10"],
      local: false,
      excluded: false,
      in_library: false,
      monitored: false,
      release_date: "2099-01-01",
    },
  ],
};

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await mockGet(page, "features", {});
  await mockGet(page, "discover/recommendations?*", {
    ...source,
    items: [suggestion],
  });
  await mockGet(page, "collections/", { items: [collection] });
  await mockGet(page, "collections/loom-collection", collection);
  await mockGet(page, "movies/actual-movie", {
    id: "actual-movie",
    title: "Original adventure",
    year: 2020,
    overview: "The original movie.",
    tmdbId: "101",
    rating: 7,
    runtime: 120,
    genres: ["Adventure"],
    releaseDate: "2020-01-01",
    status: "missing",
    monitoringStatus: "monitored",
    qualityProfileId: "qp-1",
    libraryId: "lib-1",
  });
  await mockGet(page, "movies/actual-movie/credits", { cast: [], crew: [] });
  await mockGet(page, "movies/files/actual-movie", []);
  await mockGet(page, "movies/actual-movie/history", []);
  await mockGet(page, "media-files/movie/actual-movie*", {
    files: [],
    warnings: [],
  });
  await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
  await mockGet(page, "quality-profiles", { data: [SAMPLE_QUALITY_PROFILE] });
  await mockGet(page, "media-info/preferences", {
    default_quality_profile_id: "qp-1",
  });
  await mockGet(page, "discover/movie/107", {
    title: {
      tmdb_id: "107",
      title: "A new adventure",
      overview: "A complete synopsis.",
      poster_path: "/adventure.jpg",
      rating: 7.7,
      release_date: "2024-01-01",
      genres: ["Adventure"],
    },
    credits: { cast: [], crew: [] },
    credits_error: "",
  });
  await mockGet(page, "discover/series/101?provider=tmdb", {
    title: {
      tmdb_id: "101",
      title: "Connected show",
      overview: "A connected TV series.",
      first_air_date: "2021-01-01",
      genres: ["Adventure"],
    },
    credits: { cast: [], crew: [] },
    credits_error: "",
  });
  await mockGet(page, "movies?limit=1000&offset=0", []);
  await mockGet(page, "series", []);
});

test("suggestions need no Import Lists and require an explicit add", async ({
  page,
}) => {
  const writes: string[] = [];
  const listRequests: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST") writes.push(request.url());
    if (new URL(request.url()).pathname.startsWith("/api/v1/import-lists"))
      listRequests.push(request.url());
  });
  await page.goto("/discover");
  await expect(
    page.getByRole("button", { name: "A new adventure", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("You have 2 movies starring Ryan Reynolds"),
  ).toBeVisible();
  expect(writes).toEqual([]);
  expect(listRequests).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);

  await page
    .getByRole("button", { name: "Details & add", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "A new adventure" }).last(),
  ).toBeVisible();
  await dialog
    .getByRole("button", { name: "Add to library", exact: true })
    .click();
  await expect(
    dialog.getByRole("checkbox", { name: "Search after adding" }),
  ).not.toBeChecked();
  await page.route("**/api/v1/movies", async (route) => {
    expect(route.request().postDataJSON()).toMatchObject({
      tmdb_id: "107",
      library_id: "lib-1",
      quality_profile_id: "qp-1",
      search: false,
    });
    await route.fulfill({ status: 201, json: { id: "new-movie" } });
  });
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(
    dialog.getByRole("button", { name: "In library", exact: true }),
  ).toBeVisible();
  expect(writes).toHaveLength(1);
});

test("standalone mixed collections open real library management and shared external details", async ({
  page,
}, testInfo) => {
  await page.goto("/discover");
  await expect(
    page.getByRole("tab", { name: "Collections", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("link", { name: "View collection", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Adventure collection" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Original adventure" }),
  ).toBeVisible();
  await expect(page.getByText("Monitored · No imported files")).toBeVisible();
  await page.getByRole("button", { name: "Manage Original adventure" }).click();
  const management = page.getByRole("dialog");
  await expect(
    management.getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  await expect(
    management.getByRole("button", { name: "Manual Search", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(management).not.toBeVisible();
  await page.getByRole("button", { name: "View Connected show" }).click();
  const external = page.getByRole("dialog");
  await expect(external.getByText("A connected TV series.")).toBeVisible();
  await expect(
    external.getByRole("button", { name: "Add to library", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(external).not.toBeVisible();
  await page
    .getByRole("combobox", { name: "Status", exact: true })
    .selectOption("upcoming");
  await expect(
    page.getByRole("heading", { name: "Future adventure" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Original adventure" }),
  ).toHaveCount(0);
  await page
    .getByRole("combobox", { name: "Status", exact: true })
    .selectOption("");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("collection.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "All collections", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: /Adventure collection/ }),
  ).toBeVisible();
});

test("provider failure has a visible retry path", async ({ page }) => {
  await page.route("**/api/v1/discover/recommendations?*", async (route) => {
    await route.fulfill({ status: 503, body: "TMDB access is not configured" });
  });
  await page.goto("/discover");
  await expect(page.getByRole("alert")).toContainText(
    "TMDB access is not configured",
  );
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  await expect(page.getByText("No new suggestions yet")).not.toBeVisible();
});
