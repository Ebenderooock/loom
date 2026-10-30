import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockPost,
  SAMPLE_LIBRARY,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";

const item = {
  id: "discover-1",
  list_id: "list-1",
  external_id: "42",
  tmdb_id: "42",
  title: "Discover Movie",
  year: 2026,
  media_type: "movie",
  list_name: "My picks",
  in_library: false,
  overview: "Feed overview",
  status: "pending",
  defaults: { search_on_add: false },
};

const titleDetails = {
  title: {
    tmdb_id: "42",
    title: "Discover Movie",
    year: 2026,
    overview: "Full movie overview",
    poster_path: "/poster.jpg",
    genres: ["Drama"],
    release_date: "2026-10-01",
    runtime: 123,
    rating: 8.2,
  },
  credits: { cast: [], crew: [] },
  credits_error: "",
};

async function openImportLists(page: import("@playwright/test").Page) {
  await page.goto("/discover");
  await page.getByRole("tab", { name: "Import Lists" }).click();
}

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await mockGet(page, "import-lists/discover?media_type=movie", {
    data: [item],
  });
  await mockGet(page, "import-lists/discover?media_type=series", {
    data: [
      {
        ...item,
        id: "discover-show",
        tmdb_id: "43",
        title: "Discover Show",
        media_type: "series",
      },
    ],
  });
  await mockGet(page, "discover/movie/42", titleDetails);
  await mockGet(page, "discover/series/43", {
    title: {
      tmdb_id: "43",
      title: "Discover Show",
      overview: "Full show overview",
      genres: ["Comedy"],
      first_air_date: "2026-10-02",
      seasons: 3,
      rating: 7.8,
    },
    credits: { cast: [], crew: [] },
    credits_error: "",
  });
  await mockGet(page, "libraries", {
    data: [
      SAMPLE_LIBRARY,
      { ...SAMPLE_LIBRARY, id: "series-lib", name: "TV", media_type: "series" },
    ],
  });
  await mockGet(page, "quality-profiles", { data: [SAMPLE_QUALITY_PROFILE] });
  await mockGet(page, "media-info/preferences", {
    default_quality_profile_id: "qp-1",
  });
  await mockGet(page, "movies?limit=1000&offset=0", []);
  await mockGet(page, "series", []);
  await mockPost(page, "import-lists/discover/track", { tracked: true });
});

test("full details are read-only until the shared add flow is explicitly submitted", async ({
  page,
}) => {
  const writes: { url: string; body: unknown }[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST") {
      writes.push({ url: request.url(), body: request.postDataJSON() });
    }
  });
  await mockPost(page, "movies", { id: "new-movie" }, 201);
  await openImportLists(page);
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .first()
    .click();

  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "Discover Movie" }).last(),
  ).toBeVisible();
  await expect(dialog.getByText("Full movie overview")).toBeVisible();
  await expect(dialog.getByText("123 min")).toBeVisible();
  await expect(dialog.getByText("Drama", { exact: true })).toBeVisible();
  await expect(dialog.getByText("8.2")).toBeVisible();
  expect(writes).toEqual([]);

  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(
    dialog.getByRole("checkbox", { name: "Search after adding" }),
  ).not.toBeChecked();
  await dialog.getByRole("button", { name: "Add to library" }).last().click();
  await expect(dialog).not.toBeVisible();
  expect(writes.map(({ url }) => new URL(url).pathname)).toEqual([
    "/api/v1/movies",
    "/api/v1/import-lists/discover/track",
  ]);
  expect(writes[1].body).toEqual({ item_id: "discover-1" });
  await expect(page).toHaveURL(/\/discover$/);
  await page.keyboard.press("Escape");
  await expect(
    page
      .getByRole("button", { name: "View details for Discover Movie" })
      .first(),
  ).toBeFocused();
});

test("quick add opens shared settings without bypassing the detail flow", async ({
  page,
}) => {
  await openImportLists(page);
  await page.getByRole("button", { name: "Add Discover Movie" }).click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("checkbox", { name: "Search after adding" }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("button", { name: "View details for Discover Movie" })
      .first(),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "Close" }).click();
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .first()
    .click();
  await expect(
    page.getByRole("dialog").getByText("Full movie overview"),
  ).toBeVisible();
});

test("series details use series settings and source tracking", async ({
  page,
}) => {
  let addedBody: unknown;
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST")
      writes.push(new URL(request.url()).pathname);
  });
  await mockPost(page, "series", { id: "new-series" }, 201);
  await page.route("**/api/v1/series", async (route) => {
    if (route.request().method() === "POST") {
      addedBody = route.request().postDataJSON();
      await route.fulfill({ status: 201, json: { id: "new-series" } });
    } else {
      await route.fallback();
    }
  });
  await openImportLists(page);
  await page.getByRole("tab", { name: "TV shows" }).click();
  await page
    .getByRole("button", { name: "View details for Discover Show" })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Full show overview")).toBeVisible();
  await expect(dialog.getByText("Seasons", { exact: true })).toBeVisible();
  await expect(dialog.getByText("3", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await dialog.getByRole("button", { name: "Add to library" }).last().click();
  await expect(dialog).not.toBeVisible();
  expect(addedBody).toMatchObject({
    tmdbId: "43",
    libraryId: "series-lib",
    qualityProfileId: "qp-1",
    search: false,
  });
  expect(writes).toEqual([
    "/api/v1/series",
    "/api/v1/import-lists/discover/track",
  ]);
});

test("add failures stay visible and can be retried", async ({ page }) => {
  await page.route("**/api/v1/movies", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status: 500, body: "Library unavailable" });
    } else {
      await route.fallback();
    }
  });
  await openImportLists(page);
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await dialog.getByRole("button", { name: "Add to library" }).last().click();
  await expect(dialog.getByRole("alert")).toContainText("Library unavailable");
  await expect(
    dialog.getByRole("button", { name: "Add to library" }).last(),
  ).toBeEnabled();
});

test("details resolve an exact IMDb identity when a TMDB ID is unavailable", async ({
  page,
}) => {
  await mockGet(page, "import-lists/discover?media_type=movie", {
    data: [{ ...item, tmdb_id: "", imdb_id: "tt1234567" }],
  });
  await mockGet(page, "discover/movie/tt1234567?provider=imdb", {
    ...titleDetails,
    title: { ...titleDetails.title, tmdb_id: "42" },
  });
  await openImportLists(page);
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .first()
    .click();
  await expect(
    page.getByRole("dialog").getByText("Full movie overview"),
  ).toBeVisible();
});

test("details remain usable on a small dark screen with reduced motion", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 375, height: 667 });
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  await page.addInitScript(() =>
    window.localStorage.setItem("loom.theme", "dark"),
  );
  await openImportLists(page);
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .first()
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("Full movie overview")).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Add to library" }).first(),
  ).toBeInViewport();
  await page.screenshot({
    path: testInfo.outputPath("discover-title-dark.png"),
  });
  await page.setViewportSize({ width: 667, height: 375 });
  await expect(
    dialog.getByRole("button", { name: "Add to library" }).first(),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
