import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad } from "./helpers";

test.describe("Live: Authentication", () => {
  test("can log in and reach the dashboard", async ({ page }) => {
    await liveLogin(page);
    // Dashboard should show key sections
    await expect(page.locator("main")).toBeVisible({ timeout: 15000 });
  });

  test("auth status API returns valid response", async ({ request }) => {
    const res = await request.get("/api/v1/auth/status");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body).toHaveProperty("is_authenticated");
    expect(body).toHaveProperty("setup_required");
  });
});
