import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("dashboard loads with sidebar navigation", async ({ page }) => {
    await page.goto("/");
    await waitForPageLoad(page);
    // Sidebar should have navigation links
    const sidebar = page.locator("nav, aside, [data-sidebar]").first();
    await expect(sidebar).toBeVisible();
    // Should have at least Movies and Series nav items
    await expect(sidebar.getByText(/Movies/i).first()).toBeVisible({
      timeout: 10000,
    });
  });

  test("system status API is healthy", async ({ request }) => {
    const res = await authGet(request, "/api/v1/system/status");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty("version");
    expect(body.version).toBeTruthy();
  });
});
