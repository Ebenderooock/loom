/**
 * Playwright API mocking helpers for Loom E2E tests.
 *
 * Every test must call mockBaseApp(page) in beforeEach to set up
 * the authenticated shell. Page-specific helpers layer on top.
 *
 * A catch-all route rejects unmocked /api/v1/** calls so tests fail
 * fast instead of hanging on loading spinners.
 */
import { Page, Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

export const TEST_USER = {
  id: 1,
  username: "admin",
  email: "admin@loom.test",
  role: "admin",
};

export const SYSTEM_STATUS = {
  version: "0.1.0-test",
  commit: "abc1234",
  buildDate: "2025-01-01T00:00:00Z",
  engine: "sqlite",
};

export const SAMPLE_LIBRARY = {
  id: "lib-1",
  name: "Movies",
  path: "/mnt/movies",
  media_type: "movie",
  monitor_on_add: true,
  quality_profile_id: "qp-1",
  unmonitor_on_delete: false,
  auto_archive_watched: false,
  auto_archive_days_after_watch: 0,
  created_at: "2025-01-01T00:00:00Z",
  updated_at: "2025-01-01T00:00:00Z",
  accessible: true,
  disk_space: { total_bytes: 1e12, free_bytes: 5e11, used_bytes: 5e11 },
  file_count: 42,
  unmapped_count: 0,
};

export const SAMPLE_MOVIE = {
  id: "mov-1",
  title: "Test Movie",
  year: 2024,
  tmdb_id: 12345,
  imdb_id: "tt1234567",
  status: "available_right_quality",
  quality_profile_id: "qp-1",
  monitored: true,
  overview: "A test movie for E2E testing",
  poster_url: "",
  fanart_url: "",
  added_at: "2025-01-01T00:00:00Z",
};

export const SAMPLE_SERIES = {
  id: "ser-1",
  title: "Test Series",
  year: 2024,
  tvdb_id: 67890,
  status: "continuing",
  monitored: true,
  overview: "A test series for E2E testing",
  poster_url: "",
  fanart_url: "",
  added_at: "2025-01-01T00:00:00Z",
};

export const SAMPLE_INDEXER = {
  id: "idx-1",
  name: "TestIndexer",
  enabled: true,
  type: "torrent",
  priority: 25,
  definition_name: "1337x",
};

export const SAMPLE_DOWNLOAD_CLIENT = {
  id: "dc-1",
  name: "qBittorrent",
  enabled: true,
  type: "qbittorrent",
};

export const SAMPLE_QUALITY_PROFILE = {
  id: "qp-1",
  name: "HD-1080p",
  cutoff: "bluray-1080p",
  items: [],
};

// ---------------------------------------------------------------------------
// Core helpers
// ---------------------------------------------------------------------------

type RouteHandler = (route: Route) => Promise<void> | void;

// Register a mock for a specific API path pattern.
export async function mockRoute(
  page: Page,
  method: string,
  pathGlob: string,
  handler: RouteHandler,
) {
  const pattern = "**/api/v1/" + pathGlob;
  await page.route(pattern, async (route) => {
    if (route.request().method() === method.toUpperCase()) {
      await handler(route);
    } else {
      await route.fallback();
    }
  });
}

/** Shorthand: mock a GET that returns JSON. */
export async function mockGet(page: Page, pathGlob: string, json: unknown) {
  const pattern = "**/api/v1/" + pathGlob;
  await page.route(pattern, async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ status: 200, json });
    } else {
      await route.fallback();
    }
  });
}

/** Shorthand: mock a POST that returns JSON. */
export async function mockPost(
  page: Page,
  pathGlob: string,
  json: unknown,
  status = 200,
) {
  const pattern = "**/api/v1/" + pathGlob;
  await page.route(pattern, async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status, json });
    } else {
      await route.fallback();
    }
  });
}

// ---------------------------------------------------------------------------
// Catch-all: reject unmocked API calls loudly
// ---------------------------------------------------------------------------

// IMPORTANT: Must be registered BEFORE all specific mocks.
// Playwright uses last-registered-wins priority, so specific routes
// registered after this will take precedence over this catch-all.
export async function mockUnhandledApis(page: Page) {
  await page.route("**/api/v1/**", async (route) => {
    const req = route.request();
    const msg = "[UNMOCKED API] " + req.method() + " " + req.url();
    console.error(msg);
    await route.fulfill({
      status: 599,
      contentType: "application/json",
      body: JSON.stringify({
        error: "Unmocked API: " + req.method() + " " + req.url(),
      }),
    });
  });
}

// ---------------------------------------------------------------------------
// Auth mocking (stateful)
// ---------------------------------------------------------------------------

export interface AuthState {
  authenticated: boolean;
  setupComplete: boolean;
}

/**
 * Sets up auth mocks with mutable state so login flow tests work.
 * Returns the state object; mutate state.authenticated to simulate login.
 */
export async function mockAuth(
  page: Page,
  initial: Partial<AuthState> = {},
): Promise<AuthState> {
  const state: AuthState = {
    authenticated: initial.authenticated ?? true,
    setupComplete: initial.setupComplete ?? true,
  };

  await page.route("**/api/v1/auth/status", async (route) => {
    await route.fulfill({
      status: 200,
      json: {
        setup_required: !state.setupComplete,
        is_authenticated: state.authenticated,
        user: state.authenticated ? TEST_USER : null,
      },
    });
  });

  await page.route("**/api/v1/auth/login", async (route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    const body = route.request().postDataJSON();
    if (body?.username === "admin" && body?.password === "password") {
      state.authenticated = true;
      await route.fulfill({ status: 200, json: { ok: true } });
    } else {
      await route.fulfill({
        status: 401,
        json: { error: "Invalid credentials" },
      });
    }
  });

  await page.route("**/api/v1/auth/logout", async (route) => {
    if (route.request().method() === "POST") {
      state.authenticated = false;
      await route.fulfill({ status: 200, json: { ok: true } });
    } else {
      await route.fallback();
    }
  });

  return state;
}

// ---------------------------------------------------------------------------
// Base app mocks (needed on every page)
// ---------------------------------------------------------------------------

/**
 * Sets up all the API mocks needed for the app shell to render:
 * catch-all (lowest priority), auth, system status, review count, events.
 */
export async function mockBaseApp(
  page: Page,
  opts: Partial<AuthState> = {},
): Promise<AuthState> {
  // Register catch-all FIRST — Playwright uses last-registered-wins,
  // so specific routes registered AFTER will take precedence.
  await mockUnhandledApis(page);

  const state = await mockAuth(page, opts);

  // System status
  await mockGet(page, "system/status", SYSTEM_STATUS);

  // Review count (used by layout badge)
  await mockGet(page, "reviews/count", { count: 0 });

  // Events / SSE stream — just return empty
  await page.route("**/api/v1/events/stream", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: "event: connected\ndata: {}\n\n",
    });
  });

  // General events list
  await mockGet(page, "events*", { data: [], total: 0 });

  return state;
}

// ---------------------------------------------------------------------------
// Dashboard mocks
// ---------------------------------------------------------------------------

export async function mockDashboard(page: Page) {
  await mockGet(page, "movies?limit=1", { data: [SAMPLE_MOVIE], total: 1 });
  await mockGet(page, "series?limit=1", { data: [SAMPLE_SERIES], total: 1 });
  await mockGet(page, "indexers", { indexers: [SAMPLE_INDEXER] });
  await mockGet(page, "download-clients", {
    download_clients: [SAMPLE_DOWNLOAD_CLIENT],
  });
  await mockGet(page, "indexers/health", { data: [] });
  await mockGet(page, "libraries", [SAMPLE_LIBRARY]);
}

// ---------------------------------------------------------------------------
// Movies mocks
// ---------------------------------------------------------------------------

export async function mockMovies(
  page: Page,
  movies = [SAMPLE_MOVIE],
  libraries = [SAMPLE_LIBRARY],
) {
  await mockGet(page, "movies?*", { data: movies, total: movies.length });
  await mockGet(page, "movies", { data: movies, total: movies.length });
  await mockGet(page, "libraries", libraries);
  await mockGet(page, "quality-profiles", [SAMPLE_QUALITY_PROFILE]);

  // Single movie lookup
  await mockGet(page, "movies/*", movies[0] ?? {});

  // Movie files
  await mockGet(page, "movies/*/files", { data: [], total: 0 });

  // Movie search (TMDB lookup)
  await mockGet(page, "movies/lookup*", []);
}

// ---------------------------------------------------------------------------
// Series mocks
// ---------------------------------------------------------------------------

export async function mockSeriesApi(page: Page, seriesList = [SAMPLE_SERIES]) {
  await mockGet(page, "series?*", {
    data: seriesList,
    total: seriesList.length,
  });
  await mockGet(page, "series", {
    data: seriesList,
    total: seriesList.length,
  });
  await mockGet(page, "libraries", [
    { ...SAMPLE_LIBRARY, media_type: "series" },
  ]);
  await mockGet(page, "quality-profiles", [SAMPLE_QUALITY_PROFILE]);
  await mockGet(page, "series/*", seriesList[0] ?? {});
  await mockGet(page, "series/lookup*", []);
}

// ---------------------------------------------------------------------------
// Indexer mocks
// ---------------------------------------------------------------------------

export async function mockIndexers(page: Page, indexers = [SAMPLE_INDEXER]) {
  await mockGet(page, "indexers", { indexers });
  await mockGet(page, "indexers/health", { data: [] });
  await mockGet(page, "indexers/definitions", [
    { name: "1337x", type: "torrent", description: "1337x torrent site" },
    { name: "YTS", type: "torrent", description: "YTS movie torrents" },
    { name: "EZTV", type: "torrent", description: "EZTV TV torrents" },
  ]);
  await mockGet(page, "indexers/*", indexers[0] ?? {});
  await mockGet(page, "proxies", { proxies: [] });
}

// ---------------------------------------------------------------------------
// Downloads mocks
// ---------------------------------------------------------------------------

export async function mockDownloads(page: Page) {
  await mockGet(page, "download-clients", {
    download_clients: [SAMPLE_DOWNLOAD_CLIENT],
  });
  await mockGet(page, "downloads/queue", { data: [], total: 0 });
  await mockGet(page, "downloads/history", { data: [], total: 0 });
}

// ---------------------------------------------------------------------------
// Settings mocks
// ---------------------------------------------------------------------------

export async function mockSettings(page: Page) {
  await mockGet(page, "libraries", [SAMPLE_LIBRARY]);
  await mockGet(page, "download-clients", {
    download_clients: [SAMPLE_DOWNLOAD_CLIENT],
  });
  await mockGet(page, "indexers", { indexers: [SAMPLE_INDEXER] });
  await mockGet(page, "quality-profiles", [SAMPLE_QUALITY_PROFILE]);
  await mockGet(page, "api-keys", []);
  await mockGet(page, "notifications", []);
  await mockGet(page, "connect", []);
  await mockGet(page, "system/config", {});
  await mockGet(page, "import-lists", []);
}

// ---------------------------------------------------------------------------
// Workflows mocks
// ---------------------------------------------------------------------------

export const SAMPLE_WORKFLOW_ACTIVE = {
  id: "wf-1",
  type: "movie_search",
  state: "downloading",
  mediaType: "movie",
  grabTitle: "Test.Movie.2024.1080p.BluRay.x264",
  downloadClientId: "dc-1",
  downloadId: "dl-abc",
  qualityProfileId: "qp-1",
  retryCount: 0,
  maxRetries: 3,
  lastError: "",
  createdAt: "2025-01-01T10:00:00Z",
  updatedAt: "2025-01-01T10:05:00Z",
  items: [{ workflowId: "wf-1", mediaType: "movie", mediaId: "mov-1" }],
  history: [],
};

export const SAMPLE_WORKFLOW_COMPLETED = {
  id: "wf-2",
  type: "movie_search",
  state: "completed",
  mediaType: "movie",
  grabTitle: "Another.Movie.2023.720p.WEB-DL",
  retryCount: 0,
  maxRetries: 3,
  createdAt: "2025-01-01T08:00:00Z",
  updatedAt: "2025-01-01T09:00:00Z",
  completedAt: "2025-01-01T09:00:00Z",
  items: [],
  history: [],
};

export const SAMPLE_WORKFLOW_FAILED = {
  id: "wf-3",
  type: "episode_search",
  state: "failed",
  mediaType: "episode",
  grabTitle: "Test.Show.S01E05.HDTV",
  retryCount: 3,
  maxRetries: 3,
  lastError: "all retries exhausted: no seeders available",
  createdAt: "2025-01-01T06:00:00Z",
  updatedAt: "2025-01-01T07:30:00Z",
  items: [],
  history: [],
};

export async function mockWorkflows(
  page: Page,
  workflows = [
    SAMPLE_WORKFLOW_ACTIVE,
    SAMPLE_WORKFLOW_COMPLETED,
    SAMPLE_WORKFLOW_FAILED,
  ],
) {
  await mockGet(page, "workflows", workflows);
  for (const wf of workflows) {
    await mockGet(page, `workflows/${wf.id}`, wf);
  }
  // Cancel / retry / delete
  await page.route("**/api/v1/workflows/*/cancel", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status: 204 });
    } else {
      await route.fallback();
    }
  });
  await page.route("**/api/v1/workflows/*/retry", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status: 204 });
    } else {
      await route.fallback();
    }
  });
  await page.route("**/api/v1/workflows/*", async (route) => {
    if (route.request().method() === "DELETE") {
      await route.fulfill({ status: 204 });
    } else {
      await route.fallback();
    }
  });
}

// ---------------------------------------------------------------------------
// Search mocks (SSE streaming)
// ---------------------------------------------------------------------------

export async function mockSearchStream(
  page: Page,
  results: Array<{ title: string; size: number; indexer: string }> = [],
) {
  await page.route("**/api/v1/indexers/search/stream*", async (route) => {
    const lines: string[] = [];
    lines.push(
      "event: search-start\ndata: " + JSON.stringify({ query: "test" }) + "\n",
    );

    for (const r of results) {
      lines.push(
        "event: indexer-result\ndata: " +
          JSON.stringify({
            title: r.title,
            size: r.size,
            indexer: r.indexer,
            guid: "guid-" + r.title,
            download_url: "magnet:?xt=urn:btih:test",
            seeders: 100,
            leechers: 10,
            quality: "1080p",
          }) +
          "\n",
      );
    }

    lines.push(
      "event: done\ndata: " + JSON.stringify({ total: results.length }) + "\n",
    );

    await route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: lines.join("\n"),
    });
  });
}
