import {
  TitleDialog,
  TitleSearchDialog,
} from "@/components/titles/title-experience";
import type { TitleDefaults } from "@/lib/title-api";
import type { Library } from "@/lib/libraries-api";
import type { QualityProfile } from "./types";

// Preserve the existing library-page contract while routing both search and
// preselected recommendations through the shared title experience.
export function AddMovieDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingTmdbIds: Set<string>;
  onMovieAdded: () => void;
  initialMovie?: { tmdb_id: string; title: string };
  defaults?: TitleDefaults;
}) {
  if (props.initialMovie?.tmdb_id) {
    return (
      <TitleDialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        identity={{
          media: "movie",
          id: props.initialMovie.tmdb_id,
          title: props.initialMovie.title,
        }}
        initialView="add"
        defaults={props.defaults}
        onAdded={props.onMovieAdded}
      />
    );
  }
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
