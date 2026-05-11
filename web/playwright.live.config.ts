/**
 * Playwright config for running E2E tests against a live Loom instance.
 *
 * Usage:
 *   LOOM_URL=https://loom.media.deroock.co.za \
 *   LOOM_USER=admin LOOM_PASS=secret \
 *   npx playwright test --config playwright.live.config.ts
 */
import { defineConfig, devices } from "@playwright/test";

const baseURL = process.env.LOOM_URL || "https://loom.media.deroock.co.za";

export default defineConfig({
  testDir: "./e2e/live",
  fullyParallel: false,       // serial — avoids hammering the live instance
  retries: 1,
  workers: 1,
  timeout: 60_000,            // live calls can be slow
  expect: { timeout: 15_000 },
  reporter: "list",
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    ignoreHTTPSErrors: true,
  },
  projects: [
    {
      name: "live-chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  // No webServer — we connect to the remote instance directly
});
