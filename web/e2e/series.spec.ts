import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockSeriesApi,
  SAMPLE_SERIES,
} from "./helpers/mock-api";

test.describe("Series Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockSeriesApi(page);
  });

  test("renders series page heading", async ({ page }) => {
    await page.goto("/series");
    // useSetPageHeader("TV Shows") renders as <span> in header — hidden on mobile
    await expect(
      page.locator("header").getByText("TV Shows"),
    ).toBeAttached({ timeout: 10000 });
  });

  test("displays series in the list", async ({ page }) => {
    await page.goto("/series");
    await expect(
      page.locator("main").getByText(SAMPLE_SERIES.title).first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows add series button", async ({ page }) => {
    await page.goto("/series");
    await expect(
      page.getByRole("button", { name: /add/i }),
    ).toBeVisible({ timeout: 10000 });
  });

  test("renders empty state when no series", async ({ page }) => {
    // Override with empty list
    await page.route("**/api/v1/series*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });

    await page.goto("/series");

    // Empty state shows "No series yet"
    await expect(
      page.getByText("No series yet"),
    ).toBeVisible({ timeout: 10000 });
  });
});
