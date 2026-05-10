import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockDashboard,
  mockGet,
  SAMPLE_MOVIE,
  SAMPLE_SERIES,
  SAMPLE_INDEXER,
  SAMPLE_DOWNLOAD_CLIENT,
  SAMPLE_LIBRARY,
} from "./helpers/mock-api";

test.describe("Dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockDashboard(page);
  });

  test("renders dashboard heading", async ({ page }) => {
    await page.goto("/");
    // useSetPageHeader("Dashboard") renders as a <span>, not a heading.
    // On mobile the title is hidden by CSS, so use toBeAttached.
    await expect(
      page.locator("header").getByText("Dashboard"),
    ).toBeAttached({ timeout: 10000 });
  });

  test("shows movie count stat card", async ({ page }) => {
    await page.goto("/");
    // Stat card label "Movies" rendered via StatCard
    await expect(page.locator("main").getByText("Movies").first()).toBeVisible({ timeout: 10000 });
  });

  test("shows series count stat card", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("main").getByText("TV Shows").first()).toBeVisible({ timeout: 10000 });
  });

  test("shows indexer count stat card", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("main").getByText("Indexers").first()).toBeVisible({ timeout: 10000 });
  });

  test("shows download client count", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("main").getByText("Download Clients")).toBeVisible({ timeout: 10000 });
  });

  test("command palette trigger is visible", async ({ page }) => {
    await page.goto("/");
    // Button has aria-label="Open command palette"
    await expect(
      page.getByRole("button", { name: "Open command palette" }),
    ).toBeVisible();
  });

  test("getting started section shows when no libraries", async ({ page }) => {
    // Override mocks to return empty data so isFreshInstall = true
    await page.route("**/api/v1/libraries", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: [] });
      } else {
        await route.fallback();
      }
    });
    await page.route("**/api/v1/movies?limit=1", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });
    await page.route("**/api/v1/series?limit=1", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });

    await page.goto("/");

    // WelcomeSection shows "Get started in a few steps"
    await expect(
      page.getByText("Get started in a few steps"),
    ).toBeVisible({ timeout: 10000 });
  });
});
