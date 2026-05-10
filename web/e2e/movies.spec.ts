import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockMovies,
  SAMPLE_MOVIE,
  SAMPLE_LIBRARY,
} from "./helpers/mock-api";

test.describe("Movies Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockMovies(page);
    await page.goto("/movies");
  });

  test("movies page loads when authenticated", async ({ page }) => {
    // useSetPageHeader("Movies") renders as a <span> in header — hidden on mobile
    await expect(
      page.locator("header").getByText("Movies"),
    ).toBeAttached({ timeout: 10000 });
  });

  test("displays sample movie in the list", async ({ page }) => {
    // The mock returns SAMPLE_MOVIE with title "Test Movie" — may appear
    // in grid card + toolbar subtitle, so scope to main and use .first()
    await expect(
      page.locator("main").getByText(SAMPLE_MOVIE.title).first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows add movie button", async ({ page }) => {
    // Toolbar has an "Add Movie" button
    await expect(
      page.getByRole("button", { name: /Add Movie/i }),
    ).toBeVisible({ timeout: 10000 });
  });

  test("renders empty state when no movies", async ({ page }) => {
    // Override with empty movie list
    await page.route("**/api/v1/movies*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });

    await page.goto("/movies");

    // Empty state shows "No movies yet"
    await expect(
      page.getByText("No movies yet"),
    ).toBeVisible({ timeout: 10000 });
  });

  test("empty state shows import button when libraries exist", async ({ page }) => {
    // Override with empty movie list
    await page.route("**/api/v1/movies*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });

    // listLibraries() expects { data: [...] } shape
    await page.route("**/api/v1/libraries", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: { data: [SAMPLE_LIBRARY] },
        });
      } else {
        await route.fallback();
      }
    });

    await page.goto("/movies");

    // Wait for the empty state to render
    await expect(
      page.getByText("No movies yet"),
    ).toBeVisible({ timeout: 10000 });

    await expect(
      page.getByRole("button", { name: /Import Existing/i }),
    ).toBeVisible({ timeout: 10000 });

    await expect(
      page.getByRole("button", { name: /Add Movie/i }),
    ).toBeVisible({ timeout: 10000 });
  });

  test("empty state shows library warning when no libraries", async ({ page }) => {
    // Override with empty movies and empty libraries
    await page.route("**/api/v1/movies*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });
    await page.route("**/api/v1/libraries", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [] } });
      } else {
        await route.fallback();
      }
    });

    await page.goto("/movies");

    // Shows warning about needing a library
    await expect(
      page.getByText(/Add a library in Settings/i),
    ).toBeVisible({ timeout: 10000 });
  });
});
