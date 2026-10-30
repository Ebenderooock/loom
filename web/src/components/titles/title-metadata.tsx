import { useState } from "react";
import { Star, Users } from "lucide-react";
import type { CreditPerson, Credits } from "@/components/movies/types";

export function titleImage(path?: string, size = "w342") {
  if (!path) return undefined;
  if (path.startsWith("https://image.tmdb.org/t/p/")) {
    return path.replace(/\/t\/p\/[^/]+\//, `/t/p/${size}/`);
  }
  return path.startsWith("/")
    ? `https://image.tmdb.org/t/p/${size}${path}`
    : path;
}

// Shared by provider details and the library sheets. Long synopses are never
// discarded: expansion is local presentation state, not truncated metadata.
export function TitleSynopsis({ overview }: { overview?: string }) {
  const [expanded, setExpanded] = useState(false);
  const long = (overview?.length ?? 0) > 280;
  return (
    <div>
      <p
        className={`whitespace-pre-line text-sm leading-relaxed text-muted-foreground ${long && !expanded ? "line-clamp-4" : ""}`}
      >
        {overview || "No synopsis available."}
      </p>
      {long && (
        <button
          type="button"
          className="mt-1.5 text-xs font-medium text-accent hover:underline"
          aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? "Show less" : "Read more"}
        </button>
      )}
    </div>
  );
}

export function TitleRating({ rating }: { rating?: number }) {
  if (!rating || rating <= 0) return null;
  return (
    <span className="inline-flex items-center gap-1 text-sm">
      <Star
        aria-hidden
        className="h-3.5 w-3.5 fill-yellow-400 text-yellow-400"
      />
      {rating.toFixed(1)}
      <span className="text-muted-foreground">/10</span>
    </span>
  );
}

export function TitlePersonCard({
  person,
  onClick,
}: {
  person: CreditPerson & { character?: string };
  onClick?: () => void;
}) {
  const content = (
    <>
      <div className="mb-1.5 h-[150px] w-[100px] overflow-hidden rounded-lg bg-muted/30">
        {person.profile_path ? (
          <img
            src={titleImage(person.profile_path, "w185")}
            alt=""
            className="h-full w-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="flex h-full items-center justify-center">
            <Users className="h-8 w-8 text-muted-foreground/30" />
          </div>
        )}
      </div>
      <p className="truncate text-xs font-medium" title={person.name}>
        {person.name}
      </p>
      <p
        className="truncate text-[11px] text-muted-foreground"
        title={person.character || person.role}
      >
        {person.character || person.role}
      </p>
    </>
  );
  return onClick ? (
    <button
      type="button"
      onClick={onClick}
      className="w-[100px] shrink-0 text-left"
    >
      {content}
    </button>
  ) : (
    <div className="w-[100px] shrink-0">{content}</div>
  );
}

export function TitleCredits({ credits }: { credits?: Credits }) {
  if (!credits || (!credits.cast?.length && !credits.crew?.length))
    return (
      <p className="text-sm text-muted-foreground">No credits available.</p>
    );
  return (
    <div className="min-w-0 max-w-full space-y-4">
      {(["cast", "crew"] as const).map(
        (group) =>
          credits[group]?.length > 0 && (
            <section key={group} className="min-w-0">
              <h3 className="mb-2 text-sm font-semibold capitalize">{group}</h3>
              <div className="flex min-w-0 flex-wrap gap-3 pb-2">
                {credits[group].map((person, i) => (
                  <TitlePersonCard key={`${person.id}-${i}`} person={person} />
                ))}
              </div>
            </section>
          ),
      )}
    </div>
  );
}

export function ArtistMetadata({
  artist,
}: {
  artist: {
    country?: string;
    type?: string;
    artist_type?: string;
    disambiguation?: string;
    overview?: string;
    genres?: string[];
  };
}) {
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {[
          artist.type || artist.artist_type,
          artist.country,
          artist.disambiguation,
        ]
          .filter(Boolean)
          .join(" · ")}
      </p>
      {!!artist.genres?.length && (
        <p className="text-sm">{artist.genres.join(" · ")}</p>
      )}
      <TitleSynopsis overview={artist.overview} />
    </div>
  );
}
