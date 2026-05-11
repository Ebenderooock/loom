/**
 * Shared helpers for live E2E tests.
 * No mocking — all calls go to the real Loom instance.
 */
import { Page, expect, APIRequestContext } from "@playwright/test";

const LOOM_URL = process.env.LOOM_URL || "http://localhost:8080";
const LOOM_USER = process.env.LOOM_USER || "admin";
const LOOM_PASS = process.env.LOOM_PASS || "";

/**
 * Login via API and return authenticated headers for use with the request fixture.
 * Caches the cookie per request context to avoid repeated login calls.
 */
let _cachedCookie: string | null = null;

export async function apiHeaders(request: APIRequestContext): Promise<Record<string, string>> {
  if (_cachedCookie) {
    return { Cookie: "loom_session=" + _cachedCookie };
  }
  const res = await request.post("/api/v1/auth/login", {
    data: { username: LOOM_USER, password: LOOM_PASS },
  });
  const cookies = res.headers()["set-cookie"] || "";
  const match = cookies.match(/loom_session=([^;]+)/);
  if (!match) throw new Error("Failed to extract session cookie from login response");
  _cachedCookie = match[1];
  return { Cookie: "loom_session=" + _cachedCookie };
}

/**
 * Helper for authenticated GET requests.
 */
export async function authGet(request: APIRequestContext, url: string) {
  const headers = await apiHeaders(request);
  return request.get(url, { headers });
}

/**
 * Authenticate against the live instance via the login form.
 * Stores the session cookie so subsequent navigations stay logged in.
 */
export async function liveLogin(page: Page) {
  await page.goto("/");
  await page.waitForLoadState("domcontentloaded");

  // Race: either the login form or the sidebar (already authenticated) appears
  const loginBtn = page.getByRole("button", { name: /login/i });
  const sidebar = page.locator("nav, aside, [data-sidebar]").first();

  // Wait for whichever appears first
  const winner = await Promise.race([
    loginBtn.waitFor({ state: "visible", timeout: 20000 }).then(() => "login" as const),
    sidebar.waitFor({ state: "visible", timeout: 20000 }).then(() => "sidebar" as const),
  ]);

  if (winner === "sidebar") {
    // Already authenticated
    return;
  }

  // Fill login form
  await page.getByLabel("Username").fill(LOOM_USER);
  await page.getByLabel("Password").fill(LOOM_PASS);
  await loginBtn.click();

  // Wait for redirect to authenticated app
  await expect(sidebar).toBeVisible({ timeout: 20000 });
}

/**
 * Wait for the main content area to finish its initial load.
 */
export async function waitForPageLoad(page: Page) {
  // Wait for skeleton/spinner to disappear and real content to appear
  await page.waitForLoadState("networkidle", { timeout: 30000 }).catch(() => {});
  await page.waitForTimeout(500); // small buffer for React hydration
}
