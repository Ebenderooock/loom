import { useEffect, useState } from "react";
import { AlertCircle, Disc3, Loader2, Search } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLibraries } from "@/lib/libraries-api";
import {
  useAddAlbum,
  useAudioQualityProfiles,
  useLookupAlbums,
  useMetadataProfiles,
  formatAlbumArtistCredits,
  type AlbumLookupResult,
} from "@/lib/music-api";
import { MusicArtwork } from "./artist-details";

export function AddAlbumDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [query, setQuery] = useState("");
  const [settledQuery, setSettledQuery] = useState("");
  const [qualityProfileID, setQualityProfileID] = useState("");
  const [metadataProfileID, setMetadataProfileID] = useState("none");
  const [libraryID, setLibraryID] = useState("");
  const [selected, setSelected] = useState<AlbumLookupResult | null>(null);
  const [monitored, setMonitored] = useState(true);
  const albums = useLookupAlbums(settledQuery);
  const librariesQuery = useLibraries();
  const qualityProfilesQuery = useAudioQualityProfiles();
  const metadataProfilesQuery = useMetadataProfiles();
  const addAlbum = useAddAlbum();

  const musicLibraries = (librariesQuery.data ?? []).filter(
    (library) => library.media_type === "music",
  );
  const qualityProfiles = qualityProfilesQuery.data ?? [];
  const metadataProfiles = metadataProfilesQuery.data ?? [];
  const chosenLibrary = libraryID || musicLibraries[0]?.id || "";
  const profileID =
    qualityProfileID ||
    musicLibraries.find((library) => library.id === chosenLibrary)
      ?.quality_profile_id ||
    qualityProfiles[0]?.id ||
    "";
  const settingsLoading =
    librariesQuery.isLoading ||
    qualityProfilesQuery.isLoading ||
    metadataProfilesQuery.isLoading;
  const settingsError =
    librariesQuery.error ||
    qualityProfilesQuery.error ||
    metadataProfilesQuery.error;

  useEffect(() => {
    const timer = window.setTimeout(() => setSettledQuery(query.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    if (!open) {
      setQuery("");
      setSettledQuery("");
      setSelected(null);
      setMonitored(true);
      setQualityProfileID("");
      setMetadataProfileID("none");
      setLibraryID("");
    }
  }, [open]);

  const submit = async () => {
    if (!selected || !profileID || !chosenLibrary) return;
    try {
      await addAlbum.mutateAsync({
        mbid: selected.mbid,
        qualityProfileId: profileID,
        libraryId: chosenLibrary,
        ...(metadataProfileID !== "none"
          ? { metadataProfileId: metadataProfileID }
          : {}),
        monitored,
      });
      toast.success(`Added “${selected.title}” to your music library`);
      onOpenChange(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Could not add album",
      );
    }
  };

  const resultRows = albums.data ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add album</DialogTitle>
          <DialogDescription>
            Find a release group, then choose where Loom should organize its
            tracks. Albums with multiple credited artists use the Various
            Artists folder.
          </DialogDescription>
        </DialogHeader>

        <label htmlFor="add-album-search" className="sr-only">
          Search albums
        </label>
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            id="add-album-search"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setSelected(null);
            }}
            placeholder="Search album title or artist…"
            className="pl-9"
          />
        </div>

        {selected ? (
          <div className="flex items-center gap-3 rounded-lg border border-accent/50 bg-accent/5 p-3">
            <MusicArtwork
              src={selected.cover_art_url}
              alt=""
              className="flex h-14 w-14 shrink-0 items-center justify-center rounded"
              fallback={<Disc3 className="h-6 w-6 text-muted-foreground" />}
            />
            <div className="min-w-0 flex-1">
              <p className="truncate font-medium">{selected.title}</p>
              <p className="truncate text-sm text-muted-foreground">
                {formatAlbumArtistCredits(selected.artists)}
              </p>
              <div className="mt-1 flex flex-wrap gap-1">
                {selected.album_type && (
                  <Badge variant="outline">{selected.album_type}</Badge>
                )}
                {selected.artists && selected.artists.length > 1 && (
                  <Badge variant="secondary">Various Artists</Badge>
                )}
              </div>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setSelected(null)}
            >
              Change
            </Button>
          </div>
        ) : (
          <div className="max-h-60 space-y-1 overflow-y-auto">
            {query.trim().length < 2 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                Enter at least two characters to search.
              </p>
            ) : albums.isFetching ? (
              <div className="flex items-center justify-center gap-2 py-6 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Searching MusicBrainz…
              </div>
            ) : albums.isError ? (
              <div className="flex items-center justify-center gap-2 py-6 text-sm text-destructive">
                <AlertCircle className="h-4 w-4" />
                {albums.error instanceof Error
                  ? albums.error.message
                  : "Album search failed"}
              </div>
            ) : resultRows.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                No albums found.
              </p>
            ) : (
              resultRows.map((result) => {
                const artists = formatAlbumArtistCredits(result.artists);
                return (
                  <button
                    key={result.mbid}
                    type="button"
                    disabled={result.already_added}
                    onClick={() => setSelected(result)}
                    className="flex w-full items-center gap-3 rounded-md p-2 text-left transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    <MusicArtwork
                      src={result.cover_art_url}
                      alt=""
                      className="flex h-12 w-12 shrink-0 items-center justify-center rounded"
                      fallback={
                        <Disc3 className="h-5 w-5 text-muted-foreground" />
                      }
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        {result.title}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {artists}
                        {result.release_date
                          ? ` · ${result.release_date.slice(0, 4)}`
                          : ""}
                      </span>
                    </span>
                    {result.artists && result.artists.length > 1 && (
                      <Badge
                        variant="outline"
                        className="hidden sm:inline-flex"
                      >
                        Various Artists
                      </Badge>
                    )}
                    {result.already_added && (
                      <Badge variant="secondary">Added</Badge>
                    )}
                  </button>
                );
              })
            )}
          </div>
        )}

        <div className="grid gap-3 border-t border-border/60 pt-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <label htmlFor="add-album-library" className="text-sm font-medium">
              Music library
            </label>
            <Select value={chosenLibrary} onValueChange={setLibraryID}>
              <SelectTrigger id="add-album-library">
                <SelectValue placeholder="Choose a library" />
              </SelectTrigger>
              <SelectContent>
                {musicLibraries.map((library) => (
                  <SelectItem key={library.id} value={library.id}>
                    {library.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="add-album-quality" className="text-sm font-medium">
              Quality profile
            </label>
            <Select value={profileID} onValueChange={setQualityProfileID}>
              <SelectTrigger id="add-album-quality">
                <SelectValue placeholder="Choose a quality profile" />
              </SelectTrigger>
              <SelectContent>
                {qualityProfiles.map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5 sm:col-span-2">
            <label htmlFor="add-album-metadata" className="text-sm font-medium">
              Metadata profile{" "}
              <span className="text-muted-foreground">(optional)</span>
            </label>
            <Select
              value={metadataProfileID}
              onValueChange={setMetadataProfileID}
            >
              <SelectTrigger id="add-album-metadata">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">Use all release types</SelectItem>
                {metadataProfiles.map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-center gap-2 text-sm sm:col-span-2">
            <input
              type="checkbox"
              checked={monitored}
              onChange={(event) => setMonitored(event.target.checked)}
              className="h-4 w-4 accent-accent"
            />
            Monitor this album for releases
          </label>
        </div>

        {settingsError ? (
          <p role="alert" className="text-sm text-destructive">
            Could not load music libraries or profiles:{" "}
            {settingsError instanceof Error
              ? settingsError.message
              : "Please try again."}
          </p>
        ) : !settingsLoading &&
          (!musicLibraries.length || !qualityProfiles.length) ? (
          <p className="text-sm text-destructive">
            Configure a music library and quality profile before adding an
            album.
          </p>
        ) : null}
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={
              !selected ||
              !chosenLibrary ||
              !profileID ||
              settingsLoading ||
              !!settingsError ||
              addAlbum.isPending ||
              selected.already_added
            }
            onClick={() => void submit()}
          >
            {addAlbum.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            Add album
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
