import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Series", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("series page loads", async ({ page }) => {
    await page.goto("/series");
    await waitForPageLoad(page);
    await expect(page.getByText(/Series/i).first()).toBeVisible({ timeout: 15000 });
  });

  test("series API returns valid data", async ({ request }) => {
    const res = await authGet(request, "/api/v1/series?limit=10");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const series = Array.isArray(body) ? body : body.data ?? [];
    expect(Array.isArray(series)).toBeTruthy();
    if (series.length > 0) {
      expect(series[0]).toHaveProperty("id");
      expect(series[0]).toHaveProperty("title");
    }
  });
});
