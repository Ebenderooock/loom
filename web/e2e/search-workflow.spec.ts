import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockMovies,
  mockGet,
  mockRoute,
  SAMPLE_INDEXER,
} from "./helpers/mock-api";

function makeRelease(overrides: Record<string, unknown> = {}) {
  return {
    id: "rel-1",
    title: "Test.Movie.2024.1080p.BluRay.x264",
    indexer: "TestIndexer",
    indexer_id: "idx-1",
    size: 4500000000,
    quality: "1080p",
    seeders: 50,
    leechers: 10,
    age: "2d",
    age_minutes: 2880,
    download_url: "https://example.com/download/1",
    info_url: "https://example.com/info/1",
    freeleech: false,
    guid: "guid-1",
    ...overrides,
  };
}

function makeMovie() {
  return {
    id: "mov-1",
    title: "Test Movie",
    year: 2024,
    tmdb_id: 12345,
    tmdbId: "12345",
    imdb_id: "tt1234567",
    imdbId: "tt1234567",
    status: "missing",
    monitored: true,
    monitoringStatus: "monitored",
    quality_profile_id: "qp-1",
    qualityProfileId: "qp-1",
    library_id: "lib-1",
    libraryId: "lib-1",
    posterPath: "",
    poster_url: "",
    overview: "A test movie",
    rating: 7.0,
    runtime: 120,
    genres: ["Action"],
  };
}

// Correctly formatted SSE mock matching the actual stream protocol
async function mockCorrectSearchStream(
  page: import("@playwright/test").Page,
  releases: Array<Record<string, unknown>>,
) {
  await page.route("**/api/v1/indexers/search/stream*", async (route) => {
    const events: string[] = [];

    // search-start event with indexers array
    events.push(
      "event: search-start\ndata: " +
        JSON.stringify({
          indexers: [{ id: "idx-1", name: "TestIndexer" }],
        }) +
        "\n"
    );

    // indexer-start event
    events.push(
      "event: indexer-start\ndata: " +
        JSON.stringify({ indexer_id: "idx-1", indexer_name: "TestIndexer" }) +
        "\n"
    );

    // indexer-result event with results array
    events.push(
      "event: indexer-result\ndata: " +
        JSON.stringify({
          indexer_id: "idx-1",
          indexer_name: "TestIndexer",
          results: releases,
          result_count: releases.length,
          elapsed_ms: 150,
        }) +
        "\n"
    );

    // done event
    events.push(
      "event: done\ndata: " +
        JSON.stringify({
          total_results: releases.length,
          total_errors: 0,
          search_duration_ms: 200,
        }) +
        "\n"
    );

    await route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: events.join("\n"),
    });
  });
}

async function setupMovieAndOpen(page: import("@playwright/test").Page) {
  const movie = makeMovie();
  await mockMovies(page, [movie as any]);
  await mockGet(page, "movies/mov-1", movie);
  await mockGet(page, "movies/mov-1/credits", { cast: [], crew: [] });
  await mockGet(page, "movies/mov-1/files", { data: [], total: 0 });
  await mockGet(page, "download-clients", { download_clients: [] });

  await page.goto("/movies");
  await page.locator("main").getByText("Test Movie").first().click();
  // Wait for detail sheet to open
  await expect(page.locator("[data-state='open'] h2, [role='dialog'] h2").first()).toBeVisible({ timeout: 10000 });
}

test.describe("Search Workflow", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
  });

  test("interactive search dialog opens from movie detail", async ({ page }) => {
    await setupMovieAndOpen(page);

    // Mock the stream so it does not fail
    await mockCorrectSearchStream(page, []);

    // Click Browse for interactive search
    await page.getByRole("button", { name: /Browse/i }).first().click();

    // Search dialog should open
    await expect(page.getByRole("dialog").last()).toBeVisible({ timeout: 5000 });
  });

  test("search dialog shows streaming results", async ({ page }) => {
    const releases = [
      makeRelease({ id: "rel-1", title: "Test.Movie.2024.1080p.BluRay.x264", seeders: 50, guid: "g1" }),
      makeRelease({ id: "rel-2", title: "Test.Movie.2024.720p.WEB-DL", quality: "720p", seeders: 25, size: 2000000000, guid: "g2" }),
    ];

    await setupMovieAndOpen(page);
    await mockCorrectSearchStream(page, releases);

    await page.getByRole("button", { name: /Browse/i }).first().click();
    await expect(page.getByRole("dialog").last()).toBeVisible({ timeout: 5000 });

    // Results should appear (streamed via SSE)
    await expect(page.getByText("Test.Movie.2024.1080p.BluRay.x264")).toBeVisible({ timeout: 15000 });
    await expect(page.getByText("Test.Movie.2024.720p.WEB-DL")).toBeVisible({ timeout: 5000 });
  });

  test("grab button triggers download grab", async ({ page }) => {
    const releases = [makeRelease({ guid: "g-grab" })];

    await setupMovieAndOpen(page);
    await mockCorrectSearchStream(page, releases);

    // Ensure download-clients endpoint returns a client (trailing slash)
    await page.route("**/api/v1/download-clients/*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { download_clients: [{ id: "dc-1", name: "qBittorrent", enabled: true, type: "qbittorrent" }] } });
      } else if (route.request().method() === "POST") {
        // Grab endpoint: POST /api/v1/download-clients/{id}/items
        await route.fulfill({ status: 200, json: { ok: true } });
      } else {
        await route.fallback();
      }
    });

    await page.getByRole("button", { name: /Browse/i }).first().click();
    await expect(page.getByRole("dialog").last()).toBeVisible({ timeout: 5000 });

    // Wait for results
    await expect(page.getByText("Test.Movie.2024.1080p.BluRay.x264")).toBeVisible({ timeout: 15000 });

    // Wait for the grab button to be enabled (download clients loaded)
    const grabBtn = page.getByRole("dialog").last().locator("button[title*='Grab']").first();
    await expect(grabBtn).toBeEnabled({ timeout: 10000 });
    await grabBtn.click();
  });

  test("search results show release information", async ({ page }) => {
    const releases = [
      makeRelease({
        id: "rel-info",
        title: "Info.Movie.2024.2160p.UHD.BluRay",
        quality: "2160p",
        seeders: 100,
        size: 8000000000,
        guid: "g-info",
      }),
    ];

    await setupMovieAndOpen(page);
    await mockCorrectSearchStream(page, releases);

    await page.getByRole("button", { name: /Browse/i }).first().click();
    await expect(page.getByRole("dialog").last()).toBeVisible({ timeout: 5000 });

    // Wait for results
    await expect(page.getByText("Info.Movie.2024.2160p.UHD.BluRay")).toBeVisible({ timeout: 15000 });
  });

  test("multiple results from search stream render", async ({ page }) => {
    const releases = [
      makeRelease({ id: "rel-a", title: "Alpha.2024.1080p", guid: "ga" }),
      makeRelease({ id: "rel-b", title: "Beta.2024.720p", quality: "720p", guid: "gb" }),
      makeRelease({ id: "rel-c", title: "Gamma.2024.2160p", quality: "2160p", guid: "gc" }),
    ];

    await setupMovieAndOpen(page);
    await mockCorrectSearchStream(page, releases);

    await page.getByRole("button", { name: /Browse/i }).first().click();
    await expect(page.getByRole("dialog").last()).toBeVisible({ timeout: 5000 });

    await expect(page.getByText("Alpha.2024.1080p")).toBeVisible({ timeout: 15000 });
    await expect(page.getByText("Beta.2024.720p")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText("Gamma.2024.2160p")).toBeVisible({ timeout: 5000 });
  });
});
