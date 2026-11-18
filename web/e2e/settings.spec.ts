import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet, mockSettings } from "./helpers/mock-api";

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
  test.use({ reducedMotion: "reduce" });

  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockGet(page, "features", { features: [] });
    await mockGet(page, "movies/quality-definitions", []);

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

  for (const viewport of [
    { width: 1280, height: 720 },
    { width: 375, height: 667 },
  ]) {
    test(`keeps custom format scores keyboard reachable at ${viewport.width}x${viewport.height}`, async ({
      page,
    }) => {
      await page.setViewportSize(viewport);
      const definitions = Array.from({ length: 24 }, (_, i) => ({
        id: `quality-${i}`,
        name: `Quality ${i}`,
        title: `Quality ${i}`,
        source: i < 12 ? "Web" : "BluRay",
        resolution: "1080p",
        preferred_at: i,
      }));
      const formats = Array.from({ length: 20 }, (_, i) => ({
        id: `format-${i}`,
        name: `Custom format ${i}`,
        specifications: [],
      }));
      await mockGet(page, "movies/quality-definitions", definitions);
      await mockGet(page, "custom-formats", { data: formats });
      await page.goto("/settings/quality-profiles");
      await page.getByRole("button", { name: "Add Profile" }).click();
      const dialog = page.getByRole("dialog");
      const heading = dialog.getByRole("heading", {
        name: "Add Quality Profile",
      });
      await dialog.getByLabel("Profile Name").fill("Layout test");
      const headerBounds = await heading.boundingBox();
      await expect(dialog.getByText("Qualities (0/24)")).toBeVisible();
      await dialog
        .getByRole("button", { name: "Select All", exact: true })
        .click();
      await dialog.getByLabel("Quality cutoff").selectOption("quality-0");
      await dialog.getByLabel("Quality cutoff").focus();
      const create = dialog.getByRole("button", {
        name: "Create",
        exact: true,
      });
      const footerBounds = await create.boundingBox();

      for (let i = 0; i < formats.length; i++) {
        await page.keyboard.press("Tab");
        const score = dialog.getByLabel(`Custom format ${i} score (points)`, {
          exact: true,
        });
        await expect(score).toBeFocused();
        await expect(score).toBeInViewport();
        // Viewport intersection alone cannot detect clipping by an ancestor.
        expect(
          await score.evaluate((input) => {
            const body = input.closest('[role="region"]')!;
            const bounds = body.getBoundingClientRect();
            const rect = input.getBoundingClientRect();
            // Browser focus scrolling can round fractional CSS pixels.
            return (
              rect.top >= bounds.top - 1 && rect.bottom <= bounds.bottom + 1
            );
          }),
        ).toBe(true);
      }
      await page.keyboard.press("Tab");
      await expect(
        dialog.getByLabel("Minimum custom format score (points)"),
      ).toBeFocused();
      await expect(create).toBeInViewport();
      await expect(heading).toBeInViewport();
      expect(await heading.boundingBox()).toEqual(headerBounds);
      expect(await create.boundingBox()).toEqual(footerBounds);
      expect(
        await dialog.evaluate((element) => ({
          horizontalOverflow: element.scrollWidth > element.clientWidth,
          dialogScrolls: element.scrollHeight > element.clientHeight,
          bodyScrolls: (() => {
            const body = element.querySelector('[role="region"]')!;
            return body.scrollHeight > body.clientHeight;
          })(),
        })),
      ).toEqual({
        horizontalOverflow: false,
        dialogScrolls: false,
        bodyScrolls: true,
      });
      await page.keyboard.press("Tab");
      await expect(
        dialog.getByRole("button", { name: "Cancel" }),
      ).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(create).toBeFocused();
      await page.keyboard.press("Escape");
      await expect(dialog).not.toBeVisible();
    });
  }
});

test("custom format tester explains that matches are not profile scores or import approval", async ({
  page,
}) => {
  await mockBaseApp(page);
  await mockGet(page, "features", { features: [] });
  await mockGet(page, "custom-formats", { data: [] });
  await mockGet(page, "quality-profiles", { data: [] });
  let submitted: unknown;
  await page.route("**/api/v1/custom-formats/test", async (route) => {
    submitted = route.request().postDataJSON();
    await route.fulfill({
      json: {
        release: { quality: "unknown-1080p", resolution: "1080p" },
        matches: [
          {
            custom_format_id: "match",
            custom_format_name: "Title match",
            score: 0,
          },
        ],
        score: 0,
      },
    });
  });
  await page.goto("/settings/custom-formats");
  await page.getByRole("button", { name: "Test", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toHaveAccessibleDescription(
    /Custom formats do not replace the detected base quality.*does not confirm that a release can be imported/,
  );
  await dialog
    .getByRole("textbox", { name: "Release title" })
    .fill("Example.Show.S01E01.1080p");
  await dialog.getByRole("button", { name: "Test", exact: true }).click();
  await expect(dialog.getByText("Title match", { exact: true })).toBeVisible();
  await expect(
    dialog.getByText("Match-only result — no profile score calculated."),
  ).toBeVisible();
  expect(submitted).toEqual({ title: "Example.Show.S01E01.1080p" });
  await expect(dialog.getByText(/score:\s*0/i)).toHaveCount(0);
});

test("custom format tester displays profile-assigned points and minimum outcome", async ({
  page,
}) => {
  await mockBaseApp(page);
  await mockGet(page, "features", { features: [] });
  await mockGet(page, "custom-formats", { data: [] });
  await mockGet(page, "quality-profiles", {
    data: [{ id: "profile", name: "Test profile" }],
  });
  let submitted: unknown;
  await page.route("**/api/v1/custom-formats/test", async (route) => {
    submitted = route.request().postDataJSON();
    await route.fulfill({
      json: {
        release: { quality: "unknown-1080p", resolution: "1080p" },
        quality_profile_id: "profile",
        matches: [
          {
            custom_format_id: "match",
            custom_format_name: "Title match",
            score: 75,
          },
        ],
        score: 75,
        minimum_score: 100,
        meets_minimum: false,
      },
    });
  });
  await page.goto("/settings/custom-formats");
  await page.getByRole("button", { name: "Test", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog
    .getByLabel("Quality profile for scoring")
    .selectOption("profile");
  await dialog.getByLabel("Release title").fill("Example.Show.S01E01.1080p");
  await dialog.getByRole("button", { name: "Test", exact: true }).click();
  await expect(
    dialog.getByText("Below minimum score", { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText("Total custom format score: 75 points", { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText("Minimum custom format score: 100 points", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(dialog.getByText("75 points", { exact: true })).toBeVisible();
  expect(submitted).toEqual({
    title: "Example.Show.S01E01.1080p",
    quality_profile_id: "profile",
  });
  await dialog.getByLabel("Quality profile for scoring").selectOption("");
  await expect(dialog.getByText("Below minimum score")).toHaveCount(0);
});
