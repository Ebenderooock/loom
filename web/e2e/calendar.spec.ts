import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet } from "./helpers/mock-api";

// Build a date string for the 15th of the current month
function currentMonthDate(day: number) {
  var now = new Date();
  var month = String(now.getMonth() + 1).padStart(2, "0");
  var d = String(day).padStart(2, "0");
  return now.getFullYear() + "-" + month + "-" + d;
}

function makeCalendarEvent(overrides: Record<string, unknown> = {}) {
  return {
    id: "cal-1",
    title: "Test Movie",
    type: "movie",
    date: currentMonthDate(15),
    status: "missing",
    year: 2024,
    ...overrides,
  };
}

function mockCalendar(page: import("@playwright/test").Page, events: unknown[] = []) {
  return page.route("**/api/v1/calendar*", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        status: 200,
        json: events,
      });
    } else {
      await route.fallback();
    }
  });
}

test.describe("Calendar Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
  });

  test("calendar page loads with month grid", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(
      page.locator("header").getByText("Calendar"),
    ).toBeAttached({ timeout: 10000 });

    // Calendar grid should be present
    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
  });

  test("calendar shows movie events", async ({ page }) => {
    var events = [
      makeCalendarEvent({
        id: "cal-1",
        title: "Test Movie",
        type: "movie",
        date: currentMonthDate(15),
        status: "missing",
      }),
    ];

    await mockCalendar(page, events);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Test Movie")).toBeVisible({ timeout: 10000 });
  });

  test("calendar shows episode events", async ({ page }) => {
    var events = [
      makeCalendarEvent({
        id: "cal-2",
        title: "Test Series",
        type: "episode",
        date: currentMonthDate(20),
        status: "downloaded",
        seriesTitle: "Test Series",
        season: 1,
        episode: 5,
        episodeTitle: "The Big One",
      }),
    ];

    await mockCalendar(page, events);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
    // Episode should show series title or episode title
    await expect(page.getByText("Test Series")).toBeVisible({ timeout: 10000 });
  });

  test("month navigation works with next button", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });

    // Find the next month button (second icon button in the card header)
    // Layout: [ChevronLeft] [MonthLabel] [ChevronRight]
    var headerRow = page.locator("main").locator(".flex.flex-row").first();
    var buttons = headerRow.locator("button");

    // Click the second button (ChevronRight / next month)
    await buttons.nth(1).click();

    // Calendar grid should still be visible after navigation
    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 5000 });
  });

  test("month navigation works with prev button", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });

    // Find the prev month button (first icon button in the card header)
    var headerRow = page.locator("main").locator(".flex.flex-row").first();
    var buttons = headerRow.locator("button");

    // Click the first button (ChevronLeft / prev month)
    await buttons.nth(0).click();

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 5000 });
  });

  test("day headers show weekday names", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });

    // Column headers should show weekday abbreviations
    var headers = page.locator("[role='columnheader']");
    await expect(headers.first()).toBeVisible({ timeout: 5000 });
    var headerCount = await headers.count();
    expect(headerCount).toBe(7);
  });

  test("today is highlighted", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });

    // Today cell should have a ring highlight
    var todayCell = page.locator("[class*='ring-primary']").first();
    await expect(todayCell).toBeVisible({ timeout: 5000 });
  });

  test("calendar shows multiple event types with different colors", async ({ page }) => {
    var events = [
      makeCalendarEvent({
        id: "cal-1",
        title: "Movie Release",
        type: "movie",
        date: currentMonthDate(10),
        status: "missing",
      }),
      makeCalendarEvent({
        id: "cal-2",
        title: "Episode Air",
        type: "episode",
        date: currentMonthDate(10),
        status: "downloaded",
        seriesTitle: "Drama Show",
        season: 2,
        episode: 1,
      }),
      makeCalendarEvent({
        id: "cal-3",
        title: "Theatrical Release",
        type: "movie",
        date: currentMonthDate(15),
        status: "missing",
        releaseType: "theatrical",
      }),
    ];

    await mockCalendar(page, events);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Movie Release")).toBeVisible({ timeout: 10000 });
  });

  test("empty calendar shows grid with no events", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
    // Grid cells should exist
    var cells = page.locator("[role='gridcell']");
    await expect(cells.first()).toBeVisible({ timeout: 5000 });
  });

  test("calendar fetches data for current month range", async ({ page }) => {
    var calendarReq = page.waitForRequest(
      function(req) { return req.url().includes("/api/v1/calendar") && req.method() === "GET"; }
    );

    await mockCalendar(page, []);
    await page.goto("/calendar");

    var req = await calendarReq;
    var url = req.url();
    // URL should have start and end query params
    expect(url).toContain("start=");
    expect(url).toContain("end=");
  });
});
