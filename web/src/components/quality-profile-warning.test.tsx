import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  defaultQualityProfileId,
  isMissingQualityProfile,
} from "@/components/quality-profile-warning";
import { MovieCard } from "@/components/movies/movie-card";
import { MovieListRow } from "@/components/movies/movie-list-row";
import { MovieDetailSheet } from "@/components/movies/movie-detail-sheet";
import { SeriesCard } from "@/components/series/series-card";
import { SeriesListRow } from "@/components/series/series-list-row";
import { SeriesDetailSheet } from "@/components/series/series-detail-sheet";
import { PersonDiscoverDialog } from "@/components/person-discover-dialog";
import { Table, TableBody } from "@/components/ui/table";
import type { Movie, QualityProfile } from "@/components/movies/types";
import type { Series } from "@/components/series/types";
import type { Library } from "@/lib/libraries-api";

vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { id: 1, role: "admin" }, isAuthenticated: true }),
}));
vi.mock("@/components/alt-titles", () => ({ AltTitlesSection: () => null }));
vi.mock("@/components/search/release-search-dialog", () => ({
  ReleaseSearchDialog: () => null,
}));
vi.mock("@/components/media-files-manager", () => ({
  MediaFilesManager: () => null,
}));
vi.mock("@/hooks/use-airing-now", () => ({
  useAiringNow: () => new Date("2026-10-01T12:00:00Z"),
}));

const profiles: QualityProfile[] = [
  { id: "qp-hd", name: "HD-1080p" },
  { id: "qp-any", name: "Any" },
];

const baseMovie: Movie = {
  id: "m1",
  title: "Unprofiled Movie",
  year: 2026,
  overview: "",
  rating: 0,
  runtime: 90,
  genres: [],
  releaseDate: "",
  status: "missing",
  monitoringStatus: "monitored",
  qualityProfileId: "",
  libraryId: "",
};
const baseSeries: Series = {
  id: "s1",
  title: "Unprofiled Show",
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

const fetchMock = vi.fn<typeof fetch>();
let client: QueryClient | undefined;

beforeEach(() => {
  fetchMock.mockReset().mockImplementation(async (input) => {
    const url = String(input);
    if (url.endsWith("/credits")) return Response.json({ cast: [], crew: [] });
    return Response.json([]);
  });
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});

afterEach(() => {
  client?.clear();
  vi.unstubAllGlobals();
});

function withQuery(element: React.ReactNode) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>,
  );
}

const cardProps = {
  profiles,
  selected: false,
  selectMode: false,
  onToggleSelect: vi.fn(),
  onClick: vi.fn(),
};

describe("isMissingQualityProfile", () => {
  it("flags missing, blank, or whitespace IDs without a server flag", () => {
    expect(isMissingQualityProfile({ qualityProfileId: undefined })).toBe(true);
    expect(isMissingQualityProfile({ qualityProfileId: null })).toBe(true);
    expect(isMissingQualityProfile({ qualityProfileId: "" })).toBe(true);
    expect(isMissingQualityProfile({ qualityProfileId: "   " })).toBe(true);
    expect(isMissingQualityProfile({ qualityProfileId: "qp-hd" })).toBe(false);
    // Unknown but non-empty IDs are not flagged without authoritative data.
    expect(isMissingQualityProfile({ qualityProfileId: "default" })).toBe(
      false,
    );
  });

  it("honours the authoritative server flag", () => {
    expect(
      isMissingQualityProfile({
        qualityProfileId: "default",
        qualityProfileMissing: true,
      }),
    ).toBe(true);
    expect(
      isMissingQualityProfile({
        qualityProfileId: "legacy-v1",
        qualityProfileMissing: false,
      }),
    ).toBe(false);
    // A false flag never hides a blank ID.
    expect(
      isMissingQualityProfile({
        qualityProfileId: "",
        qualityProfileMissing: false,
      }),
    ).toBe(true);
  });
});

describe("defaultQualityProfileId", () => {
  it("prefers an existing preferred profile", () => {
    expect(defaultQualityProfileId(profiles, "qp-hd")).toBe("qp-hd");
  });
  it("falls back to Any, not the first profile", () => {
    expect(defaultQualityProfileId(profiles, "")).toBe("qp-any");
    expect(defaultQualityProfileId(profiles, "deleted")).toBe("qp-any");
    expect(defaultQualityProfileId(profiles)).toBe("qp-any");
  });
  it("returns empty when no Any profile exists so the server defaults", () => {
    expect(defaultQualityProfileId([{ id: "qp-hd", name: "HD" }])).toBe("");
  });
});

describe("library cards and rows", () => {
  it("warns on a movie card without a quality profile", () => {
    render(<MovieCard movie={baseMovie} {...cardProps} />);
    expect(screen.getAllByText("No quality profile").length).toBeGreaterThan(0);
  });

  it("does not warn on a configured movie card", () => {
    render(
      <MovieCard
        movie={{ ...baseMovie, qualityProfileId: "qp-hd" }}
        {...cardProps}
      />,
    );
    expect(screen.queryByText("No quality profile")).toBeNull();
    expect(screen.getByText("Quality: HD-1080p")).toBeInTheDocument();
  });

  it("warns on a series card with a whitespace profile", () => {
    render(
      <SeriesCard
        series={{ ...baseSeries, qualityProfileId: "  " }}
        {...cardProps}
      />,
    );
    expect(screen.getAllByText("No quality profile").length).toBeGreaterThan(0);
  });

  it("does not warn on a configured series card", () => {
    render(
      <SeriesCard
        series={{ ...baseSeries, qualityProfileId: "qp-any" }}
        {...cardProps}
      />,
    );
    expect(screen.queryByText("No quality profile")).toBeNull();
  });

  it("warns on cards for a server-flagged dangling default ID", () => {
    render(
      <>
        <MovieCard
          movie={{
            ...baseMovie,
            qualityProfileId: "default",
            qualityProfileMissing: true,
          }}
          {...cardProps}
        />
        <SeriesCard
          series={{
            ...baseSeries,
            qualityProfileId: "default",
            qualityProfileMissing: true,
          }}
          {...cardProps}
        />
      </>,
    );
    const [movieCard, seriesCard] = screen.getAllByRole("button");
    expect(within(movieCard!).getAllByText("No quality profile").length).toBe(
      2,
    );
    expect(within(seriesCard!).getAllByText("No quality profile").length).toBe(
      2,
    );
  });

  it("does not warn on cards for a configured legacy profile ID", () => {
    render(
      <>
        <MovieCard
          movie={{
            ...baseMovie,
            qualityProfileId: "legacy-v1",
            qualityProfileMissing: false,
          }}
          {...cardProps}
        />
        <SeriesCard
          series={{
            ...baseSeries,
            qualityProfileId: "legacy-v1",
            qualityProfileMissing: false,
          }}
          {...cardProps}
        />
      </>,
    );
    expect(screen.queryByText("No quality profile")).toBeNull();
  });

  it("warns in rows for server-flagged IDs but not configured legacy IDs", () => {
    const rowProps = {
      profiles,
      selected: false,
      onToggleSelect: vi.fn(),
      onClick: vi.fn(),
    };
    const dangling = {
      qualityProfileId: "default",
      qualityProfileMissing: true,
    };
    const legacy = {
      qualityProfileId: "legacy-v1",
      qualityProfileMissing: false,
    };
    render(
      <Table>
        <TableBody>
          <MovieListRow movie={{ ...baseMovie, ...dangling }} {...rowProps} />
          <MovieListRow
            movie={{ ...baseMovie, id: "m2", ...legacy }}
            {...rowProps}
          />
          <SeriesListRow
            series={{ ...baseSeries, ...dangling }}
            {...rowProps}
          />
          <SeriesListRow
            series={{ ...baseSeries, id: "s2", ...legacy }}
            {...rowProps}
          />
        </TableBody>
      </Table>,
    );
    const [r0, r1, r2, r3] = screen.getAllByRole("row");
    expect(within(r0!).getByText("No quality profile")).toBeVisible();
    expect(within(r1!).queryByText("No quality profile")).toBeNull();
    expect(within(r2!).getByText("No quality profile")).toBeVisible();
    expect(within(r3!).queryByText("No quality profile")).toBeNull();
  });

  it("warns in movie and series table rows only when missing", () => {
    const rowProps = {
      profiles,
      selected: false,
      onToggleSelect: vi.fn(),
      onClick: vi.fn(),
    };
    render(
      <Table>
        <TableBody>
          <MovieListRow movie={baseMovie} {...rowProps} />
          <MovieListRow
            movie={{ ...baseMovie, id: "m2", qualityProfileId: "qp-hd" }}
            {...rowProps}
          />
          <SeriesListRow series={baseSeries} {...rowProps} />
          <SeriesListRow
            series={{ ...baseSeries, id: "s2", qualityProfileId: "qp-any" }}
            {...rowProps}
          />
        </TableBody>
      </Table>,
    );
    const rows = screen.getAllByRole("row");
    const row = (i: number) => {
      const r = rows[i];
      if (!r) throw new Error(`missing row ${i}`);
      return r;
    };
    expect(within(row(0)).getByText("No quality profile")).toBeVisible();
    expect(within(row(1)).queryByText("No quality profile")).toBeNull();
    expect(within(row(1)).getByText("HD-1080p")).toBeInTheDocument();
    expect(within(row(2)).getByText("No quality profile")).toBeVisible();
    expect(within(row(3)).queryByText("No quality profile")).toBeNull();
    expect(within(row(3)).getByText("Any")).toBeInTheDocument();
  });
});

describe("detail sheets", () => {
  const sheetProps = {
    open: true,
    onOpenChange: vi.fn(),
    onDeleted: vi.fn(),
    profiles,
    libraries: [],
  };

  it("lets the user fix a movie without a profile via the editor", async () => {
    const user = userEvent.setup();
    const onUpdated = vi.fn();
    withQuery(
      <MovieDetailSheet
        movie={baseMovie}
        onUpdated={onUpdated}
        {...sheetProps}
      />,
    );
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("No quality profile");
    await user.click(
      within(alert).getByRole("button", { name: "Set quality profile" }),
    );
    expect(screen.getByText("Editing Movie")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Set quality profile" })).toBe(
      null,
    );
  });

  it("warns and opens the editor for server-flagged dangling IDs", async () => {
    const user = userEvent.setup();
    const dangling = {
      qualityProfileId: "default",
      qualityProfileMissing: true,
    };
    const { unmount } = withQuery(
      <MovieDetailSheet
        movie={{ ...baseMovie, ...dangling }}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    let alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("No quality profile");
    await user.click(
      within(alert).getByRole("button", { name: "Set quality profile" }),
    );
    expect(screen.getByText("Editing Movie")).toBeInTheDocument();
    expect(screen.getByText("Select profile")).toBeInTheDocument();
    unmount();

    withQuery(
      <SeriesDetailSheet
        series={{ ...baseSeries, ...dangling }}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("No quality profile");
    await user.click(
      within(alert).getByRole("button", { name: "Set quality profile" }),
    );
    expect(screen.getByText("Editing Series")).toBeInTheDocument();
  });

  it("shows no warning for configured legacy IDs flagged false", async () => {
    const legacy = {
      qualityProfileId: "legacy-v1",
      qualityProfileMissing: false,
    };
    const { unmount } = withQuery(
      <MovieDetailSheet
        movie={{ ...baseMovie, ...legacy }}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    expect(
      await screen.findByRole("button", { name: /Edit/ }),
    ).toBeInTheDocument();
    expect(screen.queryByText("No quality profile")).toBeNull();
    unmount();
    withQuery(
      <SeriesDetailSheet
        series={{ ...baseSeries, ...legacy }}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    expect(
      await screen.findByRole("button", { name: /Edit/ }),
    ).toBeInTheDocument();
    expect(screen.queryByText("No quality profile")).toBeNull();
  });

  it("shows no warning for a configured movie", async () => {
    withQuery(
      <MovieDetailSheet
        movie={{ ...baseMovie, qualityProfileId: "qp-hd" }}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    expect(
      await screen.findByRole("button", { name: /Edit/ }),
    ).toBeInTheDocument();
    expect(screen.queryByText("No quality profile")).toBeNull();
  });

  it("lets the user fix a series without a profile via the editor", async () => {
    const user = userEvent.setup();
    withQuery(
      <SeriesDetailSheet
        series={baseSeries}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("No quality profile");
    await user.click(
      within(alert).getByRole("button", { name: "Set quality profile" }),
    );
    expect(screen.getByText("Editing Series")).toBeInTheDocument();
  });

  it("shows no warning for a configured series", async () => {
    withQuery(
      <SeriesDetailSheet
        series={{ ...baseSeries, qualityProfileId: "qp-hd" }}
        onUpdated={vi.fn()}
        {...sheetProps}
      />,
    );
    expect(
      await screen.findByRole("button", { name: /Edit/ }),
    ).toBeInTheDocument();
    expect(screen.queryByText("No quality profile")).toBeNull();
  });
});

describe("PersonDiscoverDialog default profile", () => {
  const library = (quality_profile_id: string): Library =>
    ({
      id: "lib-1",
      name: "Movies",
      path: "/movies",
      media_type: "movie",
      quality_profile_id,
    }) as Library;

  async function addCredit(libraries: Library[]) {
    const user = userEvent.setup();
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url === "/api/v1/discover/people/7") {
        return Response.json({
          person: { id: 7, name: "Director" },
          credits: [
            {
              tmdb_id: 42,
              media_type: "movie",
              title: "Credit Movie",
              rating: 0,
              popularity: 1,
              credit_type: "crew",
            },
          ],
        });
      }
      return Response.json({}, { status: 201 });
    });
    render(
      <PersonDiscoverDialog
        open
        onOpenChange={vi.fn()}
        personId={7}
        personName="Director"
        libraries={libraries}
        qualityProfiles={profiles}
        existingMovieIds={new Set()}
        existingSeriesIds={new Set()}
      />,
    );
    await user.click(
      await screen.findByRole("button", { name: /Credit Movie/ }),
    );
    await user.click(screen.getByRole("button", { name: "Add Movie" }));
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([u]) => String(u) === "/api/v1/movies"),
      ).toBe(true),
    );
    const call = fetchMock.mock.calls.find(
      ([u]) => String(u) === "/api/v1/movies",
    );
    return JSON.parse(String(call?.[1]?.body));
  }

  it("defaults to the Any profile when nothing is selected", async () => {
    const body = await addCredit([library("")]);
    expect(body.quality_profile_id).toBe("qp-any");
  });

  it("preserves the library's configured profile", async () => {
    const body = await addCredit([library("qp-hd")]);
    expect(body.quality_profile_id).toBe("qp-hd");
  });
});
