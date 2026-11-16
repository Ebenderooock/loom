import { useState } from "react";
import { useParams, Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  ArrowLeft,
  Music,
  Search,
  Trash2,
  Loader2,
  FolderSync,
  RefreshCw,
  Bookmark,
  BookmarkCheck,
  Pencil,
} from "lucide-react";
import { toast } from "sonner";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { AlbumSection } from "@/components/music/artist-detail-sheet";
import { AlbumDetailSheet } from "@/components/music/album-detail-sheet";
import { ArtistEditDialog } from "@/components/music/artist-edit-dialog";
import {
  ArtistFacts,
  ArtistBiography,
  MusicArtistMetadata,
  MusicArtwork,
  MusicExternalLinks,
} from "@/components/music/artist-details";
import {
  useArtist,
  useSetArtistMonitoring,
  useSetAlbumMonitored,
  useSearchAlbum,
  useDeleteArtist,
  useRescanArtist,
  useRefreshArtist,
  type Album,
} from "@/lib/music-api";

export function MusicArtistPage() {
  const { artistId } = useParams({ from: "/music/$artistId" });
  const { data: artist, isLoading, refetch } = useArtist(artistId);
  const setMonitoring = useSetArtistMonitoring();
  const setAlbumMonitored = useSetAlbumMonitored();
  const searchAlbum = useSearchAlbum();
  const deleteArtist = useDeleteArtist();
  const rescanArtist = useRescanArtist();
  const refreshArtist = useRefreshArtist();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [detailAlbum, setDetailAlbum] = useState<Album | null>(null);

  useSetPageHeader(artist?.name ?? "Artist");

  if (isLoading) {
    return (
      <div className="space-y-4 px-6 pb-6 pt-2">
        <Skeleton className="h-32 w-full rounded-lg" />
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
      </div>
    );
  }

  if (!artist) {
    return (
      <div className="px-6 py-12 text-center text-sm text-muted-foreground">
        Artist not found.{" "}
        <Link to="/music" className="text-accent underline">
          Back to Music
        </Link>
      </div>
    );
  }

  const albums = artist.albums ?? [];
  const monitored = artist.monitoring_status === "monitored";
  const stats = artist.stats;

  // Group albums by type
  const albumsByType: Record<string, typeof albums> = {};
  for (const album of albums) {
    const type = album.album_type || "Other";
    if (!albumsByType[type]) {
      albumsByType[type] = [];
    }
    albumsByType[type].push(album);
  }

  // Preferred order for album sections
  const typeOrder = ["Album", "Single", "EP", "Compilation", "Mixtape"];
  const orderedTypes = [
    ...typeOrder.filter((t) => albumsByType[t]),
    ...Object.keys(albumsByType).filter((t) => !typeOrder.includes(t)),
  ];

  const bulkMonitor = async (value: boolean) => {
    setBulkBusy(true);
    try {
      await Promise.all(
        albums
          .filter((a) => a.monitored !== value)
          .map((a) =>
            setAlbumMonitored.mutateAsync({ id: a.id, monitored: value }),
          ),
      );
      toast.success(value ? "Monitoring all albums" : "Unmonitored all albums");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Bulk update failed");
    } finally {
      setBulkBusy(false);
    }
  };

  const searchAllMissing = async () => {
    const missing = albums.filter((a) => {
      const tracks = a.tracks ?? [];
      const present = tracks.filter((t) => t.has_file).length;
      return a.monitored && (tracks.length === 0 || present < tracks.length);
    });
    if (missing.length === 0) {
      toast.info("No monitored albums are missing tracks");
      return;
    }
    setBulkBusy(true);
    let grabbed = 0;
    for (const a of missing) {
      try {
        await searchAlbum.mutateAsync(a.id);
        grabbed++;
      } catch {
        /* skip albums with no results */
      }
    }
    setBulkBusy(false);
    toast.success(`Searched ${missing.length} albums, grabbed ${grabbed}`);
  };

  return (
    <div className="px-6 pb-6 pt-2">
      <Link
        to="/music"
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Music
      </Link>

      {artist.background_url && (
        <MusicArtwork
          src={artist.background_url}
          alt=""
          className="relative mb-[-4rem] h-48 w-full rounded-lg sm:h-64"
          imgClassName="opacity-60"
          fallback={null}
        />
      )}
      <div className="relative mb-6 flex gap-4 px-1">
        <MusicArtwork
          src={artist.image_url}
          alt={artist.name}
          loading="eager"
          className="flex h-32 w-32 shrink-0 items-center justify-center rounded-lg border border-border shadow-md"
          fallback={<Music className="h-12 w-12 text-muted-foreground" />}
        />
        <div className="flex flex-1 flex-col">
          {artist.logo_url ? (
            <h1 className="text-2xl font-semibold">
              <img
                src={artist.logo_url}
                alt={artist.name}
                className="max-h-16 max-w-xs object-contain"
              />
            </h1>
          ) : (
            <h1 className="text-2xl font-semibold">{artist.name}</h1>
          )}
          <ArtistFacts artist={artist} />
          <ArtistBiography key={artist.id} overview={artist.overview} />
          <MusicArtistMetadata artist={artist} />
          <MusicExternalLinks links={artist.links} className="mt-3" />
          <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
            {stats && (
              <span className="self-center">
                {stats.albumCount} albums · {stats.trackFileCount}/
                {stats.trackCount} tracks
              </span>
            )}
          </div>
          <div
            role="group"
            aria-label="Artist actions"
            className="mt-auto flex flex-wrap items-center gap-1.5 pt-3"
          >
            <Button
              size="sm"
              variant="outline"
              className="h-8 gap-1.5 text-xs"
              disabled={bulkBusy}
              onClick={searchAllMissing}
            >
              {bulkBusy ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Search className="h-3.5 w-3.5" />
              )}
              Search
            </Button>
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
              aria-label={monitored ? "Unmonitor artist" : "Monitor artist"}
              title={monitored ? "Unmonitor artist" : "Monitor artist"}
              disabled={setMonitoring.isPending}
              onClick={() =>
                setMonitoring.mutate({
                  id: artist.id,
                  status: monitored ? "unmonitored" : "monitored",
                })
              }
            >
              {monitored ? (
                <BookmarkCheck className="h-3.5 w-3.5" />
              ) : (
                <Bookmark className="h-3.5 w-3.5" />
              )}
            </Button>
            <Button
              size="icon"
              variant="ghost"
              className="h-8 w-8"
              aria-label="Rescan folder"
              title="Rescan library folder for new files"
              disabled={rescanArtist.isPending || !artist.library_id}
              onClick={() => {
                if (!artist.library_id) {
                  toast.error("Library ID not found");
                  return;
                }
                rescanArtist.mutate(
                  { id: artist.id, libraryId: artist.library_id },
                  {
                    onSuccess: () => {
                      toast.success("Artist folder rescan started");
                    },
                    onError: (e) => {
                      toast.error(
                        e instanceof Error ? e.message : "Rescan failed",
                      );
                    },
                  },
                );
              }}
            >
              {rescanArtist.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <FolderSync className="h-3.5 w-3.5" />
              )}
            </Button>
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
                  onError: (e) =>
                    toast.error(
                      e instanceof Error ? e.message : "Refresh failed",
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
              className="ml-auto h-8 w-8 text-destructive hover:bg-destructive/10 hover:text-destructive"
              aria-label="Remove artist"
              title="Remove artist"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      </div>

      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-muted-foreground">Albums</h2>
        {albums.length > 0 && (
          <div className="flex items-center gap-1.5">
            <Button
              size="sm"
              variant="outline"
              disabled={bulkBusy}
              onClick={() => bulkMonitor(true)}
            >
              Monitor all
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={bulkBusy}
              onClick={() => bulkMonitor(false)}
            >
              Unmonitor all
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={bulkBusy}
              onClick={searchAllMissing}
            >
              {bulkBusy ? (
                <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
              ) : (
                <Search className="mr-1.5 h-4 w-4" />
              )}
              Search missing
            </Button>
          </div>
        )}
      </div>
      {albums.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">
          No albums found for this artist.
        </p>
      ) : (
        <div className="space-y-6">
          {orderedTypes.map((type) => (
            <div key={type}>
              <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {type}s
              </h3>
              <div className="space-y-2">
                {(albumsByType[type] || [])
                  .slice()
                  .sort((a, b) =>
                    (b.release_date || "").localeCompare(a.release_date || ""),
                  )
                  .map((al) => (
                    <AlbumSection
                      key={al.id}
                      album={al}
                      artistId={artist.id}
                      onMonitoredChange={() => void refetch()}
                      onDetails={setDetailAlbum}
                    />
                  ))}
              </div>
            </div>
          ))}
        </div>
      )}

      <AlbumDetailSheet
        album={detailAlbum}
        open={!!detailAlbum}
        onOpenChange={(open) => {
          if (!open) setDetailAlbum(null);
        }}
      />
      <ArtistEditDialog
        artist={artist}
        open={editOpen}
        onOpenChange={setEditOpen}
      />
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Remove {artist.name}</DialogTitle>
            <DialogDescription>
              This removes the artist from your library. Media files on disk are
              not deleted.
            </DialogDescription>
          </DialogHeader>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={deleteArtist.isPending}
              onClick={async () => {
                try {
                  await deleteArtist.mutateAsync(artist.id);
                  toast.success(`Removed ${artist.name}`);
                  window.history.back();
                } catch (e) {
                  toast.error(
                    e instanceof Error ? e.message : "Failed to remove",
                  );
                }
              }}
            >
              Remove
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
