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
    await expect(page.locator("header").getByText("Dashboard")).toBeAttached({
      timeout: 10000,
    });
  });

  test("shows movie count stat card", async ({ page }) => {
    await page.goto("/");
    // Stat card label "Movies" rendered via StatCard
    await expect(page.locator("main").getByText("Movies").first()).toBeVisible({
      timeout: 10000,
    });
  });

  test("shows series count stat card", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.locator("main").getByText("TV Shows").first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows indexer count stat card", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.locator("main").getByText("Indexers").first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows download client count", async ({ page }) => {
    await page.goto("/");
    await expect(
      page.locator("main").getByText("Download Clients"),
    ).toBeVisible({ timeout: 10000 });
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
    await expect(page.getByText("Get started in a few steps")).toBeVisible({
      timeout: 10000,
    });
  });

  test("shared storage counts capacity once and labels library usage", async ({
    page,
  }, testInfo) => {
    await mockGet(page, "libraries", {
      data: ["Movies", "TV", "Anime"].map((name, index) => ({
        ...SAMPLE_LIBRARY,
        id: `lib-${index}`,
        name,
        size_bytes: (index + 1) * 1e12,
        disk_space: {
          volume_id: "shared",
          total_bytes: 20e12,
          free_bytes: 7.1e12,
          used_bytes: 12.9e12,
        },
      })),
    });
    await page.goto("/");
    const volume = page.getByRole("region", { name: "Storage volume" });
    await expect(volume).toBeVisible();
    await expect(page.locator("main").getByRole("region")).toHaveCount(1);
    await expect(volume.getByRole("img")).toHaveAccessibleName(
      "Storage volume: 6.5 TB free of 18.2 TB, 65% used",
    );
    for (const name of ["Movies", "TV", "Anime", "Other usage"]) {
      await expect(volume.getByText(name, { exact: true })).toBeVisible();
    }
    await expect(volume).not.toContainText("estimated");
    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      )
      .toBe(true);
    await page.screenshot({
      path: testInfo.outputPath("shared-storage.png"),
      fullPage: true,
    });
  });

  test("separate storage shows a card for each volume", async ({ page }) => {
    await mockGet(page, "libraries", {
      data: [
        { ...SAMPLE_LIBRARY, size_bytes: 2e11 },
        {
          ...SAMPLE_LIBRARY,
          id: "tv",
          name: "TV",
          size_bytes: 3e11,
          disk_space: {
            total_bytes: 2e12,
            free_bytes: 1e12,
            used_bytes: 1e12,
          },
        },
      ],
    });
    await page.goto("/");
    await expect(page.getByRole("region", { name: "Volume 1" })).toBeVisible();
    await expect(page.getByRole("region", { name: "Volume 2" })).toBeVisible();
    await expect(page.getByText(/across 2 volumes/)).toHaveText(
      "1.4 TB free of 2.7 TB across 2 volumes",
    );
  });
});
