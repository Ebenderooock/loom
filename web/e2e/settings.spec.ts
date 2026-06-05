import { test, expect } from "@playwright/test";
import { mockBaseApp, mockSettings } from "./helpers/mock-api";

test.describe("Settings Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockSettings(page);
  });

  test("renders settings page heading", async ({ page }) => {
    await page.goto("/settings");
    // useSetPageHeader("Settings") renders as <span> in header — hidden on mobile
    await expect(page.locator("header").getByText("Settings")).toBeAttached({
      timeout: 10000,
    });
  });

  test("shows library section", async ({ page }) => {
    await page.goto("/settings");
    // Libraries section is under "Media Management" tab, not "General"
    await page.getByText("Media Management").first().click();
    // Scope to main to avoid matching "Library" sidebar link
    await expect(
      page
        .locator("main")
        .getByText(/librar/i)
        .first(),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows download client section", async ({ page }) => {
    await page.goto("/settings");
    await expect(
      page
        .locator("main")
        .getByText(/download/i)
        .first(),
    ).toBeVisible({ timeout: 10000 });
  });
});

test.describe("Events Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
  });

  test("renders events page", async ({ page }) => {
    await page.goto("/events");
    // useSetPageHeader("Events") renders as <span> in header — hidden on mobile
    await expect(page.locator("header").getByText("Events")).toBeAttached({
      timeout: 10000,
    });
  });
});

test.describe("Calendar Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);

    await page.route("**/api/v1/calendar*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { data: [], total: 0 } });
      } else {
        await route.fallback();
      }
    });
  });

  test("renders calendar page", async ({ page }) => {
    await page.goto("/calendar");
    // useSetPageHeader("Calendar") renders as <span> in header — hidden on mobile
    await expect(page.locator("header").getByText("Calendar")).toBeAttached({
      timeout: 10000,
    });
  });
});

test.describe("Quality Profiles Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);

    await page.route("**/api/v1/quality-profiles*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: [] });
      } else {
        await route.fallback();
      }
    });

    await page.route("**/api/v1/custom-formats*", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: [] });
      } else {
        await route.fallback();
      }
    });
  });

  test("renders quality profiles page", async ({ page }) => {
    await page.goto("/quality-profiles");
    // This page renders a real <h1> directly, not via useSetPageHeader
    await expect(
      page.getByRole("heading", { name: /Quality Profiles/i, level: 1 }),
    ).toBeVisible({ timeout: 10000 });
  });
});
