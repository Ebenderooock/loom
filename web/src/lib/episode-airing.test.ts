import { describe, expect, it } from "vitest";
import {
  episodeReleaseDay,
  formatEpisodeAirDate,
  getEpisodeStatus,
  getSeasonStats,
  hasEpisodeAired,
} from "./episode-airing";

const ep = {
  airDate: "2026-10-21",
  airDateUtc: "2026-10-22T00:00:00Z",
  monitored: true,
  hasFile: false,
};
const before = new Date("2026-10-21T23:59:59Z");
const exact = new Date(ep.airDateUtc);

describe("actual episode airing times", () => {
  it("shows the NZDT release day and the original US day", () => {
    expect(episodeReleaseDay(ep, "Pacific/Auckland")).toBe("2026-10-22");
    expect(episodeReleaseDay(ep, "America/New_York")).toBe("2026-10-21");
  });

  it("handles month boundaries and daylight saving changes", () => {
    expect(
      episodeReleaseDay(
        { airDateUtc: "2026-09-30T23:00:00Z" },
        "Pacific/Auckland",
      ),
    ).toBe("2026-10-01");
    expect(
      episodeReleaseDay(
        { airDateUtc: "2026-11-01T02:00:00Z" },
        "America/New_York",
      ),
    ).toBe("2026-10-31");
    expect(
      episodeReleaseDay(
        { airDateUtc: "2026-09-26T12:00:00Z" },
        "Pacific/Auckland",
      ),
    ).toBe("2026-09-27");
    expect(
      episodeReleaseDay(
        { airDateUtc: "2026-09-27T11:00:00Z" },
        "Pacific/Auckland",
      ),
    ).toBe("2026-09-28");
  });

  it("transitions at the exact instant, not the original date's midnight", () => {
    expect(hasEpisodeAired(ep, before)).toBe(false);
    expect(hasEpisodeAired(ep, exact)).toBe(true);
    expect(getEpisodeStatus(ep, before)).toBe("unaired");
    expect(getEpisodeStatus(ep, exact)).toBe("missing");
    expect(getSeasonStats([ep], before)).toMatchObject({
      unaired: 1,
      missing: 0,
      aired: 0,
    });
    expect(getSeasonStats([ep], exact)).toMatchObject({
      unaired: 0,
      missing: 1,
      aired: 1,
    });
  });

  it("preserves unmonitored, downloaded and grabbed precedence", () => {
    const episodes = [
      { ...ep, hasFile: true },
      { ...ep, grabbed: true },
      { ...ep, monitored: false, hasFile: true, grabbed: true },
    ];
    expect(episodes.map((e) => getEpisodeStatus(e, before))).toEqual([
      "downloaded",
      "grabbed",
      "unmonitored",
    ]);
    expect(getSeasonStats(episodes, before)).toMatchObject({
      downloaded: 1,
      grabbed: 1,
      unmonitored: 1,
      missing: 0,
      unaired: 0,
    });
  });

  it("never shifts date-only metadata and keeps its legacy availability", () => {
    const dateOnly = { airDate: "2026-10-21" };
    for (const zone of [
      "Pacific/Auckland",
      "America/New_York",
      "Pacific/Honolulu",
    ]) {
      expect(episodeReleaseDay(dateOnly, zone)).toBe("2026-10-21");
    }
    expect(hasEpisodeAired(dateOnly, new Date("2026-10-21T00:00:00Z"))).toBe(
      true,
    );
    expect(hasEpisodeAired(dateOnly, new Date("2026-10-20T23:59:59Z"))).toBe(
      false,
    );
    expect(hasEpisodeAired({}, exact)).toBe(false);
    expect(formatEpisodeAirDate({})).toBe("—");
    expect(formatEpisodeAirDate(dateOnly)).not.toMatch(/Invalid/);
    expect(formatEpisodeAirDate(ep)).not.toMatch(/Invalid/);
  });
});
