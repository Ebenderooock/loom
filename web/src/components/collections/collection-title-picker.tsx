import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, Film, Loader2 } from "lucide-react";
import {
  addCollectionMembers,
  hasTitle,
  loadLibraryTitles,
  type LoomCollection,
  type MemberInput,
} from "@/lib/collections-api";
import { searchTitles } from "@/lib/title-api";
import { titleImage } from "@/components/titles/title-metadata";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";

type Media = "movie" | "series";

interface Candidate {
  key: string;
  media: Media;
  title: string;
  year?: number;
  poster?: string;
  mediaId?: string;
  tmdb?: string;
}

function toInput(c: Candidate): MemberInput {
  return c.mediaId
    ? { media_type: c.media, media_id: c.mediaId }
    : {
        media_type: c.media,
        title: c.title,
        year: c.year,
        poster_path: c.poster,
        external_ids: { tmdb: c.tmdb },
      };
}

/**
 * Searches the library and TMDB together. Library titles are attached by
 * their local ID so membership always refers to real media; provider titles
 * keep their exact TMDB identity and can be added to the library later.
 */
export function CollectionTitlePicker({
  collection,
  open,
  onOpenChange,
  onSaved,
  initialMedia = "movie",
  initialTerm = "",
}: {
  collection: LoomCollection;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: (c: LoomCollection) => void;
  initialMedia?: Media;
  initialTerm?: string;
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full min-w-0 max-w-full flex-col gap-4 overflow-hidden p-4 sm:max-w-xl sm:p-6">
        <SheetHeader className="pr-8 text-left">
          <SheetTitle>Add titles to {collection.name}</SheetTitle>
          <SheetDescription>
            Pick titles from your library or TMDB. Membership never adds a title
            to the library on its own.
          </SheetDescription>
        </SheetHeader>
        {open && (
          <PickerBody
            collection={collection}
            initialMedia={initialMedia}
            initialTerm={initialTerm}
            onSaved={(c) => {
              onSaved(c);
              onOpenChange(false);
            }}
          />
        )}
      </SheetContent>
    </Sheet>
  );
}

function PickerBody({
  collection,
  initialMedia,
  initialTerm,
  onSaved,
}: {
  collection: LoomCollection;
  initialMedia: Media;
  initialTerm: string;
  onSaved: (c: LoomCollection) => void;
}) {
  const [media, setMedia] = useState<Media>(initialMedia);
  const [term, setTerm] = useState(initialTerm);
  const [picked, setPicked] = useState<Map<string, Candidate>>(new Map());
  const query = term.trim();
  const library = useQuery({
    queryKey: ["collection-picker-library", media],
    queryFn: ({ signal }) => loadLibraryTitles(media, signal),
    staleTime: 30_000,
  });
  const provider = useQuery({
    queryKey: ["title-search", media, query],
    queryFn: async ({ signal }) => {
      await new Promise((resolve) => setTimeout(resolve, 300));
      if (signal.aborted) return [];
      return searchTitles(media, query, signal);
    },
    enabled: query.length >= 2,
  });
  const candidates = useMemo(() => {
    const lower = query.toLowerCase();
    const byTMDB = new Map<string, Candidate>();
    const local: Candidate[] = [];
    for (const t of library.data ?? []) {
      const c: Candidate = {
        key: `${media}:local:${t.id}`,
        media,
        title: t.title,
        year: t.year,
        poster: t.poster,
        mediaId: t.id,
        tmdb: t.tmdb,
      };
      if (t.tmdb) byTMDB.set(t.tmdb, c);
      if (lower && t.title.toLowerCase().includes(lower)) local.push(c);
    }
    const remote: Candidate[] = [];
    for (const r of provider.data ?? []) {
      // A provider result already in the library is the library title.
      if (byTMDB.has(r.id)) {
        const c = byTMDB.get(r.id)!;
        if (!local.includes(c)) local.push(c);
        continue;
      }
      remote.push({
        key: `${media}:tmdb:${r.id}`,
        media,
        title: r.title,
        year: r.year,
        poster: r.poster || undefined,
        tmdb: r.id,
      });
    }
    return { local: local.slice(0, 30), remote: remote.slice(0, 20) };
  }, [library.data, provider.data, media, query]);
  const save = useMutation({
    mutationFn: () =>
      addCollectionMembers(collection, [...picked.values()].map(toInput)),
    onSuccess: onSaved,
  });
  const toggle = (c: Candidate) =>
    setPicked((prev) => {
      const next = new Map(prev);
      if (next.has(c.key)) next.delete(c.key);
      else next.set(c.key, c);
      return next;
    });
  const row = (c: Candidate) => {
    const member = hasTitle(collection, c.media, {
      media_id: c.mediaId,
      tmdb: c.tmdb,
    });
    const selected = picked.has(c.key);
    return (
      <li key={c.key}>
        <button
          type="button"
          role="checkbox"
          aria-checked={member || selected}
          disabled={member}
          onClick={() => toggle(c)}
          className={cn(
            "flex w-full min-w-0 items-center gap-3 rounded-lg border p-2 text-left text-sm transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-60",
            selected && "border-accent bg-accent/10",
          )}
        >
          <span className="flex h-16 w-11 shrink-0 items-center justify-center overflow-hidden rounded bg-muted">
            {c.poster ? (
              <img
                src={titleImage(c.poster, "w92")}
                alt=""
                className="h-full w-full object-cover"
                loading="lazy"
              />
            ) : (
              <Film className="h-5 w-5 text-muted-foreground/40" />
            )}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium">{c.title}</span>
            <span className="block text-xs text-muted-foreground">
              {c.year ?? "Year unknown"} ·{" "}
              {member
                ? "Already in collection"
                : c.mediaId
                  ? "In your library"
                  : "TMDB"}
            </span>
          </span>
          <span
            aria-hidden
            className={cn(
              "flex h-6 w-6 shrink-0 items-center justify-center rounded-full border",
              (member || selected) &&
                "border-accent bg-accent text-accent-foreground",
            )}
          >
            {(member || selected) && <Check className="h-4 w-4" />}
          </span>
        </button>
      </li>
    );
  };
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div
        role="group"
        aria-label="Media type"
        className="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"
      >
        {(["movie", "series"] as const).map((m) => (
          <button
            key={m}
            type="button"
            aria-pressed={media === m}
            onClick={() => setMedia(m)}
            className={cn(
              "h-9 rounded-md text-sm font-medium text-muted-foreground",
              media === m && "bg-background text-foreground shadow",
            )}
          >
            {m === "movie" ? "Movies" : "TV shows"}
          </button>
        ))}
      </div>
      <Input
        aria-label="Search titles"
        placeholder={`Search ${media === "movie" ? "movies" : "TV shows"}…`}
        value={term}
        onChange={(e) => setTerm(e.target.value)}
      />
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overflow-x-hidden">
        {query.length < 2 ? (
          <p className="text-sm text-muted-foreground">
            Type at least two characters to search your library and TMDB.
          </p>
        ) : (
          <>
            <section aria-label="Library results" className="space-y-2">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                In your library
              </h3>
              {library.isPending ? (
                <p role="status" className="text-sm text-muted-foreground">
                  Loading library…
                </p>
              ) : library.error ? (
                <p role="alert" className="text-sm text-destructive">
                  {library.error.message}
                </p>
              ) : candidates.local.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No library matches.
                </p>
              ) : (
                <ul className="space-y-2">{candidates.local.map(row)}</ul>
              )}
            </section>
            <section aria-label="TMDB results" className="space-y-2">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                From TMDB
              </h3>
              {provider.isFetching ? (
                <p role="status" className="text-sm text-muted-foreground">
                  Searching…
                </p>
              ) : provider.error ? (
                <p role="alert" className="text-sm text-destructive">
                  {provider.error.message}
                </p>
              ) : candidates.remote.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No other matches.
                </p>
              ) : (
                <ul className="space-y-2">{candidates.remote.map(row)}</ul>
              )}
            </section>
          </>
        )}
      </div>
      {save.error && (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      )}
      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <span className="text-sm text-muted-foreground">
          {picked.size} selected
        </span>
        <Button
          disabled={picked.size === 0 || save.isPending}
          onClick={() => save.mutate()}
        >
          {save.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Add {picked.size || ""} {picked.size === 1 ? "title" : "titles"}
        </Button>
      </div>
    </div>
  );
}
