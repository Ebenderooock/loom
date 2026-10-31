import { cleanup, render, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MovieCard } from "@/components/movies/movie-card";
import { SeriesCard } from "@/components/series/series-card";
import { SeriesListRow } from "@/components/series/series-list-row";
import type { Movie } from "@/components/movies/types";
import type { Series } from "@/components/series/types";
import { TitlePosterCard } from "../title-poster-card";

afterEach(cleanup);
const noop = () => {};
const actions = {
  profiles: [],
  selected: false,
  selectMode: false,
  onToggleSelect: noop,
  onClick: noop,
};
const movie: Movie = {
  id: "jurassic-world",
  title: "Jurassic World",
  year: 2015,
  overview: "",
  rating: 0,
  runtime: 0,
  genres: [],
  releaseDate: "",
  status: "missing",
  monitoringStatus: "monitored",
  qualityProfileId: "hd",
  libraryId: "movies",
};

function expectSamePresentation(
  library: HTMLElement,
  collection: HTMLElement,
  label: string,
) {
  const badge = within(library).getAllByText(label)[0]!;
  const counterpart = within(collection).getByText(label);
  expect(counterpart.outerHTML).toBe(badge.outerHTML);
  expect(collection.querySelector(".h-\\[3px\\]")?.outerHTML).toBe(
    library.querySelector(".h-\\[3px\\]")?.outerHTML,
  );
}

describe("canonical library card status on collection/Discover posters", () => {
  it.each([
    ["monitored", true],
    ["all", true],
    ["future", true],
    ["missing", true],
    ["existing", true],
    ["pilot", true],
    ["firstSeason", true],
    ["lastSeason", true],
    ["none", false],
    ["unmonitored", false],
    ["archived", false],
  ] as const)(
    "consistently presents series strategy %s across library cards, lists and collection posters",
    (monitoringStatus, monitored) => {
      const series: Series = {
        ...movie,
        network: "",
        seriesType: "standard",
        seasonFolder: true,
        status: "continuing",
        monitoringStatus,
      };
      const { getByTestId } = render(
        <>
          <section data-testid="library">
            <SeriesCard {...actions} series={series} />
          </section>
          <section data-testid="collection">
            <TitlePosterCard
              title={series.title}
              mediaType="series"
              inLibrary
              status={series.status}
              monitoringStatus={monitoringStatus}
              onOpen={noop}
            />
          </section>
          <table data-testid="list">
            <tbody>
              <SeriesListRow {...actions} series={series} />
            </tbody>
          </table>
        </>,
      );
      for (const id of ["library", "collection"]) {
        const label = within(getByTestId(id)).queryByText("Unmonitored");
        if (monitored) expect(label).not.toBeInTheDocument();
        else expect(label).toBeInTheDocument();
      }
      expect(
        within(getByTestId("list")).getByRole("img", {
          name: monitored ? "Monitored" : "Unmonitored",
        }),
      ).toBeInTheDocument();
    },
  );

  it.each([
    ["missing", "Missing"],
    ["downloading", "Downloading"],
    ["storing", "Importing"],
    ["available_right_quality", "Available"],
    ["available_wrong_quality", "Wrong Quality"],
    ["available_higher_quality", "HD Available"],
    ["unreleased", "Unreleased"],
  ])("matches movie %s label, badge/dot and strip exactly", (status, label) => {
    const { getByTestId } = render(
      <>
        <section data-testid="library">
          <MovieCard {...actions} movie={{ ...movie, status }} />
        </section>
        <section data-testid="collection">
          <TitlePosterCard
            title={movie.title}
            inLibrary
            status={status}
            onOpen={noop}
          />
        </section>
      </>,
    );
    expectSamePresentation(
      getByTestId("library"),
      getByTestId("collection"),
      label,
    );
  });

  it.each([0, 3, 8])(
    "matches series episode aggregation for %i/8 episodes",
    (downloaded) => {
      const series: Series = {
        ...movie,
        network: "",
        seriesType: "standard",
        seasonFolder: true,
        status: "continuing",
        episodeStats: {
          totalEpisodes: 10,
          airedEpisodes: 8,
          downloadedEpisodes: downloaded,
          monitoredEpisodes: 8,
          missingEpisodes: 8 - downloaded,
        },
      };
      const { getByTestId } = render(
        <>
          <section data-testid="library">
            <SeriesCard {...actions} series={series} />
          </section>
          <section data-testid="collection">
            <TitlePosterCard
              title={series.title}
              mediaType="series"
              inLibrary
              status={series.status}
              episodeStats={series.episodeStats}
              onOpen={noop}
            />
          </section>
        </>,
      );
      expectSamePresentation(
        getByTestId("library"),
        getByTestId("collection"),
        `${downloaded}/8 episodes`,
      );
    },
  );

  it("does not present membership as downloaded and keeps monitoring separate", () => {
    const { getByTestId } = render(
      <>
        <section data-testid="external">
          <TitlePosterCard
            title="Not owned"
            status="available_right_quality"
            onOpen={noop}
          />
        </section>
        <section data-testid="local">
          <TitlePosterCard
            title={movie.title}
            inLibrary
            status="downloading"
            monitoringStatus="unmonitored"
            onOpen={noop}
          />
        </section>
      </>,
    );
    expect(
      within(getByTestId("external")).getByText("Missing"),
    ).toBeInTheDocument();
    expect(
      within(getByTestId("external")).queryByText("Available"),
    ).not.toBeInTheDocument();
    expect(
      within(getByTestId("local")).getByText("Downloading"),
    ).toBeInTheDocument();
    expect(
      within(getByTestId("local")).getByText("Unmonitored"),
    ).toBeInTheDocument();
  });
});
