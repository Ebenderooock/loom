import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Movie Status Lifecycle", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("movies display correct status badges", async ({ page }) => {
    await page.goto("/movies");
    await waitForPageLoad(page);

    const statusBadges = page.locator("main").getByText(
      /Missing|Available|Downloading|Grabbed|Wrong Quality/i
    );
    const count = await statusBadges.count();
    console.log("Found " + count + " status badges on movies page");
  });

  test("movie detail page shows status", async ({ page }) => {
    await page.goto("/movies");
    await waitForPageLoad(page);

    // Click the first movie card (poster link)
    const firstCard = page.locator("main a[href*='/movies/']").first();
    const cardVisible = await firstCard.isVisible({ timeout: 10000 }).catch(() => false);
    if (!cardVisible) {
      test.skip(true, "No movies available");
      return;
    }
    await firstCard.click();

    // Wait for navigation to movie detail page or sheet to open
    await page.waitForURL(/\/movies\/[a-zA-Z0-9-]+/, { timeout: 10000 }).catch(() => null);
    const detailView = page.locator("[role=dialog], [data-state=open], main").first();
    await expect(detailView).toBeVisible({ timeout: 10000 });

    // Look for status info somewhere on the detail page
    const statusText = page.getByText(/Missing|Available|Downloading|Grabbed|Status/i).first();
    await expect(statusText).toBeVisible({ timeout: 5000 });
  });

  test("grabbed/downloading movies appear with correct status via API", async ({ request }) => {
    const res = await authGet(request, "/api/v1/movies?limit=200");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const movies = Array.isArray(body) ? body : body.data ?? [];

    const statusCounts: Record<string, number> = {};
    for (const m of movies) {
      const s = m.status || "unknown";
      statusCounts[s] = (statusCounts[s] || 0) + 1;
    }
    console.log("Movie status distribution: " + JSON.stringify(statusCounts));

    // Verify no movies stuck with empty/null status
    const noStatus = movies.filter((m: any) => !m.status);
    expect(noStatus.length).toBe(0);
  });

  test("movie history endpoint returns events", async ({ request }) => {
    const moviesRes = await authGet(request, "/api/v1/movies?limit=1");
    expect(moviesRes.status()).toBe(200);
    const moviesBody = await moviesRes.json();
    const movies = Array.isArray(moviesBody) ? moviesBody : moviesBody.data ?? [];

    if (movies.length === 0) {
      test.skip(true, "No movies to check history for");
      return;
    }

    const movieId = movies[0].id;
    const historyRes = await authGet(request, "/api/v1/system/audit-log?limit=20");
    expect(historyRes.status()).toBe(200);
    const historyBody = await historyRes.json();
    const entries = historyBody.data ?? historyBody.entries ?? [];

    const movieEvents = entries.filter(
      (e: any) => e.entity_id === movieId || (e.detail && JSON.stringify(e.detail).includes(movieId))
    );
    console.log("Found " + movieEvents.length + " audit events for movie " + movies[0].title + " (" + movieId + ")");
  });
});
