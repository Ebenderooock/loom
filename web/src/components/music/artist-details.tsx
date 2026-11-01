import { useState, type ReactNode } from "react";
import { ExternalLink as ExternalLinkIcon, Star } from "lucide-react";
import { cn } from "@/lib/utils";
import type { Artist, ExternalLink, ExternalLinkType } from "@/lib/music-api";

const LINK_LABELS: Record<ExternalLinkType, string> = {
  spotify: "Spotify",
  apple_music: "Apple Music",
  youtube_music: "YouTube Music",
  tidal: "TIDAL",
  deezer: "Deezer",
  amazon_music: "Amazon Music",
  bandcamp: "Bandcamp",
  soundcloud: "SoundCloud",
  youtube: "YouTube",
  website: "Website",
  wikipedia: "Wikipedia",
  wikidata: "Wikidata",
  discogs: "Discogs",
  allmusic: "AllMusic",
  lastfm: "Last.fm",
  instagram: "Instagram",
  facebook: "Facebook",
  twitter: "X / Twitter",
  tiktok: "TikTok",
};

const STREAMING: ExternalLinkType[] = [
  "spotify",
  "apple_music",
  "youtube_music",
  "tidal",
  "deezer",
  "amazon_music",
  "bandcamp",
  "soundcloud",
];

export function linkLabel(type: ExternalLinkType): string {
  return LINK_LABELS[type] ?? type;
}

function safeHref(url: string): string | undefined {
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" || parsed.protocol === "http:"
      ? parsed.href
      : undefined;
  } catch {
    return undefined;
  }
}

/**
 * An image that falls back to a placeholder when the URL is missing or fails
 * to load (Cover Art Archive URLs are built without checking they exist).
 */
export function MusicArtwork({
  src,
  alt,
  className,
  imgClassName,
  fallback,
  loading = "lazy",
}: {
  src?: string;
  alt: string;
  className?: string;
  imgClassName?: string;
  fallback: ReactNode;
  loading?: "lazy" | "eager";
}) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const showImage = !!src && failedSrc !== src;
  return (
    <div className={cn("overflow-hidden bg-muted", className)}>
      {showImage ? (
        <img
          src={src}
          alt={alt}
          loading={loading}
          className={cn("h-full w-full object-cover", imgClassName)}
          onError={() => setFailedSrc(src ?? null)}
        />
      ) : (
        fallback
      )}
    </div>
  );
}

/** "Open in…" links for streaming services, then reference links. */
export function MusicExternalLinks({
  links,
  className,
}: {
  links?: ExternalLink[];
  className?: string;
}) {
  const valid = (links ?? []).flatMap((l) => {
    const href = safeHref(l.url);
    return href ? [{ ...l, href }] : [];
  });
  if (valid.length === 0) return null;
  const streaming = valid.filter((l) => STREAMING.includes(l.type));
  const other = valid.filter((l) => !STREAMING.includes(l.type));
  const render = (items: typeof valid, label: string) =>
    items.length > 0 && (
      <div className="space-y-1.5">
        <h4 className="text-xs font-semibold uppercase text-muted-foreground">
          {label}
        </h4>
        <div className="flex flex-wrap gap-1.5">
          {items.map((l) => (
            <a
              key={l.type}
              href={l.href}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-xs transition-colors hover:border-accent/50 hover:bg-accent/10"
            >
              {linkLabel(l.type)}
              <ExternalLinkIcon className="h-3 w-3 opacity-60" aria-hidden />
            </a>
          ))}
        </div>
      </div>
    );
  return (
    <div className={cn("space-y-3", className)}>
      {render(streaming, "Listen on")}
      {render(other, "Links")}
    </div>
  );
}

export function MusicRating({
  rating,
  votes,
}: {
  rating?: number;
  votes?: number;
}) {
  if (!rating) return null;
  return (
    <span
      className="inline-flex items-center gap-1"
      title={`MusicBrainz rating from ${votes ?? 0} vote${votes === 1 ? "" : "s"}`}
    >
      <Star className="h-3.5 w-3.5 fill-current text-amber-500" aria-hidden />
      {rating.toFixed(1)}/5
      {!!votes && <span className="text-muted-foreground">({votes})</span>}
    </span>
  );
}

function year(date?: string): string | undefined {
  return date ? date.slice(0, 4) : undefined;
}

/** Formats an artist's active years, e.g. "1996 – present" or "1970 – 1980". */
export function artistLifespan(
  artist: Pick<Artist, "begin_date" | "end_date" | "ended" | "artist_type">,
): string | undefined {
  const begin = year(artist.begin_date);
  const end = year(artist.end_date);
  if (!begin && !end) return undefined;
  if (artist.artist_type === "Person") {
    if (end) return `${begin ?? "?"} – ${end}`;
    return begin ? `Born ${begin}` : undefined;
  }
  if (end || artist.ended) return `${begin ?? "?"} – ${end ?? "?"}`;
  return `${begin} – present`;
}

/** Origin, active years and rating in one muted line. */
export function ArtistFacts({ artist }: { artist: Artist }) {
  const lifespan = artistLifespan(artist);
  const parts: ReactNode[] = [];
  if (artist.begin_area) parts.push(artist.begin_area);
  if (lifespan) parts.push(lifespan);
  if (artist.rating) {
    parts.push(
      <MusicRating
        key="rating"
        rating={artist.rating}
        votes={artist.rating_votes}
      />,
    );
  }
  if (parts.length === 0) return null;
  return (
    <p className="flex flex-wrap items-center gap-x-1.5 text-sm text-muted-foreground">
      {parts.map((p, i) => (
        <span key={i} className="inline-flex items-center gap-1.5">
          {i > 0 && <span aria-hidden>·</span>}
          {p}
        </span>
      ))}
    </p>
  );
}
