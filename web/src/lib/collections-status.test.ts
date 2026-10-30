import { describe, expect, it } from "vitest";
import { matchesMember, type CollectionMember } from "./collections-api";

const member: CollectionMember = {
  id: "m",
  title: "Movie",
  media_type: "movie",
  external_ids: { tmdb: "42" },
  sources: [],
  provenance: [],
  local: true,
  excluded: false,
  in_library: true,
  monitored: true,
  has_file: false,
  status: "downloading",
};
const matches = (m: CollectionMember, status: string) =>
  matchesMember(m, "", "", status);

describe("collection status vs membership/file filters", () => {
  it("keeps Downloading separate from missing and external membership", () => {
    expect(matches(member, "downloading")).toBe(true);
    expect(matches(member, "missing")).toBe(false);
    expect(matches(member, "external")).toBe(false);
    expect(matches(member, "added")).toBe(true);
    expect(matches(member, "files")).toBe(false);
    const available = {
      ...member,
      status: "available_right_quality",
      has_file: true,
    };
    expect(matches(available, "available")).toBe(true);
    expect(matches(available, "missing")).toBe(false);
    expect(matches(available, "files")).toBe(true);
    expect(
      matches({ ...member, in_library: false, status: undefined }, "external"),
    ).toBe(true);
  });
  it("uses aired/downloaded series counts, not any-file presence or broadcast state", () => {
    const partial: CollectionMember = {
      ...member,
      media_type: "series",
      status: "continuing",
      has_file: true,
      episode_stats: {
        totalEpisodes: 10,
        airedEpisodes: 8,
        downloadedEpisodes: 3,
        missingEpisodes: 5,
        monitoredEpisodes: 8,
      },
    };
    expect(matches(partial, "missing")).toBe(true);
    expect(matches(partial, "available")).toBe(false);
    expect(matches(partial, "files")).toBe(true);
    const complete = {
      ...partial,
      episode_stats: {
        ...partial.episode_stats!,
        downloadedEpisodes: 8,
        missingEpisodes: 0,
      },
    };
    expect(matches(complete, "available")).toBe(true);
    expect(matches(complete, "missing")).toBe(false);
  });
});
