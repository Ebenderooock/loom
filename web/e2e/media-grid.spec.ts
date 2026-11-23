import { expect, test, type Locator, type Page } from "@playwright/test";
import type { LoomCollection } from "../src/lib/collections-api";
import {
  mockBaseApp,
  mockGet,
  mockMovies,
  mockSeriesApi,
  SAMPLE_MOVIE,
  SAMPLE_SERIES,
} from "./helpers/mock-api";

async function mockCollections(page: Page) {
  const collection: LoomCollection = {
    id: "collection-0",
    name: "Collection 0",
    overview: "",
    revision: 1,
    total: 20,
    in_library: 0,
    sources: [],
    members: Array.from({ length: 20 }, (_, i) => ({
      id: `member-${i}`,
      title: `Member ${i}`,
      media_type: "movie",
      poster_path: `/poster-${i}.jpg`,
      external_ids: { tmdb: String(i + 1) },
      sources: [],
      provenance: [],
      local: true,
      excluded: false,
      monitored: false,
      in_library: false,
    })),
  };
  await mockGet(page, "libraries", []);
  await mockGet(page, "collections/", {
    items: Array.from({ length: 20 }, (_, i) => ({
      ...collection,
      id: `collection-${i}`,
      name: `Collection ${i}`,
      members: collection.members.slice(0, i % 4),
    })),
  });
  await mockGet(page, "collections/collection-0", collection);
  await mockGet(page, "collections/scan", {
    total: 0,
    checked: 0,
    pending: 0,
    failed: 0,
    running: false,
    errors: [],
  });
  await page.route("**/poster-*.jpg", (route) =>
    route.fulfill({
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300"/>',
    }),
  );
}

async function measurements(grid: Locator) {
  return grid.evaluate((element) => {
    const cards = Array.from(element.children);
    const first = cards[0].getBoundingClientRect();
    return {
      width: first.width,
      columns: cards.filter(
        (card) => card.getBoundingClientRect().top === first.top,
      ).length,
    };
  });
}

async function expectResponsiveGrid(page: Page, grid: Locator) {
  await expect(grid.locator(":scope > *")).toHaveCount(20);
  await page.setViewportSize({ width: 1920, height: 1080 });
  const baseline = await measurements(grid);
  expect(baseline.columns).toBe(8);

  for (const width of [2560, 3840]) {
    await page.setViewportSize({ width, height: 1080 });
    const larger = await measurements(grid);
    expect(larger.columns).toBeGreaterThan(baseline.columns);
    expect(larger.width).toBeLessThanOrEqual(baseline.width);
  }
}

test("media grids keep 1080p card sizes on wider displays", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium");
  await mockBaseApp(page);
  await mockGet(page, "features", {});
  await mockGet(page, "music/audio-quality-profiles", []);
  await mockGet(page, "music/metadata-profiles", []);
  const movies = Array.from({ length: 20 }, (_, i) => ({
    ...SAMPLE_MOVIE,
    id: `movie-${i}`,
    title: `Movie ${i}`,
  }));
  const series = Array.from({ length: 20 }, (_, i) => ({
    ...SAMPLE_SERIES,
    id: `series-${i}`,
    title: `Show ${i}`,
  }));
  await mockMovies(page, movies);
  await mockSeriesApi(page, series);
  await mockCollections(page);
  await mockGet(
    page,
    "artists",
    Array.from({ length: 20 }, (_, i) => ({
      id: `artist-${i}`,
      name: `Artist ${i}`,
      monitoring_status: "monitored",
    })),
  );
  await mockGet(
    page,
    "albums",
    Array.from({ length: 20 }, (_, i) => ({
      id: `album-${i}`,
      title: `Album ${i}`,
      monitored: true,
      artists: [],
    })),
  );
  await mockGet(page, "discover/recommendations?*", {
    items: Array.from({ length: 20 }, (_, i) => ({
      tmdb_id: i + 1,
      media_type: "movie",
      title: `Suggestion ${i}`,
      reasons: [],
    })),
    warnings: [],
    stale: false,
  });
  await mockGet(page, "import-lists/discover?media_type=movie", {
    data: Array.from({ length: 20 }, (_, i) => ({
      id: `discover-${i}`,
      title: `Discovery ${i}`,
      tmdb_id: String(i + 1),
      media_type: "movie",
      list_name: "List",
    })),
  });

  for (const [path, title] of [
    ["/movies", "Movie 0"],
    ["/series", "Show 0"],
    ["/music", "Artist 0"],
    ["/discover", "Suggestion 0"],
    ["/collections", "Collection 0"],
    ["/collections?id=collection-0", "Member 0"],
  ]) {
    await page.goto(path);
    const grid = page.locator("main .grid").filter({ hasText: title }).first();
    await expectResponsiveGrid(page, grid);
    if (path === "/music") {
      await page.getByRole("button", { name: "Albums", exact: true }).click();
      await expectResponsiveGrid(
        page,
        page.locator("main .grid").filter({ hasText: "Album 0" }).first(),
      );
    }
    if (path === "/discover") {
      await page.getByRole("tab", { name: "Import Lists" }).click();
      await expectResponsiveGrid(
        page,
        page.locator("main .grid").filter({ hasText: "Discovery 0" }).first(),
      );
    }
  }
});

test("collection posters match movie dimensions at every card size", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium");
  await mockBaseApp(page);
  await mockGet(page, "features", {});
  await mockMovies(page);
  await mockCollections(page);

  const sizeOf = (locator: Locator) =>
    locator.evaluate((element) => {
      const { width, height } = element.getBoundingClientRect();
      return { width, height };
    });

  for (const size of ["Small", "Medium", "Large"]) {
    await page.goto("/settings/appearance");
    await page.getByLabel("Media Card Size").click();
    await page.getByRole("option", { name: size, exact: true }).click();

    for (const width of [375, 1920, 2560]) {
      await page.setViewportSize({ width, height: 1080 });
      await page.goto("/movies");
      const movie = page
        .locator("main .grid")
        .filter({ hasText: SAMPLE_MOVIE.title })
        .first()
        .locator('[class~="aspect-[2/3]"]')
        .first();
      await expect(movie).toBeVisible();
      const expected = await sizeOf(movie);

      await page.goto("/collections");
      for (let i = 0; i < 4; i++) {
        const poster = page
          .getByRole("link", { name: new RegExp(`^Collection ${i} `) })
          .locator(":scope > div")
          .first();
        await expect(poster).toBeVisible();
        const actual = await sizeOf(poster);
        expect(actual.width).toBeCloseTo(expected.width, 0);
        expect(actual.height).toBeCloseTo(expected.height, 0);
      }

      await page.goto("/collections?id=collection-0");
      const member = page
        .getByRole("button", { name: "View Member 0", exact: true })
        .locator('[class~="aspect-[2/3]"]');
      await expect(member).toBeVisible();
      const actual = await sizeOf(member);
      expect(actual.width).toBeCloseTo(expected.width, 0);
      expect(actual.height).toBeCloseTo(expected.height, 0);
    }
  }
});

test("the shared card size preference persists across views and reloads", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "chromium");
  await page.setViewportSize({ width: 1920, height: 1080 });
  await mockBaseApp(page);
  await mockGet(page, "features", {});
  await mockMovies(
    page,
    Array.from({ length: 20 }, (_, i) => ({
      ...SAMPLE_MOVIE,
      id: `movie-${i}`,
      title: `Movie ${i}`,
    })),
  );
  await page.goto("/settings/appearance");
  await page.getByLabel("Media Card Size").click();
  await page.getByRole("option", { name: "Small" }).click();
  await page.reload();
  await expect(page.getByLabel("Media Card Size")).toContainText("Small");
  await page.goto("/movies");
  const grid = page
    .locator("main .grid")
    .filter({ hasText: "Movie 0" })
    .first();
  await expect(grid.locator(":scope > *")).toHaveCount(20);
  const small = await measurements(grid);
  expect(small.columns).toBe(10);
  await page.setViewportSize({ width: 2560, height: 1080 });
  const wideSmall = await measurements(grid);
  expect(wideSmall.columns).toBeGreaterThan(small.columns);
  expect(wideSmall.width).toBeLessThanOrEqual(small.width);

  await page.goto("/settings/appearance");
  await page.getByLabel("Media Card Size").click();
  await page.getByRole("option", { name: "Large" }).click();
  await page.goto("/movies");
  await expect(grid.locator(":scope > *")).toHaveCount(20);
  await page.setViewportSize({ width: 1920, height: 1080 });
  const large = await measurements(grid);
  expect(large.columns).toBe(6);
  expect(large.width).toBeGreaterThan(small.width);
  await page.setViewportSize({ width: 2560, height: 1080 });
  const wideLarge = await measurements(grid);
  expect(wideLarge.columns).toBeGreaterThan(large.columns);
  expect(wideLarge.width).toBeLessThanOrEqual(large.width);
});
