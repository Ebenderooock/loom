import { useMemo, useState } from "react";
import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { EmptyState } from "@/components/ui/empty-state";
import { Plus, Search, Music, Disc3 } from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useArtists, type Artist } from "@/lib/music-api";
import { AddArtistDialog } from "@/components/music/add-artist-dialog";

function ArtistCard({ artist }: { artist: Artist }) {
  const stats = artist.stats;
  const missing = stats?.missingTrackCount ?? 0;
  return (
    <Link
      to="/music/$artistId"
      params={{ artistId: artist.id }}
      className="group flex flex-col overflow-hidden rounded-lg border border-border bg-card transition-colors hover:border-accent/50"
    >
      <div className="relative aspect-square overflow-hidden bg-muted">
        {artist.image_url ? (
          <img
            src={artist.image_url}
            alt={artist.name}
            loading="lazy"
            className="h-full w-full object-cover transition-transform group-hover:scale-105"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            <Music className="h-10 w-10" />
          </div>
        )}
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
          {stats ? `${stats.albumCount} albums` : "—"}
        </span>
        {missing > 0 && (
          <Badge variant="destructive" className="mt-1 w-fit text-[10px]">
            {missing} missing
          </Badge>
        )}
      </div>
    </Link>
  );
}

export function MusicPage() {
  const { data: artists = [], isLoading } = useArtists();
  const [filter, setFilter] = useState("");
  const [addOpen, setAddOpen] = useState(false);

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

  return (
    <div className="px-6 pb-6 pt-2">
      <div className="mb-4 flex items-center gap-3">
        <div className="relative max-w-xs flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter artists…"
            className="pl-9"
          />
        </div>
        <div className="ml-auto">
          <Button onClick={() => setAddOpen(true)}>
            <Plus className="mr-1.5 h-4 w-4" />
            Add Artist
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-[3/4] rounded-lg" />
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
        <p className="py-12 text-center text-sm text-muted-foreground">
          No artists match “{filter}”.
        </p>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {filtered.map((a) => (
            <ArtistCard key={a.id} artist={a} />
          ))}
        </div>
      )}

      <AddArtistDialog open={addOpen} onOpenChange={setAddOpen} />
    </div>
  );
}
