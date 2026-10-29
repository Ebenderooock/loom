import { TitleSearchDialog } from "@/components/titles/title-experience";
import type { Library } from "@/lib/libraries-api";
import type { QualityProfile } from "./types";

export function AddSeriesDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingTmdbIds: Set<string>;
  onSeriesAdded: () => void;
}) {
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
