import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet } from "./helpers/mock-api";
import { DEFAULT_APPEARANCE, type Appearance } from "../src/lib/appearance-api";
import { PALETTES } from "../src/lib/palette";

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
    page.getByText("Appearance for everyone", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Dark", exact: true }).click();
  await page.getByRole("button", { name: "Ocean", exact: true }).click();
  await page.getByLabel("Background image URL").fill(imageUrl);
  await page.getByLabel(/background dimming/i).fill("50");
  await expect(page.locator("main img")).toHaveJSProperty("naturalWidth", 1600);
  await page.getByRole("button", { name: "Save appearance" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(stored.palette).toEqual(PALETTES.Ocean);
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
    "rgb(16, 31, 42)",
  );
  const overflow = await page.evaluate(() => ({
    fits: document.documentElement.scrollWidth <= window.innerWidth,
    elements: [...document.querySelectorAll("main *")]
      .filter(
        (element) => element.getBoundingClientRect().right > window.innerWidth,
      )
      .slice(0, 10)
      .map((element) => `${element.tagName}.${element.className}`),
  }));
  expect(overflow.fits, JSON.stringify(overflow.elements)).toBe(true);
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(
    page.getByText("Appearance saved for everyone.", { exact: true }),
  ).toBeHidden();
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

test("custom colors theme the actual app and uploaded images survive reload", async ({
  page,
}, testInfo) => {
  await mockBaseApp(page);
  await mockGet(page, "features", { features: [] });
  await mockGet(page, "api-keys", []);
  let stored: Appearance = { ...DEFAULT_APPEARANCE };
  await page.route("**/api/v1/appearance", async (route) => {
    if (route.request().method() === "PUT")
      stored = route.request().postDataJSON();
    await route.fulfill({ json: stored });
  });
  const uploadedUrl = `/api/v1/appearance/background?v=${"b".repeat(64)}`;
  const png = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=",
    "base64",
  );
  await page.route("**/api/v1/appearance/background*", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().headers()["content-type"]).toContain(
        "multipart/form-data",
      );
      await route.fulfill({ json: { url: uploadedUrl } });
    } else await route.fulfill({ contentType: "image/png", body: png });
  });
  await page.goto("/settings/appearance");
  await page.waitForLoadState("networkidle");
  const brandLogoPrimary = page
    .locator('[data-testid="loom-logo"]:not([aria-label="Palette preview"] *)')
    .locator("visible=true")
    .first()
    .locator("span")
    .nth(1);
  await expect(brandLogoPrimary).toHaveCSS(
    "background-color",
    "rgb(90, 40, 189)",
  );
  await page.getByRole("button", { name: "Dark", exact: true }).click();
  await page.getByRole("button", { name: "Ember", exact: true }).click();
  await expect(page.getByLabel("Primary hex")).toBeHidden();
  await page.getByText("Customize individual colors", { exact: true }).click();
  await page.getByLabel("Primary hex").fill("#dc6b32");
  await page.getByLabel("Secondary hex").fill("#ad4182");
  await page.getByLabel("Accent hex").fill("#e8bf48");
  await page.getByLabel("Surface tint hex").fill("#462f21");
  await expect(
    page
      .getByRole("region", { name: "Palette preview" })
      .locator(".bg-primary"),
  ).toHaveCSS("background-color", "rgb(220, 107, 50)");
  await expect(
    page
      .getByRole("region", { name: "Palette preview" })
      .getByTestId("loom-logo")
      .locator("span")
      .nth(1),
  ).toHaveCSS("background-color", "rgb(220, 107, 50)");
  await expect(brandLogoPrimary).toHaveCSS(
    "background-color",
    "rgb(90, 40, 189)",
  );
  await page.getByRole("button", { name: "Upload image", exact: true }).click();
  await page.getByLabel("Choose background image").setInputFiles({
    name: "wallpaper.png",
    mimeType: "image/png",
    buffer: png,
  });
  await expect(page.locator("main img")).toHaveAttribute("src", uploadedUrl);
  await expect(page.locator("main img")).toHaveJSProperty("naturalWidth", 1);
  await page.getByRole("button", { name: "Save appearance" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect(page.getByRole("button", { name: "Save appearance" })).toHaveCSS(
    "background-color",
    "rgb(220, 107, 50)",
  );
  await expect(brandLogoPrimary).toHaveCSS(
    "background-color",
    "rgb(220, 107, 50)",
  );
  await expect(page.locator("main .rounded-xl").last()).toHaveCSS(
    "background-color",
    "rgb(48, 32, 23)",
  );
  const cardColor = await page
    .locator("main .rounded-xl")
    .first()
    .evaluate((element) => getComputedStyle(element).backgroundColor);
  expect(cardColor).not.toBe("rgb(46, 46, 72)");
  expect(stored.palette).toEqual({
    primary: "#dc6b32",
    secondary: "#ad4182",
    accent: "#e8bf48",
    background: "#462f21",
  });
  await page.evaluate(() => window.scrollTo(0, 0));
  await expect(
    page.getByText("Appearance saved for everyone.", { exact: true }),
  ).toBeHidden();
  await page.screenshot({
    path: testInfo.outputPath("custom-palette.png"),
    fullPage: true,
    animations: "disabled",
  });

  await page.reload();
  await page.getByText("Customize individual colors", { exact: true }).click();
  await expect(page.getByLabel("Primary hex")).toHaveValue("#dc6b32");
  await expect(page.locator("main img")).toHaveAttribute("src", uploadedUrl);
  await page.goto("/settings/general");
  await expect(page.getByText("General application settings")).toBeVisible();
  await expect(page.locator("main .rounded-xl").first()).toHaveCSS(
    "background-color",
    cardColor,
  );
  await page.goto("/settings/appearance");
  await page.getByRole("button", { name: "Light", exact: true }).click();
  await page.getByRole("button", { name: "Save appearance" }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  expect(
    await page
      .locator("main .rounded-xl")
      .first()
      .evaluate((element) => getComputedStyle(element).backgroundColor),
  ).not.toBe(cardColor);
  await page.getByRole("button", { name: "AMOLED", exact: true }).click();
  await page.getByRole("button", { name: "Save appearance" }).click();
  await expect(page.locator(".app-shell")).toHaveCSS(
    "background-color",
    "rgb(0, 0, 0)",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("beginner flow stays usable on a small phone, landscape, and enlarged text", async ({
  page,
}) => {
  await mockBaseApp(page);
  await mockGet(page, "features", { features: [] });
  let saves = 0;
  await page.route("**/api/v1/appearance", async (route) => {
    if (route.request().method() === "PUT") saves++;
    await route.fulfill({ json: DEFAULT_APPEARANCE });
  });
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/settings/appearance");
  await page.waitForLoadState("networkidle");
  await expect(page.getByLabel("Primary hex")).toBeHidden();
  await page.getByRole("button", { name: "Forest", exact: true }).click();
  const form = page.locator("form").filter({
    has: page.getByRole("button", { name: "Save appearance" }),
  });
  for (const button of await form.getByRole("button").all()) {
    const bounds = await button.boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
    expect(bounds?.width).toBeGreaterThanOrEqual(44);
  }
  const custom = page.getByText("Customize individual colors", { exact: true });
  await custom.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByLabel("Primary color")).toBeVisible();
  await page.getByRole("button", { name: "Discard changes" }).click();
  await expect(
    page.getByRole("button", { name: "Loom", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  expect(saves).toBe(0);
  for (const viewport of [
    { width: 375, height: 812 },
    { width: 812, height: 375 },
  ]) {
    await page.setViewportSize(viewport);
    await page.evaluate(() => {
      document.documentElement.style.fontSize = "200%";
    });
    const overflow = await page.locator("main").evaluate((main) => ({
      fits: main.scrollWidth <= main.clientWidth,
      width: main.clientWidth,
      elements: [...main.querySelectorAll("*")]
        .filter(
          (element) =>
            element.getBoundingClientRect().right >
            main.getBoundingClientRect().right,
        )
        .slice(0, 15)
        .map((element) => `${element.tagName}.${element.className}`),
    }));
    expect(overflow.fits, JSON.stringify(overflow)).toBe(true);
    await expect(
      page.getByRole("button", { name: "Save appearance" }),
    ).toBeVisible();
  }
});
