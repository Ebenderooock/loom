import { useState } from "react";
import { useParams, Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
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
  TextSearch,
  Trash2,
  Disc3,
  Loader2,
  FolderSync,
} from "lucide-react";
import { toast } from "sonner";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { AlbumSearchDialog } from "@/components/music/album-search-dialog";
import {
  useArtist,
  useSetArtistMonitoring,
  useSetAlbumMonitored,
  useSearchAlbum,
  useDeleteArtist,
  useRescanArtist,
  type Album,
} from "@/lib/music-api";

function albumYear(a: Album): string {
  return a.release_date ? a.release_date.slice(0, 4) : "—";
}

function AlbumRow({ album }: { album: Album }) {
  const setMonitored = useSetAlbumMonitored();
  const search = useSearchAlbum();
  const [searchOpen, setSearchOpen] = useState(false);
  const [imageLoadError, setImageLoadError] = useState(false);
  const tracks = album.tracks ?? [];
  const present = tracks.filter((t) => t.has_file).length;

  const handleSearch = async () => {
    try {
      const res = await search.mutateAsync(album.id);
      toast.success(
        `Grabbed "${res.title}" (${res.quality_name || "unknown"})`,
      );
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Search failed");
    }
  };

  return (
    <div className="flex items-center gap-3 rounded-md border border-border px-3 py-2.5">
      <Switch
        checked={album.monitored}
        onCheckedChange={(v) =>
          setMonitored.mutate({ id: album.id, monitored: v })
        }
        aria-label="Monitor album"
      />
      <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded bg-muted">
        {album.cover_art_url && !imageLoadError ? (
          <img
            src={album.cover_art_url}
            alt=""
            loading="lazy"
            className="h-full w-full object-cover"
            onError={() => setImageLoadError(true)}
          />
        ) : (
          <Disc3 className="h-5 w-5 text-muted-foreground" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{album.title}</div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {album.release_date && <span>{albumYear(album)}</span>}
          {album.album_type && <span>· {album.album_type}</span>}
          {tracks.length > 0 && (
            <span>
              · {present}/{tracks.length} tracks
            </span>
          )}
        </div>
      </div>
      {tracks.length > 0 && present === tracks.length ? (
        <Badge variant="secondary" className="text-[10px]">
          Complete
        </Badge>
      ) : (
        <Badge variant="outline" className="text-[10px]">
          Missing
        </Badge>
      )}
      <Button
        size="sm"
        variant="ghost"
        disabled={search.isPending}
        onClick={handleSearch}
        aria-label="Auto-search album"
        title="Auto-search and grab best"
      >
        {search.isPending ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <Search className="h-4 w-4" />
        )}
      </Button>
      <Button
        size="sm"
        variant="ghost"
        onClick={() => setSearchOpen(true)}
        aria-label="Interactive search"
        title="Interactive search"
      >
        <TextSearch className="h-4 w-4" />
      </Button>
      <AlbumSearchDialog
        albumId={album.id}
        albumTitle={album.title}
        open={searchOpen}
        onOpenChange={setSearchOpen}
      />
    </div>
  );
}

export function MusicArtistPage() {
  const { artistId } = useParams({ from: "/music/$artistId" });
  const { data: artist, isLoading } = useArtist(artistId);
  const setMonitoring = useSetArtistMonitoring();
  const setAlbumMonitored = useSetAlbumMonitored();
  const searchAlbum = useSearchAlbum();
  const deleteArtist = useDeleteArtist();
  const rescanArtist = useRescanArtist();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [bulkBusy, setBulkBusy] = useState(false);

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

      <div className="mb-6 flex gap-4">
        <div className="flex h-32 w-32 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-muted">
          {artist.image_url ? (
            <img
              src={artist.image_url}
              alt={artist.name}
              className="h-full w-full object-cover"
            />
          ) : (
            <Music className="h-12 w-12 text-muted-foreground" />
          )}
        </div>
        <div className="flex flex-1 flex-col">
          <h1 className="text-2xl font-semibold">{artist.name}</h1>
          {artist.disambiguation && (
            <p className="text-sm text-muted-foreground">
              {artist.disambiguation}
            </p>
          )}
          <div className="mt-2 flex flex-wrap gap-2 text-xs text-muted-foreground">
            {artist.artist_type && (
              <Badge variant="outline">{artist.artist_type}</Badge>
            )}
            {artist.country && (
              <Badge variant="outline">{artist.country}</Badge>
            )}
            {stats && (
              <span className="self-center">
                {stats.albumCount} albums · {stats.trackFileCount}/
                {stats.trackCount} tracks
              </span>
            )}
          </div>
          {artist.overview && (
            <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">
              {artist.overview}
            </p>
          )}
          <div className="mt-auto flex items-center gap-2 pt-3">
            <Button
              size="sm"
              variant={monitored ? "secondary" : "outline"}
              onClick={() =>
                setMonitoring.mutate({
                  id: artist.id,
                  status: monitored ? "unmonitored" : "monitored",
                })
              }
            >
              {monitored ? "Monitored" : "Unmonitored"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={rescanArtist.isPending}
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
                <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
              ) : (
                <FolderSync className="mr-1.5 h-4 w-4" />
              )}
              Rescan Folder
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 className="mr-1.5 h-4 w-4" />
              Remove
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
                    <AlbumRow key={al.id} album={al} />
                  ))}
              </div>
            </div>
          ))}
        </div>
      )}

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
