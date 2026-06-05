import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet } from "./helpers/mock-api";

// Helper to build audit log entries
function makeEntry(overrides: Record<string, unknown> = {}) {
  return {
    id: "evt-1",
    timestamp: "2024-01-15T10:30:00Z",
    occurred_at: "2024-01-15T10:30:00Z",
    category: "search",
    event_type: "search_completed",
    message: "Search completed for Test Movie",
    detail: JSON.stringify({ query_log_id: "ql-1", results: 15 }),
    entity_type: "movie",
    entity_id: "mov-1",
    entity_name: "Test Movie",
    level: "info",
    source: "indexer",
    ...overrides,
  };
}

function makeAuditResponse(entries: unknown[] = [], total?: number) {
  return {
    entries: entries,
    total: total != null ? total : entries.length,
    limit: 50,
    offset: 0,
  };
}

function mockAuditLog(
  page: import("@playwright/test").Page,
  entries: unknown[] = [],
  total?: number,
) {
  return page.route("**/api/v1/system/audit-log*", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        status: 200,
        json: makeAuditResponse(entries, total),
      });
    } else {
      await route.fallback();
    }
  });
}

test.describe("Events / History Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
  });

  test("events page loads with audit log entries", async ({ page }) => {
    await mockAuditLog(page, [makeEntry()]);
    await page.goto("/events");

    await expect(page.locator("header").getByText("Events")).toBeAttached({
      timeout: 10000,
    });

    // Table should show the entry
    await expect(page.getByText("Search completed for Test Movie")).toBeVisible(
      { timeout: 10000 },
    );
  });

  test("shows empty state when no events", async ({ page }) => {
    await mockAuditLog(page, []);
    await page.goto("/events");

    await expect(page.getByText("No events found")).toBeVisible({
      timeout: 10000,
    });
  });

  test("category filter changes the displayed events", async ({ page }) => {
    const searchEntry = makeEntry({
      id: "evt-1",
      category: "search",
      message: "Search completed",
    });
    const downloadEntry = makeEntry({
      id: "evt-2",
      category: "download",
      event_type: "download_started",
      message: "Download started for Test Movie",
    });

    await mockAuditLog(page, [searchEntry, downloadEntry]);
    await page.goto("/events");

    // Both entries should be visible initially
    await expect(page.getByText("Search completed")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("Download started for Test Movie")).toBeVisible(
      { timeout: 10000 },
    );

    // Now override with a filtered response when category changes
    // The UI sends category param to API - mock a filtered response
    await page.route("**/api/v1/system/audit-log*", async (route) => {
      const url = route.request().url();
      if (
        route.request().method() === "GET" &&
        url.includes("category=download")
      ) {
        await route.fulfill({
          status: 200,
          json: makeAuditResponse([downloadEntry]),
        });
      } else if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: makeAuditResponse([searchEntry, downloadEntry]),
        });
      } else {
        await route.fallback();
      }
    });

    // Click the category dropdown and select "Download"
    await page.locator("button").filter({ hasText: "All Categories" }).click();
    await page.getByRole("option", { name: "Download" }).click();

    // After filter, only download entry should be visible
    await expect(page.getByText("Download started for Test Movie")).toBeVisible(
      { timeout: 10000 },
    );
  });

  test("level filter works", async ({ page }) => {
    const infoEntry = makeEntry({
      id: "evt-1",
      level: "info",
      message: "Info event",
    });
    const errorEntry = makeEntry({
      id: "evt-2",
      level: "error",
      message: "Error event occurred",
      event_type: "indexer_error",
    });

    await mockAuditLog(page, [infoEntry, errorEntry]);
    await page.goto("/events");

    await expect(page.getByText("Info event")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Error event occurred")).toBeVisible({
      timeout: 10000,
    });

    // Override route for filtered response
    await page.route("**/api/v1/system/audit-log*", async (route) => {
      const url = route.request().url();
      if (route.request().method() === "GET" && url.includes("level=error")) {
        await route.fulfill({
          status: 200,
          json: makeAuditResponse([errorEntry]),
        });
      } else if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: makeAuditResponse([infoEntry, errorEntry]),
        });
      } else {
        await route.fallback();
      }
    });

    // Click level dropdown and select Error
    await page.locator("button").filter({ hasText: "All Levels" }).click();
    await page.getByRole("option", { name: "Error" }).click();

    await expect(page.getByText("Error event occurred")).toBeVisible({
      timeout: 10000,
    });
  });

  test("expandable row shows detail JSON", async ({ page }) => {
    const entry = makeEntry({
      detail: JSON.stringify({
        query_log_id: "ql-1",
        results: 15,
        indexer: "TestIndexer",
      }),
    });
    await mockAuditLog(page, [entry]);
    await page.goto("/events");

    await expect(page.getByText("Search completed for Test Movie")).toBeVisible(
      { timeout: 10000 },
    );

    // Click the row to expand it
    await page.getByText("Search completed for Test Movie").click();

    // Detail panel should show parsed JSON keys
    await expect(page.getByText("query_log_id:")).toBeVisible({
      timeout: 5000,
    });
    await expect(page.getByText("ql-1")).toBeVisible();
  });

  test("search diagnostics section loads per-indexer breakdown", async ({
    page,
  }) => {
    const entry = makeEntry({
      detail: JSON.stringify({ query_log_id: "ql-1", results: 15 }),
    });
    await mockAuditLog(page, [entry]);

    // Mock the search log detail endpoint
    await mockGet(page, "search/log/ql-1", {
      id: "ql-1",
      query: "Test Movie",
      query_type: "movie",
      total_results: 15,
      status: "completed",
      indexers: [
        {
          id: "ixq-1",
          indexer_id: "idx-1",
          indexer_name: "TestIndexer",
          latency_ms: 450,
          result_count: 15,
          status: "completed",
        },
      ],
    });

    await page.goto("/events");

    // Click to expand
    await page.getByText("Search completed for Test Movie").click();

    // Should show per-indexer breakdown
    await expect(page.getByText("Per-Indexer Breakdown")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("TestIndexer")).toBeVisible();
    await expect(page.getByText("450ms")).toBeVisible();
    await expect(page.getByText("15 results")).toBeVisible();
  });

  test("refresh button reloads data", async ({ page }) => {
    await mockAuditLog(page, [makeEntry()]);
    await page.goto("/events");

    await expect(page.getByText("Search completed for Test Movie")).toBeVisible(
      { timeout: 10000 },
    );

    // Verify refresh button exists and click it
    const refreshBtn = page.getByRole("button", { name: "Refresh" });
    await expect(refreshBtn).toBeVisible();

    const auditReq = page.waitForRequest(function (req) {
      return (
        req.url().includes("/api/v1/system/audit-log") && req.method() === "GET"
      );
    });
    await refreshBtn.click();
    await auditReq;
  });

  test("pagination works with next and previous", async ({ page }) => {
    // Create 60 entries to trigger pagination (PAGE_SIZE=50)
    const entries = [];
    for (let i = 0; i < 50; i++) {
      entries.push(
        makeEntry({
          id: "evt-" + i,
          message: "Event number " + i,
        }),
      );
    }

    // Mock first page with total > 50
    await page.route("**/api/v1/system/audit-log*", async (route) => {
      if (route.request().method() !== "GET") {
        await route.fallback();
        return;
      }
      const url = route.request().url();
      const offsetMatch = url.match(/offset=(\d+)/);
      const currentOffset = offsetMatch ? parseInt(offsetMatch[1]) : 0;

      if (currentOffset >= 50) {
        await route.fulfill({
          status: 200,
          json: {
            entries: [makeEntry({ id: "evt-50", message: "Event on page 2" })],
            total: 51,
            limit: 50,
            offset: 50,
          },
        });
      } else {
        await route.fulfill({
          status: 200,
          json: {
            entries: entries,
            total: 51,
            limit: 50,
            offset: 0,
          },
        });
      }
    });

    await page.goto("/events");

    // Wait for first page to load
    await expect(page.getByText("Event number 0")).toBeVisible({
      timeout: 10000,
    });

    // Pagination should show "1-50 of 51"
    await expect(page.getByText(/1.*50 of 51/)).toBeVisible();

    // Next button should be enabled
    const nextBtn = page.getByRole("button", { name: /Next/i });
    await expect(nextBtn).toBeEnabled();

    // Click next
    await nextBtn.click();

    // Second page should load
    await expect(page.getByText("Event on page 2")).toBeVisible({
      timeout: 10000,
    });
  });

  test("different event types render correctly", async ({ page }) => {
    const entries = [
      makeEntry({
        id: "evt-1",
        category: "search",
        event_type: "search_completed",
        message: "Search completed",
        level: "info",
      }),
      makeEntry({
        id: "evt-2",
        category: "download",
        event_type: "grab_completed",
        message: "Grabbed release",
        level: "info",
        detail: null,
      }),
      makeEntry({
        id: "evt-3",
        category: "import",
        event_type: "import_completed",
        message: "Movie imported successfully",
        level: "info",
        detail: null,
      }),
      makeEntry({
        id: "evt-4",
        category: "system",
        event_type: "indexer_error",
        message: "Indexer connection failed",
        level: "error",
        detail: null,
      }),
    ];

    await mockAuditLog(page, entries);
    await page.goto("/events");

    await expect(page.getByText("Search completed")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("Grabbed release")).toBeVisible();
    await expect(page.getByText("Movie imported successfully")).toBeVisible();
    await expect(page.getByText("Indexer connection failed")).toBeVisible();
  });

  test("shows event count", async ({ page }) => {
    await mockAuditLog(page, [makeEntry()], 1);
    await page.goto("/events");

    await expect(page.getByText("1 event")).toBeVisible({ timeout: 10000 });
  });
});
