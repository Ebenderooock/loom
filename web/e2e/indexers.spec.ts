import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockIndexers,
  SAMPLE_INDEXER,
} from "./helpers/mock-api";

test.describe("Indexers Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockIndexers(page);

    // The API fetches /api/v1/indexers/ and /api/v1/proxies/ with trailing
    // slashes. mockGet patterns omit the trailing slash, so add explicit
    // overrides for the trailing-slash variants.
    await page.route("**/api/v1/indexers/", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: { indexers: [SAMPLE_INDEXER] },
        });
      } else {
        await route.fallback();
      }
    });
    await page.route("**/api/v1/proxies/", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { proxies: [] } });
      } else {
        await route.fallback();
      }
    });
  });

  test("renders indexers page heading", async ({ page }) => {
    await page.goto("/indexers");
    // useSetPageHeader("Indexers") renders as <span> in header — hidden on mobile
    await expect(
      page.locator("header").getByText("Indexers"),
    ).toBeAttached({ timeout: 10000 });
  });

  test("displays indexer in the list", async ({ page }) => {
    await page.goto("/indexers");

    // Should show the mock indexer name
    await expect(page.getByText(SAMPLE_INDEXER.name)).toBeVisible({ timeout: 10000 });
  });

  test("shows add indexer button", async ({ page }) => {
    await page.goto("/indexers");

    await expect(
      page.getByRole("button", { name: /add.*indexer/i }),
    ).toBeVisible({ timeout: 10000 });
  });

  test("add indexer dialog opens", async ({ page }) => {
    await page.goto("/indexers");

    await page.getByRole("button", { name: /add.*indexer/i }).click();

    // Should see a dialog for adding an indexer
    await expect(
      page.getByRole("dialog"),
    ).toBeVisible({ timeout: 3000 });
  });

  test("renders empty state when no indexers", async ({ page }) => {
    // Override mock with empty list (both patterns for trailing slash)
    const emptyHandler = async (route: import("@playwright/test").Route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { indexers: [] } });
      } else {
        await route.fallback();
      }
    };
    await page.route("**/api/v1/indexers", emptyHandler);
    await page.route("**/api/v1/indexers/", emptyHandler);

    await page.goto("/indexers");

    // The page should still render with the "Add indexer" button visible
    await expect(
      page.getByRole("button", { name: /add.*indexer/i }),
    ).toBeVisible({ timeout: 10000 });
  });
});

test.describe("Indexer Health Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockIndexers(page, [
      { ...SAMPLE_INDEXER, id: "idx-1", name: "Healthy Indexer" },
      {
        ...SAMPLE_INDEXER,
        id: "idx-2",
        name: "Failing Indexer",
        enabled: true,
      },
    ]);

    // Trailing-slash overrides for indexers / proxies
    await page.route("**/api/v1/indexers/", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: {
            indexers: [
              { ...SAMPLE_INDEXER, id: "idx-1", name: "Healthy Indexer" },
              { ...SAMPLE_INDEXER, id: "idx-2", name: "Failing Indexer", enabled: true },
            ],
          },
        });
      } else {
        await route.fallback();
      }
    });
    await page.route("**/api/v1/proxies/", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({ status: 200, json: { proxies: [] } });
      } else {
        await route.fallback();
      }
    });

    // Health data with one healthy, one failing
    await page.route("**/api/v1/indexers/health", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          json: {
            data: [
              {
                indexer_id: "idx-1",
                indexer_name: "Healthy Indexer",
                status: "healthy",
                last_error: "",
                success_rate: 100,
                fail_count: 0,
              },
              {
                indexer_id: "idx-2",
                indexer_name: "Failing Indexer",
                status: "error",
                last_error: "Connection timeout",
                success_rate: 0,
                fail_count: 5,
              },
            ],
          },
        });
      } else {
        await route.fallback();
      }
    });

  });

  test("renders indexer health page", async ({ page }) => {
    await page.goto("/indexers/health");
    // useSetPageHeader("Indexer Health") renders as <span> in header — hidden on mobile
    await expect(
      page.locator("header").getByText("Indexer Health"),
    ).toBeAttached({ timeout: 10000 });
  });
});
