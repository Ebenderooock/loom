import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Movies", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("movies page loads and displays movies", async ({ page }) => {
    await page.goto("/movies");
    await waitForPageLoad(page);
    await expect(page.getByText(/Movies/i).first()).toBeVisible({ timeout: 15000 });
    // Should have at least one movie poster link or an empty state
    const movieCards = page.locator("main a[href*='/movies/']");
    const emptyState = page.getByText(/No movies|Add your first|Add Movie/i).first();
    const hasMovies = await movieCards.count().then(c => c > 0);
    const hasEmpty = await emptyState.isVisible({ timeout: 3000 }).catch(() => false);
    expect(hasMovies || hasEmpty).toBeTruthy();
  });

  test("movies API returns valid data", async ({ request }) => {
    const res = await authGet(request, "/api/v1/movies?limit=10");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const movies = Array.isArray(body) ? body : body.data ?? [];
    expect(Array.isArray(movies)).toBeTruthy();
    if (movies.length > 0) {
      const m = movies[0];
      expect(m).toHaveProperty("id");
      expect(m).toHaveProperty("title");
      expect(m).toHaveProperty("status");
    }
  });

  test("movie detail opens on card click", async ({ page }) => {
    await page.goto("/movies");
    await waitForPageLoad(page);
    // Target actual movie poster cards (links to movie detail pages)
    const firstCard = page.locator("main a[href*='/movies/']").first();
    const cardVisible = await firstCard.isVisible({ timeout: 10000 }).catch(() => false);
    if (!cardVisible) {
      test.skip(true, "No movies to open detail for");
      return;
    }
    await firstCard.click();
    // Should navigate to movie detail page or open a sheet
    // Wait for URL change or a detail view to appear
    await page.waitForURL(/\/movies\/[a-zA-Z0-9-]+/, { timeout: 10000 }).catch(() => null);
    const detailContent = page.locator("[role=dialog], [data-state=open], main h1, main h2, main [class*=detail]").first();
    await expect(detailContent).toBeVisible({ timeout: 10000 });
  });

  test("movie statuses are valid values", async ({ request }) => {
    const res = await authGet(request, "/api/v1/movies?limit=50");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const movies = Array.isArray(body) ? body : body.data ?? [];
    const validStatuses = [
      "missing", "available_right_quality", "available_wrong_quality",
      "downloading", "grabbed", "announced", "tba",
    ];
    for (const m of movies) {
      if (m.status) {
        expect(validStatuses).toContain(m.status);
      }
    }
  });

  test("quality profiles API returns data", async ({ request }) => {
    const res = await authGet(request, "/api/v1/quality-profiles");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const profiles = Array.isArray(body) ? body : body.data ?? [];
    expect(profiles.length).toBeGreaterThan(0);
    expect(profiles[0]).toHaveProperty("id");
    expect(profiles[0]).toHaveProperty("name");
  });

  test("filter toolbar renders and works", async ({ page }) => {
    await page.goto("/movies");
    await waitForPageLoad(page);
    const filterInput = page.getByPlaceholder(/filter/i).first();
    await expect(filterInput).toBeVisible({ timeout: 10000 });
  });
});
