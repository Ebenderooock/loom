import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet } from "./helpers/mock-api";
import { DEFAULT_APPEARANCE, type Appearance } from "../src/lib/appearance-api";

test("global appearance controls persist and render on desktop and mobile", async ({
  page,
}, testInfo) => {
  await mockBaseApp(page);
  await mockGet(page, "features", { features: [] });
  let stored: Appearance = { ...DEFAULT_APPEARANCE };
  await page.route("**/api/v1/appearance", async (route) => {
    if (route.request().method() === "PUT") {
      stored = route.request().postDataJSON();
    }
    await route.fulfill({ json: stored });
  });
  const imageUrl = "https://images.loom.test/wallpaper.svg";
  await page.route(imageUrl, (route) =>
    route.fulfill({
      contentType: "image/svg+xml",
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1000"><path fill="#185b8c" d="M0 0h1600v1000H0z"/><path fill="#c97b36" d="M0 1000 1600 0v1000z"/></svg>',
    }),
  );
  await page.goto("/settings/appearance");
  await page.waitForLoadState("networkidle");
  await expect(
    page.getByText("Instance appearance", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Dark", exact: true }).click();
  await page.getByRole("button", { name: "Blue", exact: true }).click();
  await page.getByLabel("Background image URL").fill(imageUrl);
  await page.getByLabel(/background dimming/i).fill("50");
  await expect(page.locator("main img")).toHaveJSProperty("naturalWidth", 1600);
  await page.getByRole("button", { name: "Save appearance" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.locator("html")).toHaveAttribute("data-accent", "blue");
  await expect(
    page.getByRole("button", { name: "Save appearance" }),
  ).toBeDisabled();
  const background = await page.locator(".app-shell").evaluate((shell) => {
    const style = getComputedStyle(shell, "::before");
    return { image: style.backgroundImage, position: style.position };
  });
  expect(background.image).toContain(imageUrl);
  expect(background.position).toBe("fixed");
  expect(stored.backgroundDim).toBe(50);
  await expect(page.locator("main .rounded-xl").last()).toHaveCSS(
    "background-color",
    "rgb(46, 46, 72)",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("appearance.png"),
    fullPage: true,
  });
  await page.reload();
  await expect(page.getByLabel("Background image URL")).toHaveValue(imageUrl);
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.getByRole("button", { name: "Reset to defaults" }).click();
  await page.getByRole("button", { name: "Save appearance" }).click();
  await expect(page.getByLabel("Background image URL")).toHaveValue("");
  await expect(page.locator("html")).toHaveAttribute("data-accent", "default");
  expect(stored).toEqual(DEFAULT_APPEARANCE);
});
