import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  SAMPLE_LIBRARY,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";

const source = {
  warnings: [],
  stale: false,
  generated_at: "2026-10-28T12:00:00Z",
};
const suggestion = {
  tmdb_id: 107,
  media_type: "movie",
  title: "A new adventure",
  year: 2024,
  rating: 7.7,
  popularity: 50,
  source: "tmdb",
  reasons: ["You have 2 movies starring Ryan Reynolds"],
  collection_id: 10,
};
const collection = {
  id: 10,
  name: "Adventure collection",
  overview: "A connected set of adventures.",
  total: 4,
  in_library: 2,
  missing: 1,
  upcoming: 1,
};

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await mockGet(page, "features", {});
  await mockGet(page, "discover/recommendations?*", {
    ...source,
    items: [suggestion],
  });
  await mockGet(page, "discover/collections", {
    ...source,
    items: [collection],
  });
  await mockGet(page, "discover/collections/10", {
    ...source,
    collection,
    items: [
      {
        ...suggestion,
        tmdb_id: 101,
        title: "Original adventure",
        in_library: true,
        has_file: true,
        monitored: true,
      },
      {
        ...suggestion,
        tmdb_id: 102,
        title: "Pending adventure",
        in_library: true,
        has_file: false,
        monitored: true,
      },
      {
        ...suggestion,
        tmdb_id: 104,
        title: "Missing adventure",
        release_date: "2020-01-01",
        in_library: false,
        monitored: false,
      },
      {
        ...suggestion,
        tmdb_id: 103,
        title: "Future adventure",
        release_date: "2099-01-01",
        in_library: false,
        monitored: false,
      },
    ],
  });
  await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
  await mockGet(page, "quality-profiles", { data: [SAMPLE_QUALITY_PROFILE] });
  await mockGet(page, "media-info/preferences", {
    default_quality_profile_id: "qp-1",
  });
});

test("suggestions need no Import Lists and require an explicit add", async ({
  page,
}) => {
  const writes: string[] = [];
  const listRequests: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "POST") writes.push(request.url());
    if (new URL(request.url()).pathname.startsWith("/api/v1/import-lists"))
      listRequests.push(request.url());
  });
  await page.goto("/discover");
  await expect(
    page.getByRole("heading", { name: "A new adventure" }),
  ).toBeVisible();
  await expect(
    page.getByText("You have 2 movies starring Ryan Reynolds"),
  ).toBeVisible();
  expect(writes).toEqual([]);
  expect(listRequests).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);

  await page.getByRole("button", { name: "Add to library" }).click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "Add Movie" }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("checkbox", { name: "Search after adding" }),
  ).not.toBeChecked();
  await page.route("**/api/v1/movies", async (route) => {
    expect(route.request().postDataJSON()).toMatchObject({
      tmdb_id: "107",
      library_id: "lib-1",
      quality_profile_id: "qp-1",
      search: false,
    });
    await route.fulfill({ status: 201, json: { id: "new-movie" } });
  });
  await dialog.getByRole("button", { name: "Add Movie" }).click();
  await expect(dialog).not.toBeVisible();
  expect(writes).toHaveLength(1);
});

test("collections show missing and upcoming entries without treating catalog membership as a file", async ({
  page,
}, testInfo) => {
  await page.goto("/discover");
  await page.getByRole("tab", { name: "Collections", exact: true }).click();
  await page.getByRole("button", { name: "View Adventure collection" }).click();
  await expect(
    page.getByRole("heading", { name: "Adventure collection" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Original adventure" }),
  ).toBeVisible();
  await expect(
    page.getByText("No imported file recorded / Monitored"),
  ).toBeVisible();
  await expect(page.getByText("Upcoming", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Add to library" }),
  ).toHaveCount(2);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("collection.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "View Adventure collection" }),
  ).toBeVisible();
});

test("provider failure has a visible retry path", async ({ page }) => {
  await page.route("**/api/v1/discover/recommendations?*", async (route) => {
    await route.fulfill({ status: 503, body: "TMDB access is not configured" });
  });
  await page.goto("/discover");
  await expect(page.getByRole("alert")).toContainText(
    "TMDB access is not configured",
  );
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
  await expect(page.getByText("No new suggestions yet")).not.toBeVisible();
});
