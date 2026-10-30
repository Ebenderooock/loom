import { useCallback, useEffect, useState, type ReactNode } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Check,
  Download,
  Layers,
  ListChecks,
  Loader2,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
} from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { usePageHeader } from "@/hooks/use-page-header";
import { useLibraries } from "@/lib/libraries-api";
import {
  collectionKeys,
  collectionPath,
  collectionRequest,
  matchesMember,
  memberIdentity,
  type CollectionMember,
  type CollectionSource,
  type LoomCollection,
} from "@/lib/collections-api";
import { LibraryMediaSheet } from "@/components/library-media-sheet";
import { TitleDialog } from "@/components/titles/title-experience";
import { TitlePosterCard } from "@/components/titles/title-poster-card";
import { titleImage } from "@/components/titles/title-metadata";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { useQualityProfiles } from "@/lib/quality-profiles-api";
import { CollectionTitlePicker } from "@/components/collections/collection-title-picker";
import {
  CollectionBulkSheet,
  type BulkKind,
} from "@/components/collections/collection-bulk-sheet";

const GRID =
  "grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6";

type CollectionScan = {
  total: number;
  checked: number;
  pending: number;
  failed: number;
  running: boolean;
  errors: { media_type: string; media_id: string; message: string }[];
};

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5 text-sm">
      {label}
      {children}
    </label>
  );
}

const SELECT = "h-10 rounded-md border bg-background px-3 text-sm";

export function CollectionsPage() {
  const { setHeader } = usePageHeader();
  useEffect(() => setHeader({ title: "Collections" }), [setHeader]);
  const { id, tmdb } = useSearch({ from: "/collections" });
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { user } = useAuth();
  const canManage = user?.role === "admin";
  const [creating, setCreating] = useState(false);
  const [library, setLibrary] = useState("");
  const [media, setMedia] = useState("");
  const [status, setStatus] = useState("");
  const libraries = useLibraries();
  const list = useQuery({
    queryKey: collectionKeys.all,
    queryFn: ({ signal }) =>
      collectionRequest<{ items: LoomCollection[] }>(
        "",
        undefined,
        "GET",
        signal,
      ),
  });
  const open = (id?: string) =>
    navigate({ to: "/collections", search: id ? { id } : {} });
  const invalidate = async () => {
    await qc.invalidateQueries({ queryKey: collectionKeys.all });
  };
  const discover = useMutation({
    mutationFn: () => collectionRequest<CollectionScan>("/discover", {}),
    onSettled: invalidate,
  });
  const scan = useQuery({
    queryKey: ["collection-scan"],
    queryFn: ({ signal }) =>
      collectionRequest<CollectionScan>("/scan", undefined, "GET", signal),
    refetchInterval: 5000,
  });
  useEffect(() => {
    if (scan.data) void qc.invalidateQueries({ queryKey: collectionKeys.all });
  }, [qc, scan.data]);
  const importSet = useMutation({
    mutationFn: () =>
      collectionRequest<LoomCollection>("", {
        source: {
          provider: "tmdb",
          kind: "collection",
          external_id: String(tmdb),
        },
      }),
    onSuccess: async (collection) => {
      await invalidate();
      await open(collection.id);
    },
  });
  const existingSet = tmdb
    ? list.data?.items.find((c) =>
        c.sources.some(
          (s) =>
            s.provider === "tmdb" &&
            s.kind === "collection" &&
            s.external_id === String(tmdb),
        ),
      )
    : undefined;
  const selectedID = id ?? existingSet?.id;

  return (
    <div className="space-y-6 p-4 sm:p-6">
      {selectedID ? (
        <CollectionDetails
          key={selectedID}
          id={selectedID}
          canManage={canManage}
          all={list.data?.items ?? []}
          onBack={() => void open()}
          onOpen={(id) => void open(id)}
        />
      ) : (
        <>
          <header className="flex flex-wrap items-start justify-between gap-4">
            <div className="max-w-2xl space-y-2">
              <h1 className="text-2xl font-semibold tracking-tight">
                Collections
              </h1>
              <p className="text-sm text-muted-foreground">
                Movies and shows together, across libraries. Collect a sequence,
                a franchise, or your own selection without adding every title to
                Loom.
              </p>
            </div>
            {canManage && (
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  disabled={discover.isPending}
                  onClick={() => discover.mutate()}
                >
                  <RefreshCw
                    className={`mr-2 h-4 w-4 ${discover.isPending ? "animate-spin" : ""}`}
                  />
                  Rescan library
                </Button>
                <Button onClick={() => setCreating(!creating)}>
                  <Plus className="mr-2 h-4 w-4" />
                  New collection
                </Button>
              </div>
            )}
          </header>
          <p className="text-xs text-muted-foreground">
            TMDB movie sets cover sequels, not entire universes. Official TVDB
            lists are candidates, not a completeness guarantee. Similar names or
            shared titles never merge collections automatically.
          </p>
          <ErrorMessage
            error={
              discover.error ?? importSet.error ?? libraries.error ?? scan.error
            }
          />
          {scan.data && (
            <section
              aria-label="Background collection discovery"
              className="space-y-2 text-sm text-muted-foreground"
            >
              <p role="status">
                Collections are found automatically in the background.{" "}
                {scan.data.checked} of {scan.data.total} library titles checked.
                {scan.data.running || scan.data.pending > 0
                  ? " Scanning…"
                  : scan.data.failed > 0
                    ? " Waiting to retry."
                    : " Up to date."}
                {scan.data.failed > 0 &&
                  ` ${scan.data.failed} titles need attention and will retry automatically.`}
              </p>
              {scan.data.errors.length > 0 && (
                <details>
                  <summary className="cursor-pointer">Discovery issues</summary>
                  {scan.data.errors.map((error) => (
                    <p
                      key={`${error.media_type}:${error.media_id}`}
                      className="break-words text-destructive"
                    >
                      {error.media_type} {error.media_id}: {error.message}
                    </p>
                  ))}
                </details>
              )}
            </section>
          )}
          {tmdb && !existingSet && !list.isPending && (
            <section className="space-y-3 rounded-lg border p-4">
              <h2 className="font-semibold">TMDB movie set {tmdb}</h2>
              <p className="text-sm text-muted-foreground">
                This source has not been saved in Loom yet.
              </p>
              {canManage ? (
                <Button
                  disabled={importSet.isPending}
                  onClick={() => importSet.mutate()}
                >
                  Import collection
                </Button>
              ) : (
                <p>Ask an administrator to import this collection.</p>
              )}
              <Button variant="ghost" onClick={() => void open()}>
                Back to collections
              </Button>
            </section>
          )}
          {creating && canManage && (
            <div className="grid gap-6 rounded-lg border p-4 md:grid-cols-2">
              <CollectionForm
                onSaved={async (c) => {
                  setCreating(false);
                  await invalidate();
                  await open(c.id);
                }}
              />
              <SourceForm
                onSaved={async (c) => {
                  setCreating(false);
                  await invalidate();
                  await open(c.id);
                }}
              />
            </div>
          )}
          <Filters
            {...{ library, setLibrary, media, setMedia, status, setStatus }}
            fileFilter={false}
          />
          {list.isPending ? (
            <Loading />
          ) : list.error ? (
            <Retry error={list.error} retry={() => void list.refetch()} />
          ) : (
            <div className={GRID}>
              {list.data?.items
                .filter(
                  (c) =>
                    (!library && !media && !status) ||
                    c.members.some((m) =>
                      matchesMember(m, library, media, status),
                    ),
                )
                .map((c) => (
                  <Link
                    key={c.id}
                    to="/collections"
                    search={{ id: c.id }}
                    className="group overflow-hidden rounded-lg border bg-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <div className="flex aspect-[4/3] overflow-hidden bg-muted">
                      {c.members
                        .filter((m) => !m.excluded && m.poster_path)
                        .slice(0, 3)
                        .map((m) => (
                          <img
                            key={m.id}
                            src={titleImage(m.poster_path) ?? ""}
                            alt=""
                            className="min-w-0 flex-1 object-cover"
                            loading="lazy"
                          />
                        ))}
                      {!c.members.some((m) => !m.excluded && m.poster_path) && (
                        <Layers
                          className="m-auto h-12 w-12 text-muted-foreground"
                          aria-hidden
                        />
                      )}
                    </div>
                    <div className="space-y-2 p-3">
                      <h2 className="font-semibold group-hover:underline">
                        {c.name}
                      </h2>
                      <p className="text-xs text-muted-foreground">
                        {c.in_library} of {c.total} added · {c.sources.length}{" "}
                        sources
                      </p>
                      {c.sources.some((s) => s.error) && (
                        <Badge variant="outline">Source needs attention</Badge>
                      )}
                    </div>
                  </Link>
                ))}
            </div>
          )}
          {!list.isPending && !list.error && list.data?.items.length === 0 && (
            <p className="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
              No collections yet. Create your own, import a source, or find
              provider-declared sets from your library.
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            This product uses the TMDB API but is not endorsed or certified by
            TMDB.
          </p>
        </>
      )}
    </div>
  );
}

function Filters({
  library,
  setLibrary,
  media,
  setMedia,
  status,
  setStatus,
  fileFilter = true,
}: {
  library: string;
  setLibrary: (v: string) => void;
  media: string;
  setMedia: (v: string) => void;
  status: string;
  setStatus: (v: string) => void;
  fileFilter?: boolean;
}) {
  const libraries = useLibraries();
  return (
    <div className="flex flex-wrap gap-3">
      {libraries.error && (
        <div className="w-full space-y-2">
          <ErrorMessage error={libraries.error} />
          <Button variant="outline" onClick={() => void libraries.refetch()}>
            Retry libraries
          </Button>
        </div>
      )}
      <Field label="Library">
        <select
          className={SELECT}
          value={library}
          onChange={(e) => setLibrary(e.target.value)}
        >
          <option value="">All libraries & external titles</option>
          {libraries.data
            ?.filter(
              (l) => l.media_type === "movie" || l.media_type === "series",
            )
            .map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
        </select>
      </Field>
      <Field label="Media">
        <select
          className={SELECT}
          value={media}
          onChange={(e) => setMedia(e.target.value)}
        >
          <option value="">Movies & shows</option>
          <option value="movie">Movies</option>
          <option value="series">TV shows</option>
        </select>
      </Field>
      <Field label="Status">
        <select
          className={SELECT}
          value={status}
          onChange={(e) => setStatus(e.target.value)}
        >
          <option value="">All active members</option>
          <option value="added">Added to Loom</option>
          <option value="missing">Not in library</option>
          <option value="upcoming">Upcoming</option>
          <option value="monitored">Monitored</option>
          {fileFilter && <option value="files">Has imported files</option>}
          <option value="excluded">Locally excluded</option>
        </select>
      </Field>
    </div>
  );
}

function CollectionDetails({
  id,
  canManage,
  all,
  onBack,
  onOpen,
}: {
  id: string;
  canManage: boolean;
  all: LoomCollection[];
  onBack: () => void;
  onOpen: (id?: string) => void;
}) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<CollectionMember | null>(null);
  const [library, setLibrary] = useState("");
  const [media, setMedia] = useState("");
  const [status, setStatus] = useState("");
  const [managing, setManaging] = useState(false);
  const [picker, setPicker] = useState<{
    media?: "movie" | "series";
    term?: string;
  }>();
  const [selecting, setSelecting] = useState(false);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [bulk, setBulk] = useState<{ kind: BulkKind; ids?: string[] }>();
  const detail = useQuery({
    queryKey: collectionKeys.detail(id),
    queryFn: ({ signal }) =>
      collectionRequest<LoomCollection>(
        `/${encodeURIComponent(id)}`,
        undefined,
        "GET",
        signal,
      ),
  });
  const changed = useCallback(() => {
    void qc.invalidateQueries({ queryKey: collectionKeys.all });
    void qc.invalidateQueries({ queryKey: ["discovery"] });
    void qc.invalidateQueries({ queryKey: ["title-membership"] });
  }, [qc]);
  const mutation = useMutation({
    mutationFn: ({
      path,
      body,
      method,
    }: {
      path: string;
      body: unknown;
      method?: string;
    }) =>
      collectionRequest<LoomCollection>(
        collectionPath(detail.data?.id ?? id, path),
        body,
        method,
      ),
    onSuccess: changed,
  });
  const c = detail.data;
  const active = selected && c?.members.find((m) => m.id === selected.id);
  const visible = c?.members.filter((m) =>
    matchesMember(m, library, media, status),
  );
  const ids = checked.size ? [...checked] : undefined;
  const toggle = (memberId: string) =>
    setChecked((prev) => {
      const next = new Set(prev);
      if (next.has(memberId)) next.delete(memberId);
      else next.add(memberId);
      return next;
    });
  const failing = c?.sources.filter((s) => s.error) ?? [];
  return (
    <section className="min-w-0 space-y-5" aria-label="Collection details">
      <Button variant="ghost" onClick={() => onBack()}>
        <ArrowLeft className="mr-2 h-4 w-4" />
        All collections
      </Button>
      {detail.isPending ? (
        <Loading />
      ) : detail.error ? (
        <Retry error={detail.error} retry={() => void detail.refetch()} />
      ) : (
        c && (
          <>
            {c.id !== id && (
              <p role="status">
                This collection was merged.{" "}
                <Button variant="link" onClick={() => onOpen(c.id)}>
                  Open retained collection
                </Button>
              </p>
            )}
            <header className="flex flex-wrap items-start justify-between gap-4">
              <div className="min-w-0 max-w-3xl space-y-2">
                <h1 className="break-words text-2xl font-semibold tracking-tight">
                  {c.name}
                </h1>
                {c.overview && (
                  <p className="text-sm text-muted-foreground">{c.overview}</p>
                )}
                <p className="text-sm">
                  <span className="font-medium">
                    {c.in_library} of {c.total} in library
                  </span>
                  <span className="text-muted-foreground">
                    {" · "}
                    {c.sources.length
                      ? `${c.sources.length} source${c.sources.length === 1 ? "" : "s"}`
                      : "Local collection"}
                  </span>
                </p>
              </div>
              {canManage && (
                <div className="flex flex-wrap items-start gap-2">
                  <Button onClick={() => setPicker({})}>
                    <Plus className="mr-2 h-4 w-4" />
                    Add titles
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => setBulk({ kind: "add" })}
                  >
                    <Download className="mr-2 h-4 w-4" />
                    Add missing
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => setBulk({ kind: "search" })}
                  >
                    <Search className="mr-2 h-4 w-4" />
                    Search
                  </Button>
                  <Button
                    variant={selecting ? "secondary" : "outline"}
                    aria-pressed={selecting}
                    onClick={() => {
                      setSelecting(!selecting);
                      setChecked(new Set());
                    }}
                  >
                    <ListChecks className="mr-2 h-4 w-4" />
                    Select
                  </Button>
                  <Button variant="outline" onClick={() => setManaging(true)}>
                    <Settings2 className="mr-2 h-4 w-4" />
                    Manage
                  </Button>
                </div>
              )}
            </header>
            {failing.length > 0 && (
              <p role="alert" className="text-sm text-destructive">
                {failing.length} source{failing.length === 1 ? "" : "s"} need
                attention: {failing[0]?.error}
              </p>
            )}
            <ErrorMessage error={mutation.error} />
            <Filters
              {...{ library, setLibrary, media, setMedia, status, setStatus }}
            />
            <div className={GRID}>
              {visible?.map((m) => {
                const unresolved = !m.in_library && !m.external_ids.tmdb;
                const isChecked = checked.has(m.id);
                return (
                  <article key={m.id} className="relative min-w-0 space-y-1.5">
                    <TitlePosterCard
                      title={m.title}
                      poster={m.poster_path}
                      year={m.year}
                      available={m.has_file}
                      inLibrary={m.in_library}
                      onOpen={() => (selecting ? toggle(m.id) : setSelected(m))}
                      onAdd={
                        canManage && !selecting
                          ? () => setSelected(m)
                          : undefined
                      }
                      label={
                        selecting
                          ? `${isChecked ? "Deselect" : "Select"} ${m.title}`
                          : `${m.in_library ? "Manage" : "View"} ${m.title}`
                      }
                    />
                    {selecting && (
                      <span
                        aria-hidden
                        className={`pointer-events-none absolute left-2 top-2 z-20 flex h-7 w-7 items-center justify-center rounded-full border-2 border-white/80 shadow ${isChecked ? "bg-accent text-accent-foreground" : "bg-black/50"}`}
                      >
                        {isChecked && <Check className="h-4 w-4" />}
                      </span>
                    )}
                    <p className="truncate text-xs text-muted-foreground">
                      {m.excluded
                        ? "Excluded"
                        : unresolved
                          ? "Needs a TMDB match"
                          : m.local && m.sources.length
                            ? "Source + local"
                            : m.local
                              ? "Added manually"
                              : `${m.sources.length} source${m.sources.length === 1 ? "" : "s"}`}
                    </p>
                    {canManage && !selecting && (
                      <div className="flex flex-wrap gap-1">
                        {unresolved && !m.excluded && (
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-8 px-2 text-xs"
                            onClick={() =>
                              setPicker({ media: m.media_type, term: m.title })
                            }
                          >
                            Find match
                          </Button>
                        )}
                        {m.local && !m.sources.length ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 px-2 text-xs"
                            disabled={mutation.isPending}
                            onClick={() =>
                              mutation.mutate({
                                path: `/members/${encodeURIComponent(m.id)}`,
                                method: "DELETE",
                                body: undefined,
                              })
                            }
                          >
                            Remove
                          </Button>
                        ) : (
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 px-2 text-xs"
                            disabled={mutation.isPending}
                            onClick={() =>
                              mutation.mutate({
                                path: `/members/${encodeURIComponent(m.id)}`,
                                method: "PATCH",
                                body: { excluded: !m.excluded },
                              })
                            }
                          >
                            {m.excluded ? "Restore" : "Exclude"}
                          </Button>
                        )}
                      </div>
                    )}
                  </article>
                );
              })}
            </div>
            {!visible?.length && (
              <div className="space-y-3 py-8 text-center text-muted-foreground">
                <p>
                  {c.members.length
                    ? "No members match these filters."
                    : "This collection is empty."}
                </p>
                {canManage && !c.members.length && (
                  <Button onClick={() => setPicker({})}>
                    <Plus className="mr-2 h-4 w-4" />
                    Add titles
                  </Button>
                )}
              </div>
            )}
            {selecting && (
              <div
                role="toolbar"
                aria-label="Selected titles"
                className="sticky bottom-4 z-30 mx-auto flex w-fit max-w-full flex-wrap items-center justify-center gap-2 rounded-xl border bg-background/95 p-2 shadow-xl backdrop-blur"
              >
                <span className="px-2 text-sm font-medium">
                  {checked.size} selected
                </span>
                <Button
                  size="sm"
                  disabled={!ids}
                  onClick={() => setBulk({ kind: "add", ids })}
                >
                  Add missing
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!ids}
                  onClick={() => setBulk({ kind: "search", ids })}
                >
                  Search
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={!ids}
                  onClick={() => setBulk({ kind: "quality", ids })}
                >
                  Set quality
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() =>
                    setChecked(
                      checked.size
                        ? new Set()
                        : new Set(visible?.map((m) => m.id)),
                    )
                  }
                >
                  {checked.size ? "Clear" : "Select all"}
                </Button>
              </div>
            )}
            {canManage && (
              <ManageSheet
                open={managing}
                onOpenChange={setManaging}
                collection={c}
                all={all}
                onChanged={changed}
                onApplyQuality={() => setBulk({ kind: "quality" })}
                onDeleted={() => {
                  setManaging(false);
                  changed();
                  onBack();
                }}
                onMerged={(retained) => onOpen(retained.id)}
              />
            )}
            {picker && (
              <CollectionTitlePicker
                open
                collection={c}
                initialMedia={picker.media}
                initialTerm={picker.term}
                onOpenChange={(open) => !open && setPicker(undefined)}
                onSaved={changed}
              />
            )}
            {bulk && (
              <CollectionBulkSheet
                collection={c}
                kind={bulk.kind}
                memberIds={bulk.ids}
                onChanged={changed}
                onOpenChange={(open) => !open && setBulk(undefined)}
              />
            )}
          </>
        )
      )}
      {selected &&
        (active?.in_library && active.media_id ? (
          <LibraryMediaSheet
            target={{ type: active.media_type, id: active.media_id }}
            onChanged={changed}
            onClose={() => {
              setSelected(null);
              changed();
            }}
          />
        ) : (
          <TitleDialog
            open
            defaults={{
              search_on_add: false,
              ...(c?.quality_profile_id
                ? { quality_profile_id: c.quality_profile_id }
                : {}),
            }}
            identity={memberIdentity(active || selected)}
            onOpenChange={(open) => {
              if (!open) {
                setSelected(null);
                changed();
              }
            }}
            onAdded={changed}
            requestAction={
              canManage ? undefined : (
                <p className="text-sm text-muted-foreground">
                  Ask an administrator to add this title.
                </p>
              )
            }
          />
        ))}
    </section>
  );
}

function ManageSheet({
  open,
  onOpenChange,
  collection: c,
  all,
  onChanged,
  onApplyQuality,
  onDeleted,
  onMerged,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  collection: LoomCollection;
  all: LoomCollection[];
  onChanged: () => void;
  onApplyQuality: () => void;
  onDeleted: () => void;
  onMerged: (retained: LoomCollection) => void;
}) {
  const [mergeFrom, setMergeFrom] = useState("");
  const [confirmMerge, setConfirmMerge] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const from = all.find((candidate) => candidate.id === mergeFrom);
  const refresh = useMutation({
    mutationFn: () =>
      collectionRequest<LoomCollection>(collectionPath(c.id, "/refresh"), {}),
    onSuccess: onChanged,
  });
  const merge = useMutation({
    mutationFn: () =>
      collectionRequest<LoomCollection>(collectionPath(c.id, "/merge"), {
        from_id: from?.id,
        revision: c.revision,
        from_revision: from?.revision,
      }),
    onSuccess: (retained) => {
      setMergeFrom("");
      setConfirmMerge(false);
      onChanged();
      onMerged(retained);
    },
  });
  const remove = useMutation({
    mutationFn: () =>
      collectionRequest<{ deleted: string }>(
        collectionPath(c.id, `?revision=${c.revision}`),
        undefined,
        "DELETE",
      ),
    onSuccess: () => {
      setDeleting(false);
      onDeleted();
    },
  });
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full min-w-0 max-w-full overflow-y-auto overflow-x-hidden p-4 sm:max-w-lg sm:p-6">
        <SheetHeader className="pr-8 text-left">
          <SheetTitle>Manage collection</SheetTitle>
          <SheetDescription>
            Changes here never add, remove or re-download library titles.
          </SheetDescription>
        </SheetHeader>
        <div className="mt-6 space-y-8">
          <ManageSection title="Details">
            <CollectionForm
              key={c.revision}
              collection={c}
              onSaved={onChanged}
              onApplyQuality={() => {
                onOpenChange(false);
                onApplyQuality();
              }}
            />
          </ManageSection>
          <ManageSection title="Sources">
            {c.sources.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No external sources. Members are managed by hand.
              </p>
            ) : (
              <ul className="space-y-2">
                {c.sources.map((s) => (
                  <li
                    key={`${s.provider}:${s.kind}:${s.external_id}`}
                    className="min-w-0 rounded-md border px-3 py-2 text-sm"
                  >
                    <span className="font-medium uppercase">{s.provider}</span>{" "}
                    {s.kind} {s.external_id}
                    <span className="block text-xs text-muted-foreground">
                      {s.last_sync
                        ? `Synced ${new Date(s.last_sync).toLocaleString()}`
                        : "Not synced"}
                    </span>
                    {s.error && (
                      <p
                        role="alert"
                        className="mt-1 break-words text-destructive"
                      >
                        {s.error}
                      </p>
                    )}
                  </li>
                ))}
              </ul>
            )}
            {c.sources.length > 0 && (
              <Button
                variant="outline"
                size="sm"
                disabled={refresh.isPending}
                onClick={() => refresh.mutate()}
              >
                <RefreshCw
                  className={`mr-2 h-4 w-4 ${refresh.isPending ? "animate-spin" : ""}`}
                />
                Refresh sources
              </Button>
            )}
            <ErrorMessage error={refresh.error} />
            <SourceForm collection={c} onSaved={onChanged} />
          </ManageSection>
          <ManageSection title="Merge">
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (from) merge.mutate();
              }}
            >
              <Field label={`Merge another collection into ${c.name}`}>
                <select
                  className={SELECT}
                  value={mergeFrom}
                  onChange={(e) => {
                    setMergeFrom(e.target.value);
                    setConfirmMerge(false);
                  }}
                >
                  <option value="">Choose collection</option>
                  {all
                    .filter((candidate) => candidate.id !== c.id)
                    .map((candidate) => (
                      <option key={candidate.id} value={candidate.id}>
                        {candidate.name}
                      </option>
                    ))}
                </select>
              </Field>
              {from && (
                <>
                  <p className="text-xs text-muted-foreground">
                    Combines sources and exact-ID members. Local additions and
                    exclusions survive; the old link redirects here.
                  </p>
                  <label className="flex gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={confirmMerge}
                      onChange={(e) => setConfirmMerge(e.target.checked)}
                    />
                    Merge {from.name} into {c.name}
                  </label>
                  <Button size="sm" disabled={!confirmMerge || merge.isPending}>
                    Merge
                  </Button>
                </>
              )}
              <ErrorMessage error={merge.error} />
            </form>
          </ManageSection>
          <ManageSection title="Delete">
            <p className="text-sm text-muted-foreground">
              Removes this collection only. Library titles, monitoring and files
              are untouched, and automatic discovery won't recreate it.
            </p>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => setDeleting(true)}
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete collection
            </Button>
          </ManageSection>
        </div>
        <Dialog open={deleting} onOpenChange={setDeleting}>
          <DialogContent className="w-[calc(100vw-2rem)] max-w-md">
            <DialogHeader>
              <DialogTitle className="break-words">
                Delete {c.name}?
              </DialogTitle>
              <DialogDescription>
                {c.total} member{c.total === 1 ? "" : "s"} will leave this
                collection. {c.in_library} library title
                {c.in_library === 1 ? "" : "s"} and their files stay exactly as
                they are.
                {c.sources.length > 0 &&
                  " Its sources won't be re-imported automatically; import them again to restore it."}
              </DialogDescription>
            </DialogHeader>
            <ErrorMessage error={remove.error} />
            <DialogFooter>
              <Button variant="outline" onClick={() => setDeleting(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                disabled={remove.isPending}
                onClick={() => remove.mutate()}
              >
                {remove.isPending && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                )}
                Delete collection
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </SheetContent>
    </Sheet>
  );
}

function ManageSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section aria-label={title} className="min-w-0 space-y-3">
      <h3 className="text-sm font-semibold">{title}</h3>
      {children}
    </section>
  );
}

function CollectionForm({
  collection,
  onSaved,
  onApplyQuality,
}: {
  collection?: LoomCollection;
  onSaved: (c: LoomCollection) => void;
  onApplyQuality?: () => void;
}) {
  const [name, setName] = useState(collection?.name ?? "");
  const [overview, setOverview] = useState(collection?.overview ?? "");
  const [profile, setProfile] = useState(collection?.quality_profile_id ?? "");
  const profiles = useQualityProfiles();
  const mutation = useMutation({
    mutationFn: () =>
      collectionRequest<LoomCollection>(
        collection ? collectionPath(collection.id) : "",
        {
          name,
          overview,
          ...(collection
            ? { revision: collection.revision, quality_profile_id: profile }
            : {}),
        },
        collection ? "PATCH" : "POST",
      ),
    onSuccess: onSaved,
  });
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        mutation.mutate();
      }}
    >
      {!collection && (
        <h2 className="font-semibold">Create a local collection</h2>
      )}
      <Field label="Name">
        <Input
          required
          maxLength={300}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Marvel, Naruto, Dragon Ball…"
        />
      </Field>
      <Field label="Description">
        <textarea
          className={`${SELECT} h-20 py-2`}
          maxLength={20000}
          value={overview}
          onChange={(e) => setOverview(e.target.value)}
        />
      </Field>
      {collection && (
        <div className="space-y-1.5">
          <Field label="Quality for new additions">
            <select
              className={SELECT}
              value={profile}
              onChange={(e) => setProfile(e.target.value)}
            >
              <option value="">Use each library's default</option>
              {profiles.data?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </Field>
          <p className="text-xs text-muted-foreground">
            Titles already in your library keep their profile.{" "}
            {collection.quality_profile_id && onApplyQuality && (
              <button
                type="button"
                className="font-medium text-foreground underline underline-offset-2"
                onClick={onApplyQuality}
              >
                Apply to existing members…
              </button>
            )}
          </p>
        </div>
      )}
      <Button disabled={mutation.isPending}>
        {collection ? "Save details" : "Create collection"}
      </Button>
      <ErrorMessage error={mutation.error} />
    </form>
  );
}

function SourceForm({
  collection,
  onSaved,
}: {
  collection?: LoomCollection;
  onSaved: (c: LoomCollection) => void;
}) {
  const [type, setType] = useState("tmdb:collection");
  const [externalID, setExternalID] = useState("");
  const mutation = useMutation({
    mutationFn: () => {
      const [provider, kind] = type.split(":");
      const source = {
        provider,
        kind,
        external_id: externalID.trim(),
      } as CollectionSource;
      return collectionRequest<LoomCollection>(
        collection ? collectionPath(collection.id, "/sources") : "",
        collection ? source : { source },
      );
    },
    onSuccess: (c) => {
      setExternalID("");
      onSaved(c);
    },
  });
  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        mutation.mutate();
      }}
    >
      <h2 className="font-semibold">
        {collection ? "Attach another source" : "Import a source collection"}
      </h2>
      <Field label="Source type">
        <select
          className={SELECT}
          value={type}
          onChange={(e) => setType(e.target.value)}
        >
          <option value="tmdb:collection">TMDB movie collection</option>
          <option value="tmdb:list">TMDB mixed list (v4)</option>
          <option value="tvdb:list">TVDB list</option>
          <option value="imdb:list">IMDb public list</option>
        </select>
      </Field>
      <Field label="External source ID">
        <Input
          required
          maxLength={20}
          value={externalID}
          onChange={(e) => setExternalID(e.target.value)}
          placeholder={type.startsWith("imdb") ? "ls123456789" : "12345"}
          pattern={type.startsWith("imdb") ? "ls[0-9]+" : "[1-9][0-9]*"}
        />
      </Field>
      <p className="text-xs text-muted-foreground">
        Only exact source references are linked automatically. If this source
        already belongs to another collection, merge the collections instead.
      </p>
      <Button disabled={mutation.isPending}>
        {mutation.isPending
          ? "Reading source…"
          : collection
            ? "Attach source"
            : "Import collection"}
      </Button>
      <ErrorMessage error={mutation.error} />
    </form>
  );
}

function Loading() {
  return (
    <p role="status" className="flex items-center gap-2 py-6">
      <Loader2 className="h-4 w-4 animate-spin" />
      Loading collections…
    </p>
  );
}
function ErrorMessage({ error }: { error: Error | null | undefined }) {
  return error ? (
    <p role="alert" className="text-sm text-destructive">
      {error.message}
    </p>
  ) : null;
}
function Retry({ error, retry }: { error: Error; retry: () => void }) {
  return (
    <div className="space-y-3">
      <ErrorMessage error={error} />
      <Button variant="outline" onClick={retry}>
        Retry
      </Button>
    </div>
  );
}
