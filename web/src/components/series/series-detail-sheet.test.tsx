import { useState } from "react";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SeriesDetailSheet } from "./series-detail-sheet";
import type { Episode, Season, Series } from "./types";

const api = vi.hoisted(() => ({ fetch: vi.fn() }));
const notifications = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}));
vi.mock("@/lib/fetch", () => ({ apiFetch: api.fetch }));
vi.mock("sonner", () => ({ toast: notifications }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { role: "admin" } }),
}));
vi.mock("@/components/media-files-manager", () => ({
  MediaFilesManager: () => null,
}));
vi.mock("@/components/collections/add-to-collection", () => ({
  AddToCollectionButton: () => null,
}));
vi.mock("@/components/alt-titles", () => ({ AltTitlesSection: () => null }));
vi.mock("@/components/person-discover-dialog", () => ({
  PersonDiscoverDialog: () => null,
}));
vi.mock("@/components/search/release-search-dialog", () => ({
  ReleaseSearchDialog: () => null,
}));
vi.mock("@/hooks/use-airing-now", () => ({
  useAiringNow: () => new Date("2026-10-01T12:00:00Z"),
}));

const series: Series = {
  id: "show",
  title: "Test show",
  year: 2026,
  overview: "",
  genres: [],
  runtime: 45,
  rating: 0,
  network: "",
  status: "continuing",
  seriesType: "standard",
  qualityProfileId: "quality",
  libraryId: "library",
  monitoringStatus: "all",
  seasonFolder: true,
};
let seasons: Season[];
const onRefresh = vi.fn();

function summary(seasonNumber: number, monitored = true): Season {
  return {
    id: `season-${seasonNumber}`,
    seriesId: series.id,
    seasonNumber,
    title: "",
    overview: "",
    monitored,
    episodeCount: 1,
    episodeStats: {
      totalEpisodes: 1,
      downloadedEpisodes: 0,
      monitoredEpisodes: monitored ? 1 : 0,
      missingEpisodes: monitored ? 1 : 0,
      airedEpisodes: monitored ? 1 : 0,
    },
  };
}

function episode(season: Season): Episode {
  return {
    id: `episode-${season.seasonNumber}`,
    seriesId: series.id,
    seasonId: season.id,
    episodeNumber: 1,
    title: `Episode from season ${season.seasonNumber}`,
    overview: "",
    airDate: "2026-01-01",
    runtime: 45,
    monitored: season.monitored,
    hasFile: false,
  };
}

function Harness({ status = "all" }: { status?: string }) {
  const [current, setCurrent] = useState({
    ...series,
    monitoringStatus: status,
  });
  return (
    <SeriesDetailSheet
      series={current}
      open
      onOpenChange={vi.fn()}
      onUpdated={setCurrent}
      onDeleted={vi.fn()}
      onRefresh={onRefresh}
      profiles={[{ id: "quality", name: "HD" }]}
      libraries={[]}
    />
  );
}

function writes() {
  return api.fetch.mock.calls.filter(([, init]) => init?.method === "PUT");
}

beforeEach(() => {
  vi.clearAllMocks();
  seasons = [summary(0), summary(1)];
  api.fetch
    .mockReset()
    .mockImplementation(async (url: string, init?: RequestInit) => {
      if (url.endsWith("/credits"))
        return Response.json({ cast: [], crew: [] });
      if (url.endsWith("/seasons")) return Response.json(seasons);
      if (url.endsWith("/monitoring")) {
        const seasonNumber = url.match(/\/seasons\/(\d+)\//)?.[1];
        if (seasonNumber === undefined) return Response.json({});
        const { monitored } = JSON.parse(String(init?.body));
        const updated = summary(Number(seasonNumber), monitored);
        seasons = seasons.map((item) =>
          item.id === updated.id ? updated : item,
        );
        const { episodeStats: _stats, ...plainSeason } = updated;
        return Response.json(plainSeason);
      }
      if (url.endsWith("/episodes")) {
        const seasonNumber = Number(url.match(/\/seasons\/(\d+)\//)?.[1]);
        return Response.json([
          episode(seasons.find((item) => item.seasonNumber === seasonNumber)!),
        ]);
      }
      throw new Error(`Unexpected request: ${url}`);
    });
});

describe("show monitoring master switch", () => {
  it.each([
    "monitored",
    "all",
    "future",
    "missing",
    "existing",
    "pilot",
    "firstSeason",
    "lastSeason",
  ])("reflects enabled strategy %s", async (status) => {
    render(<Harness status={status} />);
    expect(
      screen.getByRole("button", { name: "Monitor show" }),
    ).toHaveAttribute("aria-pressed", "true");
    await screen.findByRole("button", { name: "Monitor Specials" });
  });

  it.each(["none", "unmonitored", "archived"])(
    "reflects disabled status %s",
    async (status) => {
      render(<Harness status={status} />);
      expect(
        screen.getByRole("button", { name: "Monitor show" }),
      ).toHaveAttribute("aria-pressed", "false");
      await screen.findByRole("button", { name: "Monitor Specials" });
    },
  );

  it("only updates the master switch and preserves season selections and expansion", async () => {
    seasons[0] = summary(0, false);
    render(<Harness />);
    const user = userEvent.setup();
    const header = await screen.findByRole("button", { name: /^Season 1/ });
    await user.click(header);
    await screen.findByText("Missing");
    const toggle = screen.getByRole("button", { name: "Monitor show" });
    for (const pressed of ["false", "true"]) {
      await user.click(toggle);
      await waitFor(() =>
        expect(toggle).toHaveAttribute("aria-pressed", pressed),
      );
      expect(header).toHaveAttribute("aria-expanded", "true");
      expect(screen.getByText("Missing")).toBeVisible();
      expect(
        screen.getByRole("button", { name: "Monitor Specials" }),
      ).toHaveAttribute("aria-pressed", "false");
      expect(
        screen.getByRole("button", { name: "Monitor Season 1" }),
      ).toHaveAttribute("aria-pressed", "true");
    }
    expect(writes().map(([url, init]) => [url, JSON.parse(init.body)])).toEqual(
      [
        ["/api/v1/series/show/monitoring", { status: "unmonitored" }],
        ["/api/v1/series/show/monitoring", { status: "monitored" }],
      ],
    );
  });

  it("reports non-OK show updates and restores the enabled control", async () => {
    const base = api.fetch.getMockImplementation()!;
    api.fetch.mockImplementation((url, init) =>
      init?.method === "PUT"
        ? Promise.resolve(new Response("Unavailable", { status: 503 }))
        : base(url, init),
    );
    render(<Harness />);
    const toggle = screen.getByRole("button", { name: "Monitor show" });
    await userEvent.click(toggle);
    await waitFor(() =>
      expect(notifications.error).toHaveBeenCalledWith(
        "Failed to update monitoring",
        { description: "Monitoring update failed (503)" },
      ),
    );
    expect(toggle).toBeEnabled();
    expect(toggle).toHaveAttribute("aria-pressed", "true");
  });
});

describe.each([
  ["refresh", "Refresh metadata from TMDB"],
  ["rescan", "Rescan library folder for new files"],
] as const)("same-ID metadata %s", (action, actionLabel) => {
  it("refreshes season counts, summaries, episodes and credits while preserving expansion", async () => {
    const base = api.fetch.getMockImplementation()!;
    let updated = false;
    api.fetch.mockImplementation(async (url: string, init?: RequestInit) => {
      if (url.endsWith(`/${action}`) && init?.method === "POST") {
        updated = true;
        const season = summary(1);
        seasons = [
          seasons[0]!,
          {
            ...season,
            episodeCount: 2,
            episodeStats: {
              totalEpisodes: 2,
              airedEpisodes: 2,
              monitoredEpisodes: 2,
              downloadedEpisodes: 1,
              missingEpisodes: 1,
            },
          },
          summary(2),
        ];
        return Response.json({ matched: 1, imported: 1 });
      }
      if (url === "/api/v1/series/show") {
        return Response.json({ ...series, overview: "Refreshed synopsis" });
      }
      if (updated && url.endsWith("/seasons/1/episodes")) {
        const first = episode(seasons[1]!);
        return Response.json([
          { ...first, hasFile: true },
          {
            ...first,
            id: "episode-new",
            episodeNumber: 2,
            title: "New episode",
          },
        ]);
      }
      return base(url, init);
    });
    render(<Harness />);
    const user = userEvent.setup();
    const header = await screen.findByRole("button", { name: /^Season 1/ });
    await user.click(header);
    await screen.findByText("Missing");
    await user.click(screen.getByRole("button", { name: actionLabel }));

    await screen.findByRole("button", { name: /^Season 2/ });
    expect(screen.getByRole("button", { name: "Seasons (3)" })).toBeVisible();
    expect(await screen.findByText("New episode")).toBeVisible();
    expect(screen.getByText("Downloaded")).toBeVisible();
    expect(screen.getByText("Refreshed synopsis")).toBeVisible();
    expect(screen.getByRole("button", { name: /^Season 1/ })).toBe(header);
    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(header).toHaveTextContent("1 / 2");
    expect(
      screen.getByRole("button", { name: "Monitor Season 1" }),
    ).toHaveAttribute("aria-pressed", "true");
    for (const suffix of ["/seasons", "/seasons/1/episodes", "/credits"]) {
      expect(
        api.fetch.mock.calls.filter(([url]) => url.endsWith(suffix)),
      ).toHaveLength(2);
    }
    expect(notifications.error).not.toHaveBeenCalled();
  });
});

describe.each([
  [0, "Specials"],
  [1, "Season 1"],
] as const)("%s monitoring (%s)", (seasonNumber, label) => {
  it("persists both directions independently of expansion and refreshes collapsed stats", async () => {
    render(<Harness status="none" />);
    const user = userEvent.setup();
    const toggle = await screen.findByRole("button", {
      name: `Monitor ${label}`,
    });
    const header = screen.getByRole("button", {
      name: new RegExp(`^${label}`),
    });
    expect(document.querySelector("button button")).toBeNull();
    expect(screen.getByText(/Specials are monitored separately/)).toBeVisible();
    for (const monitored of [false, true]) {
      await user.click(toggle);
      await waitFor(() =>
        expect(toggle).toHaveAttribute("aria-pressed", String(monitored)),
      );
      await waitFor(() =>
        expect(header).toHaveTextContent(monitored ? "0 / 1" : "0 / 0"),
      );
      expect(header).toHaveAttribute("aria-expanded", "false");
      expect(
        screen.getByRole("button", { name: "Monitor show" }),
      ).toHaveAttribute("aria-pressed", "false");
      const otherLabel = seasonNumber === 0 ? "Season 1" : "Specials";
      expect(
        screen.getByRole("button", { name: `Monitor ${otherLabel}` }),
      ).toHaveAttribute("aria-pressed", "true");
    }
    expect(writes().map(([url, init]) => [url, JSON.parse(init.body)])).toEqual(
      [
        [
          `/api/v1/series/show/seasons/${seasonNumber}/monitoring`,
          { monitored: false },
        ],
        [
          `/api/v1/series/show/seasons/${seasonNumber}/monitoring`,
          { monitored: true },
        ],
      ],
    );
    expect(onRefresh).toHaveBeenCalledTimes(2);
  });

  it("reloads visible episode monitoring and statistics without closing or collapsing", async () => {
    render(<Harness />);
    const user = userEvent.setup();
    const header = await screen.findByRole("button", {
      name: new RegExp(`^${label}`),
    });
    await user.click(header);
    await screen.findByText("Missing");
    await user.click(screen.getByRole("button", { name: `Monitor ${label}` }));
    expect(await screen.findByText("Unmonitored")).toBeVisible();
    expect(header).toHaveAttribute("aria-expanded", "true");
    expect(header).toHaveTextContent("0 / 0");
    expect(
      api.fetch.mock.calls.filter(([url]) =>
        url.endsWith(`/seasons/${seasonNumber}/episodes`),
      ),
    ).toHaveLength(2);
    expect(
      api.fetch.mock.calls.filter(([url]) => url.endsWith("/seasons")),
    ).toHaveLength(2);
  });

  it("disables the pending toggle without disabling the independent accordion", async () => {
    let resolve!: (response: Response) => void;
    const pending = new Promise<Response>((done) => {
      resolve = done;
    });
    const base = api.fetch.getMockImplementation()!;
    api.fetch.mockImplementation((url, init) =>
      init?.method === "PUT" ? pending : base(url, init),
    );
    render(<Harness />);
    const user = userEvent.setup();
    const toggle = await screen.findByRole("button", {
      name: `Monitor ${label}`,
    });
    await user.click(toggle);
    expect(toggle).toBeDisabled();
    expect(toggle).toHaveAttribute("aria-busy", "true");
    await user.click(toggle);
    expect(writes()).toHaveLength(1);
    const header = screen.getByRole("button", {
      name: new RegExp(`^${label}`),
    });
    await user.click(header);
    expect(header).toHaveAttribute("aria-expanded", "true");
    await act(async () => {
      seasons = seasons.map((item) =>
        item.seasonNumber === seasonNumber
          ? summary(seasonNumber, false)
          : item,
      );
      const { episodeStats: _stats, ...plainSeason } = summary(
        seasonNumber,
        false,
      );
      resolve(Response.json(plainSeason));
    });
    await waitFor(() => expect(toggle).toBeEnabled());
    expect(toggle).toHaveAttribute("aria-busy", "false");
    expect(toggle).toHaveAttribute("aria-pressed", "false");
  });

  it.each(["http", "non-json", "network"])(
    "toasts %s errors without changing the saved selection",
    async (failure) => {
      const base = api.fetch.getMockImplementation()!;
      api.fetch.mockImplementation(async (url, init) => {
        if (init?.method !== "PUT") return base(url, init);
        if (failure === "network") throw new Error("Offline");
        return failure === "http"
          ? Response.json(
              { error: { message: "Monitoring unavailable" } },
              { status: 500 },
            )
          : new Response("Unavailable", { status: 503 });
      });
      render(<Harness />);
      const toggle = await screen.findByRole("button", {
        name: `Monitor ${label}`,
      });
      await userEvent.click(toggle);
      await waitFor(() =>
        expect(notifications.error).toHaveBeenCalledWith(
          `Could not update ${label} monitoring`,
          {
            description:
              failure === "network"
                ? "Offline"
                : failure === "http"
                  ? "Monitoring unavailable"
                  : "Monitoring update failed (503)",
          },
        ),
      );
      expect(toggle).toBeEnabled();
      expect(toggle).toHaveAttribute("aria-pressed", "true");
      expect(onRefresh).not.toHaveBeenCalled();
    },
  );

  it("supports keyboard activation without nested interactive controls", async () => {
    render(<Harness />);
    const user = userEvent.setup();
    const toggle = await screen.findByRole("button", {
      name: `Monitor ${label}`,
    });
    toggle.focus();
    await user.keyboard(" ");
    await waitFor(() =>
      expect(toggle).toHaveAttribute("aria-pressed", "false"),
    );
    const header = screen.getByRole("button", {
      name: new RegExp(`^${label}`),
    });
    expect(within(header).queryByRole("button")).not.toBeInTheDocument();
    header.focus();
    await user.keyboard("{Enter}");
    expect(header).toHaveAttribute("aria-expanded", "true");
    await screen.findByText("Unmonitored");
  });
});
