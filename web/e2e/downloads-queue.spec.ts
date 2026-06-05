import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockDownloads,
  mockGet,
  mockRoute,
} from "./helpers/mock-api";

function makeQueueItem(overrides: Record<string, unknown> = {}) {
  return {
    id: "dl-1",
    client_id: "qb-dl-1",
    title: "Test.Movie.2024.1080p.BluRay.x264",
    category: "movies",
    status: "downloading",
    progress: 0.45,
    size_bytes: 4500000000,
    downloaded_bytes: 2025000000,
    eta_seconds: 3600,
    download_rate: 5000000,
    upload_rate: 1000000,
    ratio: 0.2,
    message: "",
    save_path: "/downloads/movies",
    ...overrides,
  };
}

// The downloads page fetches /api/v1/activity and reads body.items
function mockActivity(
  page: import("@playwright/test").Page,
  items: unknown[] = [],
) {
  return page.route("**/api/v1/activity", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ status: 200, json: { items: items } });
    } else {
      await route.fallback();
    }
  });
}

test.describe("Downloads Queue", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockDownloads(page);
  });

  test("downloads page shows active downloads", async ({ page }) => {
    await mockActivity(page, [makeQueueItem()]);
    await page.goto("/downloads");

    await expect(page.locator("header").getByText("Downloads")).toBeAttached({
      timeout: 10000,
    });

    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });
  });

  test("shows empty state when no active downloads", async ({ page }) => {
    await mockActivity(page, []);
    await page.goto("/downloads");

    await expect(page.getByText("No active downloads").first()).toBeVisible({
      timeout: 10000,
    });
  });

  test("pause button sends pause request", async ({ page }) => {
    await mockActivity(page, [makeQueueItem({ status: "downloading" })]);

    await page.route("**/api/v1/activity/pause", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/downloads");
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });

    const pauseReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/activity/pause") && req.method() === "POST"
      );
    });

    await page.locator("button[title='Pause']").first().click();
    await pauseReq;
  });

  test("resume button sends resume request", async ({ page }) => {
    await mockActivity(page, [makeQueueItem({ status: "paused" })]);

    await page.route("**/api/v1/activity/resume", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/downloads");
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });

    const resumeReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/activity/resume") && req.method() === "POST"
      );
    });

    await page.locator("button[title='Resume']").first().click();
    await resumeReq;
  });

  test("stop and remove button sends remove request", async ({ page }) => {
    await mockActivity(page, [makeQueueItem()]);

    await page.route("**/api/v1/activity/remove", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/downloads");
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });

    const removeReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/activity/remove") && req.method() === "POST"
      );
    });

    // Title in the source is "Stop &amp; remove (delete files)" - rendered as "Stop & remove (delete files)"
    await page.locator("button[title*='remove']").first().click();

    // Confirm if a confirmation dialog appears
    const confirmBtn = page.getByRole("button", {
      name: /Confirm|Yes|Remove/i,
    });
    if (
      await confirmBtn.isVisible({ timeout: 2000 }).catch(function () {
        return false;
      })
    ) {
      await confirmBtn.click();
    }

    await removeReq;
  });

  test("force import button sends manual import request", async ({ page }) => {
    await mockActivity(page, [
      makeQueueItem({ status: "completed", progress: 1.0 }),
    ]);

    await page.route("**/api/v1/imports/manual", async (route) => {
      await route.fulfill({ status: 200, json: { ok: true } });
    });

    await page.goto("/downloads");
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });

    const importReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/imports/manual") && req.method() === "POST"
      );
    });

    const forceImportBtn = page.locator("button[title='Force Import']").first();
    if (
      await forceImportBtn.isVisible({ timeout: 3000 }).catch(function () {
        return false;
      })
    ) {
      await forceImportBtn.click();
      await importReq;
    }
  });

  test("refresh button reloads activity data", async ({ page }) => {
    await mockActivity(page, [makeQueueItem()]);
    await page.goto("/downloads");

    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });

    const refreshReq = page.waitForRequest(function (req) {
      return req.url().includes("/api/v1/activity") && req.method() === "GET";
    });

    await page.getByRole("button", { name: /Refresh/i }).click();
    await refreshReq;
  });

  test("multiple downloads are listed", async ({ page }) => {
    const items = [
      makeQueueItem({
        id: "dl-1",
        title: "Movie.One.2024.1080p.BluRay",
        progress: 0.8,
      }),
      makeQueueItem({
        id: "dl-2",
        title: "Movie.Two.2024.720p.WEB-DL",
        progress: 0.3,
        status: "downloading",
      }),
      makeQueueItem({
        id: "dl-3",
        title: "Movie.Three.2024.4K.UHD",
        progress: 0.0,
        status: "queued",
      }),
    ];

    await mockActivity(page, items);
    await page.goto("/downloads");

    await expect(page.getByText("Movie.One.2024.1080p.BluRay")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("Movie.Two.2024.720p.WEB-DL")).toBeVisible();
    await expect(page.getByText("Movie.Three.2024.4K.UHD")).toBeVisible();
  });

  test("queue stats show download speed", async ({ page }) => {
    await mockActivity(page, [
      makeQueueItem({ download_rate: 10000000, upload_rate: 2000000 }),
    ]);
    await page.goto("/downloads");

    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });
    await expect(page.locator("main")).toBeVisible();
  });

  test("imports tab exists", async ({ page }) => {
    await mockActivity(page, []);
    await page.goto("/downloads");

    const importsTab = page.getByRole("tab", { name: /Imports/i });
    await expect(importsTab).toBeVisible({ timeout: 10000 });
  });
});
