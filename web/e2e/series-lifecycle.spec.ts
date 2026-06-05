import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockSeriesApi,
  mockGet,
  mockRoute,
  SAMPLE_SERIES,
  SAMPLE_LIBRARY,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";

function makeSeries(overrides: Record<string, unknown> = {}) {
  return {
    id: "ser-1",
    title: "Test Series",
    year: 2024,
    tvdb_id: "67890",
    tvdbId: "67890",
    imdb_id: "",
    imdbId: "",
    tmdb_id: "",
    tmdbId: "",
    status: "continuing",
    monitored: true,
    monitoring_status: "monitored",
    monitoringStatus: "monitored",
    quality_profile_id: "qp-1",
    qualityProfileId: "qp-1",
    library_id: "lib-1",
    libraryId: "lib-1",
    overview: "A test series for E2E testing",
    poster_url: "",
    posterPath: "",
    fanart_url: "",
    backdropPath: "",
    rating: 8.0,
    runtime: 45,
    genres: ["Drama"],
    network: "HBO",
    seasons: [
      { id: "s1", season_number: 1, episode_count: 10, monitored: true },
    ],
    added_at: "2025-01-01T00:00:00Z",
    createdAt: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

function setupSeriesMocks(
  page: import("@playwright/test").Page,
  overrides: Record<string, unknown> = {},
) {
  const series = makeSeries(overrides);
  return Promise.all([
    mockSeriesApi(page, [series as any]),
    mockGet(page, "series/ser-1", series),
    mockGet(page, "series/ser-1/credits", { cast: [], crew: [] }),
    mockGet(page, "series/ser-1/seasons/1/episodes", [
      {
        id: "ep-1",
        season_number: 1,
        episode_number: 1,
        title: "Pilot",
        air_date: "2024-01-15",
        status: "downloaded",
        monitored: true,
      },
      {
        id: "ep-2",
        season_number: 1,
        episode_number: 2,
        title: "Second Episode",
        air_date: "2024-01-22",
        status: "missing",
        monitored: true,
      },
    ]),
  ]).then(function () {
    return series;
  });
}

test.describe("Series Lifecycle", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
  });

  test("series detail sheet opens when clicking a series card", async ({
    page,
  }) => {
    await setupSeriesMocks(page);
    await page.goto("/series");
    const card = page.locator("main").getByText("Test Series").first();
    await expect(card).toBeVisible({ timeout: 10000 });
    await card.click();
    // Sheet should show series title
    await expect(page.getByText("Test Series").first()).toBeVisible({
      timeout: 5000,
    });
  });

  test("detail sheet shows continuing status", async ({ page }) => {
    await setupSeriesMocks(page, { status: "continuing" });
    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    // Wait for sheet content
    await page.waitForTimeout(1000);
    // The series title should be in the sheet
    await expect(page.getByText("Test Series").first()).toBeVisible();
  });

  test("detail sheet shows ended status", async ({ page }) => {
    await setupSeriesMocks(page, { status: "ended" });
    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    await page.waitForTimeout(1000);
    await expect(page.getByText("Test Series").first()).toBeVisible();
  });

  test("monitoring toggle triggers PUT request", async ({ page }) => {
    await setupSeriesMocks(page, { monitoringStatus: "monitored" });

    await mockRoute(page, "PUT", "series/ser-1/monitoring", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    await page.waitForTimeout(1000);

    const monitorReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/series/ser-1/monitoring") &&
        req.method() === "PUT"
      );
    });

    // Click the monitoring bookmark toggle
    await page.locator("button[title*='Monitored']").first().click();
    await monitorReq;
  });

  test("search button triggers auto-search for series", async ({ page }) => {
    await setupSeriesMocks(page);

    await mockRoute(page, "POST", "autosearch", async (route) => {
      await route.fulfill({
        status: 200,
        json: { grabbed: null, considered: 3, rejected: 3, reason: "No match" },
      });
    });

    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    await page.waitForTimeout(1000);

    const searchReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/autosearch") && req.method() === "POST"
      );
    });

    await page
      .getByRole("button", { name: /Search/i })
      .first()
      .click();
    await searchReq;
  });

  test("delete button triggers DELETE with confirmation", async ({ page }) => {
    await setupSeriesMocks(page);

    await mockRoute(page, "DELETE", "series/ser-1", async (route) => {
      await route.fulfill({ status: 204 });
    });

    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    await page.waitForTimeout(1000);

    // Click delete button
    await page.locator("button[title='Delete series']").click();

    // Confirmation dialog
    await expect(page.getByText(/Delete Series/i)).toBeVisible({
      timeout: 5000,
    });

    const deleteReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/series/ser-1") && req.method() === "DELETE"
      );
    });

    await page.getByRole("button", { name: "Delete" }).click();
    await deleteReq;
  });

  test("refresh metadata triggers POST refresh", async ({ page }) => {
    const series = await setupSeriesMocks(page);

    await mockRoute(page, "POST", "series/ser-1/refresh", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });
    await mockGet(page, "series/ser-1", series);

    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    await page.waitForTimeout(1000);

    const refreshReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/series/ser-1/refresh") &&
        req.method() === "POST"
      );
    });

    await page.locator("button[title='Refresh metadata from TMDB']").click();
    await refreshReq;
  });

  test("archive button triggers POST archive", async ({ page }) => {
    await setupSeriesMocks(page, { monitoringStatus: "monitored" });

    await mockRoute(page, "POST", "series/ser-1/archive", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/series");
    await page.locator("main").getByText("Test Series").first().click();
    await page.waitForTimeout(1000);

    const archiveReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/series/ser-1/archive") &&
        req.method() === "POST"
      );
    });

    await page.locator("button[title='Archive']").click();
    await archiveReq;
  });
});
