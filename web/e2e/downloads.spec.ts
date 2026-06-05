import { test, expect } from "@playwright/test";
import { mockBaseApp, mockDownloads } from "./helpers/mock-api";

test.describe("Downloads Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockDownloads(page);

    // The ActiveDownloads component fetches /api/v1/activity
    await page.route("**/api/v1/activity", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { items: [] } });
      } else {
        await route.fallback();
      }
    });
  });

  test("renders downloads page heading", async ({ page }) => {
    await page.goto("/downloads");
    // useSetPageHeader("Downloads") renders as <span> in header — hidden on mobile
    await expect(page.locator("header").getByText("Downloads")).toBeAttached({
      timeout: 10000,
    });
  });

  test("shows empty queue state", async ({ page }) => {
    await page.goto("/downloads");

    // Empty state shows "No active downloads" — scope to main to avoid
    // the duplicate in QueueStats <span>
    await expect(
      page.locator("main").getByText("No active downloads").first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows active downloads when present", async ({ page }) => {
    // Override with active downloads
    await page.route("**/api/v1/activity", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: {
            items: [
              {
                id: "dl-1",
                title: "Test Movie Download",
                status: "downloading",
                progress: 0.455,
                size: 2000000000,
                download_rate: 5000000,
                upload_rate: 0,
                eta_seconds: 1800,
                client_id: "dc-1",
                category: "movies",
              },
            ],
          },
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto("/downloads");

    await expect(page.getByText("Test Movie Download")).toBeVisible({
      timeout: 10000,
    });
  });
});
