import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MovieDetailSheet } from "@/components/movies/movie-detail-sheet";
import { SeriesDetailSheet } from "@/components/series/series-detail-sheet";
import type { Movie } from "@/components/movies/types";
import type { Episode, Season, Series } from "@/components/series/types";

const notifications = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("sonner", () => ({ toast: { error: notifications.error } }));

const auth = vi.hoisted(() => ({
  user: { id: 1, role: "admin" },
  isAuthenticated: true,
}));
vi.mock("@/hooks/use-auth", () => ({ useAuth: () => auth }));
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

const movie: Movie = {
  id: "m1",
  title: "Test movie",
  year: 2026,
  overview: "",
  rating: 0,
  runtime: 90,
  genres: [],
  releaseDate: "",
  status: "available_right_quality",
  monitoringStatus: "monitored",
  qualityProfileId: "",
  libraryId: "",
};
const series: Series = {
  id: "s1",
  title: "Test show",
  year: 2026,
  overview: "",
  genres: [],
  runtime: 45,
  rating: 0,
  network: "",
  status: "continuing",
  seriesType: "standard",
  qualityProfileId: "",
  libraryId: "",
  monitoringStatus: "monitored",
  seasonFolder: true,
};
const seasons: Season[] = [0, 2].map((seasonNumber) => ({
  id: `season-${seasonNumber}`,
  seriesId: "s1",
  seasonNumber,
  title: "",
  overview: "",
  monitored: true,
  episodeCount: 0,
}));
const callbacks = {
  onOpenChange: vi.fn(),
  onUpdated: vi.fn(),
  onDeleted: vi.fn(),
  profiles: [],
  libraries: [],
  open: true,
};
const fetchMock = vi.fn<typeof fetch>();
let client: QueryClient;

function renderDetail(element: React.ReactNode) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  auth.user.role = "admin";
  fetchMock.mockReset().mockImplementation(async (input) => {
    const url = String(input);
    if (url.startsWith("/api/v1/media-files/")) {
      return Response.json({
        files: [{ path: "Extras/interview.mp4", size: 512, tracked: false }],
        deletionMode: "permanent",
      });
    }
    if (url.endsWith("/credits")) return Response.json({ cast: [], crew: [] });
    if (url.endsWith("/seasons")) return Response.json(seasons);
    return Response.json([]);
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  client.clear();
  vi.unstubAllGlobals();
});

async function confirmOneFile(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("checkbox", {
      name: "Select Extras/interview.mp4",
    }),
  );
  await user.click(screen.getByRole("button", { name: "Delete selected (1)" }));
  await user.click(
    screen.getByRole("button", { name: "Permanently delete files" }),
  );
}

describe("Media file manager detail entry points", () => {
  it.each(["http", "network", "malformed"] as const)(
    "retains valid movie files and shows refresh errors after deletion (%s)",
    async (failure) => {
      const base = fetchMock.getMockImplementation()!;
      let fileReads = 0;
      fetchMock.mockImplementation(async (input, init) => {
        if (String(input) === "/api/v1/movies/files/m1") {
          if (++fileReads === 1) {
            return Response.json([
              { id: "f1", filePath: "existing.mkv", size: 1024 },
            ]);
          }
          if (failure === "network") throw new Error("Offline");
          if (failure === "malformed")
            return Response.json({ unexpected: true });
          return Response.json(
            { error: { message: "Disk unavailable" } },
            { status: 500 },
          );
        }
        if (init?.method === "POST")
          return Response.json({ deleted: ["Extras/interview.mp4"] });
        return base(input, init);
      });
      const onRefresh = vi.fn();
      const detail = renderDetail(
        <MovieDetailSheet movie={movie} {...callbacks} onRefresh={onRefresh} />,
      );
      const user = userEvent.setup();
      await user.click(screen.getByRole("button", { name: "Movie Files" }));
      expect(
        (await screen.findAllByText("existing.mkv")).length,
      ).toBeGreaterThan(0);
      await user.click(screen.getByRole("button", { name: "Manage files" }));
      await confirmOneFile(user);
      await waitFor(() =>
        expect(notifications.error).toHaveBeenCalledWith(
          "Could not refresh movie files",
          {
            description:
              failure === "http"
                ? "Disk unavailable"
                : failure === "network"
                  ? "Offline"
                  : "Unexpected movie file response",
          },
        ),
      );
      expect(onRefresh).toHaveBeenCalledOnce();
      // Simulate the parent list returning a fresh object for this same title.
      detail.rerender(
        <QueryClientProvider client={client}>
          <MovieDetailSheet
            movie={{ ...movie }}
            {...callbacks}
            onRefresh={onRefresh}
          />
        </QueryClientProvider>,
      );
      const manager = screen.getByRole("dialog", {
        name: "Manage files — Test movie",
      });
      await user.click(
        within(manager).getAllByRole("button", { name: "Close" })[0]!,
      );
      expect(screen.getAllByText("existing.mkv").length).toBeGreaterThan(0);
      expect(fileReads).toBe(2);
    },
  );

  it.each([
    ["series", false],
    ["series", true],
    ["season", false],
    ["season", true],
  ] as const)(
    "refreshes expanded episode lists, collapsed-season summaries and parent stats after %s deletion (error=%s)",
    async (scope, fails) => {
      const base = fetchMock.getMockImplementation()!;
      let deleted = false;
      const episode: Episode = {
        id: "e1",
        seriesId: "s1",
        seasonId: "season-2",
        episodeNumber: 1,
        title: "Test episode",
        overview: "",
        airDate: "2026-01-01",
        runtime: 45,
        monitored: true,
        hasFile: true,
      };
      fetchMock.mockImplementation(async (input, init) => {
        const url = String(input);
        if (init?.method === "POST") {
          deleted = true;
          return fails
            ? Response.json(
                { error: { message: "Partial failure" } },
                { status: 500 },
              )
            : Response.json({ deleted: ["Extras/interview.mp4"] });
        }
        if (url.endsWith("/episodes"))
          return Response.json([{ ...episode, hasFile: !deleted }]);
        if (url.endsWith("/seasons")) {
          return Response.json(
            seasons.map((season) => ({
              ...season,
              episodeCount: 1,
              episodeStats: {
                totalEpisodes: 1,
                downloadedEpisodes: deleted ? 0 : 1,
                monitoredEpisodes: 1,
                missingEpisodes: deleted ? 1 : 0,
                airedEpisodes: 1,
              },
            })),
          );
        }
        return base(input, init);
      });
      const onRefresh = vi.fn();
      renderDetail(
        <SeriesDetailSheet
          series={series}
          {...callbacks}
          onRefresh={onRefresh}
        />,
      );
      const user = userEvent.setup();
      const header = await screen.findByRole("button", { name: /Season 2/ });
      await user.click(header);
      expect(await screen.findByText("Downloaded")).toBeVisible();
      const accordion = header.parentElement!;
      const managerButton =
        scope === "season"
          ? within(accordion).getByRole("button", { name: "Manage files" })
          : screen.getAllByRole("button", { name: "Manage files" })[0]!;
      await user.click(managerButton);
      await confirmOneFile(user);
      if (fails)
        expect(await screen.findByRole("alert")).toHaveTextContent(
          "Partial failure",
        );
      await waitFor(() => expect(onRefresh).toHaveBeenCalledOnce());
      const manager = screen.getByRole("dialog", {
        name:
          scope === "series"
            ? "Manage files — Test show"
            : "Manage files — Test show — Season 2",
      });
      await user.click(
        within(manager).getAllByRole("button", { name: "Close" })[0]!,
      );
      expect(await screen.findByText("Missing")).toBeVisible();
      expect(screen.queryByText("Downloaded")).not.toBeInTheDocument();
      expect(within(accordion).getByText("0 / 1")).toBeVisible();
      expect(
        screen.getByRole("button", { name: /Specials/ }),
      ).toHaveTextContent("0 / 1");
      expect(
        fetchMock.mock.calls.filter(([input]) =>
          String(input).endsWith("/seasons"),
        ),
      ).toHaveLength(2);
      expect(
        fetchMock.mock.calls.filter(([input]) =>
          String(input).endsWith("/episodes"),
        ),
      ).toHaveLength(2);
    },
  );

  it("reports failed season and episode refreshes without discarding existing rows", async () => {
    const base = fetchMock.getMockImplementation()!;
    let deleted = false;
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (init?.method === "POST") {
        deleted = true;
        return Response.json({ deleted: [] });
      }
      if (deleted && (url.endsWith("/seasons") || url.endsWith("/episodes"))) {
        return Response.json(
          { error: { message: "Storage offline" } },
          { status: 500 },
        );
      }
      if (url.endsWith("/episodes"))
        return Response.json([
          {
            id: "e1",
            episodeNumber: 1,
            title: "Existing episode",
            monitored: true,
            hasFile: true,
            airDate: "2026-01-01",
          },
        ]);
      return base(input, init);
    });
    renderDetail(<SeriesDetailSheet series={series} {...callbacks} />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Season 2" }));
    await screen.findByText("Existing episode");
    await user.click(
      screen.getAllByRole("button", { name: "Manage files" })[0]!,
    );
    await confirmOneFile(user);
    await waitFor(() => {
      expect(notifications.error).toHaveBeenCalledWith(
        "Could not refresh season summaries",
        { description: "Storage offline" },
      );
      expect(notifications.error).toHaveBeenCalledWith(
        "Could not refresh season episodes",
        { description: "Storage offline" },
      );
    });
    const manager = screen.getByRole("dialog", {
      name: "Manage files — Test show",
    });
    await user.click(
      within(manager).getAllByRole("button", { name: "Close" })[0]!,
    );
    expect(screen.getByText("Existing episode")).toBeVisible();
    expect(screen.getByRole("button", { name: /Specials/ })).toBeVisible();
  });

  it("opens movie file inventory from the movie detail sheet", async () => {
    renderDetail(<MovieDetailSheet movie={movie} {...callbacks} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Manage files" }));
    expect(await screen.findByText("Extras/interview.mp4")).toBeVisible();
    expect(
      screen.getByRole("dialog", { name: "Manage files — Test movie" }),
    ).toBeVisible();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/media-files/movie/m1",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("opens whole-series inventory without a season filter", async () => {
    renderDetail(<SeriesDetailSheet series={series} {...callbacks} />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Manage files" }));
    expect(await screen.findByText("Extras/interview.mp4")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/media-files/series/s1",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it.each([0, 2])(
    "opens season %s inventory from its expanded accordion, even without episodes",
    async (season) => {
      renderDetail(<SeriesDetailSheet series={series} {...callbacks} />);
      const user = userEvent.setup();
      const name = season === 0 ? "Specials" : `Season ${season}`;
      const header = await screen.findByRole("button", {
        name: new RegExp(name),
      });
      await user.click(header);
      const accordion = header.parentElement!;
      await user.click(
        within(accordion).getByRole("button", { name: "Manage files" }),
      );
      expect(await screen.findByText("Extras/interview.mp4")).toBeVisible();
      expect(
        screen.getByRole("dialog", {
          name: `Manage files — Test show — ${name}`,
        }),
      ).toBeVisible();
      expect(fetchMock).toHaveBeenCalledWith(
        `/api/v1/media-files/series/s1?season=${season}`,
        expect.objectContaining({ credentials: "include" }),
      );
    },
  );

  it("does not expose movie management to non-admins", () => {
    auth.user.role = "user";
    renderDetail(<MovieDetailSheet movie={movie} {...callbacks} />);
    expect(
      screen.queryByRole("button", { name: "Manage files" }),
    ).not.toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input).includes("/media-files/"),
      ),
    ).toBe(false);
  });

  it("does not expose whole-show or expanded-season management to non-admins", async () => {
    auth.user.role = "user";
    renderDetail(<SeriesDetailSheet series={series} {...callbacks} />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Season 2" }));
    expect(
      screen.queryByRole("button", { name: "Manage files" }),
    ).not.toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input).includes("/media-files/"),
      ),
    ).toBe(false);
  });
});
