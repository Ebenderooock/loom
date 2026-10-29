import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { titleJSON, type ExternalAlbum } from "@/lib/title-api";

export function AlbumMetadataSummary({
  title,
  date,
  type,
}: {
  title: string;
  date?: string;
  type?: string;
}) {
  return (
    <div>
      <p className="text-sm font-medium">{title}</p>
      <p className="text-xs text-muted-foreground">
        {[date, type].filter(Boolean).join(" · ")}
      </p>
    </div>
  );
}

interface Release {
  mbid: string;
  title: string;
  release_date?: string;
  country?: string;
  format?: string;
  label?: string;
  status?: string;
  tracks?: {
    mbid: string;
    track_id: string;
    title: string;
    track_number: number;
    disc_number: number;
    duration_ms?: number;
    artist_name?: string;
  }[];
}

function ReleaseTracks({ id }: { id: string }) {
  const release = useQuery({
    queryKey: ["external-release", id],
    queryFn: ({ signal }) =>
      titleJSON<Release>(`/api/v1/discover/release/${encodeURIComponent(id)}`, {
        signal,
      }),
    staleTime: 5 * 60_000,
  });
  if (release.isPending)
    return (
      <p role="status" className="text-sm">
        Loading tracks…
      </p>
    );
  if (release.isError)
    return (
      <div role="alert">
        <p className="text-sm text-destructive">{release.error.message}</p>
        <Button variant="outline" onClick={() => release.refetch()}>
          Retry tracks
        </Button>
      </div>
    );
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        {[
          release.data.label,
          release.data.format,
          release.data.country,
          release.data.status,
        ]
          .filter(Boolean)
          .join(" · ")}
      </p>
      {!release.data.tracks?.length && (
        <p className="text-sm text-muted-foreground">
          No track listing available.
        </p>
      )}
      <ol className="space-y-1">
        {release.data.tracks?.map((track, i) => (
          <li
            key={track.track_id || `${track.mbid}-${i}`}
            className="flex items-start gap-3 text-sm"
          >
            <span className="w-10 shrink-0 text-muted-foreground">
              {track.disc_number}.{track.track_number}
            </span>
            <span className="flex-1">
              {track.title}
              {track.artist_name && (
                <span className="block text-xs text-muted-foreground">
                  {track.artist_name}
                </span>
              )}
            </span>
            {track.duration_ms ? (
              <span className="text-xs text-muted-foreground">
                {Math.floor(track.duration_ms / 60000)}:
                {String(Math.floor(track.duration_ms / 1000) % 60).padStart(
                  2,
                  "0",
                )}
              </span>
            ) : null}
          </li>
        ))}
      </ol>
    </div>
  );
}

export function ExternalAlbumDetails({ album }: { album: ExternalAlbum }) {
  const [expanded, setExpanded] = useState(false);
  const [releaseId, setReleaseId] = useState("");
  const details = useQuery({
    queryKey: ["external-album", album.mbid],
    queryFn: ({ signal }) =>
      titleJSON<{ album: ExternalAlbum; releases: Release[] }>(
        `/api/v1/discover/album/${encodeURIComponent(album.mbid)}`,
        { signal },
      ),
    enabled: expanded,
    staleTime: 5 * 60_000,
  });
  const chosenRelease = releaseId || details.data?.releases?.[0]?.mbid;
  return (
    <div className="space-y-3 rounded-md border p-3">
      <button
        type="button"
        className="flex w-full items-start gap-3 text-left"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        {album.cover_art_url && (
          <img
            src={album.cover_art_url}
            alt=""
            className="h-14 w-14 rounded object-cover"
            loading="lazy"
          />
        )}
        <div className="flex-1">
          <AlbumMetadataSummary
            title={album.title}
            date={album.release_date}
            type={[album.type, ...(album.secondary_types ?? [])]
              .filter(Boolean)
              .join(" · ")}
          />
          <span className="text-xs text-accent">
            {expanded ? "Hide editions and tracks" : "View editions and tracks"}
          </span>
        </div>
      </button>
      {expanded && (
        <div className="space-y-3 border-t pt-3">
          {details.isPending && (
            <p role="status" className="text-sm">
              Loading album…
            </p>
          )}
          {details.isError && (
            <div role="alert">
              <p className="text-sm text-destructive">
                {details.error.message}
              </p>
              <Button variant="outline" onClick={() => details.refetch()}>
                Retry album
              </Button>
            </div>
          )}
          {!!details.data?.album.genres?.length && (
            <p className="text-xs text-muted-foreground">
              {details.data.album.genres.join(" · ")}
            </p>
          )}
          {!!details.data?.releases?.length && (
            <label className="block space-y-1 text-sm">
              Edition
              <select
                className="block h-10 w-full rounded-md border bg-background px-2 text-sm"
                value={chosenRelease}
                onChange={(e) => setReleaseId(e.target.value)}
              >
                {details.data.releases.map((release) => (
                  <option key={release.mbid} value={release.mbid}>
                    {[
                      release.title,
                      release.release_date,
                      release.country,
                      release.format,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </option>
                ))}
              </select>
            </label>
          )}
          {details.isSuccess && !details.data.releases?.length && (
            <p className="text-sm text-muted-foreground">
              No editions available.
            </p>
          )}
          {chosenRelease && (
            <ReleaseTracks key={chosenRelease} id={chosenRelease} />
          )}
        </div>
      )}
    </div>
  );
}
