import { describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import {
  ArtistFacts,
  MusicArtwork,
  MusicExternalLinks,
  artistLifespan,
} from "@/components/music/artist-details";
import type { Artist } from "@/lib/music-api";

const baseArtist: Artist = {
  id: "a1",
  mbid: "mb1",
  name: "Linkin Park",
  monitoring_status: "monitored",
  created_at: "",
  updated_at: "",
};

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
