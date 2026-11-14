import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockMovies,
  mockRoute,
  SAMPLE_LIBRARY,
} from "./helpers/mock-api";

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await mockMovies(page, []);
  await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
  await mockGet(page, "features", { features: [] });
  await mockGet(page, "media-info/preferences", {});
  await mockGet(page, "quality-profiles", [
    { id: "qp-1", name: "Any" },
    { id: "1080p", name: "1080p" },
    { id: "4k", name: "4K" },
  ]);
  await mockRoute(page, "GET", "movies/lookup?*", (route) =>
    route.fulfill({
      json:
        new URL(route.request().url()).searchParams.get("term") === "Other"
          ? [{ tmdb_id: "43", title: "Other movie" }]
          : [
              { tmdb_id: "41", title: "First movie" },
              { tmdb_id: "42", title: "Second movie" },
            ],
    }),
  );
  await mockRoute(page, "GET", "discover/movie/*", (route) =>
    route.fulfill({
      json: {
        title: {
          tmdb_id: route.request().url().split("/").pop(),
          title: "Movie with a long synopsis",
          overview:
            "A long description of this movie and its characters. ".repeat(20),
          poster_path: "/poster.jpg",
          backdrop_path: "/backdrop.jpg",
        },
        credits: { cast: [], crew: [] },
      },
    }),
  );
});

for (const viewport of [
  { width: 375, height: 667 },
  { width: 667, height: 375 },
]) {
  test(`compact add at ${viewport.width}x${viewport.height} preserves search and quality`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await mockRoute(page, "POST", "movies", (route) =>
      route.fulfill({ json: { id: "created" } }),
    );
    await page.goto("/movies");
    await page.getByRole("button", { name: "Add Movie" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("textbox", { name: "Search titles" }).fill("Movies");
    await dialog
      .getByRole("button", { name: "Add", exact: true })
      .first()
      .click();
    const quality = dialog.getByLabel("Quality profile");
    await expect(quality).toBeVisible();
    await quality.selectOption("1080p");
    const add = dialog.getByRole("button", {
      name: "Add to library",
      exact: true,
    });
    await expect(add).toBeEnabled();
    expect(await dialog.locator("img").count()).toBe(1);
    const synopsis = dialog.getByText(/^A long description/);
    await expect(synopsis).toHaveClass(/line-clamp-2/);
    await dialog.getByRole("button", { name: "Read more" }).click();
    await expect(synopsis).not.toHaveClass(/line-clamp/);
    await dialog.getByRole("button", { name: "Show less" }).click();
    await add.scrollIntoViewIfNeeded();
    await expect(add).toBeInViewport();
    const dimensions = await dialog.evaluate((element) => ({
      width: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.width);
    await add.click();
    await expect(
      dialog.getByRole("textbox", { name: "Search titles" }),
    ).toHaveValue("Movies");
    await expect(
      dialog.getByRole("button", { name: "In library", exact: true }),
    ).toBeDisabled();
    await dialog.getByRole("button", { name: "Add", exact: true }).click();
    await expect(quality).toHaveValue("1080p");
    await quality.selectOption("4k");
    await add.click();
    await expect(
      dialog.getByRole("textbox", { name: "Search titles" }),
    ).toHaveValue("Movies");
    await dialog.getByRole("button", { name: "Close", exact: true }).click();
    await page.getByRole("button", { name: "Add Movie" }).click();
    await dialog.getByRole("textbox", { name: "Search titles" }).fill("Other");
    await dialog.getByRole("button", { name: "Add", exact: true }).click();
    await expect(quality).toHaveValue("4k");
  });
}

test("adds a selection spanning searches with a shared quality", async ({
  page,
}) => {
  const writes: Record<string, unknown>[] = [];
  await mockRoute(page, "POST", "movies", (route) => {
    writes.push(route.request().postDataJSON());
    return route.fulfill({ json: { id: `created-${writes.length}` } });
  });
  await page.goto("/movies");
  await page.getByRole("button", { name: "Add Movie" }).click();
  const dialog = page.getByRole("dialog");
  const search = dialog.getByRole("textbox", { name: "Search titles" });
  await search.fill("Movies");
  await dialog.getByRole("checkbox", { name: "Select First movie" }).check();
  await search.fill("Other");
  await dialog.getByRole("checkbox", { name: "Select Other movie" }).check();
  await dialog.getByRole("button", { name: "Review selected" }).click();
  await dialog.getByLabel("Quality profile").selectOption("1080p");
  await dialog.getByRole("button", { name: "Add 2 selected" }).click();
  await expect(search).toHaveValue("Other");
  expect(writes.map((body) => body.tmdb_id)).toEqual(["41", "43"]);
  expect(writes.every((body) => body.quality_profile_id === "1080p")).toBe(
    true,
  );
});
