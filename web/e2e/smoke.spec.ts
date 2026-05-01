import { test, expect } from "@playwright/test";

test("dashboard renders with sidebar and command palette trigger", async ({
  page,
}) => {
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Dashboard", level: 1 }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /open command palette/i }),
  ).toBeVisible();
});
