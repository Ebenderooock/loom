import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockWorkflows,
  mockGet,
  SAMPLE_WORKFLOW_ACTIVE,
  SAMPLE_WORKFLOW_COMPLETED,
  SAMPLE_WORKFLOW_FAILED,
} from "./helpers/mock-api";

test.describe("Workflows Page", () => {
  test.beforeEach(async ({ page }) => {
    await mockBaseApp(page);
    await mockWorkflows(page);
  });

  test("renders workflows page heading", async ({ page }) => {
    await page.goto("/workflows");
    await expect(page.locator("header").getByText("Workflows")).toBeAttached({
      timeout: 10000,
    });
  });

  test("shows all workflows in the table", async ({ page }) => {
    await page.goto("/workflows");

    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({
      timeout: 10000,
    });
    await expect(
      page.getByText("Another.Movie.2023.720p.WEB-DL"),
    ).toBeVisible();
    await expect(page.getByText("Test.Show.S01E05.HDTV")).toBeVisible();
  });

  test("filter tabs show correct counts", async ({ page }) => {
    await page.goto("/workflows");

    // All tab shows total
    await expect(page.getByRole("tab", { name: /All \(3\)/ })).toBeVisible({
      timeout: 10000,
    });
    // Active tab: only downloading workflow
    await expect(page.getByRole("tab", { name: /Active \(1\)/ })).toBeVisible();
    // Completed tab
    await expect(
      page.getByRole("tab", { name: /Completed \(1\)/ }),
    ).toBeVisible();
    // Failed tab: failed + cancelled
    await expect(page.getByRole("tab", { name: /Failed \(1\)/ })).toBeVisible();
  });

  test("active filter shows only active workflows", async ({ page }) => {
    await page.goto("/workflows");

    await page.getByRole("tab", { name: /Active/ }).click();

    // Active workflow should be visible
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).toBeVisible({ timeout: 10000 });
    // Completed/failed should NOT be visible
    await expect(
      page.getByText("Another.Movie.2023.720p.WEB-DL"),
    ).not.toBeVisible();
    await expect(page.getByText("Test.Show.S01E05.HDTV")).not.toBeVisible();
  });

  test("completed filter shows only completed workflows", async ({ page }) => {
    await page.goto("/workflows");

    await page.getByRole("tab", { name: /Completed/ }).click();

    await expect(page.getByText("Another.Movie.2023.720p.WEB-DL")).toBeVisible({
      timeout: 10000,
    });
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).not.toBeVisible();
  });

  test("failed filter shows only failed workflows", async ({ page }) => {
    await page.goto("/workflows");

    await page.getByRole("tab", { name: /Failed/ }).click();

    await expect(page.getByText("Test.Show.S01E05.HDTV")).toBeVisible({
      timeout: 10000,
    });
    await expect(
      page.getByText("Test.Movie.2024.1080p.BluRay.x264"),
    ).not.toBeVisible();
  });

  test("displays state badges correctly", async ({ page }) => {
    await page.goto("/workflows");

    // State labels appear within table rows (not tabs)
    const table = page.locator("table");
    await expect(table.getByText("Downloading")).toBeVisible({
      timeout: 10000,
    });
    await expect(table.getByText("Completed")).toBeVisible();
    await expect(table.getByText("Failed")).toBeVisible();
  });

  test("shows error message for failed workflow", async ({ page }) => {
    await page.goto("/workflows");

    // Error text should be visible (truncated)
    await expect(page.getByText(/all retries exhausted/)).toBeVisible({
      timeout: 10000,
    });
  });

  test("shows retry count for failed workflow", async ({ page }) => {
    await page.goto("/workflows");

    // Retry count "3/3"
    await expect(page.getByText("3/3")).toBeVisible({ timeout: 10000 });
  });

  test("action menu opens for active workflow", async ({ page }) => {
    await page.goto("/workflows");

    // Find the row containing the active workflow, then its action trigger
    const row = page
      .locator("tr")
      .filter({ hasText: "Test.Movie.2024.1080p.BluRay.x264" });
    await row.locator("button").last().click();

    // Cancel option should be visible for active workflows
    await expect(page.getByRole("menuitem", { name: /Cancel/ })).toBeVisible({
      timeout: 5000,
    });
  });

  test("action menu opens for failed workflow with retry option", async ({
    page,
  }) => {
    await page.goto("/workflows");

    // Click actions on failed workflow row
    const row = page.locator("tr").filter({ hasText: "Test.Show.S01E05.HDTV" });
    await row.locator("button").last().click();

    // Retry and Delete should be available
    await expect(page.getByRole("menuitem", { name: /Retry/ })).toBeVisible({
      timeout: 5000,
    });
    await expect(page.getByRole("menuitem", { name: /Delete/ })).toBeVisible();
  });

  test("cancel action shows confirmation dialog", async ({ page }) => {
    await page.goto("/workflows");

    const row = page
      .locator("tr")
      .filter({ hasText: "Test.Movie.2024.1080p.BluRay.x264" });
    await row.locator("button").last().click();
    await page.getByRole("menuitem", { name: /Cancel/ }).click();

    // Confirmation dialog should appear
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/Cancel the workflow/)).toBeVisible();
  });

  test("delete action shows confirmation dialog", async ({ page }) => {
    await page.goto("/workflows");

    const row = page.locator("tr").filter({ hasText: "Test.Show.S01E05.HDTV" });
    await row.locator("button").last().click();
    await page.getByRole("menuitem", { name: /Delete/ }).click();

    // Confirmation dialog should appear
    await expect(page.getByRole("dialog")).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/Delete the workflow/)).toBeVisible();
  });

  test("shows empty state when no workflows exist", async ({ page }) => {
    // Override with empty workflows
    await mockGet(page, "workflows", []);

    await page.goto("/workflows");

    await expect(page.getByText(/Workflows will appear here/)).toBeVisible({
      timeout: 10000,
    });
  });

  test("shows workflow type labels", async ({ page }) => {
    await page.goto("/workflows");

    await expect(page.getByText("Movie Search").first()).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("Episode Search")).toBeVisible();
  });
});
