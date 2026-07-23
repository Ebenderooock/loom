import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Search, Check } from "lucide-react";
import { toast } from "sonner";
import { useLibraries } from "@/lib/libraries-api";
import {
  lookupArtists,
  useAddArtist,
  useAudioQualityProfiles,
  useMetadataProfiles,
  type ArtistLookupResult,
} from "@/lib/music-api";

export function AddArtistDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<ArtistLookupResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [selected, setSelected] = useState<ArtistLookupResult | null>(null);

  const { data: allLibraries = [] } = useLibraries();
  const libraries = allLibraries.filter((l) => l.media_type === "music");
  const { data: qualityProfiles = [] } = useAudioQualityProfiles();
  const { data: metadataProfiles = [] } = useMetadataProfiles();
  const addArtist = useAddArtist();

  const [libraryId, setLibraryId] = useState("");
  const [qualityProfileId, setQualityProfileId] = useState("");
  const [metadataProfileId, setMetadataProfileId] = useState("");
  const [monitored, setMonitored] = useState(true);
  const [searchOnAdd, setSearchOnAdd] = useState(true);

  // Seed defaults once data arrives.
  useEffect(() => {
    if (!libraryId && libraries[0]) setLibraryId(libraries[0].id);
  }, [libraries, libraryId]);
  useEffect(() => {
    if (!qualityProfileId && qualityProfiles[0])
      setQualityProfileId(qualityProfiles[0].id);
  }, [qualityProfiles, qualityProfileId]);
  useEffect(() => {
    if (!metadataProfileId && metadataProfiles[0])
      setMetadataProfileId(metadataProfiles[0].id);
  }, [metadataProfiles, metadataProfileId]);

  // Reset transient state when closed.
  useEffect(() => {
    if (!open) {
      setQuery("");
      setResults([]);
      setSelected(null);
    }
  }, [open]);

  // Debounced lookup.
  useEffect(() => {
    if (!query.trim()) {
      setResults([]);
      return;
    }
    const ctrl = new AbortController();
    setSearching(true);
    const t = setTimeout(() => {
      lookupArtists(query, ctrl.signal)
        .then((r) => setResults(r))
        .catch(() => {
          /* ignore aborted/failed */
        })
        .finally(() => setSearching(false));
    }, 350);
    return () => {
      clearTimeout(t);
      ctrl.abort();
    };
  }, [query]);

  const canAdd =
    !!selected && !!libraryId && !!qualityProfileId && !addArtist.isPending;

  const handleAdd = async () => {
    if (!selected) return;
    try {
      await addArtist.mutateAsync({
        mbid: selected.mbid,
        libraryId,
        qualityProfileId,
        metadataProfileId: metadataProfileId || undefined,
        monitoringStatus: monitored ? "monitored" : "unmonitored",
        search: searchOnAdd,
      });
      toast.success(`Added ${selected.name}`);
      onOpenChange(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to add artist");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add Artist</DialogTitle>
          <DialogDescription>
            Search MusicBrainz for an artist to add to your library.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search artists…"
              className="pl-9"
            />
          </div>

          <div className="max-h-64 space-y-1 overflow-y-auto">
            {searching && results.length === 0 && (
              <>
                <Skeleton className="h-12 w-full" />
                <Skeleton className="h-12 w-full" />
              </>
            )}
            {!searching && query && results.length === 0 && (
              <p className="py-4 text-center text-sm text-muted-foreground">
                No artists found.
              </p>
            )}
            {results.map((r) => {
              const isSelected = selected?.mbid === r.mbid;
              return (
                <button
                  key={r.mbid}
                  type="button"
                  disabled={r.already_added}
                  onClick={() => setSelected(r)}
                  className={`flex w-full items-center justify-between gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors ${
                    isSelected
                      ? "border-accent bg-accent/10"
                      : "border-border hover:bg-accent/5"
                  } ${r.already_added ? "opacity-50" : ""}`}
                >
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{r.name}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {[r.type, r.disambiguation, r.country]
                        .filter(Boolean)
                        .join(" · ")}
                    </span>
                  </span>
                  {r.already_added ? (
                    <Badge variant="secondary">Added</Badge>
                  ) : isSelected ? (
                    <Check className="h-4 w-4 text-accent" />
                  ) : null}
                </button>
              );
            })}
          </div>

          {selected && (
            <div className="space-y-3 border-t pt-4">
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label>Library</Label>
                  <Select value={libraryId} onValueChange={setLibraryId}>
                    <SelectTrigger>
                      <SelectValue placeholder="Select library" />
                    </SelectTrigger>
                    <SelectContent>
                      {libraries.map((l) => (
                        <SelectItem key={l.id} value={l.id}>
                          {l.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label>Quality Profile</Label>
                  <Select
                    value={qualityProfileId}
                    onValueChange={setQualityProfileId}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select profile" />
                    </SelectTrigger>
                    <SelectContent>
                      {qualityProfiles.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label>Metadata Profile</Label>
                  <Select
                    value={metadataProfileId}
                    onValueChange={setMetadataProfileId}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select profile" />
                    </SelectTrigger>
                    <SelectContent>
                      {metadataProfiles.map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="flex flex-wrap gap-4">
                <div className="flex items-center gap-2 text-sm">
                  <Checkbox
                    id="artist-monitored"
                    checked={monitored}
                    onCheckedChange={(v) => setMonitored(v === true)}
                  />
                  <Label htmlFor="artist-monitored">Monitored</Label>
                </div>
                <div className="flex items-center gap-2 text-sm">
                  <Checkbox
                    id="artist-search-on-add"
                    checked={searchOnAdd}
                    onCheckedChange={(v) => setSearchOnAdd(v === true)}
                  />
                  <Label htmlFor="artist-search-on-add">
                    Search for albums on add
                  </Label>
                </div>
              </div>
              {libraries.length === 0 && (
                <p className="text-xs text-destructive">
                  No music library configured. Add one under Settings →
                  Libraries.
                </p>
              )}
            </div>
          )}
        </div>

        <div className="mt-2 flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={!canAdd} onClick={handleAdd}>
            {addArtist.isPending ? "Adding…" : "Add Artist"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
