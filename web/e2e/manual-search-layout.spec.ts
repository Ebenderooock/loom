import { test, expect } from "@playwright/test";
import {
  mockBaseApp,
  mockGet,
  mockMovies,
  mockPost,
  SAMPLE_DOWNLOAD_CLIENT,
} from "./helpers/mock-api";

test.use({ reducedMotion: "reduce" });

const longTitle = `Guardians.of.the.Galaxy.Vol.2.2017.2160p.${"UnbrokenReleaseTitle".repeat(16)}.BluRay`;
const releases = Array.from({ length: 120 }, (_, i) => ({
  indexer_id: "TestIndexerWithAnUnbrokenName",
  title:
    i === 0
      ? longTitle
      : `Guardians.of.the.Galaxy.Vol.2.2017.1080p.Release-${i}`,
  link: `https://releases.test/${i}`,
  info_url: `https://releases.test/details/${i}`,
  quality: i === 0 ? "2160p" : "1080p",
  size_bytes: 4500000000,
  seeders: 123,
  leechers: 7,
  score: 120,
  publish_date: "2026-10-29T00:00:00Z",
  freeleech: i === 0,
}));

test.beforeEach(async ({ page }) => {
  await mockBaseApp(page);
  await mockGet(page, "features", { features: [] });
  const movie = {
    id: "mov-1",
    title: "Guardians of the Galaxy Vol. 2",
    year: 2017,
    tmdb_id: 283995,
    imdb_id: "tt3896198",
    status: "missing",
    quality_profile_id: "qp-1",
    monitored: true,
    overview: "A deterministic manual-search layout fixture.",
    poster_url: "",
    fanart_url: "",
    added_at: "2025-01-01T00:00:00Z",
  };
  await mockMovies(page, [movie]);
  await mockGet(page, "movies/mov-1/credits", { cast: [], crew: [] });
  await mockGet(page, "movies/files/mov-1", { data: [], total: 0 });
  await mockGet(page, "movies/mov-1/history", { data: [], total: 0 });
  await mockGet(page, "download-clients/", {
    download_clients: [SAMPLE_DOWNLOAD_CLIENT],
  });
  await mockPost(page, "blocklist/check", {
    blocked: releases.map(() => false),
  });
  await mockPost(page, "autosearch/evaluate", {
    preview: false,
    results: [],
    total: releases.length,
    passed: releases.length,
  });
  await page.route("**/api/v1/indexers/search/stream*", async (route) => {
    const events = [
      ["search-start", { indexers: [{ id: "ix", name: "TestIndexer" }] }],
      ["indexer-start", { indexer_id: "ix", indexer_name: "TestIndexer" }],
      [
        "indexer-result",
        {
          indexer_id: "ix",
          indexer_name: "TestIndexer",
          results: releases,
          result_count: releases.length,
          elapsed_ms: 150,
        },
      ],
      [
        "indexer-error",
        {
          indexer_id: "failed",
          indexer_name: "UnavailableIndexer",
          error: "Connection refused",
          status: "error",
          elapsed_ms: 100,
        },
      ],
      ["done", { total_results: releases.length, total_errors: 1 }],
    ];
    await route.fulfill({
      contentType: "text/event-stream",
      body: events
        .map(
          ([event, data]) =>
            `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`,
        )
        .join(""),
    });
  });
});

for (const viewport of [
  { width: 1280, height: 720 },
  { width: 1440, height: 900 },
  { width: 375, height: 812 },
  { width: 768, height: 900 },
]) {
  test(`release actions stay in bounds at ${viewport.width}x${viewport.height}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto("/movies");
    await page
      .locator("main")
      .getByText("Guardians of the Galaxy Vol. 2")
      .first()
      .click();
    await page
      .getByRole("button", { name: "Manual Search", exact: true })
      .first()
      .click();
    const dialog = page.getByRole("dialog").last();
    const firstDownload = dialog.getByRole("button", {
      name: `Download ${longTitle}`,
      exact: true,
    });
    await expect(firstDownload).toBeVisible();
    await expect(firstDownload).toBeInViewport();
    await expect(dialog.getByText("1 indexer reported errors")).toBeVisible();
    await expect(
      dialog.getByText("120 results", { exact: true }).last(),
    ).toBeVisible();

    const geometry = await dialog.evaluate((element) => {
      const results = element.querySelector<HTMLElement>(
        '[aria-label="Release results"]',
      )!;
      const bounds = element.getBoundingClientRect();
      const actions = Array.from(results.querySelectorAll("button"));
      return {
        dialogOverflow: element.scrollWidth > element.clientWidth,
        pageOverflow: document.documentElement.scrollWidth > window.innerWidth,
        resultsOverflow: results.scrollWidth > results.clientWidth,
        dialogScrolls: element.scrollHeight > element.clientHeight,
        resultsScroll: results.scrollHeight > results.clientHeight,
        actionsInBounds: actions.every((button) => {
          const rect = button.getBoundingClientRect();
          const row = button.closest("tr")!.getBoundingClientRect();
          return (
            rect.left >= bounds.left &&
            rect.right <= bounds.right &&
            rect.left >= row.left &&
            rect.right <= row.right &&
            rect.top >= row.top &&
            rect.bottom <= row.bottom
          );
        }),
      };
    });
    expect(geometry).toEqual({
      dialogOverflow: false,
      pageOverflow: false,
      resultsOverflow: false,
      dialogScrolls: false,
      resultsScroll: true,
      actionsInBounds: true,
    });
    const artifact = `manual-search-${viewport.width}x${viewport.height}.png`;
    await page.screenshot({
      path: process.env.LOOM_SCREENSHOT_DIR
        ? `${process.env.LOOM_SCREENSHOT_DIR}/${artifact}`
        : testInfo.outputPath(artifact),
    });

    await dialog
      .getByRole("button", { name: "1 indexer reported errors" })
      .click();
    await expect(dialog.getByText("Connection refused")).toBeVisible();
    await dialog
      .getByRole("button", { name: "1 indexer reported errors" })
      .click();
    await dialog
      .getByText("Explain matching / preview policy", { exact: true })
      .click();
    await dialog.getByRole("button", { name: "Filters", exact: true }).click();
    await dialog.getByLabel("Ignore blocklist").scrollIntoViewIfNeeded();
    await expect(dialog.getByLabel("Ignore blocklist")).toBeInViewport();
    await expect(firstDownload).toBeInViewport();
    expect(
      await dialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    await dialog.getByRole("button", { name: "Filters", exact: true }).click();
    await dialog
      .getByText("Explain matching / preview policy", { exact: true })
      .click();

    const searchBox = dialog.getByRole("textbox", { name: "Search query" });
    const before = await searchBox.boundingBox();
    await dialog
      .getByRole("region", { name: "Release results" })
      .evaluate((element) => {
        element.scrollTop = element.scrollHeight;
      });
    await expect(
      dialog.getByRole("button", {
        name: `Download ${releases[119]!.title}`,
        exact: true,
      }),
    ).toBeInViewport();
    expect(await searchBox.boundingBox()).toEqual(before);
    await dialog
      .getByRole("region", { name: "Release results" })
      .evaluate((element) => {
        element.scrollTop = 0;
      });

    let finish!: () => void;
    const pending = new Promise<void>((resolve) => (finish = resolve));
    const requests: unknown[] = [];
    await page.route("**/api/v1/download-clients/dc-1/items", async (route) => {
      requests.push(route.request().postDataJSON());
      await pending;
      await route.fulfill({
        json: { client_id: "dc-1", item_id: "mock-download" },
      });
    });
    await firstDownload.dblclick();
    const sending = dialog.getByRole("button", {
      name: `Sending… ${longTitle}`,
      exact: true,
    });
    await expect(sending).toBeDisabled();
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({
      torrent_url: releases[0]!.link,
      title: longTitle,
      indexer_id: releases[0]!.indexer_id,
      media_type: "movie",
      movie_id: "mov-1",
    });
    expect(requests[0]).not.toHaveProperty("ignore_blocklist");
    finish();
    await expect(
      dialog.getByRole("button", {
        name: `Sent ${longTitle}`,
        exact: true,
      }),
    ).toBeDisabled();
    await searchBox.focus();
    await page.keyboard.press("Escape");
    await expect(
      dialog.getByRole("textbox", { name: "Search query" }),
    ).not.toBeVisible();
  });
}
