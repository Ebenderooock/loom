import { test, expect } from "@playwright/test";
import { mockBaseApp, mockGet, mockPost } from "./helpers/mock-api";

const item = {
  id: "discover-1",
  list_id: "list-1",
  external_id: "42",
  tmdb_id: "42",
  title: "Discover Movie",
  year: 2026,
  media_type: "movie",
  list_name: "My picks",
  in_library: false,
  overview: "Feed overview",
  status: "pending",
};

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await mockGet(page, "import-lists/discover?media_type=movie", {
    data: [item],
  });
  await mockGet(page, "import-lists/discover?media_type=series", {
    data: [
      {
        ...item,
        id: "discover-show",
        title: "Discover Show",
        media_type: "series",
      },
    ],
  });
  await mockGet(page, "discover/movie/42", {
    title: "Discover Movie",
    year: 2026,
    overview: "Full movie overview",
    genres: ["Drama"],
    release_date: "2026-10-01",
    runtime: 123,
    rating: 8.2,
  });
  await mockGet(page, "discover/series/42", {
    title: "Discover Show",
    overview: "Full show overview",
    genres: ["Comedy"],
    first_air_date: "2026-10-02",
    seasons: 3,
    rating: 7.8,
  });
});

test("details open a large drawer and add from there without navigation", async ({
  page,
}) => {
  const adds: unknown[] = [];
  await page.route("**/api/v1/import-lists/discover/add", async (route) => {
    adds.push(route.request().postDataJSON());
    await route.fulfill({ json: { data: { ...item, status: "added" } } });
  });
  await page.goto("/discover");
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .click();
  const drawer = page.getByRole("dialog");
  await expect(
    drawer.getByRole("heading", { name: "Discover Movie" }),
  ).toBeVisible();
  await expect(drawer.getByText("Full movie overview")).toBeVisible();
  await expect(drawer.getByText("123 min")).toBeVisible();
  await expect(drawer.getByText("Drama", { exact: true })).toBeVisible();
  await expect(drawer.getByText("8.2 / 10")).toBeVisible();
  expect(adds).toHaveLength(0);
  await drawer.getByRole("button", { name: "Add to library" }).click();
  await expect(
    drawer.getByRole("button", { name: "In Library", exact: true }),
  ).toBeDisabled();
  expect(adds).toEqual([{ item_id: "discover-1" }]);
  await expect(page).toHaveURL(/\/discover$/);
  await page.keyboard.press("Escape");
  await expect(drawer).not.toBeVisible();
  await expect(
    page.getByRole("button", { name: "View details for Discover Movie" }),
  ).toBeFocused();
});

test("quick add stays independent from details", async ({ page }) => {
  await mockPost(page, "import-lists/discover/add", {
    data: { ...item, status: "added" },
  });
  await page.goto("/discover");
  await page.getByRole("button", { name: "Quick add Discover Movie" }).click();
  await expect(
    page.getByRole("button", { name: "Added", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .click();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "In Library", exact: true }),
  ).toBeDisabled();
});

test("shows full series metadata and uses the series item for add", async ({
  page,
}) => {
  let added: unknown;
  await page.route("**/api/v1/import-lists/discover/add", async (route) => {
    added = route.request().postDataJSON();
    await route.fulfill({ json: { data: { ...item, status: "added" } } });
  });
  await page.goto("/discover");
  await page.getByRole("tab", { name: "TV Shows" }).click();
  await page
    .getByRole("button", { name: "View details for Discover Show" })
    .click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("Full show overview")).toBeVisible();
  await expect(drawer.getByText("Seasons", { exact: true })).toBeVisible();
  await expect(drawer.getByText("3", { exact: true })).toBeVisible();
  await drawer.getByRole("button", { name: "Add to library" }).click();
  await expect(
    drawer.getByRole("button", { name: "In Library", exact: true }),
  ).toBeDisabled();
  expect(added).toEqual({ item_id: "discover-show" });
});

test("add failures stay visible in the drawer and can be retried", async ({
  page,
}) => {
  await page.route("**/api/v1/import-lists/discover/add", async (route) =>
    route.fulfill({ status: 500, json: { error: "Library unavailable" } }),
  );
  await page.goto("/discover");
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .click();
  const drawer = page.getByRole("dialog");
  await drawer.getByRole("button", { name: "Add to library" }).click();
  await expect(drawer.getByRole("alert")).toHaveText("Library unavailable");
  await expect(
    drawer.getByRole("button", { name: "Add to library" }),
  ).toBeEnabled();
});

test("items without a TMDB ID still show available details and can be added", async ({
  page,
}) => {
  await mockGet(page, "import-lists/discover?media_type=movie", {
    data: [{ ...item, tmdb_id: "" }],
  });
  await page.goto("/discover");
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("Feed overview")).toBeVisible();
  await expect(
    drawer.getByRole("button", { name: "Add to library" }),
  ).toBeEnabled();
  await expect(drawer.getByText("Loading full details…")).not.toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("drawer stays usable on a small screen and landscape in dark reduced-motion mode", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 375, height: 667 });
  await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
  await page.addInitScript(() =>
    window.localStorage.setItem("loom.theme", "dark"),
  );
  await page.goto("/discover");
  await page
    .getByRole("button", { name: "View details for Discover Movie" })
    .click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("Full movie overview")).toBeVisible();
  await expect(
    drawer.getByRole("button", { name: "Add to library" }),
  ).toBeInViewport();
  await page.screenshot({
    path: testInfo.outputPath("discover-drawer-dark.png"),
  });
  await page.setViewportSize({ width: 667, height: 375 });
  await expect(
    drawer.getByRole("button", { name: "Add to library" }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
