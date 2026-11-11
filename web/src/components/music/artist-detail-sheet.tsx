import { useState } from "react";
import { Link } from "@tanstack/react-router";
import {
  ArrowUpRight,
  Bookmark,
  BookmarkCheck,
  ChevronDown,
  ChevronRight,
  Disc3,
  FileAudio,
  History,
  Info,
  Loader2,
  Music,
  Pencil,
  RefreshCw,
  Search,
  TextSearch,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { AlbumSearchDialog } from "@/components/music/album-search-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  useAlbum,
  useArtist,
  useRefreshArtist,
  useRescanArtist,
  useSearchAlbum,
  useSetAlbumMonitored,
  useSetArtistMonitoring,
  useSetTrackMonitored,
  type Album,
  type Artist,
} from "@/lib/music-api";
import {
  ArtistFacts,
  ArtistBiography,
  MusicArtistMetadata,
  albumTracksAbsent,
  MusicArtwork,
  MusicExternalLinks,
  MusicRating,
} from "./artist-details";
import { ArtistFilesList, ArtistHistoryList } from "./artist-activity";
import { ArtistEditDialog } from "./artist-edit-dialog";
import { AlbumDetailSheet } from "./album-detail-sheet";

function trackDuration(durationMs?: number): string | undefined {
  if (!durationMs || durationMs <= 0) return undefined;
  const totalSeconds = Math.round(durationMs / 1000);
  return `${Math.floor(totalSeconds / 60)}:${String(totalSeconds % 60).padStart(2, "0")}`;
}

function DrawerSection({
  title,
  icon: Icon,
  children,
}: {
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  children: (open: boolean) => React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <section className="border-t border-border/60">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center gap-2 py-3 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground"
      >
        <Icon className="h-4 w-4" />
        {title}
        <ChevronRight
          className={`ml-auto h-4 w-4 transition-transform duration-200 ${open ? "rotate-90" : ""}`}
        />
      </button>
      {open && <div className="pb-4">{children(open)}</div>}
    </section>
  );
}

export function AlbumSection({
  album,
  artistId,
  onMonitoredChange,
  onDetails,
}: {
  album: Album;
  artistId: string;
  onMonitoredChange: (album: Album, monitored: boolean) => void;
  onDetails: (album: Album) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [interactiveOpen, setInteractiveOpen] = useState(false);
  const setTrackMonitored = useSetTrackMonitored();
  const {
    data: details,
    isFetching,
    isError,
    refetch,
  } = useAlbum(expanded ? album.id : undefined);
  const setAlbumMonitored = useSetAlbumMonitored();
  const searchAlbum = useSearchAlbum();
  const currentAlbum = details ?? album;
  const tracks = currentAlbum.tracks ?? [];
  const tracksAbsent = albumTracksAbsent(currentAlbum);
  const showTracks = expanded && !tracksAbsent;
  const availableTracks = tracks.filter((track) => track.has_file).length;
  const complete = tracks.length > 0 && availableTracks === tracks.length;

  const toggleMonitoring = () => {
    const monitored = !album.monitored;
    setAlbumMonitored.mutate(
      { id: album.id, monitored },
      {
        onSuccess: (updatedAlbum) => {
          onMonitoredChange(updatedAlbum, monitored);
          toast.success(
            monitored
              ? `Monitoring ${album.title}`
              : `Unmonitored ${album.title}`,
          );
        },
        onError: (error) =>
          toast.error(
            error instanceof Error
              ? error.message
              : "Could not update album monitoring",
          ),
      },
    );
  };

  const search = () => {
    searchAlbum.mutate(album.id, {
      onSuccess: (result) =>
        toast.success(
          `Grabbed "${result.title}" (${result.quality_name || "unknown"})`,
        ),
      onError: (error) =>
        toast.error(error instanceof Error ? error.message : "Search failed"),
    });
  };

  return (
    <section className="overflow-hidden rounded-md border border-border">
      <div className="flex items-center gap-3 p-3">
        <MusicArtwork
          src={currentAlbum.cover_art_url}
          alt=""
          className="flex h-12 w-12 shrink-0 items-center justify-center rounded"
          fallback={<Disc3 className="h-5 w-5 text-muted-foreground" />}
        />
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <h4 className="truncate text-sm font-medium">{album.title}</h4>
            <MusicRating rating={album.rating} votes={album.rating_votes} />
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {[album.album_type, album.release_date?.slice(0, 4)]
              .filter(Boolean)
              .join(" · ") || "Album"}
            {tracks.length > 0 &&
              ` · ${availableTracks}/${tracks.length} tracks`}
          </p>
        </div>
        {tracks.length > 0 && (
          <Badge
            variant={complete ? "secondary" : "outline"}
            className="hidden shrink-0 text-[10px] sm:inline-flex"
          >
            {complete ? "Complete" : "Missing"}
          </Badge>
        )}
        <Button
          size="icon"
          variant="ghost"
          aria-label={`Album details for ${album.title}`}
          title="Album details"
          onClick={() => onDetails(currentAlbum)}
        >
          <Info className="h-4 w-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          aria-label={album.monitored ? "Unmonitor album" : "Monitor album"}
          title={album.monitored ? "Unmonitor album" : "Monitor album"}
          disabled={setAlbumMonitored.isPending}
          onClick={toggleMonitoring}
        >
          {album.monitored ? (
            <BookmarkCheck className="h-4 w-4" />
          ) : (
            <Bookmark className="h-4 w-4" />
          )}
        </Button>
        <Button
          size="icon"
          variant="ghost"
          aria-label="Search album"
          title="Search album"
          disabled={!album.monitored || searchAlbum.isPending}
          onClick={search}
        >
          {searchAlbum.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Search className="h-4 w-4" />
          )}
        </Button>
        <Button
          size="icon"
          variant="ghost"
          aria-label="Interactive search"
          title="Interactive search"
          onClick={() => setInteractiveOpen(true)}
        >
          <TextSearch className="h-4 w-4" />
        </Button>
        <Button
          size="icon"
          variant="ghost"
          aria-label={
            tracksAbsent
              ? "No tracks available"
              : showTracks
                ? "Hide tracks"
                : "Show tracks"
          }
          aria-expanded={showTracks}
          disabled={tracksAbsent}
          title={
            tracksAbsent
              ? "No track list is available for this album"
              : showTracks
                ? "Hide tracks"
                : "Show tracks"
          }
          onClick={() => setExpanded((open) => !open)}
        >
          {showTracks ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </Button>
      </div>

      {tracksAbsent && (
        <p role="status" className="px-3 pb-3 text-xs text-muted-foreground">
          No track list is available for this album.
        </p>
      )}
      {showTracks && (
        <div className="border-t border-border/60 px-3 py-3">
          {isFetching ? (
            <div
              role="status"
              className="flex items-center gap-2 py-2 text-sm text-muted-foreground"
            >
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading track availability…
            </div>
          ) : isError || tracks.length === 0 ? (
            <div
              role="alert"
              className="flex items-center justify-between gap-3 py-1 text-sm text-muted-foreground"
            >
              <span>
                {isError
                  ? "Could not load tracks."
                  : "Track metadata could not be loaded."}
              </span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void refetch()}
              >
                Retry
              </Button>
            </div>
          ) : (
            <ol className="space-y-1">
              {tracks.map((track) => (
                <li
                  key={track.id}
                  className="flex items-center gap-3 rounded px-2 py-1.5 text-sm hover:bg-muted/40"
                >
                  <span className="w-8 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                    {track.disc_number > 1
                      ? `${track.disc_number}.${track.track_number}`
                      : track.track_number}
                  </span>
                  <span className="min-w-0 flex-1 truncate">
                    {track.title || "Untitled track"}
                  </span>
                  {track.artist_name && (
                    <span className="hidden max-w-32 truncate text-xs text-muted-foreground sm:block">
                      {track.artist_name}
                    </span>
                  )}
                  {trackDuration(track.duration_ms) && (
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                      {trackDuration(track.duration_ms)}
                    </span>
                  )}
                  <Badge
                    variant={track.has_file ? "secondary" : "outline"}
                    className="shrink-0 text-[10px]"
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
                    title={
                      track.monitored ? "Unmonitor track" : "Monitor track"
                    }
                    disabled={setTrackMonitored.isPending}
                    onClick={() =>
                      setTrackMonitored.mutate(
                        {
                          albumId: album.id,
                          trackId: track.id,
                          monitored: !track.monitored,
                          artistId,
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
                      <Bookmark className="h-3.5 w-3.5 text-muted-foreground" />
                    )}
                  </Button>
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
      {interactiveOpen && (
        <AlbumSearchDialog
          albumId={album.id}
          albumTitle={album.title}
          open={interactiveOpen}
          onOpenChange={setInteractiveOpen}
        />
      )}
    </section>
  );
}

export function ArtistDetailSheet({
  artist: selectedArtist,
  open,
  onOpenChange,
  onDelete,
}: {
  artist: Artist | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: (id: string) => void;
}) {
  const {
    data: artistData,
    isLoading,
    isError,
    refetch,
  } = useArtist(open ? (selectedArtist?.id ?? undefined) : undefined);
  const artist = artistData ?? selectedArtist;
  const setMonitoring = useSetArtistMonitoring();
  const refreshArtist = useRefreshArtist();
  const rescanArtist = useRescanArtist();
  const [editOpen, setEditOpen] = useState(false);
  const [detailAlbum, setDetailAlbum] = useState<Album | null>(null);
  const [albumDetailOpen, setAlbumDetailOpen] = useState(false);

  if (!artist) {
    return null;
  }

  const albums = artistData?.albums ?? [];
  const stats = artistData?.stats ?? artist.stats;
  const monitored = artist.monitoring_status === "monitored";
  const albumTypes = [
    ...new Set(albums.map((album) => album.album_type || "Other")),
  ];
  const typeOrder = ["Album", "EP", "Single"];
  const orderedTypes = [
    ...typeOrder.filter((type) => albumTypes.includes(type)),
    ...albumTypes.filter((type) => !typeOrder.includes(type)),
  ];

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full overflow-y-auto p-0 sm:max-w-2xl"
      >
        <SheetHeader className="sr-only">
          <SheetTitle>{artist.name}</SheetTitle>
          <SheetDescription>
            Artist library details for {artist.name}
          </SheetDescription>
        </SheetHeader>

        <div className="relative h-64 overflow-hidden bg-muted">
          {artist.background_url ? (
            <MusicArtwork
              src={artist.background_url}
              alt=""
              className="h-full w-full"
              fallback={null}
            />
          ) : artist.image_url ? (
            <MusicArtwork
              src={artist.image_url}
              alt=""
              className="h-full w-full scale-110 opacity-30 blur-md"
              fallback={null}
            />
          ) : null}
          <div className="absolute inset-0 bg-gradient-to-t from-background via-background/70 to-black/20" />
          <div className="absolute bottom-[-38px] left-6 z-10">
            <MusicArtwork
              src={artist.image_url}
              alt={artist.name}
              loading="eager"
              className="flex h-28 w-28 items-center justify-center overflow-hidden rounded-lg border-4 border-background bg-muted shadow-xl"
              fallback={
                <Music className="h-10 w-10 text-muted-foreground/50" />
              }
            />
          </div>
          <div className="absolute bottom-4 left-36 right-16 z-10 min-w-0">
            {artist.logo_url ? (
              <img
                src={artist.logo_url}
                alt={artist.name}
                className="max-h-14 max-w-full object-contain object-left"
              />
            ) : (
              <h2 className="truncate text-2xl font-bold text-white drop-shadow-lg">
                {artist.name}
              </h2>
            )}
          </div>
          <Button
            size="icon"
            variant="ghost"
            className="absolute right-14 top-4 z-20 rounded-full bg-black/40 text-white hover:bg-black/60 hover:text-white"
            aria-label={monitored ? "Unmonitor artist" : "Monitor artist"}
            title={
              monitored ? "Monitored — click to unmonitor" : "Click to monitor"
            }
            disabled={setMonitoring.isPending}
            onClick={() =>
              setMonitoring.mutate(
                {
                  id: artist.id,
                  status: monitored ? "unmonitored" : "monitored",
                },
                {
                  onSuccess: () =>
                    toast.success(
                      monitored ? "Artist unmonitored" : "Artist monitored",
                    ),
                  onError: (error) =>
                    toast.error(
                      error instanceof Error
                        ? error.message
                        : "Could not update artist monitoring",
                    ),
                },
              )
            }
          >
            {monitored ? (
              <BookmarkCheck className="h-5 w-5" />
            ) : (
              <Bookmark className="h-5 w-5" />
            )}
          </Button>
        </div>

        <div className="h-12" />
        <div className="space-y-5 px-6 pb-6">
          <div
            role="group"
            aria-label="Artist actions"
            className="flex flex-wrap items-center gap-1.5"
          >
            <Button
              size="sm"
              variant="outline"
              className="h-8 gap-1.5 text-xs"
              onClick={() => setEditOpen(true)}
            >
              <Pencil className="h-3.5 w-3.5" />
              Edit
            </Button>
            <div aria-hidden className="mx-0.5 h-5 w-px bg-border" />
            <Button
              size="icon"
              variant="ghost"
              className="h-8 w-8"
              aria-label="Refresh metadata"
              title="Refresh metadata"
              disabled={refreshArtist.isPending}
              onClick={() =>
                refreshArtist.mutate(artist.id, {
                  onSuccess: () => toast.success("Artist metadata refreshed"),
                  onError: (error) =>
                    toast.error(
                      error instanceof Error
                        ? error.message
                        : "Could not refresh artist",
                    ),
                })
              }
            >
              {refreshArtist.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" />
              )}
            </Button>
            <Button
              size="icon"
              variant="ghost"
              className="h-8 w-8"
              aria-label="Rescan folder"
              title="Rescan library folder for new files"
              disabled={rescanArtist.isPending || !artist.library_id}
              onClick={() =>
                artist.library_id &&
                rescanArtist.mutate(
                  { id: artist.id, libraryId: artist.library_id },
                  {
                    onSuccess: () =>
                      toast.success("Artist folder rescan started"),
                    onError: (error) =>
                      toast.error(
                        error instanceof Error
                          ? error.message
                          : "Could not rescan artist",
                      ),
                  },
                )
              }
            >
              {rescanArtist.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" />
              )}
            </Button>
            <Link
              to="/music/$artistId"
              params={{ artistId: artist.id }}
              onClick={() => onOpenChange(false)}
            >
              <Button size="sm" variant="ghost" className="h-8 gap-1.5 text-xs">
                Full page
                <ArrowUpRight className="ml-1 h-4 w-4" />
              </Button>
            </Link>
            <Button
              size="icon"
              variant="ghost"
              aria-label="Delete artist"
              title="Delete artist"
              className="ml-auto h-8 w-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
              onClick={() => {
                onDelete(artist.id);
                onOpenChange(false);
              }}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>

          <ArtistFacts artist={artist} />
          <ArtistBiography key={artist.id} overview={artist.overview} />
          <MusicArtistMetadata artist={artist} />
          <MusicExternalLinks links={artist.links} />

          <section className="space-y-3 border-t border-border/60 pt-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div>
                <h3 className="text-sm font-semibold">Albums and tracks</h3>
                {stats && (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {stats.albumCount} albums · {stats.trackFileCount}/
                    {stats.trackCount} tracks available
                    {stats.missingTrackCount > 0 &&
                      ` · ${stats.missingTrackCount} missing`}
                  </p>
                )}
              </div>
              <Badge variant={monitored ? "secondary" : "outline"}>
                {monitored ? "Artist monitored" : "Artist unmonitored"}
              </Badge>
            </div>

            {isLoading ? (
              <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading albums…
              </div>
            ) : isError && !artistData ? (
              <div className="flex items-center justify-between gap-3 py-4 text-sm text-muted-foreground">
                <span>Could not load the artist catalogue.</span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void refetch()}
                >
                  Retry
                </Button>
              </div>
            ) : albums.length === 0 ? (
              <p className="py-4 text-sm text-muted-foreground">
                No albums are available for this artist yet.
              </p>
            ) : (
              orderedTypes.map((type) => (
                <div key={type} className="space-y-2">
                  <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    {type}s
                  </h4>
                  {albums
                    .filter((album) => (album.album_type || "Other") === type)
                    .slice()
                    .sort((a, b) =>
                      (b.release_date || "").localeCompare(
                        a.release_date || "",
                      ),
                    )
                    .map((album) => (
                      <AlbumSection
                        key={album.id}
                        album={album}
                        artistId={artist.id}
                        onMonitoredChange={() => void refetch()}
                        onDetails={(selected) => {
                          setDetailAlbum(selected);
                          setAlbumDetailOpen(true);
                        }}
                      />
                    ))}
                </div>
              ))
            )}
          </section>

          <div>
            <DrawerSection
              title={stats ? `Files (${stats.trackFileCount})` : "Files"}
              icon={FileAudio}
            >
              {(sectionOpen) => (
                <ArtistFilesList artistId={artist.id} enabled={sectionOpen} />
              )}
            </DrawerSection>
            <DrawerSection title="History" icon={History}>
              {(sectionOpen) => (
                <ArtistHistoryList artistId={artist.id} enabled={sectionOpen} />
              )}
            </DrawerSection>
          </div>
        </div>
        {editOpen && (
          <ArtistEditDialog
            artist={artist}
            open={editOpen}
            onOpenChange={setEditOpen}
          />
        )}
      </SheetContent>
      <AlbumDetailSheet
        album={detailAlbum}
        open={albumDetailOpen}
        onOpenChange={setAlbumDetailOpen}
      />
    </Sheet>
  );
}
