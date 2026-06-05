import * as React from "react";
import { usePageHeader } from "@/hooks/use-page-header";
import { useAuth } from "@/hooks/use-auth";
import { apiFetch } from "@/lib/fetch";
import {
  useMyRequests,
  useAllRequests,
  useCreateRequest,
  useApproveRequest,
  useRejectRequest,
  useQuotaStatus,
  useQuotaConfig,
  useUpdateQuotaConfig,
  type MediaRequest,
  type MediaQuota,
  type RequestMediaType,
  type RequestStatus,
} from "@/lib/requests-api";
import { useLibraries } from "@/lib/libraries-api";
import { useQualityProfiles } from "@/lib/quality-profiles-api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Check, Loader2, Plus, Search, X, Inbox, Gauge } from "lucide-react";
import { toast } from "sonner";

const TMDB_IMG = "https://image.tmdb.org/t/p";

interface LookupResult {
  tmdbId: string;
  title: string;
  year?: number;
  posterPath?: string;
  overview?: string;
}

const STATUS_STYLES: Record<RequestStatus, { label: string; className: string }> = {
  pending: { label: "Pending", className: "bg-amber-500/15 text-amber-500 border-amber-500/30" },
  approving: { label: "Approving", className: "bg-blue-500/15 text-blue-500 border-blue-500/30" },
  approved: { label: "Approved", className: "bg-emerald-500/15 text-emerald-500 border-emerald-500/30" },
  available: { label: "Available", className: "bg-emerald-500/15 text-emerald-500 border-emerald-500/30" },
  rejected: { label: "Rejected", className: "bg-red-500/15 text-red-500 border-red-500/30" },
  failed: { label: "Failed", className: "bg-red-500/15 text-red-500 border-red-500/30" },
};

function StatusBadge({ status }: { status: RequestStatus }) {
  const s = STATUS_STYLES[status] ?? { label: status, className: "" };
  return (
    <Badge variant="outline" className={s.className}>
      {s.label}
    </Badge>
  );
}

async function lookup(
  mediaType: RequestMediaType,
  term: string,
  signal?: AbortSignal,
): Promise<LookupResult[]> {
  if (mediaType === "movie") {
    const res = await apiFetch(
      `/api/v1/movies/lookup?term=${encodeURIComponent(term)}`,
      { signal },
    );
    if (!res.ok) return [];
    const data = (await res.json()) as Array<{
      tmdb_id?: string;
      title: string;
      year?: number;
      poster_path?: string;
      overview?: string;
    }>;
    return (data ?? [])
      .filter((m) => m.tmdb_id)
      .map((m) => ({
        tmdbId: m.tmdb_id!,
        title: m.title,
        year: m.year,
        posterPath: m.poster_path,
        overview: m.overview,
      }));
  }
  const res = await apiFetch(
    `/api/v1/series/search?q=${encodeURIComponent(term)}`,
    { signal },
  );
  if (!res.ok) return [];
  const body = (await res.json()) as {
    data?: Array<{
      tmdbId?: string;
      title: string;
      year?: number;
      posterPath?: string;
      overview?: string;
    }>;
  };
  return (body.data ?? [])
    .filter((s) => s.tmdbId)
    .map((s) => ({
      tmdbId: s.tmdbId!,
      title: s.title,
      year: s.year,
      posterPath: s.posterPath,
      overview: s.overview,
    }));
}

function SearchAndRequest() {
  const [mediaType, setMediaType] = React.useState<RequestMediaType>("movie");
  const [term, setTerm] = React.useState("");
  const [results, setResults] = React.useState<LookupResult[]>([]);
  const [searching, setSearching] = React.useState(false);
  const [requested, setRequested] = React.useState<Record<string, string>>({});
  const debounceRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const create = useCreateRequest();
  const { data: mine } = useMyRequests();

  // Mark titles already requested by this user so we can disable them.
  const existing = React.useMemo(() => {
    const m: Record<string, RequestStatus> = {};
    for (const r of mine ?? []) m[`${r.media_type}:${r.tmdb_id}`] = r.status;
    return m;
  }, [mine]);

  const runSearch = React.useCallback(
    (mt: RequestMediaType, q: string) => {
      if (q.trim().length < 2) {
        setResults([]);
        return;
      }
      setSearching(true);
      const controller = new AbortController();
      lookup(mt, q, controller.signal)
        .then(setResults)
        .catch(() => setResults([]))
        .finally(() => setSearching(false));
    },
    [],
  );

  const onTermChange = (val: string) => {
    setTerm(val);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => runSearch(mediaType, val), 400);
  };

  const onMediaTypeChange = (mt: RequestMediaType) => {
    setMediaType(mt);
    setResults([]);
    if (term.trim().length >= 2) runSearch(mt, term);
  };

  const submit = async (r: LookupResult) => {
    try {
      await create.mutateAsync({
        media_type: mediaType,
        tmdb_id: r.tmdbId,
        title: r.title,
        year: r.year,
        poster_path: r.posterPath,
        overview: r.overview,
      });
      setRequested((prev) => ({ ...prev, [`${mediaType}:${r.tmdbId}`]: "ok" }));
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Request failed";
      setRequested((prev) => ({ ...prev, [`${mediaType}:${r.tmdbId}`]: msg }));
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Tabs value={mediaType} onValueChange={(v) => onMediaTypeChange(v as RequestMediaType)}>
          <TabsList>
            <TabsTrigger value="movie">Movies</TabsTrigger>
            <TabsTrigger value="series">TV Shows</TabsTrigger>
          </TabsList>
        </Tabs>
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={term}
            onChange={(e) => onTermChange(e.target.value)}
            placeholder={`Search for a ${mediaType === "movie" ? "movie" : "TV show"} to request…`}
            className="pl-9"
          />
        </div>
      </div>

      {searching && (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Searching…
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
        {results.map((r) => {
          const key = `${mediaType}:${r.tmdbId}`;
          const alreadyStatus = existing[key];
          const localResult = requested[key];
          const isRequested = localResult === "ok" || (!!alreadyStatus && alreadyStatus !== "rejected" && alreadyStatus !== "failed");
          const errMsg = localResult && localResult !== "ok" ? localResult : undefined;
          return (
            <div
              key={key}
              className="flex flex-col overflow-hidden rounded-lg border border-border bg-card"
            >
              <div className="aspect-[2/3] bg-muted">
                {r.posterPath ? (
                  <img
                    src={`${TMDB_IMG}/w300${r.posterPath}`}
                    alt={r.title}
                    className="h-full w-full object-cover"
                    loading="lazy"
                  />
                ) : (
                  <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
                    No image
                  </div>
                )}
              </div>
              <div className="flex flex-1 flex-col gap-2 p-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium" title={r.title}>
                    {r.title}
                  </p>
                  {r.year ? (
                    <p className="text-xs text-muted-foreground">{r.year}</p>
                  ) : null}
                </div>
                <Button
                  size="sm"
                  variant={isRequested ? "secondary" : "default"}
                  disabled={isRequested || create.isPending}
                  onClick={() => submit(r)}
                  className="mt-auto w-full"
                >
                  {isRequested ? (
                    <>
                      <Check className="mr-1 h-3.5 w-3.5" /> Requested
                    </>
                  ) : (
                    <>
                      <Plus className="mr-1 h-3.5 w-3.5" /> Request
                    </>
                  )}
                </Button>
                {errMsg && <p className="text-xs text-red-500">{errMsg}</p>}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function RequestRow({
  r,
  admin,
  onApprove,
  onReject,
}: {
  r: MediaRequest;
  admin?: boolean;
  onApprove?: (r: MediaRequest) => void;
  onReject?: (r: MediaRequest) => void;
}) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-border bg-card p-3">
      <div className="h-16 w-11 shrink-0 overflow-hidden rounded bg-muted">
        {r.poster_path ? (
          <img
            src={`${TMDB_IMG}/w92${r.poster_path}`}
            alt={r.title}
            className="h-full w-full object-cover"
            loading="lazy"
          />
        ) : null}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate font-medium" title={r.title}>
            {r.title}
          </p>
          {r.year ? (
            <span className="text-xs text-muted-foreground">{r.year}</span>
          ) : null}
          <Badge variant="outline" className="text-[10px] uppercase">
            {r.media_type === "movie" ? "Movie" : "TV"}
          </Badge>
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <StatusBadge status={r.status} />
          {admin && <span>by {r.username || "unknown"}</span>}
          {r.reason ? <span className="text-red-500">· {r.reason}</span> : null}
        </div>
      </div>
      {admin && r.status === "pending" && (
        <div className="flex shrink-0 gap-2">
          <Button size="sm" variant="outline" onClick={() => onReject?.(r)}>
            <X className="mr-1 h-3.5 w-3.5" /> Reject
          </Button>
          <Button size="sm" onClick={() => onApprove?.(r)}>
            <Check className="mr-1 h-3.5 w-3.5" /> Approve
          </Button>
        </div>
      )}
    </div>
  );
}

function ApproveDialog({
  request,
  onClose,
}: {
  request: MediaRequest | null;
  onClose: () => void;
}) {
  const { data: libraries } = useLibraries();
  const { data: profiles } = useQualityProfiles();
  const approve = useApproveRequest();
  const [libraryId, setLibraryId] = React.useState("");
  const [qpId, setQpId] = React.useState("");
  const [error, setError] = React.useState("");

  const eligibleLibraries = React.useMemo(
    () => (libraries ?? []).filter((l) => l.media_type === request?.media_type),
    [libraries, request],
  );

  // Default the selection whenever a new request opens.
  React.useEffect(() => {
    if (!request) return;
    setError("");
    const lib = eligibleLibraries[0];
    setLibraryId(lib?.id ?? "");
    setQpId(lib?.quality_profile_id || (profiles?.[0]?.id ?? ""));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [request?.id]);

  const onLibraryChange = (id: string) => {
    setLibraryId(id);
    const lib = eligibleLibraries.find((l) => l.id === id);
    if (lib?.quality_profile_id) setQpId(lib.quality_profile_id);
  };

  const submit = async () => {
    if (!request || !libraryId || !qpId) return;
    setError("");
    try {
      await approve.mutateAsync({
        id: request.id,
        qualityProfileId: qpId,
        libraryId,
      });
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to approve");
    }
  };

  return (
    <Dialog open={!!request} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Approve request</DialogTitle>
          <DialogDescription>
            {request?.title}
            {request?.year ? ` (${request.year})` : ""} — choose where to add it.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <label htmlFor="request-approve-library" className="text-sm font-medium">Library</label>
            <Select value={libraryId} onValueChange={onLibraryChange}>
              <SelectTrigger id="request-approve-library">
                <SelectValue placeholder="Select a library" />
              </SelectTrigger>
              <SelectContent>
                {eligibleLibraries.map((l) => (
                  <SelectItem key={l.id} value={l.id}>
                    {l.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {eligibleLibraries.length === 0 && (
              <p className="text-xs text-red-500">
                No {request?.media_type === "movie" ? "movie" : "TV"} library configured.
              </p>
            )}
          </div>
          <div className="space-y-1.5">
            <label htmlFor="request-approve-qp" className="text-sm font-medium">Quality profile</label>
            <Select value={qpId} onValueChange={setQpId}>
              <SelectTrigger id="request-approve-qp">
                <SelectValue placeholder="Select a quality profile" />
              </SelectTrigger>
              <SelectContent>
                {(profiles ?? []).map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {error && <p className="text-sm text-red-500">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={submit}
            disabled={!libraryId || !qpId || approve.isPending}
          >
            {approve.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Approve &amp; add
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RejectDialog({
  request,
  onClose,
}: {
  request: MediaRequest | null;
  onClose: () => void;
}) {
  const reject = useRejectRequest();
  const [reason, setReason] = React.useState("");
  const [error, setError] = React.useState("");

  React.useEffect(() => {
    setReason("");
    setError("");
  }, [request?.id]);

  const submit = async () => {
    if (!request) return;
    setError("");
    try {
      await reject.mutateAsync({ id: request.id, reason });
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to reject");
    }
  };

  return (
    <Dialog open={!!request} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reject request</DialogTitle>
          <DialogDescription>
            {request?.title}
            {request?.year ? ` (${request.year})` : ""}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2 py-2">
          <label htmlFor="reject-reason" className="text-sm font-medium">Reason (optional)</label>
          <Input
            id="reject-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Why is this being rejected?"
          />
          {error && <p className="text-sm text-red-500">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={submit} disabled={reject.isPending}>
            {reject.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Reject
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MyRequests() {
  const { data, isLoading } = useMyRequests();
  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading…</p>;
  }
  if (!data || data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        You haven&apos;t requested anything yet. Search above to make a request.
      </p>
    );
  }
  return (
    <div className="space-y-2">
      {data.map((r) => (
        <RequestRow key={r.id} r={r} />
      ))}
    </div>
  );
}

const STATUS_FILTERS: Array<{ value: RequestStatus | "all"; label: string }> = [
  { value: "pending", label: "Pending" },
  { value: "approved", label: "Approved" },
  { value: "available", label: "Available" },
  { value: "rejected", label: "Rejected" },
  { value: "failed", label: "Failed" },
  { value: "all", label: "All" },
];

function ManageRequests() {
  const [status, setStatus] = React.useState<RequestStatus | "all">("pending");
  const { data, isLoading } = useAllRequests(status);
  const [approving, setApproving] = React.useState<MediaRequest | null>(null);
  const [rejecting, setRejecting] = React.useState<MediaRequest | null>(null);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">Filter</span>
        <Select value={status} onValueChange={(v) => setStatus(v as RequestStatus | "all")}>
          <SelectTrigger className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_FILTERS.map((f) => (
              <SelectItem key={f.value} value={f.value}>
                {f.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : !data || data.length === 0 ? (
        <p className="text-sm text-muted-foreground">No requests in this state.</p>
      ) : (
        <div className="space-y-2">
          {data.map((r) => (
            <RequestRow
              key={r.id}
              r={r}
              admin
              onApprove={setApproving}
              onReject={setRejecting}
            />
          ))}
        </div>
      )}

      <ApproveDialog request={approving} onClose={() => setApproving(null)} />
      <RejectDialog request={rejecting} onClose={() => setRejecting(null)} />
    </div>
  );
}

function QuotaPill({ label, q }: { label: string; q: MediaQuota }) {
  if (q.unlimited) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-md border border-border/60 px-2.5 py-1 text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{label}</span> unlimited
      </span>
    );
  }
  const exhausted = q.remaining <= 0;
  return (
    <span
      className={
        "inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs " +
        (exhausted
          ? "border-destructive/40 text-destructive"
          : "border-border/60 text-muted-foreground")
      }
    >
      <span className="font-medium text-foreground">{label}</span>
      {q.used} of {q.limit} used
    </span>
  );
}

function QuotaBanner() {
  const { data, isLoading } = useQuotaStatus();
  if (isLoading || !data) return null;
  if (data.movie.unlimited && data.series.unlimited) return null;
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2">
      <Gauge className="h-4 w-4 text-muted-foreground" />
      <QuotaPill label="Movies" q={data.movie} />
      <QuotaPill label="TV" q={data.series} />
      <span className="text-xs text-muted-foreground">
        in the last {data.window_days} day{data.window_days === 1 ? "" : "s"}
      </span>
    </div>
  );
}

function QuotaConfigCard() {
  const { data, isLoading } = useQuotaConfig();
  const update = useUpdateQuotaConfig();
  const [movie, setMovie] = React.useState("");
  const [series, setSeries] = React.useState("");
  const [windowDays, setWindowDays] = React.useState("");

  React.useEffect(() => {
    if (data) {
      setMovie(String(data.movie_limit));
      setSeries(String(data.series_limit));
      setWindowDays(String(data.window_days));
    }
  }, [data]);

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading quota…</p>;
  }

  const toInt = (s: string) => Math.max(0, Math.floor(Number(s) || 0));

  function save(e: React.FormEvent) {
    e.preventDefault();
    update.mutate(
      {
        movie_limit: toInt(movie),
        series_limit: toInt(series),
        window_days: Math.max(1, toInt(windowDays) || 7),
      },
      {
        onSuccess: () => toast.success("Request quota saved"),
        onError: (err) =>
          toast.error(err instanceof Error ? err.message : "Failed to save quota"),
      },
    );
  }

  return (
    <form
      onSubmit={save}
      className="flex flex-col gap-4 rounded-lg border border-border/60 p-4 sm:flex-row sm:items-end"
    >
      <div className="flex-1 space-y-1">
        <label htmlFor="quota-movie" className="text-xs font-medium text-muted-foreground">
          Movies per user
        </label>
        <Input
          id="quota-movie"
          type="number"
          min={0}
          value={movie}
          onChange={(e) => setMovie(e.target.value)}
        />
      </div>
      <div className="flex-1 space-y-1">
        <label htmlFor="quota-series" className="text-xs font-medium text-muted-foreground">
          TV shows per user
        </label>
        <Input
          id="quota-series"
          type="number"
          min={0}
          value={series}
          onChange={(e) => setSeries(e.target.value)}
        />
      </div>
      <div className="flex-1 space-y-1">
        <label htmlFor="quota-window" className="text-xs font-medium text-muted-foreground">
          Window (days)
        </label>
        <Input
          id="quota-window"
          type="number"
          min={1}
          value={windowDays}
          onChange={(e) => setWindowDays(e.target.value)}
        />
      </div>
      <Button type="submit" disabled={update.isPending}>
        {update.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
        Save
      </Button>
      <p className="text-xs text-muted-foreground sm:max-w-[12rem]">
        0 = unlimited. Admins are exempt from quotas.
      </p>
    </form>
  );
}

export function RequestsPage() {
  const { setHeader } = usePageHeader();
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  React.useEffect(() => setHeader({ title: "Requests" }), [setHeader]);

  return (
    <div className="space-y-8 p-6">
      <section className="space-y-3">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Inbox className="h-5 w-5" /> Request media
        </h2>
        <QuotaBanner />
        <SearchAndRequest />
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">My requests</h2>
        <MyRequests />
      </section>

      {isAdmin && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold">Request quota</h2>
          <p className="text-sm text-muted-foreground">
            Limit how many requests each non-admin user can make in a rolling
            window.
          </p>
          <QuotaConfigCard />
        </section>
      )}

      {isAdmin && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold">Manage requests</h2>
          <ManageRequests />
        </section>
      )}
    </div>
  );
}
