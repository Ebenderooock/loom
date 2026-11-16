import { describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import {
  ArtistFacts,
  ArtistBiography,
  MusicArtwork,
  MusicExternalLinks,
  artistLifespan,
  albumTracksAbsent,
} from "@/components/music/artist-details";
import type { Album, Artist } from "@/lib/music-api";

const baseArtist: Artist = {
  id: "a1",
  mbid: "mb1",
  name: "Linkin Park",
  monitoring_status: "monitored",
  created_at: "",
  updated_at: "",
};

describe("ArtistBiography", () => {
  it("uses a short preview and an unobtrusive accessible disclosure", () => {
    const overview = `${"Artist history and music. ".repeat(20)}Final paragraph.`;
    render(<ArtistBiography overview={overview} />);
    expect(screen.queryByText(overview)).not.toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Read more" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(screen.getByText(overview)).toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(screen.getByRole("button", { name: "Read less" }));
    expect(screen.queryByText(overview)).not.toBeInTheDocument();
  });

  it("does not offer disclosure for short or missing biographies", () => {
    const { rerender } = render(<ArtistBiography overview="A short intro." />);
    expect(screen.getByText("A short intro.")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    rerender(<ArtistBiography />);
    expect(screen.queryByText("A short intro.")).not.toBeInTheDocument();
  });

  it("resets the preview when the biography changes", () => {
    const overview = "A longer intro. ".repeat(40);
    const { rerender } = render(<ArtistBiography overview={overview} />);
    fireEvent.click(screen.getByRole("button", { name: "Read more" }));
    rerender(<ArtistBiography overview={"Another intro. ".repeat(40)} />);
    expect(screen.getByRole("button", { name: "Read more" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });
});

describe("albumTracksAbsent", () => {
  const album: Album = {
    id: "al1",
    mbid: "mb1",
    title: "Record",
    monitored: true,
  };
  it("does not mistake an unloaded or failed track lookup for proven absence", () => {
    expect(albumTracksAbsent(album)).toBe(false);
    expect(albumTracksAbsent({ ...album, tracks: [] })).toBe(false);
    expect(
      albumTracksAbsent({
        ...album,
        tracks: [],
        tracks_fetched_at: "2024-01-01",
      }),
    ).toBe(false);
  });

  it("recognizes a successful lookup with no editions", () => {
    expect(
      albumTracksAbsent({
        ...album,
        releases: [],
        releases_fetched_at: "2024-01-01",
      }),
    ).toBe(true);
    // Go's omitempty excludes zero-length releases and tracks on the wire.
    expect(
      albumTracksAbsent({ ...album, releases_fetched_at: "2024-01-01" }),
    ).toBe(true);
  });
});

describe("MusicArtwork", () => {
  it("falls back when the image fails to load", () => {
    render(
      <MusicArtwork
        src="https://coverartarchive.org/missing"
        alt="cover"
        fallback={<span>placeholder</span>}
      />,
    );
    fireEvent.error(screen.getByAltText("cover"));
    expect(screen.getByText("placeholder")).toBeInTheDocument();
  });

  it("shows the fallback when there is no image", () => {
    render(<MusicArtwork alt="cover" fallback={<span>none</span>} />);
    expect(screen.getByText("none")).toBeInTheDocument();
  });
});

describe("MusicExternalLinks", () => {
  it("groups streaming links and drops unsafe URLs", () => {
    render(
      <MusicExternalLinks
        links={[
          { type: "spotify", url: "https://open.spotify.com/artist/x" },
          { type: "wikipedia", url: "https://en.wikipedia.org/wiki/X" },
          { type: "website", url: "javascript:alert(1)" },
        ]}
      />,
    );
    expect(screen.getByText("Listen on")).toBeInTheDocument();
    const spotify = screen.getByRole("link", { name: /Spotify/ });
    expect(spotify).toHaveAttribute(
      "href",
      "https://open.spotify.com/artist/x",
    );
    expect(spotify).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByRole("link", { name: /Wikipedia/ })).toBeInTheDocument();
    expect(screen.queryByText("Website")).not.toBeInTheDocument();
  });

  it("renders nothing without links", () => {
    const { container } = render(<MusicExternalLinks links={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("artist facts", () => {
  it("formats active years for groups and people", () => {
    expect(artistLifespan({ begin_date: "1996-01-01" })).toBe("1996 – present");
    expect(
      artistLifespan({ begin_date: "1970", end_date: "1980-05", ended: true }),
    ).toBe("1970 – 1980");
    expect(
      artistLifespan({ artist_type: "Person", begin_date: "1958-08-29" }),
    ).toBe("Born 1958");
    expect(artistLifespan({})).toBeUndefined();
  });

  it("shows origin, years and rating", () => {
    render(
      <ArtistFacts
        artist={{
          ...baseArtist,
          begin_area: "Agoura Hills",
          begin_date: "1996",
          rating: 4.25,
          rating_votes: 10,
        }}
      />,
    );
    expect(screen.getByText("Agoura Hills")).toBeInTheDocument();
    expect(screen.getByText("1996 – present")).toBeInTheDocument();
    expect(screen.getByText(/4\.3\/5/)).toBeInTheDocument();
  });
});
