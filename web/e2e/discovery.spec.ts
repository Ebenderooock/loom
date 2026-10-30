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
  poster_path: "/adventure.jpg",
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
  await page.route("https://image.tmdb.org/**", async (route) => {
    await route.fulfill({
      contentType: "image/svg+xml",
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="342" height="513" viewBox="0 0 342 513"><defs><linearGradient id="sky" x2="0" y2="1"><stop stop-color="#173552"/><stop offset="1" stop-color="#c06b43"/></linearGradient></defs><rect width="342" height="513" fill="url(#sky)"/><circle cx="250" cy="130" r="70" fill="#edc784"/><path d="M0 400L95 140L200 400L270 250L342 400V513H0Z" fill="#192d39"/><path d="M45 400L96 240L150 400Z" fill="#366070"/></svg>`,
    });
  });
  await mockGet(page, "features", {});
  await mockGet(page, "discover/recommendations?*", {
    ...source,
    items: [suggestion],
  });
  await mockGet(page, "collections/", { items: [collection] });
  await mockGet(page, "collections/scan", {
    total: 1020,
    checked: 540,
    pending: 480,
    failed: 0,
    running: true,
    errors: [],
  });
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
    page.getByRole("button", { name: "View A new adventure", exact: true }),
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
    .getByRole("button", { name: "View A new adventure", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "A new adventure" }).last(),
  ).toBeVisible();
  await expect(
    dialog.getByRole("checkbox", { name: "Search after adding" }),
  ).not.toBeChecked();
  let added = false;
  // Backend movie IDs keep title punctuation; the drawer must request the
  // persisted ID returned by POST, browser-encoded, not a fake local ID.
  const movieID = "a/new-adventure:-part-#1?-50%25-2024";
  const detailPaths: string[] = [];
  await page.route("**/api/v1/movies?limit=1000&offset=0", async (route) => {
    await route.fulfill({
      json: added ? [{ id: movieID, tmdbId: "107" }] : [],
    });
  });
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/v1/movies/") &&
      decodeURIComponent(url.pathname).includes(movieID),
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      detailPaths.push(path);
      const decoded = decodeURIComponent(path);
      await route.fulfill({
        json: decoded.endsWith("/credits")
          ? { cast: [], crew: [] }
          : decoded.endsWith("/history") || decoded.includes("/files/")
            ? []
            : {
                id: movieID,
                tmdbId: "107",
                title: "A new adventure",
                status: "missing",
                year: 2024,
                monitoringStatus: "monitored",
                libraryId: "lib-1",
                qualityProfileId: "qp-1",
                genres: [],
              },
      });
    },
  );
  await mockGet(page, "media-files/movie/**", {
    files: [],
    warnings: [],
  });
  await page.route("**/api/v1/movies", async (route) => {
    expect(route.request().postDataJSON()).toMatchObject({
      tmdb_id: "107",
      library_id: "lib-1",
      quality_profile_id: "qp-1",
      search: false,
    });
    added = true;
    await route.fulfill({ status: 201, json: { id: movieID } });
  });
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(
    dialog.getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  const encodedID = encodeURIComponent(movieID);
  await expect
    .poll(() => [...detailPaths].sort())
    .toEqual(
      expect.arrayContaining([
        `/api/v1/movies/${encodedID}`,
        `/api/v1/movies/${encodedID}/credits`,
        `/api/v1/movies/${encodedID}/history`,
        `/api/v1/movies/files/${encodedID}`,
      ]),
    );
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

test("collection management stays in one place and never overflows", async ({
  page,
}, testInfo) => {
  const posts: { url: string; method: string; body: unknown }[] = [];
  await page.route(
    "**/api/v1/collections/loom-collection/**",
    async (route) => {
      const req = route.request();
      if (req.method() === "GET") return route.fallback();
      const body = req.postData() ? JSON.parse(req.postData()!) : {};
      posts.push({
        url: new URL(req.url()).pathname,
        method: req.method(),
        body,
      });
      if (req.url().endsWith("/bulk/add"))
        return route.fulfill({
          json: {
            kind: "add",
            total: 3,
            counts: { eligible: 1, in_library: 1, unreleased: 1 },
            items: [
              {
                member_id: "member-2",
                title: "Connected show",
                media_type: "series",
                status: "eligible",
                library_id: "lib-1",
                profile_id: "qp-1",
              },
              {
                member_id: "member-1",
                title: "Original adventure",
                media_type: "movie",
                status: "skipped",
                reason: "in_library",
              },
              {
                member_id: "member-3",
                title: "Future adventure",
                media_type: "movie",
                status: "skipped",
                reason: "unreleased",
              },
            ],
            libraries: {
              movie: [
                { id: "lib-1", name: "Movies", quality_profile_id: "qp-1" },
              ],
              series: [
                { id: "lib-tv", name: "TV", quality_profile_id: "qp-1" },
              ],
            },
            profiles: [{ id: "qp-1", name: "HD-1080p" }],
            settings: {
              movie: {
                library_id: "lib-1",
                quality_profile_id: "qp-1",
                monitored: true,
              },
              series: {
                library_id: "lib-tv",
                quality_profile_id: "qp-1",
                monitored: true,
              },
            },
          },
        });
      return route.fulfill({ json: collection });
    },
  );
  await mockGet(page, "movies?limit=1000&offset=0", {
    data: [{ id: "actual-movie", title: "Original adventure", tmdbId: 101 }],
  });
  await mockGet(page, "movies/lookup?*", [
    {
      tmdbId: 990001,
      title:
        "An extraordinarily long adventure title that must wrap inside the picker",
      year: 2023,
      posterPath: "/adventure.jpg",
    },
  ]);
  const noOverflow = async () => {
    // Let sheet/dialog enter animations finish before measuring.
    await page.waitForTimeout(500);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    for (const dialog of await page.getByRole("dialog").all()) {
      await expect(dialog).toBeInViewport({ ratio: 1 });
      expect(
        await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
      ).toBe(true);
    }
  };

  await page.goto("/collections?id=loom-collection");
  await expect(
    page.getByRole("heading", { name: "Adventure collection" }),
  ).toBeVisible();

  await page.getByRole("button", { name: "Add titles", exact: true }).click();
  const picker = page.getByRole("dialog", {
    name: "Add titles to Adventure collection",
  });
  await picker.getByLabel("Search titles").fill("adventure");
  await expect(
    picker.getByRole("checkbox", { name: /Original adventure/ }),
  ).toBeDisabled();
  await picker
    .getByRole("checkbox", { name: /extraordinarily long adventure/ })
    .click();
  await expect(
    picker.getByRole("button", { name: "Add 1 title" }),
  ).toBeEnabled();
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-picker.png") });
  await picker.getByRole("button", { name: "Add 1 title" }).click();
  await expect(picker).not.toBeVisible();
  expect(posts.at(-1)).toMatchObject({
    url: "/api/v1/collections/loom-collection/members",
    body: {
      items: [{ media_type: "movie", external_ids: { tmdb: "990001" } }],
    },
  });

  await page.getByRole("button", { name: "Select", exact: true }).click();
  await page.getByRole("button", { name: "Select Connected show" }).click();
  await page.getByRole("button", { name: "Select Future adventure" }).click();
  const bar = page.getByRole("toolbar", { name: "Selected titles" });
  await expect(bar).toContainText("2 selected");
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-select.png") });
  await bar.getByRole("button", { name: "Add missing" }).click();
  const bulk = page.getByRole("dialog", { name: "Add missing titles" });
  await expect(bulk.getByText("Connected show")).toBeVisible();
  await expect(bulk.getByLabel("Library").first()).toHaveValue("lib-1");
  expect(
    posts.every((p) => (p.body as { execute?: boolean }).execute !== true),
  ).toBe(true);
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-bulk.png") });
  await page.keyboard.press("Escape");
  await expect(bulk).not.toBeVisible();

  await page.getByRole("button", { name: "Manage", exact: true }).click();
  const manage = page.getByRole("dialog", { name: "Manage collection" });
  await expect(manage.getByLabel("Quality for new additions")).toBeVisible();
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-manage.png") });
  await manage.getByRole("button", { name: "Delete collection" }).click();
  const confirm = page.getByRole("dialog", {
    name: "Delete Adventure collection?",
  });
  await expect(confirm).toContainText("stay exactly as they are");
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-delete.png") });
  expect(posts.some((p) => p.method === "DELETE")).toBe(false);
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

for (const width of [375, 768, 1440]) {
  test(`title drawer stays within ${width}px with long titles and credits`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const longTitle = "An extraordinarily long adventure title ".repeat(5);
    await mockGet(page, "discover/movie/107", {
      title: {
        tmdb_id: "107",
        title: longTitle,
        overview: "A synopsis with a long identifier: " + "x".repeat(300),
        poster_path: "/adventure.jpg",
        backdrop_path: "/adventure.jpg",
      },
      credits: {
        cast: Array.from({ length: 30 }, (_, index) => ({
          id: index,
          name: `A performer with a very long name ${index}`,
          character: "An unusually long character description",
        })),
        crew: [],
      },
    });
    await page.goto("/discover");
    const opener = page.getByRole("button", {
      name: "View A new adventure",
      exact: true,
    });
    await opener.click();
    const drawer = page.getByRole("dialog");
    await expect(drawer.getByLabel("Library", { exact: true })).toBeVisible();
    await expect(
      drawer.getByRole("button", { name: "Close", exact: true }),
    ).toBeInViewport();
    await expect(
      drawer.getByRole("button", { name: "Add to library", exact: true }),
    ).toBeEnabled();
    await expect(
      drawer.getByText("A performer with a very long name 29", { exact: true }),
    ).toHaveCount(1);
    await expect
      .poll(async () => {
        const bounds = await drawer.boundingBox();
        return bounds
          ? Math.abs(bounds.x + bounds.width - width)
          : Number.POSITIVE_INFINITY;
      })
      .toBeLessThanOrEqual(1);
    const bounds = await drawer.boundingBox();
    expect(bounds).not.toBeNull();
    expect(Math.abs(bounds!.x + bounds!.width - width)).toBeLessThanOrEqual(1);
    expect(bounds!.width).toBeLessThanOrEqual(Math.min(width, 768));
    expect(
      await drawer.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`drawer-${width}.png`),
      fullPage: true,
    });
    await page.keyboard.press("Escape");
    await expect(opener).toBeFocused();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.screenshot({
      path: testInfo.outputPath(`posters-${width}.png`),
      fullPage: true,
    });
    await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
    await opener.click();
    await expect(drawer.getByLabel("Library", { exact: true })).toBeVisible();
    await expect(
      drawer.getByRole("button", { name: "Close", exact: true }),
    ).toBeInViewport();
    await expect
      .poll(async () => {
        const bounds = await drawer.boundingBox();
        return bounds
          ? Math.abs(bounds.x + bounds.width - width)
          : Number.POSITIVE_INFINITY;
      })
      .toBeLessThanOrEqual(1);
    expect(
      await drawer.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`drawer-dark-${width}.png`),
      fullPage: true,
    });
  });
}
