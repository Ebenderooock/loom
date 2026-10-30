import { useEffect, useState } from "react";
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useQualityProfiles } from "@/lib/quality-profiles-api";
import { Loader2, RotateCcw } from "lucide-react";
import {
  SKIP_REASONS,
  collectionPath,
  collectionRequest,
  type AddSettings,
  type BulkItem,
  type BulkPlan,
  type CollectionJob,
  type LoomCollection,
} from "@/lib/collections-api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";

export type BulkKind = "add" | "search" | "quality";
type Media = "movie" | "series";

const COPY: Record<
  BulkKind,
  { title: string; action: string; description: string }
> = {
  add: {
    title: "Add missing titles",
    action: "Add",
    description:
      "Adds eligible titles to your library with these settings. Nothing changes until you confirm.",
  },
  search: {
    title: "Search for missing media",
    action: "Search",
    description:
      "Searches monitored titles that are missing files. Search never adds titles to the library.",
  },
  quality: {
    title: "Apply quality profile",
    action: "Apply to",
    description:
      "Changes the quality profile of the selected library titles only.",
  },
};

const STATUS_STYLE: Record<string, string> = {
  eligible: "border-emerald-500/40 text-emerald-600 dark:text-emerald-400",
  done: "border-emerald-500/40 text-emerald-600 dark:text-emerald-400",
  failed: "border-destructive/50 text-destructive",
  running: "border-accent/50",
  queued: "",
  skipped: "text-muted-foreground",
};

const SELECT =
  "h-10 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/**
 * Previews eligibility for selected members (or the whole collection) and
 * runs only after explicit confirmation. Add/search run as background jobs
 * with per-item results and retry of failed titles.
 */
export function CollectionBulkSheet({
  collection,
  kind,
  memberIds,
  onOpenChange,
  onChanged,
}: {
  collection: LoomCollection;
  kind: BulkKind;
  memberIds?: string[];
  onOpenChange: (open: boolean) => void;
  onChanged: () => void;
}) {
  const copy = COPY[kind];
  return (
    <Sheet open onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full min-w-0 max-w-full flex-col gap-4 overflow-hidden p-4 sm:max-w-xl sm:p-6">
        <SheetHeader className="pr-8 text-left">
          <SheetTitle>{copy.title}</SheetTitle>
          <SheetDescription>
            {memberIds
              ? `${memberIds.length} selected in ${collection.name}. `
              : `All of ${collection.name}. `}
            {copy.description}
          </SheetDescription>
        </SheetHeader>
        <BulkBody
          collection={collection}
          kind={kind}
          memberIds={memberIds}
          onChanged={onChanged}
        />
      </SheetContent>
    </Sheet>
  );
}

function BulkBody({
  collection,
  kind,
  memberIds: initialIds,
  onChanged,
}: {
  collection: LoomCollection;
  kind: BulkKind;
  memberIds?: string[];
  onChanged: () => void;
}) {
  const [memberIds, setMemberIds] = useState(initialIds);
  const [settings, setSettings] = useState<Partial<Record<Media, AddSettings>>>(
    {},
  );
  const [unreleased, setUnreleased] = useState(false);
  const [profile, setProfile] = useState(collection.quality_profile_id ?? "");
  const [jobId, setJobId] = useState<string>();
  const [applied, setApplied] = useState<BulkPlan>();
  const path = kind === "quality" ? "/quality" : `/bulk/${kind}`;
  const body = (execute: boolean) => ({
    ...(memberIds ? { member_ids: memberIds } : { all: true }),
    execute,
    ...(kind === "add"
      ? {
          include_unreleased: unreleased,
          movie: settings.movie ?? {},
          series: settings.series ?? {},
        }
      : {}),
    ...(kind === "quality" ? { quality_profile_id: profile } : {}),
  });
  const preview = useQuery({
    queryKey: [
      "collection-bulk",
      collection.id,
      kind,
      memberIds,
      settings,
      unreleased,
      profile,
    ],
    queryFn: ({ signal }) =>
      collectionRequest<BulkPlan>(
        collectionPath(collection.id, path),
        body(false),
        "POST",
        signal,
      ),
    enabled: !jobId && !applied && (kind !== "quality" || !!profile),
    placeholderData: keepPreviousData,
    retry: false,
  });
  const run = useMutation({
    mutationFn: () =>
      collectionRequest<BulkPlan>(
        collectionPath(collection.id, path),
        body(true),
      ),
    onSuccess: (plan) => {
      if (plan.job) setJobId(plan.job.id);
      else setApplied(plan);
      onChanged();
    },
  });
  const job = useQuery({
    queryKey: ["collection-job", collection.id, jobId],
    queryFn: ({ signal }) =>
      collectionRequest<CollectionJob>(
        collectionPath(collection.id, `/jobs/${jobId}`),
        undefined,
        "GET",
        signal,
      ),
    enabled: !!jobId,
    refetchInterval: (q) => (q.state.data?.status === "done" ? false : 1500),
  });
  const finished = job.data?.status === "done";
  useEffect(() => {
    if (finished) onChanged();
  }, [finished, onChanged]);
  const plan = applied ?? preview.data;
  const items = job.data?.items ?? plan?.items ?? [];
  const counts = job.data?.counts ?? plan?.counts ?? {};
  const eligible = counts.eligible ?? 0;
  const failed = items.filter((i) => i.status === "failed");
  const types = [...new Set(items.map((i) => i.media_type))].sort() as Media[];
  const profileName = (id?: string) =>
    plan?.profiles?.find((p) => p.id === id)?.name ?? id ?? "—";
  const retry = () => {
    setMemberIds(failed.map((i) => i.member_id));
    setJobId(undefined);
    setApplied(undefined);
    run.reset();
  };
  const set = (media: Media, patch: AddSettings) =>
    setSettings((prev) => ({
      ...prev,
      [media]: { ...prev[media], ...patch },
    }));
  const executing = !!jobId || !!applied;
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      {!executing && kind === "add" && plan?.settings && (
        <div className="space-y-3">
          {types.map((media) => {
            const resolved = plan.settings?.[media] ?? {};
            const libraries = plan.libraries?.[media] ?? [];
            const label = media === "movie" ? "Movies" : "TV shows";
            return (
              <fieldset
                key={media}
                className="grid min-w-0 gap-3 rounded-lg border p-3 sm:grid-cols-2"
              >
                <legend className="px-1 text-sm font-medium">{label}</legend>
                <label className="flex min-w-0 flex-col gap-1.5 text-sm">
                  Library
                  <select
                    className={SELECT}
                    value={resolved.library_id ?? ""}
                    onChange={(e) => set(media, { library_id: e.target.value })}
                  >
                    {!resolved.library_id && (
                      <option value="">Choose library</option>
                    )}
                    {libraries.map((l) => (
                      <option key={l.id} value={l.id}>
                        {l.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex min-w-0 flex-col gap-1.5 text-sm">
                  Quality profile
                  <select
                    className={SELECT}
                    value={resolved.quality_profile_id ?? ""}
                    onChange={(e) =>
                      set(media, { quality_profile_id: e.target.value })
                    }
                  >
                    {!resolved.quality_profile_id && (
                      <option value="">Choose profile</option>
                    )}
                    {plan.profiles?.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={resolved.monitored ?? true}
                    onChange={(e) =>
                      set(media, { monitored: e.target.checked })
                    }
                  />
                  Monitor
                </label>
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={!!resolved.search}
                    onChange={(e) => set(media, { search: e.target.checked })}
                  />
                  Search after adding
                </label>
              </fieldset>
            );
          })}
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={unreleased}
              onChange={(e) => setUnreleased(e.target.checked)}
            />
            Include unreleased titles
          </label>
        </div>
      )}
      {!executing && kind === "quality" && (
        <ProfileSelect
          collection={collection}
          value={profile}
          onChange={setProfile}
          options={plan?.profiles}
        />
      )}
      {(preview.error || run.error || job.error) && (
        <p role="alert" className="text-sm text-destructive">
          {(preview.error ?? run.error ?? job.error)?.message}
        </p>
      )}
      <Summary counts={counts} />
      {plan?.note && (
        <p className="text-xs text-muted-foreground">{plan.note}</p>
      )}
      <ul
        aria-label="Titles"
        className="min-h-0 flex-1 space-y-1 overflow-y-auto overflow-x-hidden"
      >
        {preview.isFetching && !plan && (
          <li role="status" className="flex items-center gap-2 text-sm">
            <Loader2 className="h-4 w-4 animate-spin" /> Checking titles…
          </li>
        )}
        {items.map((it) => (
          <ItemRow
            key={it.member_id}
            item={it}
            quality={kind === "quality"}
            profileName={profileName}
          />
        ))}
      </ul>
      <div className="flex flex-wrap items-center justify-end gap-2 border-t pt-3">
        {executing ? (
          <>
            <span
              role="status"
              className="mr-auto text-sm text-muted-foreground"
            >
              {job.data && !finished
                ? "Running in the background…"
                : "Finished"}
            </span>
            {failed.length > 0 && (finished || applied) && (
              <Button variant="outline" onClick={retry}>
                <RotateCcw className="mr-2 h-4 w-4" />
                Retry {failed.length} failed
              </Button>
            )}
          </>
        ) : (
          <Button
            disabled={eligible === 0 || run.isPending || preview.isFetching}
            onClick={() => run.mutate()}
          >
            {run.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {COPY[kind].action} {eligible} {eligible === 1 ? "title" : "titles"}
          </Button>
        )}
      </div>
    </div>
  );
}

export function ProfileSelect({
  collection,
  value,
  onChange,
  options,
}: {
  collection: LoomCollection;
  value: string;
  onChange: (v: string) => void;
  options?: { id: string; name: string }[];
}) {
  const profiles = useQualityProfiles();
  const list = options ?? profiles.data ?? [];
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-sm">
      Quality profile
      <select
        className={SELECT}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        {!value && <option value="">Choose profile</option>}
        {list.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
            {p.id === collection.quality_profile_id
              ? " (collection default)"
              : ""}
          </option>
        ))}
      </select>
    </label>
  );
}

function Summary({ counts }: { counts: BulkPlan["counts"] }) {
  const parts = (
    ["eligible", "queued", "running", "done", "failed", "skipped"] as const
  ).filter((k) => counts[k]);
  if (!parts.length) return null;
  return (
    <p className="flex flex-wrap gap-2 text-sm" aria-label="Summary">
      {parts.map((k) => (
        <Badge key={k} variant="outline" className={STATUS_STYLE[k]}>
          {counts[k]} {k === "eligible" ? "ready" : k}
        </Badge>
      ))}
    </p>
  );
}

function ItemRow({
  item,
  quality,
  profileName,
}: {
  item: BulkItem;
  quality: boolean;
  profileName: (id?: string) => string;
}) {
  const label =
    item.status === "skipped"
      ? (SKIP_REASONS[item.reason ?? ""] ?? item.reason)
      : item.status === "eligible"
        ? "Ready"
        : item.status;
  return (
    <li className="flex min-w-0 items-start gap-3 rounded-md px-2 py-1.5 text-sm">
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">
          {item.title}
          {item.year ? (
            <span className="font-normal text-muted-foreground">
              {" "}
              ({item.year})
            </span>
          ) : null}
        </span>
        {quality && item.current_profile_id && (
          <span className="block text-xs text-muted-foreground">
            {profileName(item.current_profile_id)} →{" "}
            {profileName(item.new_profile_id)}
          </span>
        )}
        {item.missing_episodes ? (
          <span className="block text-xs text-muted-foreground">
            {item.missing_episodes} missing episodes
          </span>
        ) : null}
        {item.status === "failed" && item.detail && (
          <span className="block break-words text-xs text-destructive">
            {item.detail}
          </span>
        )}
      </span>
      <Badge
        variant="outline"
        className={`shrink-0 capitalize ${STATUS_STYLE[item.status] ?? ""}`}
      >
        {label}
      </Badge>
    </li>
  );
}
