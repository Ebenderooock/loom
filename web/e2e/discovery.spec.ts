import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  SAMPLE_LIBRARY,
  SAMPLE_QUALITY_PROFILE,
} from "./helpers/mock-api";
import { DEFAULT_APPEARANCE } from "../src/lib/appearance-api";
import type { CollectionCover } from "../src/lib/collections-api";

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
  poster_path: "/adventure.jpg",
  reasons: ["You have 2 movies starring Ryan Reynolds"],
  collection_id: 10,
};
const collection = {
  id: "loom-collection",
  name: "Adventure collection",
  overview: "A connected set of adventures.",
  revision: 1,
  total: 3,
  in_library: 1,
  sources: [{ provider: "tmdb", kind: "collection", external_id: "10" }],
  members: [
    {
      id: "member-1",
      media_type: "movie",
      title: "Original adventure",
      external_ids: { tmdb: "101" },
      sources: ["tmdb:collection:10"],
      provenance: ["tmdb:collection:10"],
      local: false,
      excluded: false,
      in_library: true,
      media_id: "actual-movie",
      library_id: "lib-1",
      status: "missing",
      monitoring_status: "monitored",
      has_file: false,
      monitored: true,
    },
    {
      id: "member-2",
      media_type: "series",
      title: "Connected show",
      external_ids: { tmdb: "101" },
      sources: ["tmdb:collection:10"],
      provenance: ["tmdb:collection:10"],
      local: false,
      excluded: false,
      in_library: false,
      monitored: false,
    },
    {
      id: "member-3",
      media_type: "movie",
      title: "Future adventure",
      external_ids: { tmdb: "103" },
      sources: ["tmdb:collection:10"],
      provenance: ["tmdb:collection:10"],
      local: false,
      excluded: false,
      in_library: false,
      monitored: false,
      release_date: "2099-01-01",
    },
  ],
};

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await page.route("https://image.tmdb.org/**", async (route) => {
    await route.fulfill({
      contentType: "image/svg+xml",
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="342" height="513" viewBox="0 0 342 513"><defs><linearGradient id="sky" x2="0" y2="1"><stop stop-color="#173552"/><stop offset="1" stop-color="#c06b43"/></linearGradient></defs><rect width="342" height="513" fill="url(#sky)"/><circle cx="250" cy="130" r="70" fill="#edc784"/><path d="M0 400L95 140L200 400L270 250L342 400V513H0Z" fill="#192d39"/><path d="M45 400L96 240L150 400Z" fill="#366070"/></svg>`,
    });
  });
  await mockGet(page, "features", {});
  await mockGet(page, "discover/recommendations?*", {
    ...source,
    items: [suggestion],
  });
  await mockGet(page, "collections/", { items: [collection] });
  await mockGet(page, "collections/scan", {
    total: 1020,
    checked: 540,
    pending: 480,
    failed: 0,
    running: true,
    errors: [],
  });
  await mockGet(page, "collections/loom-collection", collection);
  await mockGet(page, "movies/actual-movie", {
    id: "actual-movie",
    title: "Original adventure",
    year: 2020,
    overview: "The original movie.",
    tmdbId: "101",
    rating: 7,
    runtime: 120,
    genres: ["Adventure"],
    releaseDate: "2020-01-01",
    status: "missing",
    monitoringStatus: "monitored",
    qualityProfileId: "qp-1",
    libraryId: "lib-1",
  });
  await mockGet(page, "movies/actual-movie/credits", { cast: [], crew: [] });
  await mockGet(page, "movies/files/actual-movie", []);
  await mockGet(page, "movies/actual-movie/history", []);
  await mockGet(page, "media-files/movie/actual-movie*", {
    files: [],
    warnings: [],
  });
  await mockGet(page, "libraries", { data: [SAMPLE_LIBRARY] });
  await mockGet(page, "quality-profiles", { data: [SAMPLE_QUALITY_PROFILE] });
  await mockGet(page, "media-info/preferences", {
    default_quality_profile_id: "qp-1",
  });
  await mockGet(page, "discover/movie/107", {
    title: {
      tmdb_id: "107",
      title: "A new adventure",
      overview: "A complete synopsis.",
      poster_path: "/adventure.jpg",
      rating: 7.7,
      release_date: "2024-01-01",
      genres: ["Adventure"],
    },
    credits: { cast: [], crew: [] },
    credits_error: "",
  });
  await mockGet(page, "discover/series/101?provider=tmdb", {
    title: {
      tmdb_id: "101",
      title: "Connected show",
      overview: "A connected TV series.",
      first_air_date: "2021-01-01",
      genres: ["Adventure"],
    },
    credits: { cast: [], crew: [] },
    credits_error: "",
  });
  await mockGet(page, "movies?limit=1000&offset=0", []);
  await mockGet(page, "series", []);
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
    page.getByRole("button", { name: "View A new adventure", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("You have 2 movies starring Ryan Reynolds"),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Add to library", exact: true }),
  ).toHaveCount(0);
  expect(writes).toEqual([]);
  expect(listRequests).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);

  await page
    .getByRole("button", { name: "View A new adventure", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("heading", { name: "A new adventure" }).last(),
  ).toBeVisible();
  await expect(
    dialog.getByRole("checkbox", { name: "Search after adding" }),
  ).not.toBeChecked();
  let added = false;
  // Backend movie IDs keep title punctuation; the drawer must request the
  // persisted ID returned by POST, browser-encoded, not a fake local ID.
  const movieID = "a/new-adventure:-part-#1?-50%25-2024";
  const detailPaths: string[] = [];
  await page.route("**/api/v1/movies?limit=1000&offset=0", async (route) => {
    await route.fulfill({
      json: added ? [{ id: movieID, tmdbId: "107" }] : [],
    });
  });
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/v1/movies/") &&
      decodeURIComponent(url.pathname).includes(movieID),
    async (route) => {
      const path = new URL(route.request().url()).pathname;
      detailPaths.push(path);
      const decoded = decodeURIComponent(path);
      await route.fulfill({
        json: decoded.endsWith("/credits")
          ? { cast: [], crew: [] }
          : decoded.endsWith("/history") || decoded.includes("/files/")
            ? []
            : {
                id: movieID,
                tmdbId: "107",
                title: "A new adventure",
                status: "missing",
                year: 2024,
                monitoringStatus: "monitored",
                libraryId: "lib-1",
                qualityProfileId: "qp-1",
                genres: [],
              },
      });
    },
  );
  await mockGet(page, "media-files/movie/**", {
    files: [],
    warnings: [],
  });
  await page.route("**/api/v1/movies", async (route) => {
    expect(route.request().postDataJSON()).toMatchObject({
      tmdb_id: "107",
      library_id: "lib-1",
      quality_profile_id: "qp-1",
      search: false,
    });
    added = true;
    await route.fulfill({ status: 201, json: { id: movieID } });
  });
  await dialog.getByRole("button", { name: "Add to library" }).click();
  await expect(
    dialog.getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  const encodedID = encodeURIComponent(movieID);
  await expect
    .poll(() => [...detailPaths].sort())
    .toEqual(
      expect.arrayContaining([
        `/api/v1/movies/${encodedID}`,
        `/api/v1/movies/${encodedID}/credits`,
        `/api/v1/movies/${encodedID}/history`,
        `/api/v1/movies/files/${encodedID}`,
      ]),
    );
  expect(writes).toHaveLength(1);
});

test("library and collection card status stay identical during acquisition", async ({
  page,
}, testInfo) => {
  await page.clock.install();
  let status = "missing";
  const movie = () => ({
    id: "actual-movie",
    tmdbId: "101",
    title: "Jurassic World",
    year: 2015,
    status,
    posterPath: "/jurassic.jpg",
    monitoringStatus: "monitored",
    qualityProfileId: "qp-1",
    libraryId: "lib-1",
    genres: [],
    overview: "",
    rating: 0,
  });
  const currentCollection = () => ({
    ...collection,
    members: collection.members.map((m, i) =>
      i === 0
        ? {
            ...m,
            title: movie().title,
            poster_path: movie().posterPath,
            status,
            has_file: status === "available_right_quality",
          }
        : m,
    ),
  });
  await page.route("**/api/v1/movies?*", (route) =>
    route.fulfill({ json: { data: [movie()], total: 1 } }),
  );
  await page.route("**/api/v1/collections/loom-collection", (route) =>
    route.fulfill({ json: currentCollection() }),
  );

  status = "downloading";
  await page.goto("/movies");
  await page.waitForLoadState("networkidle");
  const libraryCard = page
    .getByRole("button")
    .filter({ hasText: "Jurassic World" })
    .first();
  await expect(
    libraryCard.getByText("Downloading", { exact: true }).first(),
  ).toBeVisible();
  const libraryBadge = await libraryCard
    .getByText("Downloading", { exact: true })
    .first()
    .evaluate((el) => ({
      color: getComputedStyle(el).color,
      dot: getComputedStyle(el.querySelector("span")!).backgroundColor,
    }));
  const libraryStrip = await libraryCard
    .locator(".h-\\[3px\\]")
    .evaluate((el) => el.getAttribute("style"));
  await page.screenshot({
    path: testInfo.outputPath("library-downloading.png"),
    fullPage: true,
  });

  await page.goto("/collections?id=loom-collection");
  await page.waitForLoadState("networkidle");
  const card = page.getByRole("button", {
    name: "Manage Jurassic World",
    exact: true,
  });
  await expect(card.getByText("Downloading", { exact: true })).toBeVisible();
  expect(
    await card.getByText("Downloading", { exact: true }).evaluate((el) => ({
      color: getComputedStyle(el).color,
      dot: getComputedStyle(el.querySelector("span")!).backgroundColor,
    })),
  ).toEqual(libraryBadge);
  expect(
    await card
      .locator(".h-\\[3px\\]")
      .evaluate((el) => el.getAttribute("style")),
  ).toBe(libraryStrip);
  await page.screenshot({
    path: testInfo.outputPath("collection-downloading.png"),
    fullPage: true,
  });

  status = "available_right_quality";
  await page.clock.fastForward(30_000);
  await expect(card.getByText("Available", { exact: true })).toBeVisible();
  await expect(card.getByText("available", { exact: true })).toHaveCount(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("collection-available.png"),
    fullPage: true,
  });
});

test("standalone mixed collections open real library management and shared external details", async ({
  page,
}, testInfo) => {
  await page.goto("/discover");
  await expect(
    page.getByRole("tab", { name: "Collections", exact: true }),
  ).toHaveCount(0);
  await page.goto("/collections?tmdb=10");
  await expect(
    page.getByRole("heading", { name: "Adventure collection" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Original adventure" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Manage Original adventure" }).click();
  const management = page.getByRole("dialog");
  await expect(
    management.getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  await expect(
    management.getByRole("button", { name: "Manual Search", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(management).not.toBeVisible();
  await page.getByRole("button", { name: "View Connected show" }).click();
  const external = page.getByRole("dialog");
  await expect(external.getByText("A connected TV series.")).toBeVisible();
  await expect(
    external.getByRole("button", { name: "Add to library", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(external).not.toBeVisible();
  await page
    .getByRole("combobox", { name: "Status", exact: true })
    .selectOption("upcoming");
  await expect(
    page.getByRole("heading", { name: "Future adventure" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Original adventure" }),
  ).toHaveCount(0);
  await page
    .getByRole("combobox", { name: "Status", exact: true })
    .selectOption("");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("collection.png"),
    fullPage: true,
  });

  await page
    .getByRole("button", { name: "All collections", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: /Adventure collection/ }),
  ).toBeVisible();
});

test("recommendation dismissal is compact, translucent and revealed by hover or focus", async ({
  page,
  isMobile,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/discover");
  const poster = page.getByRole("button", {
    name: "View A new adventure",
    exact: true,
  });
  const remove = page.getByRole("button", {
    name: "Dismiss A new adventure",
  });
  await expect(poster).toBeVisible();
  await expect(remove).toHaveCSS("opacity", isMobile ? "1" : "0");
  if (!isMobile) {
    await poster.hover();
    await expect(remove).toHaveCSS("opacity", "1");
    await expect(remove).toHaveCSS("width", "28px");
    await page.getByRole("heading", { name: "What to add next" }).hover();
    await expect(remove).toHaveCSS("opacity", "0");
    await poster.focus();
    await expect(remove).toHaveCSS("opacity", "1");
    await page.keyboard.press("Tab");
    await expect(remove).toBeFocused();
  } else {
    await expect(remove).toHaveCSS("width", "44px");
  }
  await expect(remove).toHaveCSS("background-color", "rgba(0, 0, 0, 0.5)");
  await page.route("**/api/v1/discover/dismissals", (route) =>
    route.fulfill({
      status: 503,
      json: { error: "Could not dismiss suggestion" },
    }),
  );
  await remove.click();
  await expect(page.getByRole("alert")).toContainText(
    "Could not dismiss suggestion",
  );
  await expect(poster).toBeVisible();
});

test("collection mosaics grow square grids while retaining 2:3 posters and empty cells", async ({
  page,
}, testInfo) => {
  for (const count of [2, 4, 6, 7, 10]) {
    const members = Array.from({ length: count }, (_, index) => ({
      ...collection.members[0],
      id: `poster-${index}`,
      poster_path:
        count === 7 && index === 1 ? undefined : `/poster-${index}.jpg`,
    }));
    await mockGet(page, "collections/", {
      items: [
        {
          ...collection,
          total: count,
          members: [
            ...members,
            { ...members[0], id: "excluded", excluded: true },
          ],
        },
      ],
    });
    await page.goto("/collections");
    const card = page.getByRole("link", { name: /Adventure collection/ });
    await expect(card).toBeVisible();
    const grid = card.locator(".grid");
    const size = Math.max(2, Math.ceil(Math.sqrt(count)));
    await expect(grid.locator(":scope > div")).toHaveCount(size * size);
    await expect(grid.locator("img")).toHaveCount(
      count === 7 ? count - 1 : count,
    );
    const geometry = await grid.evaluate((element) => {
      const frame = element.parentElement!.getBoundingClientRect();
      return {
        frameRatio: frame.width / frame.height,
        cells: Array.from(element.children, (cell) => {
          const box = cell.getBoundingClientRect();
          const img = cell.querySelector("img");
          return {
            ratio: box.width / box.height,
            top: box.top,
            left: box.left,
            fits:
              box.left >= frame.left - 1 &&
              box.right <= frame.right + 1 &&
              box.top >= frame.top - 1 &&
              box.bottom <= frame.bottom + 1,
            objectFit: img ? getComputedStyle(img).objectFit : null,
          };
        }),
      };
    });
    expect(geometry.frameRatio).toBeCloseTo(2 / 3, 2);
    for (const cell of geometry.cells) {
      expect(cell.ratio).toBeCloseTo(2 / 3, 2);
      expect(cell.fits).toBe(true);
      if (cell.objectFit) expect(cell.objectFit).toBe("contain");
    }
    for (let row = 0; row < size; row++) {
      for (let column = 1; column < size; column++) {
        expect(geometry.cells[row * size]!.top).toBeCloseTo(
          geometry.cells[row * size + column]!.top,
          1,
        );
        expect(geometry.cells[row * size + column]!.left).toBeGreaterThan(
          geometry.cells[row * size + column - 1]!.left,
        );
      }
      if (row > 0)
        expect(geometry.cells[row * size]!.top).toBeGreaterThan(
          geometry.cells[(row - 1) * size]!.top,
        );
    }
    if (count === 2) {
      await expect(
        grid.locator(":scope > div").nth(2).locator("img"),
      ).toHaveCount(0);
      await expect(
        grid.locator(":scope > div").nth(3).locator("img"),
      ).toHaveCount(0);
    }
    if (count === 7) {
      await expect(
        grid.locator(":scope > div").nth(1).locator("img"),
      ).toHaveCount(0);
      await expect(
        grid.locator(":scope > div").nth(2).locator("img"),
      ).toHaveAttribute("src", /poster-2.jpg/);
    }
  }
  await page.screenshot({
    path: testInfo.outputPath("collection-mosaic.png"),
    fullPage: true,
  });
});

test("collection cover choices persist across navigation and reload without altering titles", async ({
  page,
}, testInfo) => {
  let cover: CollectionCover = { mode: "mosaic" };
  let revision = 1;
  let fail = false;
  const writes: unknown[] = [];
  const artworkCollection = () => ({
    ...collection,
    revision,
    cover,
    sources: [{ ...collection.sources[0], poster_path: "/source-cover.jpg" }],
    members: collection.members.map((member, index) => ({
      ...member,
      poster_path: `/member-${index}.jpg`,
    })),
  });
  await page.route("**/api/v1/collections/", (route) =>
    route.fulfill({ json: { items: [artworkCollection()] } }),
  );
  await page.route("**/api/v1/collections/loom-collection", async (route) => {
    if (route.request().method() === "PATCH") {
      const update: { cover: CollectionCover; revision: number } = route
        .request()
        .postDataJSON();
      writes.push(update);
      if (fail) {
        await route.fulfill({ status: 500, body: "Cover could not be saved" });
        return;
      }
      expect(update.revision).toBe(revision);
      cover = update.cover;
      revision++;
    }
    await route.fulfill({ json: artworkCollection() });
  });
  for (const choice of [
    { mode: "source", path: "/source-cover.jpg" },
    { mode: "member", path: "/member-1.jpg" },
    { mode: "mosaic", path: undefined },
  ]) {
    await page.goto("/collections?id=loom-collection");
    await page.getByRole("button", { name: "Manage", exact: true }).click();
    const controls = page.getByRole("region", { name: "Cover", exact: true });
    await controls.getByLabel("Cover style").selectOption(choice.mode);
    if (choice.mode === "member")
      await controls
        .getByRole("combobox", { name: "Title poster", exact: true })
        .selectOption("member-2");
    await controls
      .getByRole("button", { name: "Save cover", exact: true })
      .click();
    await expect(
      controls.getByRole("button", { name: "Save cover", exact: true }),
    ).toBeEnabled();
    await expect.poll(() => cover.mode).toBe(choice.mode);
    await page.reload();
    await page.getByRole("button", { name: "Manage", exact: true }).click();
    await expect(controls.getByLabel("Cover style")).toHaveValue(choice.mode);
    await page.keyboard.press("Escape");
    await page
      .getByRole("button", { name: "All collections", exact: true })
      .click();
    const card = page.getByRole("link", { name: /Adventure collection/ });
    if (choice.path) {
      await expect(card.locator("img")).toHaveCount(1);
      await expect(card.locator("img")).toHaveAttribute(
        "src",
        new RegExp(choice.path),
      );
      await expect(card.locator("img")).toHaveCSS("object-fit", "contain");
    } else {
      await expect(card.locator(".grid")).toBeVisible();
    }
    await page.screenshot({
      path: testInfo.outputPath(`collection-cover-${choice.mode}.png`),
      fullPage: true,
    });
  }
  expect(writes).toHaveLength(3);
  expect(writes).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        cover: { mode: "source", source_key: "tmdb:collection:10" },
      }),
      expect.objectContaining({
        cover: { mode: "member", member_id: "member-2" },
      }),
    ]),
  );
  fail = true;
  await page.goto("/collections?id=loom-collection");
  await page.getByRole("button", { name: "Manage", exact: true }).click();
  const controls = page.getByRole("region", { name: "Cover", exact: true });
  await controls.getByLabel("Cover style").selectOption("source");
  await controls
    .getByRole("button", { name: "Save cover", exact: true })
    .click();
  await expect(controls.getByRole("alert")).toHaveText(
    "Cover could not be saved",
  );
  expect(cover.mode).toBe("mosaic");
});

for (const theme of ["light", "dark", "amoled"] as const) {
  test(`${theme} page sections stay opaque over an undimmed background`, async ({
    page,
  }, testInfo) => {
    const backgroundUrl = "https://images.loom.test/bright.svg";
    await page.route(backgroundUrl, (route) =>
      route.fulfill({
        contentType: "image/svg+xml",
        body: '<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1000"><path fill="#fff" d="M0 0h1600v1000H0z"/><path fill="#f40" d="M0 1000 1600 0v1000z"/></svg>',
      }),
    );
    await mockGet(page, "appearance", {
      ...DEFAULT_APPEARANCE,
      theme,
      backgroundUrl,
      backgroundDim: 0,
    });
    await mockGet(page, "requests/mine", { data: [] });
    await mockGet(page, "requests?*", { data: [] });
    await mockGet(page, "requests/quota", {
      window_days: 7,
      movie: { limit: 0, used: 0, remaining: -1, unlimited: true },
      series: { limit: 0, used: 0, remaining: -1, unlimited: true },
      music: { limit: 0, used: 0, remaining: -1, unlimited: true },
    });
    await mockGet(page, "requests/quota/config", {
      movie_limit: 0,
      series_limit: 0,
      music_limit: 0,
      window_days: 7,
    });
    await mockGet(page, "requests/reconciliation", { data: [] });
    await mockGet(page, "music/audio-quality-profiles", { data: [] });
    for (const path of [
      "/discover",
      "/collections",
      "/collections?id=loom-collection",
      "/requests",
    ]) {
      await page.goto(path);
      await page.waitForLoadState("networkidle");
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
      const surfaces =
        path === "/discover"
          ? page
              .getByRole("heading", { name: "What to add next" })
              .locator("../..")
          : path === "/collections"
            ? page
                .getByRole("heading", { name: "Collections", exact: true })
                .locator("../../..")
            : path.startsWith("/collections?")
              ? page.locator("main header")
              : page.locator("main section");
      const colors = await surfaces.evaluateAll((elements) =>
        elements.map((element) => getComputedStyle(element).backgroundColor),
      );
      expect(colors.length).toBeGreaterThan(0);
      for (const color of colors)
        expect(color).toMatch(/^rgb\(\d+, \d+, \d+\)$/);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
      ).toBe(true);
      await page.screenshot({
        path: testInfo.outputPath(
          `${theme}-${path.split("?")[0]!.slice(1)}${path.includes("?") ? "-detail" : ""}.png`,
        ),
        fullPage: true,
      });
    }
  });
}
test("collection management stays in one place and never overflows", async ({
  page,
}, testInfo) => {
  const posts: { url: string; method: string; body: unknown }[] = [];
  await page.route(
    "**/api/v1/collections/loom-collection/**",
    async (route) => {
      const req = route.request();
      if (req.method() === "GET") return route.fallback();
      const body = req.postData() ? JSON.parse(req.postData()!) : {};
      posts.push({
        url: new URL(req.url()).pathname,
        method: req.method(),
        body,
      });
      if (req.url().endsWith("/bulk/add"))
        return route.fulfill({
          json: {
            kind: "add",
            total: 3,
            counts: { eligible: 1, in_library: 1, unreleased: 1 },
            items: [
              {
                member_id: "member-2",
                title: "Connected show",
                media_type: "series",
                status: "eligible",
                library_id: "lib-1",
                profile_id: "qp-1",
              },
              {
                member_id: "member-1",
                title: "Original adventure",
                media_type: "movie",
                status: "skipped",
                reason: "in_library",
              },
              {
                member_id: "member-3",
                title: "Future adventure",
                media_type: "movie",
                status: "skipped",
                reason: "unreleased",
              },
            ],
            libraries: {
              movie: [
                { id: "lib-1", name: "Movies", quality_profile_id: "qp-1" },
              ],
              series: [
                { id: "lib-tv", name: "TV", quality_profile_id: "qp-1" },
              ],
            },
            profiles: [{ id: "qp-1", name: "HD-1080p" }],
            settings: {
              movie: {
                library_id: "lib-1",
                quality_profile_id: "qp-1",
                monitored: true,
              },
              series: {
                library_id: "lib-tv",
                quality_profile_id: "qp-1",
                monitored: true,
              },
            },
          },
        });
      return route.fulfill({ json: collection });
    },
  );
  await mockGet(page, "movies?limit=1000&offset=0", {
    data: [{ id: "actual-movie", title: "Original adventure", tmdbId: 101 }],
  });
  await mockGet(page, "movies/lookup?*", [
    {
      tmdbId: 990001,
      title:
        "An extraordinarily long adventure title that must wrap inside the picker",
      year: 2023,
      posterPath: "/adventure.jpg",
    },
  ]);
  const noOverflow = async () => {
    // Let sheet/dialog enter animations finish before measuring.
    await page.waitForTimeout(500);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    for (const dialog of await page.getByRole("dialog").all()) {
      await expect(dialog).toBeInViewport({ ratio: 1 });
      expect(
        await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth),
      ).toBe(true);
    }
  };

  await page.goto("/collections?id=loom-collection");
  await expect(
    page.getByRole("heading", { name: "Adventure collection" }),
  ).toBeVisible();

  await page.getByRole("button", { name: "Add titles", exact: true }).click();
  const picker = page.getByRole("dialog", {
    name: "Add titles to Adventure collection",
  });
  await picker.getByLabel("Search titles").fill("adventure");
  await expect(
    picker.getByRole("checkbox", { name: /Original adventure/ }),
  ).toBeDisabled();
  await picker
    .getByRole("checkbox", { name: /extraordinarily long adventure/ })
    .click();
  await expect(
    picker.getByRole("button", { name: "Add 1 title" }),
  ).toBeEnabled();
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-picker.png") });
  await picker.getByRole("button", { name: "Add 1 title" }).click();
  await expect(picker).not.toBeVisible();
  expect(posts.at(-1)).toMatchObject({
    url: "/api/v1/collections/loom-collection/members",
    body: {
      items: [{ media_type: "movie", external_ids: { tmdb: "990001" } }],
    },
  });

  await page.getByRole("button", { name: "Select", exact: true }).click();
  await page.getByRole("button", { name: "Select Connected show" }).click();
  await page.getByRole("button", { name: "Select Future adventure" }).click();
  const bar = page.getByRole("toolbar", { name: "Selected titles" });
  await expect(bar).toContainText("2 selected");
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-select.png") });
  await bar.getByRole("button", { name: "Add missing" }).click();
  const bulk = page.getByRole("dialog", { name: "Add missing titles" });
  await expect(bulk.getByText("Connected show")).toBeVisible();
  await expect(bulk.getByLabel("Library").first()).toHaveValue("lib-1");
  expect(
    posts.every((p) => (p.body as { execute?: boolean }).execute !== true),
  ).toBe(true);
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-bulk.png") });
  await page.keyboard.press("Escape");
  await expect(bulk).not.toBeVisible();

  await page.getByRole("button", { name: "Manage", exact: true }).click();
  const manage = page.getByRole("dialog", { name: "Manage collection" });
  await expect(manage.getByLabel("Quality for new additions")).toBeVisible();
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-manage.png") });
  await manage.getByRole("button", { name: "Delete collection" }).click();
  const confirm = page.getByRole("dialog", {
    name: "Delete Adventure collection?",
  });
  await expect(confirm).toContainText("stay exactly as they are");
  await noOverflow();
  await page.screenshot({ path: testInfo.outputPath("collection-delete.png") });
  expect(posts.some((p) => p.method === "DELETE")).toBe(false);
});

test("provider failure has a visible retry path", async ({ page }) => {
  await page.route("**/api/v1/discover/recommendations?*", async (route) => {
    await route.fulfill({ status: 503, body: "TMDB access is not configured" });
  });
  await page.goto("/discover");
  await expect(page.getByRole("alert")).toContainText(
    "TMDB access is not configured",
  );
  await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
  await expect(page.getByText("No new suggestions yet")).not.toBeVisible();
});

for (const width of [375, 768, 1440]) {
  test(`title drawer stays within ${width}px with long titles and credits`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const longTitle = "An extraordinarily long adventure title ".repeat(5);
    await mockGet(page, "discover/movie/107", {
      title: {
        tmdb_id: "107",
        title: longTitle,
        overview: "A synopsis with a long identifier: " + "x".repeat(300),
        poster_path: "/adventure.jpg",
        backdrop_path: "/adventure.jpg",
      },
      credits: {
        cast: Array.from({ length: 30 }, (_, index) => ({
          id: index,
          name: `A performer with a very long name ${index}`,
          character: "An unusually long character description",
        })),
        crew: [],
      },
    });
    await page.goto("/discover");
    const opener = page.getByRole("button", {
      name: "View A new adventure",
      exact: true,
    });
    await opener.click();
    const drawer = page.getByRole("dialog");
    await expect(drawer.getByLabel("Library", { exact: true })).toBeVisible();
    await expect(
      drawer.getByRole("button", { name: "Close", exact: true }),
    ).toBeInViewport();
    await expect(
      drawer.getByRole("button", { name: "Add to library", exact: true }),
    ).toBeEnabled();
    await expect(
      drawer.getByText("A performer with a very long name 29", { exact: true }),
    ).toHaveCount(1);
    await expect
      .poll(async () => {
        const bounds = await drawer.boundingBox();
        return bounds
          ? Math.abs(bounds.x + bounds.width - width)
          : Number.POSITIVE_INFINITY;
      })
      .toBeLessThanOrEqual(1);
    const bounds = await drawer.boundingBox();
    expect(bounds).not.toBeNull();
    expect(Math.abs(bounds!.x + bounds!.width - width)).toBeLessThanOrEqual(1);
    expect(bounds!.width).toBeLessThanOrEqual(Math.min(width, 768));
    expect(
      await drawer.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`drawer-${width}.png`),
      fullPage: true,
    });
    await page.keyboard.press("Escape");
    await expect(opener).toBeFocused();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await page.screenshot({
      path: testInfo.outputPath(`posters-${width}.png`),
      fullPage: true,
    });
    await page.emulateMedia({ colorScheme: "dark", reducedMotion: "reduce" });
    await opener.click();
    await expect(drawer.getByLabel("Library", { exact: true })).toBeVisible();
    await expect(
      drawer.getByRole("button", { name: "Close", exact: true }),
    ).toBeInViewport();
    await expect
      .poll(async () => {
        const bounds = await drawer.boundingBox();
        return bounds
          ? Math.abs(bounds.x + bounds.width - width)
          : Number.POSITIVE_INFINITY;
      })
      .toBeLessThanOrEqual(1);
    expect(
      await drawer.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`drawer-dark-${width}.png`),
      fullPage: true,
    });
  });
}
