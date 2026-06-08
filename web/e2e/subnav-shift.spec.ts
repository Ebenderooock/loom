import { test, expect } from "@playwright/test";
import { mockBaseApp, mockSettings } from "./helpers/mock-api";

// Regression test for the Settings sub-nav jumping down on tall panels.
// Root cause was `<main>` using `overflow-x-hidden`, which forces overflow-y to
// compute to `auto`, making <main> the scroll container; the sub-nav's
// `sticky top-20` was then measured from <main>'s content box and landed 80px
// too low on tall panels (e.g. Libraries & Naming). Animations are disabled so
// the `page-enter` transform can't perturb the measurement.
test.use({ reducedMotion: "reduce" });

test.describe("Settings sub-nav stability", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockSettings(page);
  });

  test("sub-nav top stays fixed across section switches", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/settings");

    const subnav = page.getByRole("navigation", {
      name: "Settings sections",
    });
    await expect(subnav).toBeVisible({ timeout: 10000 });

    const tops: number[] = [];
    const sections = [
      "General",
      "Libraries & Naming", // tall panel that previously triggered the jump
      "Indexers",
      "Notifications",
      "Workflows",
      "General",
      "Appearance",
    ];

    for (const label of sections) {
      await subnav.getByRole("link", { name: label, exact: true }).click();
      await page.waitForTimeout(250);
      const box = await subnav.boundingBox();
      tops.push(Math.round(box?.y ?? -1));
    }

    const delta = Math.max(...tops) - Math.min(...tops);
    expect(delta, `sub-nav tops: ${tops.join(", ")}`).toBeLessThanOrEqual(1);

    // It must also remain sticky (pinned just below the header) while scrolling
    // a tall panel.
    await subnav
      .getByRole("link", { name: "Libraries & Naming", exact: true })
      .click();
    await page.waitForTimeout(250);
    await page.evaluate(() => window.scrollTo({ top: 400 }));
    await page.waitForTimeout(150);
    const stuckTop = (await subnav.boundingBox())?.y ?? -1;
    const headerTop =
      (await page.locator("header").first().boundingBox())?.y ?? -1;
    expect(stuckTop).toBeGreaterThanOrEqual(56);
    expect(stuckTop).toBeLessThanOrEqual(96);
    expect(headerTop).toBe(0);
  });
});
