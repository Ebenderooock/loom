import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty-state";
import {
  Plus,
  Search,
  Music,
  Disc3,
  RefreshCw,
  FolderSync,
} from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { apiFetch } from "@/lib/fetch";
import {
  useAlbums,
  useArtists,
  formatAlbumArtistCredits,
  type Album,
  type Artist,
} from "@/lib/music-api";
import { AddArtistDialog } from "@/components/music/add-artist-dialog";
import { MusicArtwork } from "@/components/music/artist-details";
import { ArtistDetailSheet } from "@/components/music/artist-detail-sheet";
import { AddAlbumDialog } from "@/components/music/add-album-dialog";
import { AlbumDetailSheet } from "@/components/music/album-detail-sheet";
import { useLibraries } from "@/lib/libraries-api";
import { toast } from "sonner";

function ArtistCard({
  artist,
  onSelect,
}: {
  artist: Artist;
  onSelect: (artist: Artist) => void;
}) {
  const stats = artist.stats;
  const missing = stats?.missingTrackCount ?? 0;
  return (
    <button
      onClick={() => onSelect(artist)}
      className="group flex flex-col overflow-hidden rounded-lg border border-border bg-card transition-colors hover:border-accent/50 hover:shadow-md"
    >
      <div className="relative aspect-square overflow-hidden bg-gradient-to-br from-muted to-muted-foreground/20">
        <MusicArtwork
          src={artist.image_url}
          alt={artist.name}
          className="h-full w-full bg-transparent"
          imgClassName="transition-transform group-hover:scale-105"
          fallback={
            <div className="flex h-full w-full flex-col items-center justify-center gap-2 text-muted-foreground">
              <Music className="h-12 w-12 opacity-40" />
              <span className="text-xs font-medium opacity-60">No artwork</span>
            </div>
          }
        />
        {artist.monitoring_status !== "monitored" && (
          <Badge
            variant="secondary"
            className="absolute left-2 top-2 text-[10px]"
          >
            Unmonitored
          </Badge>
        )}
      </div>
      <div className="flex flex-1 flex-col gap-1 p-3">
        <span className="truncate text-sm font-medium">{artist.name}</span>
        <span className="text-xs text-muted-foreground">
          {stats
            ? `${stats.albumCount} album${stats.albumCount === 1 ? "" : "s"}`
            : "—"}
        </span>
        {missing > 0 && (
          <Badge variant="destructive" className="mt-1 w-fit text-[10px]">
            {missing} missing
          </Badge>
        )}
      </div>
    </button>
  );
}

function AlbumCard({
  album,
  onSelect,
}: {
  album: Album;
  onSelect: (album: Album) => void;
}) {
  const artists = formatAlbumArtistCredits(album.artists);
  const isVarious =
    (album.artists?.length ?? 0) > 1 ||
    album.secondary_types?.some((type) =>
      type.toLowerCase().includes("compilation"),
    );
  return (
    <button
      type="button"
      onClick={() => onSelect(album)}
      className="group flex flex-col overflow-hidden rounded-lg border border-border bg-card text-left transition-colors hover:border-accent/50 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="relative aspect-square overflow-hidden bg-gradient-to-br from-muted to-muted-foreground/20">
        <MusicArtwork
          src={album.cover_art_url}
          alt={`${album.title} cover`}
          className="h-full w-full bg-transparent"
          imgClassName="transition-transform group-hover:scale-105"
          fallback={
            <div className="flex h-full w-full items-center justify-center text-muted-foreground">
              <Disc3 className="h-12 w-12 opacity-40" />
            </div>
          }
        />
        {isVarious && (
          <Badge
            variant="secondary"
            className="absolute left-2 top-2 text-[10px]"
          >
            Various Artists
          </Badge>
        )}
        {!album.monitored && (
          <Badge
            variant="outline"
            className="absolute bottom-2 left-2 text-[10px]"
          >
            Unmonitored
          </Badge>
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-1 p-3">
        <span className="truncate text-sm font-medium">{album.title}</span>
        <span className="truncate text-xs text-muted-foreground">
          {artists}
        </span>
        <span className="text-[11px] text-muted-foreground">
          {[album.album_type, album.release_date?.slice(0, 4)]
            .filter(Boolean)
            .join(" · ") || "Album"}
        </span>
      </div>
    </button>
  );
}

export function MusicPage() {
  const { data: artists = [], isLoading, refetch } = useArtists();
  const {
    data: albums = [],
    isLoading: albumsLoading,
    isError: albumsError,
    refetch: refetchAlbums,
  } = useAlbums();
  const { data: allLibraries = [] } = useLibraries();
  const libraries = allLibraries.filter(
    (library) => library.media_type === "music",
  );
  const [filter, setFilter] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [selectedArtist, setSelectedArtist] = useState<Artist | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [rescanningLibraries, setRescanningLibraries] = useState(false);
  const [addAlbumOpen, setAddAlbumOpen] = useState(false);
  const [activeView, setActiveView] = useState<"artists" | "albums">("artists");
  const [selectedAlbum, setSelectedAlbum] = useState<Album | null>(null);
  const [albumDetailOpen, setAlbumDetailOpen] = useState(false);

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const list = q
      ? artists.filter((a) => a.name.toLowerCase().includes(q))
      : artists;
    return [...list].sort((a, b) =>
      (a.sort_name || a.name).localeCompare(b.sort_name || b.name),
    );
  }, [artists, filter]);
  const filteredAlbums = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const list = q
      ? albums.filter(
          (album) =>
            album.title.toLowerCase().includes(q) ||
            album.artists?.some((artist) =>
              artist.name.toLowerCase().includes(q),
            ),
        )
      : albums;
    return [...list].sort(
      (a, b) =>
        (b.release_date || "").localeCompare(a.release_date || "") ||
        a.title.localeCompare(b.title),
    );
  }, [albums, filter]);

  const subtitle = artists.length
    ? `${artists.length} artist${artists.length === 1 ? "" : "s"}`
    : undefined;
  useSetPageHeader("Music", subtitle);

  const handleRefreshAll = async () => {
    setRefreshingAll(true);
    try {
      const res = await apiFetch("/api/v1/artists/refresh", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = (await res.json()) as { count?: number };
      toast.success(
        `Refreshing ${data.count ?? artists.length} artist${(data.count ?? artists.length) === 1 ? "" : "s"} in the background`,
      );
      void refetch();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to refresh artists",
      );
    } finally {
      setRefreshingAll(false);
    }
  };

  const handleRescanLibraries = async () => {
    setRescanningLibraries(true);
    try {
      const res = await apiFetch("/api/v1/artists/rescan", { method: "POST" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const data = (await res.json()) as { libraryCount?: number };
      toast.success(
        `Rescanning ${data.libraryCount ?? libraries.length} music librar${(data.libraryCount ?? libraries.length) === 1 ? "y" : "ies"} in the background`,
      );
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to rescan music libraries",
      );
    } finally {
      setRescanningLibraries(false);
    }
  };

  const handleDeleteArtist = async (id: string) => {
    try {
      const res = await apiFetch(`/api/v1/artists/${id}`, { method: "DELETE" });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      toast.success("Artist deleted");
      void refetch();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to delete artist",
      );
    }
  };

  return (
    <div className="px-6 pb-6 pt-2">
      {/* Toolbar */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative max-w-sm flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder={
              activeView === "artists"
                ? "Search artists…"
                : "Search albums or artists…"
            }
            className="pl-9"
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <div className="mr-1 flex rounded-md border border-border p-0.5">
            <Button
              variant={activeView === "artists" ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setActiveView("artists")}
            >
              Artists
            </Button>
            <Button
              variant={activeView === "albums" ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setActiveView("albums")}
            >
              Albums
            </Button>
          </div>
          {activeView === "artists" && (
            <>
              <Button
                variant="outline"
                onClick={handleRefreshAll}
                disabled={refreshingAll}
                size="sm"
              >
                <RefreshCw className="mr-1.5 h-4 w-4" />
                Refresh
              </Button>
              <Button
                variant="outline"
                onClick={handleRescanLibraries}
                disabled={rescanningLibraries}
                size="sm"
              >
                <FolderSync className="mr-1.5 h-4 w-4" />
                Rescan
              </Button>
            </>
          )}
          {activeView === "artists" ? (
            <Button onClick={() => setAddOpen(true)} size="sm">
              <Plus className="mr-1.5 h-4 w-4" />
              Add Artist
            </Button>
          ) : (
            <Button onClick={() => setAddAlbumOpen(true)} size="sm">
              <Plus className="mr-1.5 h-4 w-4" />
              Add Album
            </Button>
          )}
        </div>
      </div>

      {/* Content */}
      {activeView === "artists" && isLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-square rounded-lg" />
          ))}
        </div>
      ) : activeView === "artists" && artists.length === 0 ? (
        <EmptyState
          icon={<Disc3 className="h-10 w-10" />}
          title="No artists yet"
          description="Add an artist to start building your music library."
          action={
            <Button onClick={() => setAddOpen(true)}>
              <Plus className="mr-1.5 h-4 w-4" />
              Add Artist
            </Button>
          }
        />
      ) : activeView === "artists" && filtered.length === 0 ? (
        <p className="py-12 text-center text-sm text-muted-foreground">
          No artists match "{filter}".
        </p>
      ) : activeView === "artists" ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {filtered.map((a) => (
            <ArtistCard
              key={a.id}
              artist={a}
              onSelect={(artist) => {
                setSelectedArtist(artist);
                setDetailOpen(true);
              }}
            />
          ))}
        </div>
      ) : albumsLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-square rounded-lg" />
          ))}
        </div>
      ) : albumsError ? (
        <EmptyState
          icon={<Disc3 className="h-10 w-10" />}
          title="Could not load albums"
          description="The album library is unavailable. Please try again."
          action={<Button onClick={() => void refetchAlbums()}>Retry</Button>}
        />
      ) : filteredAlbums.length === 0 ? (
        <EmptyState
          icon={<Disc3 className="h-10 w-10" />}
          title={
            albums.length ? "No albums match your search" : "No albums yet"
          }
          description={
            albums.length
              ? "Try another album title or credited artist."
              : "Add an album to start building your music library."
          }
          action={
            <Button onClick={() => setAddAlbumOpen(true)}>
              <Plus className="mr-1.5 h-4 w-4" />
              Add Album
            </Button>
          }
        />
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {filteredAlbums.map((album) => (
            <AlbumCard
              key={album.id}
              album={album}
              onSelect={(selected) => {
                setSelectedAlbum(selected);
                setAlbumDetailOpen(true);
              }}
            />
          ))}
        </div>
      )}

      <AddArtistDialog open={addOpen} onOpenChange={setAddOpen} />
      <AddAlbumDialog open={addAlbumOpen} onOpenChange={setAddAlbumOpen} />
      <ArtistDetailSheet
        artist={selectedArtist}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        onDelete={handleDeleteArtist}
      />
      <AlbumDetailSheet
        album={selectedAlbum}
        open={albumDetailOpen}
        onOpenChange={setAlbumDetailOpen}
      />
    </div>
  );
}
