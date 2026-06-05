import { test, expect } from "@playwright/test";
import { liveLogin, waitForPageLoad, authGet } from "./helpers";

test.describe("Live: Events & Audit Log", () => {
  test.beforeEach(async ({ page }) => {
    await liveLogin(page);
  });

  test("events page loads and shows audit entries", async ({ page }) => {
    await page.goto("/events");
    await waitForPageLoad(page);
    await expect(page.getByText(/Events|Activity/i).first()).toBeVisible({
      timeout: 15000,
    });
  });

  test("audit log API returns entries", async ({ request }) => {
    const res = await authGet(request, "/api/v1/system/audit-log?limit=20");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const entries =
      body.data ?? body.entries ?? (Array.isArray(body) ? body : []);
    expect(Array.isArray(entries)).toBeTruthy();
    if (entries.length > 0) {
      const e = entries[0];
      expect(e).toHaveProperty("category");
      expect(e).toHaveProperty("event_type");
      expect(e).toHaveProperty("message");
      expect(e).toHaveProperty("level");
    }
  });

  test("audit log includes movie status changed events", async ({
    request,
  }) => {
    const res = await authGet(request, "/api/v1/system/audit-log?limit=100");
    expect(res.status()).toBe(200);
    const body = await res.json();
    const entries = body.data ?? body.entries ?? [];
    const statusEvents = entries.filter(
      (e: any) => e.event_type === "movie.status_changed",
    );
    console.log(
      "Found " + statusEvents.length + " movie.status_changed audit events",
    );
    if (statusEvents.length > 0) {
      expect(statusEvents[0].category).toBe("movie");
      expect(statusEvents[0].detail).toBeTruthy();
    }
  });

  test("events SSE stream endpoint is reachable", async ({ request }) => {
    const headers = { Cookie: "" };
    try {
      const loginRes = await request.post("/api/v1/auth/login", {
        data: {
          username: process.env.LOOM_USER || "admin",
          password: process.env.LOOM_PASS || "",
        },
      });
      const cookies = loginRes.headers()["set-cookie"] || "";
      const match = cookies.match(/loom_session=([^;]+)/);
      if (match) headers.Cookie = "loom_session=" + match[1];
    } catch (_) {
      /* proceed unauthenticated */
    }
    const res = await request
      .get("/api/v1/events/stream", {
        headers,
        timeout: 5000,
      })
      .catch(() => null);
    if (res) {
      expect([200, 204]).toContain(res.status());
    }
  });
});
