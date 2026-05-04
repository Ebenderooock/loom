import { test, expect } from "@playwright/test";

test.describe("Movies Library Management", () => {
  test.beforeEach(async ({ page }) => {
    // Ensure we're logged in by checking auth status
    // If not authenticated, log in first
    await page.goto("/");

    // Check if we need to log in
    const setupPage = page.url();
    if (setupPage.includes("setup")) {
      // Skip if on setup page - assume already set up in test environment
      return;
    }

    // Navigate to movies page
    await page.goto("/movies");
  });

  test("movies page loads when authenticated", async ({ page }) => {
    // Should see the Movies heading
    await expect(
      page.getByRole("heading", { name: "Movies", level: 1 }),
    ).toBeVisible();

    // Should see Library Folders section
    await expect(
      page.getByRole("heading", { name: "Library Folders" }),
    ).toBeVisible();

    // Should see Add Library button
    await expect(
      page.getByRole("button", { name: /add library/i }),
    ).toBeVisible();
  });

  test("add library modal flow works", async ({ page }) => {
    // Click Add Library button
    await page.getByRole("button", { name: /add library/i }).click();

    // Should see type selection modal
    await expect(
      page.getByRole("heading", { name: "Add Library" }),
    ).toBeVisible();

    // Should see Movies and Series options
    const moviesButton = page.locator("button").filter({
      has: page.locator("text=Movies"),
    });
    const seriesButton = page.locator("button").filter({
      has: page.locator("text=Series"),
    });

    await expect(moviesButton).toBeVisible();
    await expect(seriesButton).toBeVisible();
  });

  test("can select movies library type", async ({ page }) => {
    // Click Add Library button
    await page.getByRole("button", { name: /add library/i }).click();

    // Click Movies button
    const moviesButton = page.locator("button").filter({
      has: page.locator("text=Movies"),
    });
    await moviesButton.click();

    // Should see input method selection
    await expect(
      page.getByRole("heading", { name: /add movie library/i }),
    ).toBeVisible();

    // Should see Browse and Enter Manually options
    await expect(page.getByText("Browse")).toBeVisible();
    await expect(page.getByText("Enter Manually")).toBeVisible();
  });

  test("can enter library path manually", async ({ page }) => {
    // Click Add Library button
    await page.getByRole("button", { name: /add library/i }).click();

    // Select Movies
    const moviesButton = page.locator("button").filter({
      has: page.locator("text=Movies"),
    });
    await moviesButton.click();

    // Click Enter Manually
    await page.getByText("Enter Manually").click();

    // Should see input field
    const input = page.locator('input[placeholder*="/mnt/movies"]');
    await expect(input).toBeVisible();

    // Add Library button should be disabled initially
    const addButton = page.getByRole("button", { name: "Add Library" });
    await expect(addButton).toBeDisabled();

    // Type a path
    await input.fill("/mnt/movies");

    // Now button should be enabled
    await expect(addButton).toBeEnabled();
  });

  test("back button works in modal", async ({ page }) => {
    // Click Add Library
    await page.getByRole("button", { name: /add library/i }).click();

    // Should see type selection
    await expect(
      page.getByRole("heading", { name: "Add Library" }),
    ).toBeVisible();

    // Click Back should close modal (or go back a step)
    // Let's select a type first
    const moviesButton = page.locator("button").filter({
      has: page.locator("text=Movies"),
    });
    await moviesButton.click();

    // Now click back
    await page.getByRole("button", { name: "Back" }).click();

    // Should be back to type selection
    await expect(
      page.getByRole("heading", { name: "Add Library" }),
    ).toBeVisible();
  });

  test("your movies section displays", async ({ page }) => {
    // Should see Your Movies section
    await expect(
      page.getByRole("heading", { name: /your movies/i }),
    ).toBeVisible();

    // Initially should show "No movies found" message
    await expect(
      page.getByText(
        /no movies found\. add library folders and scan them/i,
      ),
    ).toBeVisible();
  });
});
