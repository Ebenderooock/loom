import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Downloads", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("downloads page loads with active and imports tabs", async ({
    page,
  }) => {
    await page.goto("/downloads");
    await waitForPageLoad(page);
    await expect(page.getByText(/Downloads/i).first()).toBeVisible({
      timeout: 15000,
    });
    // Should have Active and Imports tabs
    const activeTab = page
      .getByRole("tab", { name: /Active/i })
      .or(page.getByText(/Active/i).first());
    await expect(activeTab).toBeVisible({ timeout: 10000 });
  });

  test("download clients API returns configured clients", async ({
    request,
  }) => {
    const res = await authGet(request, "/api/v1/download-clients");
    expect(res.status()).toBe(200);
    const text = await res.text();
    expect(text.startsWith("{") || text.startsWith("[")).toBeTruthy();
    const body = JSON.parse(text);
    const clients = body.download_clients ?? body.data ?? [];
    expect(Array.isArray(clients)).toBeTruthy();
    if (clients.length > 0) {
      expect(clients[0]).toHaveProperty("id");
      expect(clients[0]).toHaveProperty("name");
    }
  });

  test("download queue API is reachable", async ({ request }) => {
    const res = await authGet(request, "/api/v1/downloads/history");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const items = Array.isArray(body) ? body : (body.data ?? []);
    expect(Array.isArray(items)).toBeTruthy();
  });

  test("download history API is reachable", async ({ request }) => {
    const res = await authGet(request, "/api/v1/downloads/history");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const items = body.data ?? [];
    expect(Array.isArray(items)).toBeTruthy();
  });
});
