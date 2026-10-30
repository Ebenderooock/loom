import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  TitleContent,
  useTitleDialogFocus,
} from "@/components/titles/title-experience";
import { TitleRating, TitleSynopsis } from "@/components/titles/title-metadata";
import { titleJSON, type TitleIdentity } from "@/lib/title-api";
import type { Library } from "@/lib/libraries-api";
import type { QualityProfile } from "@/components/movies/types";

interface CreditItem {
  tmdb_id: number;
  media_type: "movie" | "tv";
  title: string;
  year?: number;
  poster_path?: string;
  overview?: string;
  release_date?: string;
  rating: number;
  character?: string;
  job?: string;
}

interface PersonFilmography {
  person: {
    name: string;
    biography?: string;
    profile_path?: string;
    known_for?: string;
  };
  credits: CreditItem[];
}

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  personId: number;
  personName: string;
  libraries: Library[];
  qualityProfiles: QualityProfile[];
  existingMovieIds: Set<number>;
  existingSeriesIds: Set<number>;
  onAdded?: () => void;
};

export function PersonDiscoverDialog(props: Props) {
  const focus = useTitleDialogFocus();
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        {...focus}
        className="max-h-[85vh] max-w-3xl overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>{props.personName}</DialogTitle>
          <DialogDescription>
            Explore this person's movies and TV shows.
          </DialogDescription>
        </DialogHeader>
        {props.open && (
          <PersonFilmographyContent key={props.personId} {...props} />
        )}
      </DialogContent>
    </Dialog>
  );
}

function PersonFilmographyContent({
  personId,
  existingMovieIds,
  existingSeriesIds,
  onAdded,
}: Props) {
  const [selection, setSelection] = useState<{
    identity: TitleIdentity;
    view: "details" | "add";
  }>();
  const [added, setAdded] = useState<Set<string>>(new Set());
  const [tab, setTab] = useState("movie");
  const [lastIdentity, setLastIdentity] = useState("");
  const backFocus = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!selection) backFocus.current?.focus();
  }, [selection]);

  const data = useQuery({
    queryKey: ["person-filmography", personId],
    queryFn: ({ signal }) =>
      titleJSON<PersonFilmography>(`/api/v1/discover/people/${personId}`, {
        signal,
      }),
  });

  if (selection) {
    return (
      <TitleContent
        key={`${selection.identity.media}:${selection.identity.id}`}
        identity={selection.identity}
        initialView={selection.view}
        onBack={() => setSelection(undefined)}
        onAdded={() => {
          setAdded(
            (old) =>
              new Set([
                ...old,
                `${selection.identity.media}:${selection.identity.id}`,
              ]),
          );
          onAdded?.();
          setSelection(undefined);
        }}
      />
    );
  }
  if (data.isPending) return <p role="status">Loading filmography…</p>;
  if (data.isError) {
    return (
      <div role="alert">
        <p className="text-sm text-destructive">{data.error.message}</p>
        <Button variant="outline" onClick={() => void data.refetch()}>
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex items-start gap-4">
        {data.data.person.profile_path && (
          <img
            src={data.data.person.profile_path}
            alt=""
            className="w-24 rounded-lg"
          />
        )}
        <div className="space-y-2">
          {data.data.person.known_for && (
            <Badge variant="secondary">{data.data.person.known_for}</Badge>
          )}
          <TitleSynopsis overview={data.data.person.biography} />
        </div>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="movie">Movies</TabsTrigger>
          <TabsTrigger value="tv">TV shows</TabsTrigger>
        </TabsList>
        {(["movie", "tv"] as const).map((type) => (
          <TabsContent key={type} value={type}>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
              {data.data.credits
                .filter((item) => item.media_type === type)
                .map((item) => {
                  const media = type === "tv" ? "series" : "movie";
                  const inLibrary =
                    (type === "movie"
                      ? existingMovieIds
                      : existingSeriesIds
                    ).has(item.tmdb_id) ||
                    added.has(`${media}:${item.tmdb_id}`);
                  const identity: TitleIdentity = {
                    media,
                    id: String(item.tmdb_id),
                    title: item.title,
                    inLibrary,
                  };
                  const key = `${media}:${item.tmdb_id}`;
                  const open = (view: "details" | "add") => {
                    setLastIdentity(key);
                    setSelection({ identity, view });
                  };
                  return (
                    <div
                      key={key}
                      className="overflow-hidden rounded-lg border"
                    >
                      <button
                        ref={key === lastIdentity ? backFocus : undefined}
                        type="button"
                        className="block w-full text-left"
                        aria-label={`View details for ${item.title}`}
                        onClick={() => open("details")}
                      >
                        {item.poster_path ? (
                          <img
                            src={item.poster_path}
                            alt=""
                            className="aspect-[2/3] w-full object-cover"
                            loading="lazy"
                          />
                        ) : (
                          <div className="flex aspect-[2/3] items-center justify-center bg-muted text-sm">
                            No artwork
                          </div>
                        )}
                        <div className="space-y-1 p-2">
                          <p className="text-sm font-medium">{item.title}</p>
                          <p className="text-xs text-muted-foreground">
                            {[item.year, item.character || item.job]
                              .filter(Boolean)
                              .join(" · ")}
                          </p>
                          <TitleRating rating={item.rating} />
                        </div>
                      </button>
                      <div className="p-2 pt-0">
                        <Button
                          size="sm"
                          className="w-full"
                          disabled={inLibrary}
                          onClick={() => open("add")}
                        >
                          {inLibrary ? "In library" : "Add"}
                        </Button>
                      </div>
                    </div>
                  );
                })}
            </div>
            {!data.data.credits.some((item) => item.media_type === type) && (
              <p className="py-6 text-sm text-muted-foreground">
                No credits found.
              </p>
            )}
          </TabsContent>
        ))}
      </Tabs>
    </div>
  );
}
