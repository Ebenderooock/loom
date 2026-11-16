import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockSettings,
  mockWorkflows,
} from "./helpers/mock-api";

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
    await mockWorkflows(page);
    await mockGet(page, "features", { features: [] });
    await mockGet(page, "workflows/wf-3/events", []);
  });

  test("nested workflow detail keeps the same top and visible focus edges", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 600 });
    await page.goto("/settings/general");
    const nav = page.getByRole("navigation", { name: "Settings sections" });
    await expect(nav).toBeVisible();
    const generalTop = (await nav.boundingBox())!.y;
    await page.goto("/settings/workflows/wf-3");
    await expect(page.getByText("Test.Show.S01E05.HDTV")).toBeVisible();
    if (process.env.LOOM_SCREENSHOT_DIR) {
      await page.screenshot({
        path: `${process.env.LOOM_SCREENSHOT_DIR}/sidebar-${process.env.LOOM_SCREENSHOT_PHASE}.png`,
      });
    }
    expect(
      Math.abs((await nav.boundingBox())!.y - generalTop),
    ).toBeLessThanOrEqual(1);

    const link = nav.getByRole("link", { name: "General", exact: true });
    await link.focus();
    const metrics = await link.evaluate((el) => {
      const nav = el.closest("nav")!;
      const item = el.getBoundingClientRect();
      const bounds = nav.getBoundingClientRect();
      return {
        left: item.left - bounds.left,
        right: bounds.right - item.right,
        scrollWidth: nav.scrollWidth,
        clientWidth: nav.clientWidth,
        scrollHeight: nav.scrollHeight,
        clientHeight: nav.clientHeight,
        outlineStyle: getComputedStyle(el).outlineStyle,
        outlineWidth: parseFloat(getComputedStyle(el).outlineWidth),
      };
    });
    expect(metrics.left).toBeGreaterThanOrEqual(2);
    expect(metrics.right).toBeGreaterThanOrEqual(2);
    expect(metrics.scrollWidth).toBe(metrics.clientWidth);
    expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);
    expect(metrics.outlineStyle).not.toBe("none");
    expect(metrics.outlineWidth).toBeGreaterThanOrEqual(1);
    await nav.evaluate((el) => (el.scrollTop = el.scrollHeight));
    expect(await nav.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);

    await page.evaluate(
      () => (document.documentElement.style.fontSize = "20px"),
    );
    expect(await nav.evaluate((el) => el.scrollWidth)).toBe(
      await nav.evaluate((el) => el.clientWidth),
    );
    await nav.evaluate((el) => (el.scrollTop = el.scrollHeight));
    expect(await nav.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
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
