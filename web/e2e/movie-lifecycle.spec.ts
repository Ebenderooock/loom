import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockMovies,
  mockGet,
  mockRoute,
  SAMPLE_MOVIE,
  SAMPLE_LIBRARY,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";

// Movie fixture providing both snake_case (API compat) and camelCase (code reads)
function makeMovie(overrides: Record<string, unknown> = {}) {
  return {
    id: "mov-1",
    title: "Test Movie",
    year: 2024,
    tmdb_id: 12345,
    tmdbId: "12345",
    imdb_id: "tt1234567",
    imdbId: "tt1234567",
    status: "available_right_quality",
    monitored: true,
    monitoringStatus: "monitored",
    quality_profile_id: "qp-1",
    qualityProfileId: "qp-1",
    library_id: "lib-1",
    libraryId: "lib-1",
    overview: "A test movie for E2E testing",
    poster_url: "",
    posterPath: "",
    fanart_url: "",
    backdropPath: "",
    rating: 7.5,
    runtime: 120,
    genres: ["Action"],
    release_date: "2024-06-15",
    releaseDate: "2024-06-15",
    added_at: "2025-01-01T00:00:00Z",
    createdAt: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

function setupMovieMocks(page: import("@playwright/test").Page, movieOverrides: Record<string, unknown> = {}) {
  const movie = makeMovie(movieOverrides);
  return Promise.all([
    mockMovies(page, [movie as any]),
    mockGet(page, "movies/mov-1", movie),
    mockGet(page, "movies/mov-1/credits", { cast: [], crew: [] }),
    mockGet(page, "movies/mov-1/files", { data: [], total: 0 }),
  ]).then(() => movie);
}

test.describe("Movie Lifecycle", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
  });

  test("movie detail sheet opens when clicking a movie card", async ({ page }) => {
    await setupMovieMocks(page);
    await page.goto("/movies");
    // Wait for movie card to appear then click it
    const card = page.locator("main").getByText("Test Movie").first();
    await expect(card).toBeVisible({ timeout: 10000 });
    await card.click();
    // Sheet should open with movie title
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });
  });

  test("detail sheet shows Available badge for available_right_quality", async ({ page }) => {
    await setupMovieMocks(page, { status: "available_right_quality" });
    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    // Wait for sheet to open by checking for the h2 title inside
    await expect(page.locator("[role='dialog'] h2, [data-state='open'] h2").first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Available").first()).toBeVisible({ timeout: 5000 });
  });

  test("detail sheet shows Missing badge for missing status", async ({ page }) => {
    await setupMovieMocks(page, { status: "missing" });
    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.locator("[role='dialog'] h2, [data-state='open'] h2").first()).toBeVisible({ timeout: 10000 });
    await expect(page.locator("[role='dialog']").getByText("Missing").first()).toBeVisible({ timeout: 5000 });
  });

  test("detail sheet shows Downloading badge for downloading status", async ({ page }) => {
    await setupMovieMocks(page, { status: "downloading" });
    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.locator("[role='dialog'] h2, [data-state='open'] h2").first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Downloading").first()).toBeVisible({ timeout: 5000 });
  });

  test("detail sheet shows Wrong Quality badge", async ({ page }) => {
    await setupMovieMocks(page, { status: "available_wrong_quality" });
    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.locator("[role='dialog'] h2, [data-state='open'] h2").first()).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Wrong Quality").first()).toBeVisible({ timeout: 5000 });
  });

  test("monitoring toggle triggers PUT request", async ({ page }) => {
    await setupMovieMocks(page, { monitoringStatus: "monitored" });

    // Mock the PUT monitoring endpoint
    await mockRoute(page, "PUT", "movies/mov-1/monitoring", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    // Wait for sheet to open
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    // Set up request listener before clicking
    const monitorRequest = page.waitForRequest(
      (req) => req.url().includes("/api/v1/movies/mov-1/monitoring") && req.method() === "PUT"
    );

    // Click the monitoring toggle (bookmark button)
    await page.locator("button[title*='Monitored']").click();
    await monitorRequest;
  });

  test("search button triggers auto-search", async ({ page }) => {
    await setupMovieMocks(page);

    // Mock auto-search endpoint
    await mockRoute(page, "POST", "autosearch", async (route) => {
      await route.fulfill({
        status: 200,
        json: { grabbed: null, considered: 5, rejected: 5, reason: "No match" },
      });
    });

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    const searchRequest = page.waitForRequest(
      (req) => req.url().includes("/api/v1/autosearch") && req.method() === "POST"
    );

    await page.getByRole("button", { name: /Search/i }).first().click();
    await searchRequest;
  });

  test("archive button triggers POST archive", async ({ page }) => {
    await setupMovieMocks(page, { monitoringStatus: "monitored" });

    await mockRoute(page, "POST", "movies/mov-1/archive", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    const archiveReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/movies/mov-1/archive") && req.method() === "POST"
    );

    await page.locator("button[title='Archive']").click();
    await archiveReq;
  });

  test("delete button opens confirmation and triggers DELETE", async ({ page }) => {
    await setupMovieMocks(page);

    await mockRoute(page, "DELETE", "movies/mov-1", async (route) => {
      await route.fulfill({ status: 204 });
    });

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    // Click delete button
    await page.locator("button[title='Delete movie']").click();

    // Confirmation dialog should appear
    await expect(page.getByText("Delete Movie")).toBeVisible({ timeout: 5000 });

    const deleteReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/movies/mov-1") && req.method() === "DELETE"
    );

    // Click the destructive Delete button in the dialog
    await page.getByRole("button", { name: "Delete" }).click();
    await deleteReq;
  });

  test("refresh metadata triggers POST refresh", async ({ page }) => {
    const movie = await setupMovieMocks(page);

    await mockRoute(page, "POST", "movies/mov-1/refresh", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });
    // Re-mock the single movie GET for after refresh
    await mockGet(page, "movies/mov-1", movie);

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    const refreshReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/movies/mov-1/refresh") && req.method() === "POST"
    );

    await page.locator("button[title='Refresh metadata from TMDB']").click();
    await refreshReq;
  });

  test("edit mode allows changing quality profile", async ({ page }) => {
    await setupMovieMocks(page);

    await mockRoute(page, "PUT", "movies/mov-1", async (route) => {
      const body = route.request().postDataJSON();
      await route.fulfill({
        status: 200,
        json: makeMovie({ qualityProfileId: body.quality_profile_id }),
      });
    });

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    // Click Edit button
    await page.getByRole("button", { name: /Edit/i }).first().click();
    // Editing Movie label should appear
    await expect(page.getByText("Editing Movie")).toBeVisible({ timeout: 5000 });

    // Save Changes button should be visible
    await expect(page.getByRole("button", { name: /Save Changes/i })).toBeVisible();

    const saveReq = page.waitForRequest(
      (req) => req.url().includes("/api/v1/movies/mov-1") && req.method() === "PUT"
    );

    await page.getByRole("button", { name: /Save Changes/i }).click();
    await saveReq;
  });

  test("browse button opens release search dialog", async ({ page }) => {
    await setupMovieMocks(page);
    // Mock search stream and download clients
    await mockGet(page, "download-clients", { download_clients: [] });

    await page.goto("/movies");
    await page.locator("main").getByText("Test Movie").first().click();
    await expect(page.getByText("Movie details for Test Movie")).toBeAttached({ timeout: 5000 });

    // Click Browse button to open search dialog
    await page.getByRole("button", { name: /Browse/i }).first().click();

    // Release search dialog should open
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5000 });
  });
});
