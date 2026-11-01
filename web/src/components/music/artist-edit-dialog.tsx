import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  useAudioQualityProfiles,
  useMetadataProfiles,
  useRefreshArtist,
  useUpdateArtist,
  type Artist,
  type UpdateArtistRequest,
} from "@/lib/music-api";

export function ArtistEditDialog({
  artist,
  open,
  onOpenChange,
}: {
  artist: Artist;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { data: qualityProfiles } = useAudioQualityProfiles();
  const { data: metadataProfiles } = useMetadataProfiles();
  const updateArtist = useUpdateArtist();
  const refreshArtist = useRefreshArtist();
  const [monitored, setMonitored] = useState(
    artist.monitoring_status === "monitored",
  );
  const [qualityProfileId, setQualityProfileId] = useState(
    artist.quality_profile_id ?? "",
  );
  const [metadataProfileId, setMetadataProfileId] = useState(
    artist.metadata_profile_id ?? "",
  );

  useEffect(() => {
    if (!open) return;
    setMonitored(artist.monitoring_status === "monitored");
    setQualityProfileId(artist.quality_profile_id ?? "");
    setMetadataProfileId(artist.metadata_profile_id ?? "");
  }, [open, artist]);

  const saving = updateArtist.isPending || refreshArtist.isPending;

  const save = async () => {
    const req: UpdateArtistRequest = {
      monitoringStatus: monitored ? "monitored" : "unmonitored",
      qualityProfileId,
      metadataProfileId,
    };
    const metadataChanged =
      metadataProfileId !== (artist.metadata_profile_id ?? "");
    try {
      await updateArtist.mutateAsync({ id: artist.id, req });
      if (metadataChanged) {
        // The metadata profile decides which releases are in the catalogue,
        // so re-apply it immediately instead of waiting for the next refresh.
        await refreshArtist.mutateAsync(artist.id);
      }
      toast.success(`Updated ${artist.name}`);
      onOpenChange(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Could not update artist",
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Edit {artist.name}</DialogTitle>
          <DialogDescription>
            Change monitoring and the profiles used for this artist.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="artist-edit-monitored">Monitored</Label>
              <p className="text-xs text-muted-foreground">
                Search for missing and newly released albums.
              </p>
            </div>
            <Switch
              id="artist-edit-monitored"
              checked={monitored}
              onCheckedChange={setMonitored}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="artist-edit-quality">Quality profile</Label>
            <Select
              value={qualityProfileId || undefined}
              onValueChange={setQualityProfileId}
            >
              <SelectTrigger id="artist-edit-quality">
                <SelectValue placeholder="Select a quality profile" />
              </SelectTrigger>
              <SelectContent>
                {(qualityProfiles ?? []).map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="artist-edit-metadata">Metadata profile</Label>
            <Select
              value={metadataProfileId || undefined}
              onValueChange={setMetadataProfileId}
            >
              <SelectTrigger id="artist-edit-metadata">
                <SelectValue placeholder="Select a metadata profile" />
              </SelectTrigger>
              <SelectContent>
                {(metadataProfiles ?? []).map((profile) => (
                  <SelectItem key={profile.id} value={profile.id}>
                    {profile.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Changing it refreshes the artist so the catalogue matches.
            </p>
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            Cancel
          </Button>
          <Button onClick={() => void save()} disabled={saving}>
            {saving && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
