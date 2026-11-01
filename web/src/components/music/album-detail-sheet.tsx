import { useState } from "react";
import {
  Bookmark,
  BookmarkCheck,
  Disc3,
  Download,
  ExternalLink,
  Loader2,
  Music2,
  Search,
  TextSearch,
} from "lucide-react";
import { toast } from "sonner";
import { AlbumSearchDialog } from "@/components/music/album-search-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  useAlbum,
  useSearchAlbum,
  useSelectAlbumRelease,
  useSetAlbumMonitored,
  useSetTrackMonitored,
  formatAlbumArtistCredits,
  type Album,
} from "@/lib/music-api";
import {
  MusicArtwork,
  MusicExternalLinks,
  MusicRating,
} from "./artist-details";

function durationLabel(durationMs?: number): string {
  if (!durationMs || durationMs <= 0) return "";
  const seconds = Math.round(durationMs / 1000);
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;
}

function albumArtists(album: Album): string {
  return formatAlbumArtistCredits(album.artists);
}

export function AlbumDetailSheet({
  album: selectedAlbum,
  open,
  onOpenChange,
}: {
  album: Album | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const {
    data: albumData,
    isLoading,
    isError,
    refetch,
  } = useAlbum(open ? (selectedAlbum?.id ?? undefined) : undefined);
  const album = albumData ?? selectedAlbum;
  const [interactiveOpen, setInteractiveOpen] = useState(false);
  const setAlbumMonitored = useSetAlbumMonitored();
  const setTrackMonitored = useSetTrackMonitored();
  const selectRelease = useSelectAlbumRelease();
  const searchAlbum = useSearchAlbum();

  if (!album) return null;

  const tracks = album.tracks ?? [];
  const availableCount = tracks.filter((track) => track.has_file).length;
  const missingCount = tracks.length - availableCount;
  const monitored = album.monitored;
  const release = album.releases?.find(
    (item) => item.id === album.selected_release_id,
  );
  const search = () => {
    searchAlbum.mutate(album.id, {
      onSuccess: (result) =>
        toast.success(
          `Grabbed “${result.title}” (${result.quality_name || "unknown"})`,
        ),
      onError: (error) =>
        toast.error(error instanceof Error ? error.message : "Search failed"),
    });
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full overflow-y-auto p-0 sm:max-w-2xl"
      >
        <SheetHeader className="sr-only">
          <SheetTitle>{album.title}</SheetTitle>
          <SheetDescription>
            Album details, releases, tracks, and monitoring controls
          </SheetDescription>
        </SheetHeader>

        <header className="relative min-h-72 overflow-hidden bg-muted">
          {album.cover_art_url && (
            <MusicArtwork
              src={album.cover_art_url}
              alt=""
              className="absolute inset-0 h-full w-full scale-110 opacity-30 blur-2xl"
              fallback={null}
            />
          )}
          <div className="absolute inset-0 bg-gradient-to-t from-background via-background/80 to-background/25" />
          <div className="relative flex min-h-72 items-end gap-5 px-6 pb-6 pt-12">
            <MusicArtwork
              src={album.cover_art_url}
              alt={`${album.title} cover`}
              loading="eager"
              className="flex h-36 w-36 shrink-0 items-center justify-center overflow-hidden rounded-md border border-white/15 bg-card shadow-2xl sm:h-44 sm:w-44"
              fallback={<Disc3 className="h-12 w-12 text-muted-foreground" />}
            />
            <div className="min-w-0 flex-1 pb-1">
              <p className="mb-2 text-[10px] font-semibold uppercase tracking-[0.2em] text-foreground/65">
                {album.album_type || "Album"}
                {album.release_date
                  ? ` · ${album.release_date.slice(0, 4)}`
                  : ""}
              </p>
              <h2 className="line-clamp-3 text-2xl font-bold leading-tight sm:text-3xl">
                {album.title}
              </h2>
              <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">
                {albumArtists(album)}
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <MusicRating rating={album.rating} votes={album.rating_votes} />
                {album.secondary_types?.map((type) => (
                  <Badge key={type} variant="outline" className="text-[10px]">
                    {type}
                  </Badge>
                ))}
              </div>
            </div>
          </div>
        </header>

        <div className="space-y-6 px-6 py-5">
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant={monitored ? "secondary" : "outline"}
              disabled={setAlbumMonitored.isPending}
              onClick={() =>
                setAlbumMonitored.mutate(
                  { id: album.id, monitored: !monitored },
                  {
                    onSuccess: () =>
                      toast.success(
                        monitored ? "Album unmonitored" : "Album monitored",
                      ),
                    onError: (error) =>
                      toast.error(
                        error instanceof Error
                          ? error.message
                          : "Could not update album monitoring",
                      ),
                  },
                )
              }
            >
              {monitored ? (
                <BookmarkCheck className="mr-1.5 h-4 w-4" />
              ) : (
                <Bookmark className="mr-1.5 h-4 w-4" />
              )}
              {monitored ? "Monitored" : "Monitor"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!monitored || searchAlbum.isPending}
              onClick={search}
            >
              {searchAlbum.isPending ? (
                <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
              ) : (
                <Search className="mr-1.5 h-4 w-4" />
              )}
              Search
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setInteractiveOpen(true)}
            >
              <TextSearch className="mr-1.5 h-4 w-4" />
              Interactive search
            </Button>
            {album.links?.some((link) => link.type === "spotify") && (
              <a
                href={album.links.find((link) => link.type === "spotify")?.url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-9 items-center rounded-md border border-input px-3 text-sm hover:bg-accent hover:text-accent-foreground"
              >
                <ExternalLink className="mr-1.5 h-4 w-4" />
                Listen
              </a>
            )}
          </div>

          {(album.overview || album.genres?.length || album.links?.length) && (
            <section className="space-y-3 border-t border-border/60 pt-4">
              {album.overview && (
                <p className="whitespace-pre-line text-sm leading-relaxed text-muted-foreground">
                  {album.overview}
                </p>
              )}
              {album.genres && album.genres.length > 0 && (
                <div className="flex flex-wrap gap-1.5">
                  {album.genres.map((genre) => (
                    <Badge key={genre} variant="secondary">
                      {genre}
                    </Badge>
                  ))}
                </div>
              )}
              <MusicExternalLinks links={album.links} />
            </section>
          )}

          <section className="space-y-3 border-t border-border/60 pt-4">
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div>
                <h3 className="text-sm font-semibold">Edition and tracks</h3>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {tracks.length
                    ? `${availableCount} of ${tracks.length} tracks available`
                    : "Track availability"}
                  {tracks.length > 0 && missingCount > 0
                    ? ` · ${missingCount} missing`
                    : ""}
                </p>
              </div>
              {album.releases && album.releases.length > 0 && (
                <Select
                  value={album.selected_release_id ?? ""}
                  onValueChange={(releaseID) =>
                    selectRelease.mutate(
                      { albumId: album.id, releaseId: releaseID },
                      {
                        onSuccess: () => toast.success("Album edition updated"),
                        onError: (error) =>
                          toast.error(
                            error instanceof Error
                              ? error.message
                              : "Could not change edition",
                          ),
                      },
                    )
                  }
                  disabled={selectRelease.isPending}
                >
                  <SelectTrigger
                    className="h-9 w-full sm:w-64"
                    aria-label="Album edition"
                  >
                    <SelectValue placeholder="Choose an edition" />
                  </SelectTrigger>
                  <SelectContent>
                    {album.releases.map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {[item.title || album.title, item.country, item.format]
                          .filter(Boolean)
                          .join(" · ")}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
            {release && (
              <p className="flex flex-wrap gap-x-2 text-xs text-muted-foreground">
                {release.label && <span>{release.label}</span>}
                {release.release_date && <span>{release.release_date}</span>}
                {release.status && <span>{release.status}</span>}
                {release.track_count > 0 && (
                  <span>{release.track_count} tracks</span>
                )}
              </p>
            )}

            {isLoading ? (
              <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading album details…
              </div>
            ) : isError && !albumData ? (
              <div className="flex items-center justify-between gap-3 py-4 text-sm text-muted-foreground">
                <span>Could not load this album’s releases and tracks.</span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void refetch()}
                >
                  Retry
                </Button>
              </div>
            ) : tracks.length === 0 ? (
              <p className="py-4 text-sm text-muted-foreground">
                No track list is available yet. Open the album again to retry
                metadata loading.
              </p>
            ) : (
              <ol className="divide-y divide-border/60">
                {tracks.map((track) => (
                  <li
                    key={track.id}
                    className="flex min-h-12 items-center gap-3 py-2 text-sm"
                  >
                    <span className="w-8 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                      {track.disc_number > 1
                        ? `${track.disc_number}.${track.track_number}`
                        : track.track_number}
                    </span>
                    <Music2
                      className={`h-3.5 w-3.5 shrink-0 ${track.has_file ? "text-primary" : "text-muted-foreground/50"}`}
                    />
                    <span className="min-w-0 flex-1 truncate">
                      {track.title || "Untitled track"}
                    </span>
                    {track.artist_name && (
                      <span className="hidden max-w-36 truncate text-xs text-muted-foreground sm:block">
                        {track.artist_name}
                      </span>
                    )}
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                      {durationLabel(track.duration_ms)}
                    </span>
                    <Badge
                      variant={track.has_file ? "secondary" : "outline"}
                      className="hidden shrink-0 text-[10px] sm:inline-flex"
                    >
                      {track.has_file ? "Available" : "Missing"}
                    </Badge>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-7 w-7 shrink-0"
                      aria-label={
                        track.monitored
                          ? `Unmonitor ${track.title || "track"}`
                          : `Monitor ${track.title || "track"}`
                      }
                      disabled={setTrackMonitored.isPending}
                      onClick={() =>
                        setTrackMonitored.mutate(
                          {
                            albumId: album.id,
                            trackId: track.id,
                            monitored: !track.monitored,
                            artistId: album.artist_id,
                          },
                          {
                            onError: (error) =>
                              toast.error(
                                error instanceof Error
                                  ? error.message
                                  : "Could not update track monitoring",
                              ),
                          },
                        )
                      }
                    >
                      {track.monitored ? (
                        <BookmarkCheck className="h-3.5 w-3.5" />
                      ) : (
                        <Bookmark className="h-3.5 w-3.5" />
                      )}
                    </Button>
                  </li>
                ))}
              </ol>
            )}
            {album.library_id && (
              <p className="flex items-center gap-1.5 pt-1 text-[11px] text-muted-foreground">
                <Download className="h-3 w-3" />
                Organized in the selected music library
              </p>
            )}
          </section>
        </div>
      </SheetContent>
      {interactiveOpen && (
        <AlbumSearchDialog
          albumId={album.id}
          albumTitle={album.title}
          open={interactiveOpen}
          onOpenChange={setInteractiveOpen}
        />
      )}
    </Sheet>
  );
}
