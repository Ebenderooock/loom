import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { useLibraries } from "@/lib/libraries-api";
import { useMediaPreferences } from "@/lib/media-info-api";
import {
  loadTitle,
  searchTitles,
  titleJSON,
  titleKey,
  type TitleDefaults,
  type TitleDetails,
  type TitleIdentity,
  type TitleMedia,
} from "@/lib/title-api";
import {
  ArtistMetadata,
  TitleCredits,
  TitleRating,
  TitleSynopsis,
  titleImage,
} from "./title-metadata";
import { ExternalAlbumDetails } from "./album-metadata";
import {
  LibraryMediaSheet,
  type LibraryMediaTarget,
} from "@/components/library-media-sheet";

type Profile = { id: string; name: string };
type Membership = Record<string, string>;

// These controlled dialogs are opened by page cards, not DialogTrigger. Keep
// the actual opener so closing restores keyboard focus rather than the body.
export function useTitleDialogFocus() {
  const opener = useRef<HTMLElement | null>(null);
  return {
    onOpenAutoFocus: () => {
      opener.current =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null;
    },
    onCloseAutoFocus: (event: Event) => {
      event.preventDefault();
      if (opener.current?.isConnected) opener.current.focus();
    },
  };
}

async function loadMembership(
  media: TitleMedia,
  signal?: AbortSignal,
): Promise<Membership> {
  const found: Membership = {};
  let offset = 0;
  for (;;) {
    const url =
      media === "movie"
        ? `/api/v1/movies?limit=1000&offset=${offset}`
        : `/api/v1/${media === "artist" ? "artists" : "series"}`;
    const body = await titleJSON<
      | Array<{ id: string; tmdbId?: string; mbid?: string }>
      | { data: Array<{ id: string; tmdbId?: string; mbid?: string }> }
    >(url, { signal });
    const rows = Array.isArray(body) ? body : (body.data ?? []);
    for (const row of rows) {
      const id = media === "artist" ? row.mbid : row.tmdbId;
      if (id) found[id] = row.id;
    }
    if (media !== "movie" || rows.length < 1000) return found;
    offset += rows.length;
  }
}

function useMembership(media: TitleMedia, enabled = true) {
  return useQuery({
    queryKey: ["title-membership", media],
    queryFn: ({ signal }) => loadMembership(media, signal),
    enabled,
    staleTime: 0,
  });
}

function SelectField({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: Profile[];
}) {
  const id = useId();
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {!value && <option value="">Select {label.toLowerCase()}</option>}
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </select>
    </div>
  );
}

function Toggle({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="h-4 w-4 accent-accent"
      />
      {label}
    </label>
  );
}

export interface TitleContentProps {
  identity: TitleIdentity;
  initialView?: "details" | "add";
  defaults?: TitleDefaults;
  sourceItemId?: string;
  sourceName?: string;
  onAdded?: () => void;
  onManage?: (target: LibraryMediaTarget) => void;
  onBack?: () => void;
  // Requests deliberately supply an action rather than getting library add
  // controls. Viewing metadata must never turn a request into a direct add.
  requestAction?: ReactNode;
}

function AddTitleSettings({
  identity,
  details,
  defaults,
  sourceItemId,
  sourceName,
  onAdded,
}: TitleContentProps & { details: TitleDetails }) {
  const qc = useQueryClient();
  const librariesQuery = useLibraries();
  const { data: allLibraries = [], isLoading: librariesLoading } =
    librariesQuery;
  const mediaType = identity.media === "artist" ? "music" : identity.media;
  const libraries = allLibraries.filter(
    (library) => library.media_type === mediaType,
  );
  const preferencesQuery = useMediaPreferences();
  const { data: preferences } = preferencesQuery;
  const profiles = useQuery({
    queryKey: ["title-profiles", mediaType],
    queryFn: async ({ signal }) => {
      const body = await titleJSON<Profile[] | { data: Profile[] }>(
        mediaType === "music"
          ? "/api/v1/music/audio-quality-profiles"
          : "/api/v1/quality-profiles",
        { signal },
      );
      return Array.isArray(body) ? body : (body.data ?? []);
    },
  });
  const metadataProfiles = useQuery({
    queryKey: ["title-metadata-profiles"],
    queryFn: ({ signal }) =>
      titleJSON<Profile[]>("/api/v1/music/metadata-profiles", { signal }),
    enabled: mediaType === "music",
  });
  const membership = useMembership(identity.media);
  const [libraryChoice, setLibrary] = useState<string>();
  const [profileChoice, setProfile] = useState<string>();
  const [metadataChoice, setMetadata] = useState<string>();
  const [monitorChoice, setMonitor] = useState<string>();
  const [search, setSearch] = useState(defaults?.search_on_add ?? true);
  const [seasonFolder, setSeasonFolder] = useState(true);
  const [seriesType, setSeriesType] = useState("standard");
  const [pending, setPending] = useState(false);
  const [persisted, setPersisted] = useState(false);
  const [completed, setCompleted] = useState(false);
  const [error, setError] = useState("");
  const lock = useRef(false);
  const libraryId =
    libraryChoice ??
    libraries.find((l) => l.path === defaults?.library_path)?.id ??
    libraries[0]?.id ??
    "";
  const library = libraries.find((l) => l.id === libraryId);
  const qualityProfiles = profiles.data ?? [];
  const preferred = [
    profileChoice,
    defaults?.quality_profile_id,
    library?.quality_profile_id,
    preferences?.default_quality_profile_id,
  ].find((id) => qualityProfiles.some((profile) => profile.id === id));
  const profileId = preferred ?? qualityProfiles[0]?.id ?? "";
  const metadataId = metadataChoice ?? metadataProfiles.data?.[0]?.id ?? "";
  const monitoring =
    monitorChoice ??
    defaults?.monitor_type ??
    (library?.monitor_on_add === false ? "none" : "all");
  const providerId =
    identity.media === "artist" ? details.title.mbid : details.title.tmdb_id;
  const inLibrary = identity.inLibrary || !!membership.data?.[providerId ?? ""];

  const trackSource = async () => {
    if (sourceItemId)
      await titleJSON("/api/v1/import-lists/discover/track", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ item_id: sourceItemId }),
      });
  };
  const finish = async () => {
    await trackSource();
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["title-membership", identity.media] }),
      qc.invalidateQueries({ queryKey: ["import-lists"] }),
      qc.invalidateQueries({ queryKey: ["music"] }),
    ]);
    setCompleted(true);
    onAdded?.();
  };
  const submit = async () => {
    if (lock.current) return;
    lock.current = true;
    setPending(true);
    setError("");
    try {
      if (!persisted) {
        // Recheck identity immediately before writing, including titles added in
        // another entry point while this form was open.
        const latest = await membership.refetch();
        if (latest.error) throw latest.error;
        if (!providerId)
          throw new Error(
            "The provider did not return a valid title identity.",
          );
        if (!latest.data?.[providerId] && !identity.inLibrary) {
          const title = details.title;
          const payload =
            identity.media === "movie"
              ? {
                  ...title,
                  title: title.title || identity.title,
                  tmdb_id: providerId,
                  metadata_provider: "tmdb",
                  quality_profile_id: profileId,
                  library_id: libraryId,
                  monitoring_status:
                    monitoring === "none" ? "unmonitored" : "monitored",
                  search,
                }
              : identity.media === "series"
                ? {
                    tmdbId: providerId,
                    qualityProfileId: profileId,
                    libraryId,
                    monitoringStatus: monitoring,
                    seasonFolder,
                    seriesType,
                    search,
                  }
                : {
                    mbid: providerId,
                    qualityProfileId: profileId,
                    libraryId,
                    metadataProfileId: metadataId || undefined,
                    monitoringStatus:
                      monitoring === "none" ? "unmonitored" : "monitored",
                    search,
                  };
          await titleJSON(
            `/api/v1/${identity.media === "movie" ? "movies" : identity.media === "series" ? "series" : "artists"}`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(payload),
            },
          );
        }
        setPersisted(true);
        qc.setQueryData<Membership>(
          ["title-membership", identity.media],
          (old) => ({ ...old, [providerId!]: "added" }),
        );
      }
      await finish();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Unable to add title");
    } finally {
      lock.current = false;
      setPending(false);
    }
  };
  const dataError =
    librariesQuery.error ||
    profiles.error ||
    membership.error ||
    (mediaType === "music" ? metadataProfiles.error : preferencesQuery.error);
  const disabled =
    pending ||
    librariesLoading ||
    profiles.isPending ||
    membership.isPending ||
    !!dataError ||
    !library ||
    !profileId ||
    !providerId ||
    (mediaType === "music"
      ? metadataProfiles.isPending
      : preferencesQuery.isPending);

  return (
    <div className="space-y-4">
      {sourceName && (
        <p className="text-sm text-muted-foreground">
          Settings from {sourceName}. You can change them before adding.
        </p>
      )}
      {inLibrary && !persisted ? (
        <p role="status" className="text-sm">
          Already in your library.
        </p>
      ) : null}
      {!persisted && (
        <fieldset disabled={pending || !!inLibrary} className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <SelectField
              label="Library"
              value={libraryId}
              options={libraries}
              onChange={(id) => {
                setLibrary(id);
                setProfile(
                  libraries.find((l) => l.id === id)?.quality_profile_id ||
                    preferences?.default_quality_profile_id ||
                    qualityProfiles[0]?.id,
                );
              }}
            />
            <SelectField
              label="Quality profile"
              value={profileId}
              options={qualityProfiles}
              onChange={setProfile}
            />
            {mediaType === "music" && (
              <SelectField
                label="Metadata profile"
                value={metadataId}
                options={metadataProfiles.data ?? []}
                onChange={setMetadata}
              />
            )}
            {identity.media === "series" && (
              <>
                <SelectField
                  label="Series type"
                  value={seriesType}
                  onChange={setSeriesType}
                  options={["standard", "daily", "anime"].map((id) => ({
                    id,
                    name: id,
                  }))}
                />
                <SelectField
                  label="Monitor"
                  value={monitoring}
                  onChange={setMonitor}
                  options={[
                    { id: "all", name: "All episodes" },
                    { id: "future", name: "Future episodes" },
                    { id: "missing", name: "Missing episodes" },
                    { id: "existing", name: "Existing episodes" },
                    { id: "pilot", name: "Pilot only" },
                    { id: "firstSeason", name: "First season" },
                    { id: "lastSeason", name: "Latest season" },
                    { id: "none", name: "None" },
                  ]}
                />
              </>
            )}
          </div>
          {identity.media !== "series" && (
            <Toggle
              label="Monitor for downloads"
              checked={monitoring !== "none"}
              onChange={(value) => setMonitor(value ? "all" : "none")}
            />
          )}
          {identity.media === "series" && (
            <Toggle
              label="Season folders"
              checked={seasonFolder}
              onChange={setSeasonFolder}
            />
          )}
          <Toggle
            label={
              identity.media === "artist"
                ? "Search for albums after adding"
                : "Search after adding"
            }
            checked={search}
            onChange={setSearch}
          />
        </fieldset>
      )}
      {!librariesLoading && !libraries.length && (
        <p role="alert" className="text-sm text-destructive">
          Configure a {mediaType} library in Settings → Libraries before adding.
        </p>
      )}
      {!profiles.isPending && !qualityProfiles.length && (
        <p role="alert" className="text-sm text-destructive">
          Configure a quality profile before adding.
        </p>
      )}
      {!!dataError && (
        <div role="alert">
          <p className="text-sm text-destructive">
            Unable to load add settings.
          </p>
          <Button
            variant="outline"
            onClick={() => {
              void librariesQuery.refetch();
              void profiles.refetch();
              void membership.refetch();
              if (mediaType === "music") void metadataProfiles.refetch();
              else void preferencesQuery.refetch();
            }}
          >
            Retry settings
          </Button>
        </div>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {persisted
            ? "Title is in your library, but source tracking failed. Retry without adding again. "
            : ""}
          {error}
        </p>
      )}
      <Button
        onClick={submit}
        disabled={
          completed ||
          (persisted ? pending : disabled || (!!inLibrary && !sourceItemId))
        }
      >
        {pending
          ? "Saving…"
          : completed
            ? "In library"
            : persisted
              ? "Retry source tracking"
              : inLibrary && sourceItemId
                ? "Update source tracking"
                : "Add to library"}
      </Button>
    </div>
  );
}

// Embeddable content lets a search or person dialog replace its own contents,
// instead of piling another modal/focus trap on top.
export function TitleContent(props: TitleContentProps) {
  const { identity, onBack, requestAction } = props;
  const [view, setView] = useState(props.initialView ?? "details");
  const [settingsVisited, setSettingsVisited] = useState(
    props.initialView === "add",
  );
  const focusRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const frame = requestAnimationFrame(() => focusRef.current?.focus());
    return () => cancelAnimationFrame(frame);
  }, []);
  const details = useQuery({
    queryKey: titleKey(identity),
    queryFn: ({ signal }) => loadTitle(identity, signal),
    staleTime: 5 * 60_000,
  });
  const membership = useMembership(identity.media, !requestAction);
  const providerId =
    identity.media === "artist"
      ? details.data?.title.mbid
      : details.data?.title.tmdb_id;
  const inLibrary = identity.inLibrary || !!membership.data?.[providerId ?? ""];
  const title = details.data?.title;
  const date = title?.release_date || title?.first_air_date;
  return (
    <div
      ref={focusRef}
      tabIndex={-1}
      aria-label={`${identity.title} ${view === "add" ? "add settings" : "details"}`}
      className="space-y-5 outline-none"
    >
      <div className="flex flex-wrap gap-2">
        {onBack && (
          <Button variant="ghost" size="sm" onClick={onBack}>
            Back to results
          </Button>
        )}
        {!requestAction &&
        inLibrary &&
        !props.sourceItemId &&
        view === "details" ? (
          props.onManage &&
          identity.media !== "artist" &&
          membership.data?.[providerId ?? ""] ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() =>
                props.onManage?.({
                  type: identity.media as "movie" | "series",
                  id: membership.data?.[providerId ?? ""] ?? "",
                })
              }
            >
              Manage in library
            </Button>
          ) : (
            <Badge variant="secondary">In library</Badge>
          )
        ) : (
          !requestAction && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setSettingsVisited(true);
                setView(view === "details" ? "add" : "details");
              }}
            >
              {view === "add"
                ? "View full details"
                : inLibrary
                  ? "In library"
                  : "Add to library"}
            </Button>
          )
        )}
        {requestAction}
      </div>
      {details.isPending && (
        <p role="status" className="text-sm text-muted-foreground">
          Loading full details…
        </p>
      )}
      {details.isError && (
        <div role="alert">
          <p className="text-sm text-destructive">{details.error.message}</p>
          <Button variant="outline" onClick={() => details.refetch()}>
            Retry details
          </Button>
        </div>
      )}
      {title && details.data && (
        <>
          {title.backdrop_path && (
            <img
              src={titleImage(title.backdrop_path, "w780")}
              alt=""
              className="max-h-52 w-full rounded-lg object-cover"
            />
          )}
          <div className="flex gap-4">
            {titleImage(title.poster_path || title.image_url) && (
              <img
                src={titleImage(title.poster_path || title.image_url)}
                alt=""
                className={`w-28 shrink-0 self-start rounded-lg object-cover ${identity.media === "artist" ? "aspect-square" : "aspect-[2/3]"}`}
              />
            )}
            <div className="min-w-0 space-y-3">
              <h2 className="text-2xl font-semibold">
                {title.title || title.name || identity.title}
              </h2>
              <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
                {date && <span>{date}</span>}
                {title.runtime ? <span>{title.runtime} min</span> : null}
                {title.seasons ? <span>{title.seasons} seasons</span> : null}
                <TitleRating rating={title.rating} />
                {title.status && <span>{title.status}</span>}
                {title.network && <span>{title.network}</span>}
              </div>
              {!!title.genres?.length && identity.media !== "artist" && (
                <div className="flex flex-wrap gap-1">
                  {title.genres.map((genre) => (
                    <Badge key={genre} variant="secondary">
                      {genre}
                    </Badge>
                  ))}
                </div>
              )}
              {title.tagline && (
                <p className="text-sm italic text-muted-foreground">
                  {title.tagline}
                </p>
              )}
            </div>
          </div>
          {settingsVisited && !requestAction && (
            <div hidden={view !== "add"}>
              <AddTitleSettings {...props} details={details.data} />
            </div>
          )}
          {(view === "details" || requestAction) && (
            <>
              {identity.media === "artist" ? (
                <ArtistMetadata artist={title} />
              ) : (
                <section className="space-y-2">
                  <h3 className="text-sm font-semibold">Synopsis</h3>
                  <TitleSynopsis overview={title.overview} />
                </section>
              )}
              {identity.media !== "artist" &&
                (details.data.credits_error ? (
                  <div role="alert">
                    <p className="text-sm text-destructive">
                      {details.data.credits_error}
                    </p>
                    <Button variant="outline" onClick={() => details.refetch()}>
                      Retry credits
                    </Button>
                  </div>
                ) : (
                  <TitleCredits credits={details.data.credits} />
                ))}
              {identity.media === "artist" && (
                <section className="space-y-3">
                  <h3 className="text-sm font-semibold">Albums</h3>
                  {!details.data.albums?.length && (
                    <p className="text-sm text-muted-foreground">
                      No albums available.
                    </p>
                  )}
                  {details.data.albums?.map((album) => (
                    <ExternalAlbumDetails key={album.mbid} album={album} />
                  ))}
                </section>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}

export function TitleDialog({
  open,
  onOpenChange,
  ...props
}: TitleContentProps & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const focus = useTitleDialogFocus();
  const qc = useQueryClient();
  const [managing, setManaging] = useState<LibraryMediaTarget | null>(null);
  const managementChanged = () => {
    void qc.invalidateQueries({ queryKey: ["title-membership"] });
    void qc.invalidateQueries({ queryKey: ["collections"] });
    void qc.invalidateQueries({ queryKey: ["discovery"] });
  };
  return (
    <>
      <Dialog open={open && !managing} onOpenChange={onOpenChange}>
        <DialogContent
          {...focus}
          className="max-h-[85vh] max-w-3xl overflow-y-auto"
        >
          <DialogHeader>
            <DialogTitle>{props.identity.title}</DialogTitle>
            <DialogDescription>
              {props.requestAction
                ? "View full details before requesting this title."
                : "View details or choose how to add this title."}
            </DialogDescription>
          </DialogHeader>
          {open && (
            <TitleContent
              key={`${props.identity.media}:${props.identity.id}`}
              {...props}
              onManage={setManaging}
            />
          )}
        </DialogContent>
      </Dialog>
      {open && managing && (
        <LibraryMediaSheet
          target={managing}
          onChanged={managementChanged}
          onClose={() => {
            setManaging(null);
            managementChanged();
          }}
        />
      )}
    </>
  );
}

export function TitleSearchDialog({
  media,
  open,
  onOpenChange,
  existingIds,
  onAdded,
}: {
  media: TitleMedia;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  existingIds?: Set<string>;
  onAdded?: () => void;
}) {
  const focus = useTitleDialogFocus();
  // Remount the contents each time, so settings and pending searches cannot
  // leak from the previous search session.
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        {...focus}
        className="max-h-[85vh] max-w-3xl overflow-y-auto"
      >
        <DialogHeader>
          <DialogTitle>
            Search{" "}
            {media === "movie"
              ? "movies"
              : media === "series"
                ? "TV shows"
                : "artists"}
          </DialogTitle>
          <DialogDescription>
            Open a title for full details, or choose Add to configure it.
          </DialogDescription>
        </DialogHeader>
        {open && (
          <TitleSearch
            media={media}
            existingIds={existingIds}
            onAdded={() => {
              onAdded?.();
              onOpenChange(false);
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function TitleSearch({
  media,
  existingIds,
  onAdded,
}: {
  media: TitleMedia;
  existingIds?: Set<string>;
  onAdded: () => void;
}) {
  const [term, setTerm] = useState("");
  const [selection, setSelection] = useState<{
    identity: TitleIdentity;
    view: "details" | "add";
  }>();
  const searchInput = useRef<HTMLInputElement>(null);
  const viewedTitle = useRef(false);
  useEffect(() => {
    if (selection) viewedTitle.current = true;
    else if (viewedTitle.current) searchInput.current?.focus();
  }, [selection]);
  const results = useQuery({
    queryKey: ["title-search", media, term],
    queryFn: async ({ signal }) => {
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(resolve, 350);
        signal.addEventListener(
          "abort",
          () => {
            clearTimeout(timer);
            reject(new DOMException("Aborted", "AbortError"));
          },
          { once: true },
        );
      });
      return searchTitles(media, term, signal);
    },
    enabled: term.trim().length >= 2,
  });
  const membership = useMembership(media);
  if (selection)
    return (
      <TitleContent
        key={`${selection.identity.media}:${selection.identity.id}`}
        identity={selection.identity}
        initialView={selection.view}
        onBack={() => setSelection(undefined)}
        onAdded={onAdded}
      />
    );
  return (
    <div className="space-y-4">
      <Input
        ref={searchInput}
        aria-label="Search titles"
        placeholder="Search titles…"
        value={term}
        onChange={(e) => setTerm(e.target.value)}
      />
      {results.isFetching && (
        <p role="status" className="text-sm text-muted-foreground">
          Searching…
        </p>
      )}
      {results.isError && (
        <p role="alert" className="text-sm text-destructive">
          {results.error.message}
        </p>
      )}
      {term.trim().length >= 2 &&
        results.isSuccess &&
        results.data.length === 0 && (
          <p className="text-sm text-muted-foreground">No titles found.</p>
        )}
      {results.data?.map((identity) => {
        const inLibrary =
          identity.inLibrary ||
          existingIds?.has(identity.id) ||
          !!membership.data?.[identity.id];
        const selected = { ...identity, inLibrary };
        return (
          <div
            key={identity.id}
            className="flex items-center gap-3 rounded-lg border p-3"
          >
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-3 text-left text-sm font-medium hover:underline"
              onClick={() =>
                setSelection({ identity: selected, view: "details" })
              }
            >
              {identity.poster && (
                <img
                  src={titleImage(identity.poster, "w92")}
                  alt=""
                  className="h-16 w-12 rounded object-cover"
                />
              )}
              <span>
                {identity.title}
                {identity.year ? (
                  <span className="block text-xs text-muted-foreground">
                    {identity.year}
                  </span>
                ) : null}
              </span>
            </button>
            <Button
              size="sm"
              variant="outline"
              disabled={inLibrary}
              onClick={() => setSelection({ identity: selected, view: "add" })}
            >
              {inLibrary ? "In library" : "Add"}
            </Button>
          </div>
        );
      })}
    </div>
  );
}
