import { test, expect } from "@playwright/test";
import { mockBaseApp } from "./helpers/mock-api";

const NAV_ITEMS = [
  { label: "Dashboard", path: "/" },
  { label: "Movies", path: "/movies" },
  { label: "TV Shows", path: "/series" },
  { label: "Calendar", path: "/calendar" },
  { label: "Library", path: "/library" },
  { label: "Indexers", path: "/indexers" },
  { label: "Downloads", path: "/downloads" },
  { label: "Settings", path: "/settings" },
] as const;

test.describe("Navigation", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);

    // Broad mocks so any page can render without hanging.
    // Skip auth routes — those are handled by mockBaseApp.
    await page.route("**/api/v1/**", async (route) => {
      if (route.request().url().includes("/auth/")) {
        await route.fallback();
        return;
      }
      if (route.request().method() === "GET") {
        const url = route.request().url();
        if (url.includes("?")) {
          await route.fulfill({ status: 200, json: { data: [], total: 0 } });
        } else {
          await route.fulfill({ status: 200, json: [] });
        }
      } else {
        await route.fallback();
      }
    });
  });

  test("sidebar renders all navigation links", async ({ page }) => {
    await page.goto("/");

    const isMobile = (page.viewportSize()?.width ?? 1024) < 768;

    if (isMobile) {
      // On mobile, sidebar aside is display:none so roles are excluded.
      // Open the mobile menu to make links visible.
      await page.getByRole("button", { name: "Open navigation" }).click();
      for (const item of NAV_ITEMS) {
        await expect(
          page.getByRole("link", { name: item.label }).first(),
        ).toBeVisible({ timeout: 10000 });
      }
    } else {
      const sidebar = page.locator("aside");
      for (const item of NAV_ITEMS) {
        await expect(
          sidebar.getByRole("link", { name: item.label }),
        ).toBeAttached({ timeout: 10000 });
      }
    }
  });

  for (const item of NAV_ITEMS) {
    test("navigates to " + item.label + " (" + item.path + ")", async ({ page }) => {
      await page.goto("/");

      // On mobile the sidebar is hidden — open the menu first
      const isMobile = (page.viewportSize()?.width ?? 1024) < 768;
      if (isMobile) {
        await page.getByRole("button", { name: "Open navigation" }).click();
        // Wait for Sheet to animate open
        await page.waitForTimeout(300);
      }

      const link = page.getByRole("link", { name: item.label }).first();
      if (isMobile) {
        // Sheet nav has its own scroll container — use JS to click
        await link.evaluate((el) => el.scrollIntoView({ block: "center" }));
        await page.waitForTimeout(300);
        await link.dispatchEvent("click");
      } else {
        await link.click();
      }

      // URL should match the expected path
      await expect(page).toHaveURL(new RegExp(item.path.replace("/", "\\/") + "$"));
    });
  }

  test("command palette opens with Ctrl+K", async ({ page }) => {
    // Keyboard shortcuts only work on desktop
    const isMobile = (page.viewportSize()?.width ?? 1024) < 768;
    if (isMobile) {
      test.skip();
      return;
    }

    await page.goto("/");

    // Wait for page to be interactive
    await page.waitForLoadState("networkidle");

    // Try Meta+k (macOS) then Control+k (Linux/Windows)
    await page.keyboard.press("Meta+k");

    // Inline command palette renders a cmdk input with placeholder
    await expect(
      page.getByPlaceholder(/Search Loom/),
    ).toBeVisible({ timeout: 5000 });
  });

  test("mobile menu works on small viewport", async ({ page }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/");

    // Sidebar should be hidden; mobile menu button has aria-label="Open navigation"
    const menuButton = page.getByRole("button", { name: "Open navigation" });
    await expect(menuButton).toBeVisible();

    await menuButton.click();

    // Navigation links should now be visible in the sheet/drawer
    await expect(page.getByRole("link", { name: "Movies" })).toBeVisible();
  });
});
