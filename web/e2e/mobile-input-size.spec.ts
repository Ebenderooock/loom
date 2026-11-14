import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockPost,
  mockSettings,
} from "./helpers/mock-api";

test.use({ reducedMotion: "reduce" });

test("mobile editable controls retain a readable focus size without disabling zoom", async ({
  page,
}) => {
  await mockBaseApp(page);
  await mockSettings(page);
  await mockGet(page, "features", { features: [] });
  await mockGet(page, "download-clients/", { download_clients: [] });
  await mockGet(page, "download-clients/remote-path-mappings", []);
  await mockGet(page, "notifications/history?*", []);
  await mockGet(page, "collections/", []);
  await mockGet(page, "collections/scan", { total: 0 });
  await mockGet(page, "collections/loom-1", {
    id: "loom-1",
    name: "Test collection",
    revision: 1,
    total: 1,
    in_library: 0,
    sources: [],
    members: [],
  });
  await mockPost(page, "collections/loom-1/bulk/add", {
    settings: { movie: {} },
    libraries: { movie: [] },
    profiles: [],
    counts: { eligible: 1 },
    items: [{ member_id: "m1", title: "Test movie", media_type: "movie" }],
  });
  for (const viewport of [
    { width: 375, height: 812 },
    { width: 440, height: 956 },
    { width: 956, height: 440 },
  ]) {
    await page.setViewportSize(viewport);
    await page.goto("/settings/general");
    await page.getByRole("button", { name: "Open command palette" }).click();
    const search = page.locator('input[placeholder*="Search"]').first();
    await expect(search).toBeVisible();
    expect(
      parseFloat(await search.evaluate((el) => getComputedStyle(el).fontSize)),
    ).toBeGreaterThanOrEqual(16);
    await page.keyboard.press("Escape");

    const sizes = await page.evaluate(() => {
      const container = document.createElement("div");
      container.setAttribute("role", "dialog");
      container.innerHTML = `<input type="text" class="text-xs"><input type="search" class="text-sm"><input type="password" style="font-size: 12px"><input type="number" class="text-xs"><textarea class="text-sm"></textarea><select class="text-sm"><option>Example</option></select>`;
      document.body.append(container);
      const sizes = Array.from(container.children, (el) =>
        parseFloat(getComputedStyle(el).fontSize),
      );
      container.remove();
      return sizes;
    });
    expect(
      sizes.every((size) => size >= 16),
      JSON.stringify(sizes),
    ).toBe(true);
    const viewportMeta = await page
      .locator('meta[name="viewport"]')
      .getAttribute("content");
    expect(viewportMeta).not.toMatch(/user-scalable\s*=\s*no|maximum-scale/i);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(viewport.width);
  }

  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/settings/download-clients");
  await page.getByRole("button", { name: "Add Client" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const inputs = dialog.locator('input:not([type="checkbox"])');
  expect(await inputs.count()).toBeGreaterThan(2);
  for (const input of await inputs.all()) {
    expect(
      parseFloat(await input.evaluate((el) => getComputedStyle(el).fontSize)),
    ).toBeGreaterThanOrEqual(16);
  }
  await page.goto("/settings/notifications");
  await page.getByRole("button", { name: "Add notification" }).click();
  const textarea = page.getByRole("dialog").locator("textarea");
  await expect(textarea).toBeVisible();
  expect(
    parseFloat(await textarea.evaluate((el) => getComputedStyle(el).fontSize)),
  ).toBeGreaterThanOrEqual(16);

  await page.evaluate(() => {
    document.documentElement.style.fontSize = "24px";
  });
  await expect
    .poll(() =>
      textarea.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
    )
    .toBeGreaterThan(16);
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.evaluate(() => {
    document.documentElement.style.fontSize = "";
  });
  if (!(await page.evaluate(() => matchMedia("(pointer: coarse)").matches))) {
    await expect
      .poll(() =>
        textarea.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
      )
      .toBe(14);
  }

  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/collections?id=loom-1");
  await page.getByRole("button", { name: "Add missing" }).click();
  const sheet = page.getByRole("dialog");
  const sheetSelect = sheet.getByRole("combobox", {
    name: "Library",
    exact: true,
  });
  await expect(sheetSelect).toBeVisible();
  expect(
    parseFloat(
      await sheetSelect.evaluate((el) => getComputedStyle(el).fontSize),
    ),
  ).toBeGreaterThanOrEqual(16);
});
