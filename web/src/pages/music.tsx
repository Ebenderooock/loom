import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty-state";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Plus,
  Search,
  Music,
  Disc3,
  RefreshCw,
  FolderSync,
  Edit,
  Trash2,
} from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { apiFetch } from "@/lib/fetch";
import { useArtists, type Artist } from "@/lib/music-api";
import { AddArtistDialog } from "@/components/music/add-artist-dialog";
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
      className="group border-border bg-card hover:border-accent/50 flex flex-col overflow-hidden rounded-lg border transition-colors hover:shadow-md"
    >
      <div className="from-muted to-muted-foreground/20 relative aspect-square overflow-hidden bg-gradient-to-br">
        {artist.image_url ? (
          <img
            src={artist.image_url}
            alt={artist.name}
            loading="lazy"
            className="h-full w-full object-cover transition-transform group-hover:scale-105"
          />
        ) : (
          <div className="text-muted-foreground flex h-full w-full flex-col items-center justify-center gap-2">
            <Music className="h-12 w-12 opacity-40" />
            <span className="text-xs font-medium opacity-60">No artwork</span>
          </div>
        )}
        {artist.monitoring_status !== "monitored" && (
          <Badge
            variant="secondary"
            className="absolute top-2 left-2 text-[10px]"
          >
            Unmonitored
          </Badge>
        )}
      </div>
      <div className="flex flex-1 flex-col gap-1 p-3">
        <span className="truncate text-sm font-medium">{artist.name}</span>
        <span className="text-muted-foreground text-xs">
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

function ArtistDetailSheet({
  artist,
  open,
  onOpenChange,
  onDelete,
}: {
  artist: Artist | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: (id: string) => void;
}) {
  const stats = artist?.stats;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="overflow-y-auto">
        {artist && (
          <div className="space-y-6">
            <SheetHeader>
              <SheetTitle>{artist.name}</SheetTitle>
            </SheetHeader>

            <div className="space-y-4">
              {/* Artwork */}
              {artist.image_url && (
                <img
                  src={artist.image_url}
                  alt={artist.name}
                  className="w-full rounded-lg"
                />
              )}

              {/* Info section */}
              <div className="space-y-3">
                <div>
                  <h4 className="text-muted-foreground text-xs font-semibold">
                    MONITORING
                  </h4>
                  <p className="mt-1 text-sm capitalize">
                    {artist.monitoring_status}
                  </p>
                </div>

                {stats && (
                  <>
                    <div>
                      <h4 className="text-muted-foreground text-xs font-semibold">
                        ALBUMS
                      </h4>
                      <p className="mt-1 text-sm">{stats.albumCount}</p>
                    </div>
                    <div>
                      <h4 className="text-muted-foreground text-xs font-semibold">
                        TRACKS
                      </h4>
                      <p className="mt-1 text-sm">{stats.trackCount}</p>
                    </div>
                    {stats.missingTrackCount > 0 && (
                      <div>
                        <h4 className="text-muted-foreground text-xs font-semibold">
                          MISSING
                        </h4>
                        <p className="text-destructive mt-1 text-sm">
                          {stats.missingTrackCount} track
                          {stats.missingTrackCount === 1 ? "" : "s"}
                        </p>
                      </div>
                    )}
                  </>
                )}

                {artist.country && (
                  <div>
                    <h4 className="text-muted-foreground text-xs font-semibold">
                      COUNTRY
                    </h4>
                    <p className="mt-1 text-sm">{artist.country}</p>
                  </div>
                )}

                {artist.disambiguation && (
                  <div>
                    <h4 className="text-muted-foreground text-xs font-semibold">
                      DISAMBIGUATION
                    </h4>
                    <p className="mt-1 text-sm">{artist.disambiguation}</p>
                  </div>
                )}
              </div>

              {/* Actions */}
              <div className="space-y-2 border-t pt-4">
                <Link
                  to="/music/$artistId"
                  params={{ artistId: artist.id }}
                  className="block"
                >
                  <Button variant="outline" className="w-full">
                    <Edit className="mr-2 h-4 w-4" />
                    View Details
                  </Button>
                </Link>
                <Button
                  variant="destructive"
                  className="w-full"
                  onClick={() => {
                    onDelete(artist.id);
                    onOpenChange(false);
                  }}
                >
                  <Trash2 className="mr-2 h-4 w-4" />
                  Delete Artist
                </Button>
              </div>
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}

export function MusicPage() {
  const { data: artists = [], isLoading, refetch } = useArtists();
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

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const list = q
      ? artists.filter((a) => a.name.toLowerCase().includes(q))
      : artists;
    return [...list].sort((a, b) =>
      (a.sort_name || a.name).localeCompare(b.sort_name || b.name),
    );
  }, [artists, filter]);

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
    <div className="px-6 pt-2 pb-6">
      {/* Toolbar */}
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative max-w-sm flex-1">
          <Search className="text-muted-foreground absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Search artists…"
            className="pl-9"
          />
        </div>
        <div className="flex flex-wrap gap-2">
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
          <Button onClick={() => setAddOpen(true)} size="sm">
            <Plus className="mr-1.5 h-4 w-4" />
            Add Artist
          </Button>
        </div>
      </div>

      {/* Content */}
      {isLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-square rounded-lg" />
          ))}
        </div>
      ) : artists.length === 0 ? (
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
      ) : filtered.length === 0 ? (
        <p className="text-muted-foreground py-12 text-center text-sm">
          No artists match "{filter}".
        </p>
      ) : (
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
      )}

      <AddArtistDialog open={addOpen} onOpenChange={setAddOpen} />
      <ArtistDetailSheet
        artist={selectedArtist}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        onDelete={handleDeleteArtist}
      />
    </div>
  );
}
