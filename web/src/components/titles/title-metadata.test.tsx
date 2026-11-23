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

  it.each(["w92", "w154", "w185", "w342", "w500", "w780"])(
    "sizes IMDb images for %s",
    (size) => {
      expect(
        titleImage(
          "https://m.media-amazon.com/images/M/example@._V1_.jpg",
          size,
        ),
      ).toBe(
        `https://m.media-amazon.com/images/M/example@._V1_SX${size.slice(1)}.jpg`,
      );
    },
  );

  it("replaces existing IMDb transformations and preserves the extension and query", () => {
    expect(
      titleImage(
        "https://ia.media-imdb.com/images/M/example@@._V1_UX100_CR0,0,100,150_.jpg?x=1",
        "w185",
      ),
    ).toBe("https://ia.media-imdb.com/images/M/example@@._V1_SX185.jpg?x=1");
  });

  it("leaves unsupported sizes and non-matching URLs unchanged", () => {
    const url = "https://m.media-amazon.com/images/M/example@._V1_.jpg";
    expect(titleImage(url, "original")).toBe(url);
    expect(titleImage(url, "w300")).toBe(url);
    expect(
      titleImage("https://other.example/images/M/example@._V1_.jpg", "w342"),
    ).toBe("https://other.example/images/M/example@._V1_.jpg");
    expect(
      titleImage("https://m.media-amazon.com/other/example@._V1_.jpg"),
    ).toBe("https://m.media-amazon.com/other/example@._V1_.jpg");
    expect(titleImage("https://m.media-amazon.com/images/M/example.jpg")).toBe(
      "https://m.media-amazon.com/images/M/example.jpg",
    );
  });
});
