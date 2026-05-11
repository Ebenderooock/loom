import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Indexers", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("indexers page loads", async ({ page }) => {
    await page.goto("/indexers");
    await waitForPageLoad(page);
    await expect(page.getByText(/Indexers/i).first()).toBeVisible({ timeout: 15000 });
  });

  test("indexers API returns configured indexers", async ({ request }) => {
    const res = await authGet(request, "/api/v1/indexers");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const indexers = body.indexers ?? body.data ?? (Array.isArray(body) ? body : []);
    expect(Array.isArray(indexers)).toBeTruthy();
    if (indexers.length > 0) {
      expect(indexers[0]).toHaveProperty("id");
      expect(indexers[0]).toHaveProperty("name");
      expect(indexers[0]).toHaveProperty("enabled");
    }
  });

  test("indexer health API is reachable", async ({ request }) => {
    const res = await authGet(request, "/api/v1/indexers/health");
    expect(res.status()).toBe(200);
  });

  test("indexer definitions API returns available definitions", async ({ request }) => {
    const res = await authGet(request, "/api/v1/indexers/definitions");
    expect(res.status()).toBe(200);
    const defs = await res.json();
    const list = Array.isArray(defs) ? defs : defs.data ?? [];
    expect(list.length).toBeGreaterThan(0);
  });
});
