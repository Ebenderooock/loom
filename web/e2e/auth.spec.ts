import { test, expect } from "@playwright/test";
import { mockBaseApp, mockDashboard, TEST_USER } from "./helpers/mock-api";

test.describe("Authentication", () => {
  test("shows login page when not authenticated", async ({ page }) => {
    await mockBaseApp(page, { authenticated: false });
    await page.goto("/");

    // The login page renders a real h1: "Welcome back to Loom"
    await expect(
      page.getByRole("heading", { name: "Welcome back to Loom", level: 1 }),
    ).toBeVisible();
    await expect(page.getByLabel("Username")).toBeVisible();
    await expect(page.getByLabel("Password")).toBeVisible();
  });

  test("shows setup page when setup not complete", async ({ page }) => {
    await mockBaseApp(page, { authenticated: false, setupComplete: false });
    await page.goto("/");

    // Setup page renders a real h1: "Welcome to Loom"
    await expect(
      page.getByRole("heading", { name: "Welcome to Loom", level: 1 }),
    ).toBeVisible();
  });

  test("login with valid credentials redirects to dashboard", async ({
    page,
  }) => {
    await mockBaseApp(page, { authenticated: false });
    await mockDashboard(page);

    await page.goto("/");

    // Fill in credentials
    await page.getByLabel("Username").fill("admin");
    await page.getByLabel("Password").fill("password");

    // Submit — button text is "Login"
    await page.getByRole("button", { name: "Login" }).click();

    // After successful login, should redirect to dashboard
    await expect(page).toHaveURL("/", { timeout: 10000 });
  });

  test("login with invalid credentials shows error", async ({ page }) => {
    await mockBaseApp(page, { authenticated: false });
    await page.goto("/");

    await page.getByLabel("Username").fill("admin");
    await page.getByLabel("Password").fill("wrongpassword");

    await page.getByRole("button", { name: "Login" }).click();

    // Should show error message — server returns "Invalid credentials"
    await expect(page.getByText(/Invalid credentials/i)).toBeVisible({
      timeout: 5000,
    });
  });
});
