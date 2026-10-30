import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Layers, Loader2, Plus, RefreshCw } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import { usePageHeader } from "@/hooks/use-page-header";
import { useLibraries } from "@/lib/libraries-api";
import {
  collectionKeys,
  collectionRequest,
  matchesMember,
  memberIdentity,
  type CollectionMember,
  type CollectionSource,
  type LoomCollection,
} from "@/lib/collections-api";
import { LibraryMediaSheet } from "@/components/library-media-sheet";
import { TitleDialog } from "@/components/titles/title-experience";
import { titleImage } from "@/components/titles/title-metadata";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";

const GRID =
  "grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6";

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
    mutationFn: () =>
      collectionRequest<{ warnings: string[] }>("/discover", {}),
    onSettled: invalidate,
  });
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
                  Find from library
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
            error={discover.error ?? importSet.error ?? libraries.error}
          />
          {discover.data?.warnings.map((warning) => (
            <p role="status" key={warning} className="text-sm text-amber-600">
              {warning}
            </p>
          ))}
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
  onOpen: (id: string) => void;
}) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<CollectionMember | null>(null);
  const [library, setLibrary] = useState("");
  const [media, setMedia] = useState("");
  const [status, setStatus] = useState("");
  const [editing, setEditing] = useState(false);
  const [mergeFrom, setMergeFrom] = useState("");
  const [showMerge, setShowMerge] = useState(false);
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
  const changed = () => {
    void qc.invalidateQueries({ queryKey: collectionKeys.all });
    void qc.invalidateQueries({ queryKey: ["discovery"] });
    void qc.invalidateQueries({ queryKey: ["title-membership"] });
  };
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
        `/${detail.data?.id ?? id}${path}`,
        body,
        method,
      ),
    onSuccess: () => {
      changed();
      setShowMerge(false);
    },
  });
  const c = detail.data;
  const from = all.find((candidate) => candidate.id === mergeFrom);
  const active = selected && c?.members.find((m) => m.id === selected.id);
  return (
    <section className="space-y-5" aria-label="Collection details">
      <Button variant="ghost" onClick={onBack}>
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
            <header className="flex flex-wrap justify-between gap-4">
              <div className="max-w-3xl space-y-2">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {c.name}
                </h1>
                <p className="text-sm text-muted-foreground">{c.overview}</p>
                <p className="font-medium">
                  {c.in_library} of {c.total} added to Loom
                </p>
                <p className="text-xs text-muted-foreground">
                  Membership, monitoring and imported files are separate. Open
                  an added title to manage it without leaving this collection.
                </p>
              </div>
              {canManage && (
                <div className="flex flex-wrap items-start gap-2">
                  <Button
                    variant="outline"
                    onClick={() => setEditing(!editing)}
                  >
                    Edit collection
                  </Button>
                  <Button
                    variant="outline"
                    disabled={mutation.isPending || !c.sources.length}
                    onClick={() =>
                      mutation.mutate({ path: "/refresh", body: {} })
                    }
                  >
                    <RefreshCw className="mr-2 h-4 w-4" />
                    {mutation.isPending ? "Working…" : "Refresh sources"}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => setShowMerge(!showMerge)}
                  >
                    Merge collections
                  </Button>
                </div>
              )}
            </header>
            <ErrorMessage error={mutation.error} />
            {showMerge && canManage && (
              <form
                className="space-y-3 rounded-lg border p-4"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (from)
                    mutation.mutate({
                      path: "/merge",
                      body: {
                        from_id: from.id,
                        revision: c.revision,
                        from_revision: from.revision,
                      },
                    });
                }}
              >
                <Field label={`Merge another collection into ${c.name}`}>
                  <select
                    className={SELECT}
                    required
                    value={mergeFrom}
                    onChange={(e) => setMergeFrom(e.target.value)}
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
                <p className="text-sm text-muted-foreground">
                  Combines sources and exact-ID members. Local additions and
                  exclusions survive. The old link redirects to this collection.
                  This cannot be undone automatically.
                </p>
                <label className="flex gap-2 text-sm">
                  <input type="checkbox" required key={mergeFrom} />I confirm
                  these collections should be one.
                </label>
                <Button disabled={!from || mutation.isPending}>
                  Merge into {c.name}
                </Button>
              </form>
            )}
            {editing && canManage && (
              <div className="grid gap-6 rounded-lg border p-4 md:grid-cols-2">
                <CollectionForm
                  collection={c}
                  onSaved={() => {
                    changed();
                    setEditing(false);
                  }}
                />
                <SourceForm collection={c} onSaved={changed} />
                <MemberForm collection={c} onSaved={changed} />
              </div>
            )}
            <section aria-label="Collection sources" className="space-y-2">
              {c.sources.length === 0 && (
                <p className="text-xs text-muted-foreground">
                  Local collection — no external sources.
                </p>
              )}
              {c.sources.map((s) => (
                <div
                  key={`${s.provider}:${s.kind}:${s.external_id}`}
                  className="rounded-md border px-3 py-2 text-sm"
                >
                  <span className="font-medium uppercase">{s.provider}</span>{" "}
                  {s.kind} {s.external_id}
                  <span className="ml-3 text-xs text-muted-foreground">
                    {s.last_sync
                      ? `Last synced ${new Date(s.last_sync).toLocaleString()}`
                      : "Not synced"}
                  </span>
                  {s.error && (
                    <p role="alert" className="mt-1 text-destructive">
                      {s.error}
                    </p>
                  )}
                </div>
              ))}
            </section>
            <Filters
              {...{ library, setLibrary, media, setMedia, status, setStatus }}
            />
            <div className={GRID}>
              {c.members
                .filter((m) => matchesMember(m, library, media, status))
                .map((m) => (
                  <article
                    key={m.id}
                    className="overflow-hidden rounded-lg border bg-card"
                  >
                    <button
                      type="button"
                      className="block w-full text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                      onClick={() => setSelected(m)}
                      aria-label={`${m.in_library ? "Manage" : "View"} ${m.title}`}
                    >
                      <div className="aspect-[2/3] bg-muted">
                        {m.poster_path ? (
                          <img
                            src={titleImage(m.poster_path) ?? ""}
                            alt=""
                            className="h-full w-full object-cover"
                            loading="lazy"
                          />
                        ) : (
                          <Layers
                            className="h-full w-full p-10 text-muted-foreground"
                            aria-hidden
                          />
                        )}
                      </div>
                      <div className="space-y-2 p-3">
                        <h2 className="text-sm font-semibold">{m.title}</h2>
                        <p className="text-xs text-muted-foreground">
                          {m.media_type === "movie" ? "Movie" : "TV show"}
                          {m.year ? ` · ${m.year}` : ""}
                        </p>
                        <Badge variant={m.in_library ? "secondary" : "outline"}>
                          {m.in_library ? "Added to Loom" : "Not in library"}
                        </Badge>
                        {m.in_library && (
                          <p className="text-xs text-muted-foreground">
                            {m.monitored ? "Monitored" : "Unmonitored"} ·{" "}
                            {m.has_file === undefined
                              ? "Files unknown"
                              : m.has_file
                                ? "Imported files"
                                : "No imported files"}
                          </p>
                        )}
                        <p className="text-xs text-muted-foreground">
                          {m.local
                            ? "Local addition"
                            : `${m.sources.length} source${m.sources.length === 1 ? "" : "s"}`}
                        </p>
                        <span className="block text-xs font-medium text-accent">
                          {m.in_library
                            ? "Open management"
                            : "Details & add settings"}
                        </span>
                      </div>
                    </button>
                    {canManage && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="m-2"
                        disabled={mutation.isPending}
                        onClick={() =>
                          mutation.mutate({
                            path: `/members/${m.id}`,
                            method: "PATCH",
                            body: { excluded: !m.excluded },
                          })
                        }
                      >
                        {m.excluded ? "Restore member" : "Exclude locally"}
                      </Button>
                    )}
                  </article>
                ))}
            </div>
            {!c.members.some((m) =>
              matchesMember(m, library, media, status),
            ) && (
              <p className="py-8 text-center text-muted-foreground">
                No members match these filters. Edit the collection to add a
                source or a local member.
              </p>
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
            defaults={{ search_on_add: false }}
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

function CollectionForm({
  collection,
  onSaved,
}: {
  collection?: LoomCollection;
  onSaved: (c: LoomCollection) => void;
}) {
  const [name, setName] = useState(collection?.name ?? "");
  const [overview, setOverview] = useState(collection?.overview ?? "");
  const mutation = useMutation({
    mutationFn: () =>
      collectionRequest<LoomCollection>(
        collection ? `/${collection.id}` : "",
        {
          name,
          overview,
          ...(collection ? { revision: collection.revision } : {}),
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
      <h2 className="font-semibold">
        {collection ? "Collection details" : "Create a local collection"}
      </h2>
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
      <Button disabled={mutation.isPending}>
        {collection ? "Save changes" : "Create collection"}
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
        collection ? `/${collection.id}/sources` : "",
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

function MemberForm({
  collection,
  onSaved,
}: {
  collection: LoomCollection;
  onSaved: (c: LoomCollection) => void;
}) {
  const [title, setTitle] = useState("");
  const [media, setMedia] = useState("movie");
  const [provider, setProvider] = useState("tmdb");
  const [externalID, setExternalID] = useState("");
  const mutation = useMutation({
    mutationFn: () =>
      collectionRequest<LoomCollection>(`/${collection.id}/members`, {
        title: title.trim(),
        media_type: media,
        external_ids: { [provider]: externalID.trim() },
      }),
    onSuccess: (c) => {
      setTitle("");
      setExternalID("");
      onSaved(c);
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    mutation.mutate();
  };
  return (
    <form className="space-y-3" onSubmit={submit}>
      <h2 className="font-semibold">Add a local member</h2>
      <Field label="Title">
        <Input
          required
          maxLength={500}
          value={title}
          onChange={(e) => setTitle(e.target.value)}
        />
      </Field>
      <Field label="Member media type">
        <select
          className={SELECT}
          value={media}
          onChange={(e) => setMedia(e.target.value)}
        >
          <option value="movie">Movie</option>
          <option value="series">TV show</option>
        </select>
      </Field>
      <Field label="Title ID provider">
        <select
          className={SELECT}
          value={provider}
          onChange={(e) => setProvider(e.target.value)}
        >
          <option value="tmdb">TMDB</option>
          <option value="imdb">IMDb</option>
          <option value="tvdb">TVDB</option>
        </select>
      </Field>
      <Field label="External title ID">
        <Input
          required
          maxLength={20}
          value={externalID}
          onChange={(e) => setExternalID(e.target.value)}
          pattern={provider === "imdb" ? "tt[0-9]+" : "[1-9][0-9]*"}
        />
      </Field>
      <p className="text-xs text-muted-foreground">
        Adds membership only, not the title to a library. TVDB-only movies need
        an IMDb or TMDB mapping to use the shared add form.
      </p>
      <Button disabled={mutation.isPending}>Add member</Button>
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
