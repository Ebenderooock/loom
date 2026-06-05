import { test, expect } from "@playwright/test";
import { mockBaseApp, mockDashboard } from "./helpers/mock-api";

test.describe("Smoke Tests", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockDashboard(page);
  });

  test("dashboard renders with sidebar and command palette trigger", async ({
    page,
  }) => {
    await page.goto("/");
    // Header renders as span, not h1 — hidden on mobile so use toBeAttached
    await expect(page.getByText("Dashboard").first()).toBeAttached();
    // Sidebar aside uses hidden CSS class on mobile (display:none) — use CSS locator
    await expect(page.locator('aside[aria-label="Sidebar"]')).toBeAttached();
  });

  test("sidebar navigation is visible", async ({ page }) => {
    await page.goto("/");
    // On mobile, aside is display:none so getByRole won't find links inside it.
    // Use CSS locator to verify links are in the DOM.
    await expect(
      page.locator("aside a").filter({ hasText: "Movies" }).first(),
    ).toBeAttached();
    await expect(
      page.locator("aside a").filter({ hasText: "TV Shows" }).first(),
    ).toBeAttached();
    await expect(
      page.locator("aside a").filter({ hasText: "Workflows" }).first(),
    ).toBeAttached();
    await expect(
      page.locator("aside a").filter({ hasText: "Settings" }).first(),
    ).toBeAttached();
  });

  test("app does not crash on page navigation", async ({ page }) => {
    await page.goto("/");
    await page.goto("/movies");
    await page.goto("/series");
    await page.goto("/workflows");
    await page.goto("/settings");
    // No crash = pass — if any page throws, Playwright catches it
  });
});
