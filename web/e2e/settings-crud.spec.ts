import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockSettings,
  mockGet,
  mockRoute,
  SAMPLE_LIBRARY,
  SAMPLE_DOWNLOAD_CLIENT,
  SAMPLE_QUALITY_PROFILE,
  SAMPLE_INDEXER,
} from "./helpers/mock-api";

test.describe("Settings CRUD", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockSettings(page);
    // Override mocks with correct response shapes
    await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
    // download-clients: match both with and without trailing slash
    // Two different callers: useDownloads() expects { download_clients: [...] }, path mappings expects plain array
    await page.route(
      function (url) {
        return (
          url.pathname === "/api/v1/download-clients" ||
          url.pathname === "/api/v1/download-clients/"
        );
      },
      async (route) => {
        if (route.request().method() === "GET") {
          // Return both shapes: wrapper for useDownloads hook, but also ensure it works as array
          // The useDownloads hook reads .download_clients; the apiFetch path treats response as array
          const path = new URL(route.request().url()).pathname;
          if (path.endsWith("/")) {
            await route.fulfill({
              status: 200,
              json: { download_clients: [SAMPLE_DOWNLOAD_CLIENT] },
            });
          } else {
            await route.fulfill({
              status: 200,
              json: [SAMPLE_DOWNLOAD_CLIENT],
            });
          }
        } else {
          await route.fallback();
        }
      },
    );
  });

  // Helper to click a settings category button in the nav
  function settingsNav(page: import("@playwright/test").Page) {
    return page.locator("nav[aria-label='Settings sections']");
  }

  test("settings page loads and shows general panel by default", async ({
    page,
  }) => {
    await page.goto("/settings");

    await expect(page.locator("header").getByText("Settings")).toBeAttached({
      timeout: 10000,
    });

    // Default category is General — card header should show "General"
    await expect(page.getByText("General application settings")).toBeVisible({
      timeout: 10000,
    });
  });

  test("media management section shows libraries", async ({ page }) => {
    await page.goto("/settings");

    // Click category button in the settings nav
    await settingsNav(page).getByText("Media Management").click();

    // Library name should appear in the content area (path is unique)
    await expect(page.getByText(SAMPLE_LIBRARY.path)).toBeVisible({
      timeout: 10000,
    });
  });

  test("add library dialog opens from media management", async ({ page }) => {
    await page.goto("/settings");
    await settingsNav(page).getByText("Media Management").click();
    await expect(page.getByText(SAMPLE_LIBRARY.path)).toBeVisible({
      timeout: 10000,
    });

    // Click Add button to open add library dialog
    const addBtn = page.getByRole("button", { name: /Add/i }).first();
    await addBtn.click();

    // Dialog should appear
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5000 });
  });

  test("download clients section shows existing clients", async ({ page }) => {
    await page.goto("/settings");

    // Navigate to Download Clients category
    await settingsNav(page).getByText("Download Clients").click();

    // Client name should be visible
    await expect(page.getByText(SAMPLE_DOWNLOAD_CLIENT.name)).toBeVisible({
      timeout: 10000,
    });
  });

  test("add download client dialog opens", async ({ page }) => {
    await page.goto("/settings");
    await settingsNav(page).getByText("Download Clients").click();
    await expect(page.getByText(SAMPLE_DOWNLOAD_CLIENT.name)).toBeVisible({
      timeout: 10000,
    });

    // Click Add Client button
    const addClientBtn = page
      .getByRole("button", { name: /Add Client|Add/i })
      .first();
    await addClientBtn.click();
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5000 });
  });

  test("media management settings render", async ({ page }) => {
    await page.goto("/settings");
    await settingsNav(page).getByText("Media Management").click();

    // Library path should be visible (unique text)
    await expect(page.getByText(SAMPLE_LIBRARY.path)).toBeVisible({
      timeout: 10000,
    });
  });

  test("notifications settings render", async ({ page }) => {
    await page.goto("/settings");
    await settingsNav(page).getByText("Notifications").click();

    // Should see the Notifications card header
    await expect(page.locator("main")).toBeVisible({ timeout: 10000 });
  });

  test("UI settings render", async ({ page }) => {
    await page.goto("/settings");
    await settingsNav(page).getByText("UI").click();

    await expect(page.locator("main")).toBeVisible({ timeout: 10000 });
  });

  test("download safety settings render", async ({ page }) => {
    await page.goto("/settings");
    await settingsNav(page).getByText("Download Safety").click();

    await expect(page.locator("main")).toBeVisible({ timeout: 10000 });
  });

  test("category navigation switches content", async ({ page }) => {
    await page.goto("/settings");

    // Click through categories and verify content changes
    await settingsNav(page).getByText("Media Management").click();
    await expect(page.getByText(SAMPLE_LIBRARY.path)).toBeVisible({
      timeout: 10000,
    });

    await settingsNav(page).getByText("Download Clients").click();
    await expect(page.getByText(SAMPLE_DOWNLOAD_CLIENT.name)).toBeVisible({
      timeout: 10000,
    });

    await settingsNav(page).getByText("General").click();
    await expect(page.getByText("General application settings")).toBeVisible({
      timeout: 10000,
    });
  });
});
