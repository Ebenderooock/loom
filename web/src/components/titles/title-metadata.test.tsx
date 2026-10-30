import { describe, expect, it } from "vitest";
import { titleImage } from "./title-metadata";

describe("provider artwork", () => {
  it("supports relative paths, already-absolute CDN paths, and other providers", () => {
    expect(titleImage("/poster.jpg", "w185")).toBe(
      "https://image.tmdb.org/t/p/w185/poster.jpg",
    );
    expect(
      titleImage("https://image.tmdb.org/t/p/original/poster.jpg", "w342"),
    ).toBe("https://image.tmdb.org/t/p/w342/poster.jpg");
    expect(titleImage("https://coverartarchive.org/cover.jpg")).toBe(
      "https://coverartarchive.org/cover.jpg",
    );
    expect(titleImage("")).toBeUndefined();
  });
});
