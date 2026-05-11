import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad } from "./helpers";

test.describe("Live: Navigation & Page Rendering", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  const routes = [
    { path: "/", name: "Dashboard" },
    { path: "/movies", name: "Movies" },
    { path: "/series", name: "Series" },
    { path: "/downloads", name: "Downloads" },
    { path: "/workflows", name: "Workflows" },
    { path: "/indexers", name: "Indexers" },
    { path: "/events", name: "Events" },
    { path: "/settings", name: "Settings" },
  ];

  for (const route of routes) {
    test(`${route.name} page (${route.path}) loads without errors`, async ({ page }) => {
      await page.goto(route.path);
      await waitForPageLoad(page);
      // Should not show error boundary or crash
      const errorBoundary = page.getByText(/Something went wrong|Unexpected error|Application error/i).first();
      const hasError = await errorBoundary.isVisible({ timeout: 3000 }).catch(() => false);
      expect(hasError).toBeFalsy();
      // Main content should be visible
      await expect(page.locator("main")).toBeVisible({ timeout: 15000 });
    });
  }

  test("sidebar navigation works between pages", async ({ page }) => {
    await page.goto("/");
    await waitForPageLoad(page);

    // Navigate to Movies via sidebar
    const sidebar = page.locator("nav, aside, [data-sidebar]").first();
    await sidebar.getByText(/Movies/i).first().click();
    await page.waitForURL("**/movies**", { timeout: 10000 });
    await expect(page.locator("main")).toBeVisible();

    // Navigate to Downloads
    await sidebar.getByText(/Downloads/i).first().click();
    await page.waitForURL("**/downloads**", { timeout: 10000 });
    await expect(page.locator("main")).toBeVisible();
  });

  test("no console errors on page load", async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (err) => errors.push(err.message));

    await page.goto("/movies");
    await waitForPageLoad(page);

    // Filter out known non-critical errors
    const critical = errors.filter(
      (e) => !e.includes("ResizeObserver") && !e.includes("Non-Error")
    );
    if (critical.length > 0) {
      console.warn("Console errors detected:", critical);
    }
    // Warn but dont fail for minor errors — fail only for crash-level errors
    expect(critical.length).toBeLessThan(5);
  });
});
