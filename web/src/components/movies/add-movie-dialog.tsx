import { TitleSearchDialog } from "@/components/titles/title-experience";
import type { Library } from "@/lib/libraries-api";
import type { QualityProfile } from "./types";

// Keep the library page's public contract; settings live only in the shared
// title experience and are loaded there for every entry point.
export function AddMovieDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingTmdbIds: Set<string>;
  onMovieAdded: () => void;
}) {
  return (
    <TitleSearchDialog
      media="movie"
      open={props.open}
      onOpenChange={props.onOpenChange}
      existingIds={props.existingTmdbIds}
      onAdded={props.onMovieAdded}
    />
  );
}
