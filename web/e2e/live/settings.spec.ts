import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Settings", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("settings page loads with sections", async ({ page }) => {
    await page.goto("/settings");
    await waitForPageLoad(page);
    await expect(page.getByText(/Settings/i).first()).toBeVisible({
      timeout: 15000,
    });
    await expect(
      page.getByText(/Libraries|Download|General/i).first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("libraries API returns configured libraries", async ({ request }) => {
    const res = await authGet(request, "/api/v1/libraries");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const libs = Array.isArray(body) ? body : (body.data ?? []);
    expect(Array.isArray(libs)).toBeTruthy();
    if (libs.length > 0) {
      expect(libs[0]).toHaveProperty("id");
      expect(libs[0]).toHaveProperty("name");
      expect(libs[0]).toHaveProperty("path");
      expect(libs[0]).toHaveProperty("media_type");
    }
  });

  test("notifications API is reachable", async ({ request }) => {
    const res = await authGet(request, "/api/v1/notifications");
    expect(res.status()).toBe(200);
  });
});
