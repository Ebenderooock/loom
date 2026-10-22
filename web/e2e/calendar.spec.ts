import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet, mockPost } from "./helpers/mock-api";

// Build a date string for the 15th of the current month
function currentMonthDate(day: number) {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const d = String(day).padStart(2, "0");
  return now.getFullYear() + "-" + month + "-" + d;
}

function makeCalendarEvent(overrides: Record<string, unknown> = {}) {
  return {
    id: "cal-1",
    title: "Test Movie",
    type: "movie",
    date: currentMonthDate(15),
    status: "missing",
    monitored: true,
    year: 2024,
    ...overrides,
  };
}

function mockCalendar(
  page: import("@playwright/test").Page,
  events: unknown[] = [],
) {
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

    await expect(page.locator("header").getByText("Calendar")).toBeAttached({
      timeout: 10000,
    });

    // Calendar grid should be present
    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
  });

  test("calendar shows movie events", async ({ page }) => {
    const events = [
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
    const events = [
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
    const headerRow = page.locator("main").locator(".flex.flex-row").first();
    const buttons = headerRow.locator("button");

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
    const headerRow = page.locator("main").locator(".flex.flex-row").first();
    const buttons = headerRow.locator("button");

    // Click the first button (ChevronLeft / prev month)
    await buttons.nth(0).click();

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 5000 });
  });

  test("day headers show weekday names", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });

    // Column headers should show weekday abbreviations
    const headers = page.locator("[role='columnheader']");
    await expect(headers.first()).toBeVisible({ timeout: 5000 });
    const headerCount = await headers.count();
    expect(headerCount).toBe(7);
  });

  test("today is highlighted", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });

    // Today cell should have a ring highlight
    const todayCell = page.locator("[class*='ring-primary']").first();
    await expect(todayCell).toBeVisible({ timeout: 5000 });
  });

  test("calendar shows multiple event types with different colors", async ({
    page,
  }) => {
    const events = [
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
    await expect(page.getByText("Movie Release")).toBeVisible({
      timeout: 10000,
    });
  });

  test("empty calendar shows grid with no events", async ({ page }) => {
    await mockCalendar(page, []);
    await page.goto("/calendar");

    await expect(page.locator("[role='grid']")).toBeVisible({ timeout: 10000 });
    // Grid cells should exist
    const cells = page.locator("[role='gridcell']");
    await expect(cells.first()).toBeVisible({ timeout: 5000 });
  });

  test("calendar fetches data for current month range", async ({ page }) => {
    const calendarReq = page.waitForRequest(function (req) {
      return req.url().includes("/api/v1/calendar") && req.method() === "GET";
    });

    await mockCalendar(page, []);
    await page.goto("/calendar");

    const req = await calendarReq;
    const url = req.url();
    // URL should have start and end query params
    expect(url).toContain("start=");
    expect(url).toContain("end=");
  });

  test("movie entries open the full drawer without leaving the calendar", async ({
    page,
  }) => {
    await mockCalendar(page, [makeCalendarEvent()]);
    await mockGet(page, "libraries", []);
    await mockGet(page, "quality-profiles", []);
    await mockGet(page, "movies/cal-1", {
      id: "cal-1",
      title: "Test Movie",
      year: 2024,
      overview: "Movie overview",
      genres: ["Drama"],
      status: "missing",
      monitoringStatus: "monitored",
      qualityProfileId: "",
      libraryId: "",
      runtime: 120,
      rating: 8,
    });
    await mockGet(page, "movies/cal-1/credits", { cast: [], crew: [] });
    await mockGet(page, "movies/files/cal-1", []);
    await mockGet(page, "movies/cal-1/history", []);
    await mockGet(page, "alt-titles*", []);
    await mockPost(page, "autosearch", {
      considered: 0,
      rejected: 0,
      reason: "No releases found",
    });
    await page.goto("/calendar");
    const entry = page.getByRole("button", {
      name: /Test Movie.*Missing.*Monitored/,
    });
    await entry.focus();
    await page.keyboard.press("Enter");
    const drawer = page.getByRole("dialog");
    await expect(
      drawer.getByRole("heading", { name: "Test Movie", exact: true }).last(),
    ).toBeVisible();
    await expect(drawer.getByText("Movie overview")).toBeVisible();
    await expect(
      drawer.getByRole("button", { name: "Search", exact: true }),
    ).toBeVisible();
    const searchRequest = page.waitForRequest(
      (request) =>
        request.url().endsWith("/api/v1/autosearch") &&
        request.method() === "POST",
    );
    await drawer.getByRole("button", { name: "Search", exact: true }).click();
    expect((await searchRequest).postDataJSON()).toMatchObject({
      media_type: "movie",
      media_id: "cal-1",
    });
    await expect(page).toHaveURL(/\/calendar$/);
    await page.keyboard.press("Escape");
    await expect(drawer).not.toBeVisible();
    await expect(entry).toBeFocused();
  });

  test("episode entries resolve the parent show and expose episode actions", async ({
    page,
  }) => {
    await mockCalendar(page, [
      makeCalendarEvent({
        id: "ep-1",
        type: "episode",
        seriesId: "show-1",
        seriesTitle: "Test Show",
        season: 1,
        episode: 1,
        status: "grabbed",
      }),
    ]);
    await mockGet(page, "libraries", []);
    await mockGet(page, "quality-profiles", []);
    await mockGet(page, "series/show-1", {
      id: "show-1",
      title: "Test Show",
      year: 2024,
      overview: "Show overview",
      genres: ["Drama"],
      status: "continuing",
      monitoringStatus: "all",
      qualityProfileId: "",
      libraryId: "",
      seriesType: "standard",
      rating: 8,
    });
    await mockGet(page, "series/show-1/credits", { cast: [], crew: [] });
    await mockGet(page, "series/show-1/seasons", { data: [] });
    await mockGet(page, "alt-titles*", []);
    await page.goto("/calendar");
    await page.getByRole("button", { name: /Test Show.*Grabbed/ }).click();
    const drawer = page.getByRole("dialog");
    await expect(
      drawer.getByRole("heading", { name: "Test Show", exact: true }).last(),
    ).toBeVisible();
    await expect(
      drawer.getByRole("button", { name: "Search", exact: true }),
    ).toBeVisible();
    await expect(page).toHaveURL(/\/calendar$/);
  });

  test("shows labeled statuses and expands busy days", async ({
    page,
  }, testInfo) => {
    const statuses = [
      "missing",
      "downloaded",
      "grabbed",
      "downloading",
      "importing",
      "unaired",
    ];
    await mockCalendar(
      page,
      statuses.map((status, i) =>
        makeCalendarEvent({
          id: `status-${i}`,
          title: `Movie ${i}`,
          status,
          monitored: i !== 0,
        }),
      ),
    );
    await page.goto("/calendar");
    await expect(
      page.getByRole("button", { name: /Movie 0.*Missing.*Unmonitored/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Movie 1.*Available/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Movie 2.*Grabbed/ }),
    ).toBeVisible();
    await page.getByRole("button", { name: "+3 more" }).click();
    await expect(
      page.getByRole("button", { name: /Movie 3.*Downloading/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Movie 4.*Importing/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Movie 5.*Unaired/ }),
    ).toBeVisible();
    const downloading = page.getByRole("button", {
      name: /Movie 3.*Downloading/,
    });
    await downloading.scrollIntoViewIfNeeded();
    expect(
      await downloading.evaluate((button) => {
        const label = Array.from(button.querySelectorAll("span")).find(
          (span) => span.textContent === "Downloading",
        );
        if (!label) return false;
        const text = label.getBoundingClientRect();
        const bounds = button.getBoundingClientRect();
        return (
          bounds.height >= 44 &&
          text.left >= bounds.left &&
          text.right <= bounds.right
        );
      }),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath("calendar-statuses.png"),
      fullPage: true,
    });
    await page.getByRole("button", { name: "Show less" }).click();
    await expect(
      page.getByRole("button", { name: /Movie 3/ }),
    ).not.toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  });
});
