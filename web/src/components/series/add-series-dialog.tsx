import {
  TitleDialog,
  TitleSearchDialog,
} from "@/components/titles/title-experience";
import type { TitleDefaults } from "@/lib/title-api";
import type { Library } from "@/lib/libraries-api";
import type { QualityProfile } from "./types";

// Keep the existing library-page props while sharing the configured add flow.
export function AddSeriesDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingTmdbIds: Set<string>;
  onSeriesAdded: () => void;
  initialSeries?: { tmdbId: string; title: string };
  defaults?: TitleDefaults;
}) {
  if (props.initialSeries?.tmdbId) {
    return (
      <TitleDialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        identity={{
          media: "series",
          id: props.initialSeries.tmdbId,
          title: props.initialSeries.title,
        }}
        initialView="add"
        defaults={props.defaults}
        onAdded={props.onSeriesAdded}
      />
    );
  }
  return (
    <TitleSearchDialog
      media="series"
      open={props.open}
      onOpenChange={props.onOpenChange}
      existingIds={props.existingTmdbIds}
      onAdded={props.onSeriesAdded}
    />
  );
}
