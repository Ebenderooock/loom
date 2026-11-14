import { test, expect, type Locator } from "@playwright/test";
import { mockBaseApp, mockSettings } from "./helpers/mock-api";

async function expectHighlightAligned(nav: Locator) {
  await expect
    .poll(() =>
      nav.evaluate((element) => {
        const highlight = element.querySelector("[data-nav-highlight]");
        const active = element.querySelector('[aria-current="page"]');
        if (!highlight || !active) return Infinity;
        const a = highlight.getBoundingClientRect();
        const b = active.getBoundingClientRect();
        return Math.max(
          Math.abs(a.top - b.top),
          Math.abs(a.height - b.height),
          Math.abs(a.left - b.left),
          Math.abs(a.width - b.width),
        );
      }),
    )
    .toBeLessThan(1);
}

test.describe("Navigation highlight", () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockSettings(page);
    await page.goto("/settings/general");
  });

  for (const { name, target } of [
    { name: "Primary", target: "Movies" },
    { name: "Settings sections", target: "Download Clients" },
  ]) {
    test(`slides the ${name} highlight to the selected item`, async ({
      page,
    }) => {
      const nav = page.getByRole("navigation", { name, exact: true });
      const highlight = nav.locator("[data-nav-highlight]");
      await expectHighlightAligned(nav);
      const originalTransform = await highlight.evaluate(
        (element) => getComputedStyle(element).transform,
      );

      // Freeze the real CSS transition halfway through to verify spatial motion.
      await highlight.evaluate((element) => {
        element.addEventListener(
          "transitionrun",
          () => {
            for (const animation of element.getAnimations()) {
              animation.pause();
              animation.currentTime = 100;
            }
          },
          { once: true },
        );
      });
      await nav.getByRole("link", { name: target, exact: true }).click();
      await expect
        .poll(() =>
          highlight.evaluate((element) =>
            element.getAnimations().some((a) => a.playState === "paused"),
          ),
        )
        .toBe(true);

      expect(
        await highlight.evaluate(
          (element) => getComputedStyle(element).transform,
        ),
      ).not.toBe(originalTransform);
      const distanceFromTarget = await highlight.evaluate((element) => {
        const active = element.parentElement!.querySelector(
          '[aria-current="page"]',
        )!;
        return Math.abs(
          element.getBoundingClientRect().top -
            active.getBoundingClientRect().top,
        );
      });
      expect(distanceFromTarget).toBeGreaterThan(1);

      await highlight.evaluate((element) =>
        element.getAnimations().forEach((animation) => animation.finish()),
      );
      await expectHighlightAligned(nav);

      if (name === "Primary") {
        await page.getByRole("button", { name: "Collapse sidebar" }).click();
        await expectHighlightAligned(nav);
      }
    });
  }

  test("moves immediately when reduced motion is enabled", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    const settings = page.getByRole("navigation", {
      name: "Settings sections",
    });
    await settings.getByRole("link", { name: "System Logs" }).click();
    await expectHighlightAligned(settings);
    expect(
      await settings
        .locator("[data-nav-highlight]")
        .evaluate((element) => element.getAnimations().length),
    ).toBe(0);

    const primary = page.getByRole("navigation", { name: "Primary" });
    await primary.getByRole("link", { name: "Movies", exact: true }).click();
    await expectHighlightAligned(primary);
    expect(
      await primary
        .locator("[data-nav-highlight]")
        .evaluate((element) => element.getAnimations().length),
    ).toBe(0);
  });
});
