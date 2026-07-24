import { test, expect } from "@playwright/test";
import { mockBaseApp, mockSeriesApi, SAMPLE_SERIES } from "./helpers/mock-api";

test.describe("Series Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockSeriesApi(page);
  });

  test("renders series page heading", async ({ page }) => {
    await page.goto("/series");
    // useSetPageHeader("TV Shows") renders as <span> in header — hidden on mobile
    await expect(page.locator("header").getByText("TV Shows")).toBeAttached({
      timeout: 10000,
    });
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
      page.getByRole("button", { name: /^Add Series$/i }),
    ).toBeVisible({
      timeout: 10000,
    });
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
    await expect(page.getByText("No series yet")).toBeVisible({
      timeout: 10000,
    });
  });

  test.describe("Series Page mobile add flow", () => {
    test.use({ viewport: { width: 393, height: 851 } });

    test("populated library keeps Add Series reachable and opens dialog", async ({
      page,
    }) => {
      await mockBaseApp(page);
      await mockSeriesApi(page);
      await page.goto("/series");

      const addSeriesButton = page
        .getByRole("button", { name: /^Add Series$/i })
        .first();
      await expect(addSeriesButton).toBeVisible({ timeout: 10000 });
      await addSeriesButton.click();
      await expect(
        page.getByRole("heading", { name: "Search Series" }),
      ).toBeVisible({ timeout: 10000 });
    });

    test("empty library keeps Add Series reachable and opens dialog", async ({
      page,
    }) => {
      await mockBaseApp(page);
      await mockSeriesApi(page, []);
      await page.goto("/series");

      const addSeriesButton = page
        .getByRole("button", { name: /^Add Series$/i })
        .first();
      await expect(addSeriesButton).toBeVisible({ timeout: 10000 });
      await addSeriesButton.click();
      await expect(
        page.getByRole("heading", { name: "Search Series" }),
      ).toBeVisible({ timeout: 10000 });
    });

    test("mobile overflow menu keeps secondary actions reachable", async ({
      page,
    }) => {
      await mockBaseApp(page);
      await mockSeriesApi(page);
      await page.goto("/series");

      const menuButton = page.getByRole("button", { name: "More actions" });
      await expect(menuButton).toBeVisible({ timeout: 10000 });
      await menuButton.click();
      await expect(
        page.getByRole("menuitem", { name: "Organize" }),
      ).toBeVisible({
        timeout: 10000,
      });
      await expect(
        page.getByRole("menuitem", { name: "Rescan Libraries" }),
      ).toBeVisible({ timeout: 10000 });
    });
  });
});
