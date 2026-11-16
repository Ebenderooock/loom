import { expect, test, type Locator, type Page } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockMovies,
  mockSeriesApi,
  SAMPLE_MOVIE,
  SAMPLE_SERIES,
} from "./helpers/mock-api";

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
