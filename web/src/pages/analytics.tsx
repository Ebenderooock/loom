import * as React from "react";
import { usePageHeader } from "@/hooks/use-page-header";
import { useAuth } from "@/hooks/use-auth";
import {
  useActiveStreams,
  useAnalyticsStats,
  useAnalyticsHistory,
  formatWatched,
  formatBitrate,
  type MediaStat,
  type UserStat,
} from "@/lib/analytics-api";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { EmptyState } from "@/components/ui/empty-state";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Activity,
  ShieldAlert,
  PlayCircle,
  PauseCircle,
  Users,
  Film,
  Clock,
  Tv,
  MonitorPlay,
  Zap,
} from "lucide-react";

function StatCard({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        <div className="rounded-md bg-accent/10 p-2 text-accent">{icon}</div>
        <div>
          <p className="text-2xl font-semibold leading-none">{value}</p>
          <p className="mt-1 text-xs text-muted-foreground">{label}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function MediaTypeIcon({ type }: { type: string }) {
  if (type === "movie") return <Film className="h-3.5 w-3.5" />;
  if (type === "episode") return <Tv className="h-3.5 w-3.5" />;
  return <MonitorPlay className="h-3.5 w-3.5" />;
}

function ActiveStreams() {
  const { data: streams, isLoading } = useActiveStreams();
  const totalBandwidth = (streams ?? []).reduce(
    (sum, s) => sum + (s.bitrate_kbps || 0),
    0,
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Activity className="h-4 w-4" /> Active streams
          {streams && streams.length > 0 && (
            <Badge variant="secondary">{streams.length}</Badge>
          )}
          {totalBandwidth > 0 && (
            <Badge variant="outline" className="ml-auto gap-1 font-normal">
              <Zap className="h-3 w-3" /> {formatBitrate(totalBandwidth)} total
            </Badge>
          )}
        </CardTitle>
        <CardDescription>
          Live playback across your media servers.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : !streams || streams.length === 0 ? (
          <EmptyState
            icon={<MonitorPlay />}
            title="Nothing playing right now"
            description="Active streams from Plex, Emby, and Jellyfin appear here in real time."
          />
        ) : (
          <div className="grid gap-3 sm:grid-cols-2">
            {streams.map((s) => (
              <div
                key={`${s.connection_id}-${s.session_key}-${s.media_id}`}
                className="rounded-lg border bg-card/50 p-3"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="flex items-center gap-1.5 truncate font-medium">
                      <MediaTypeIcon type={s.media_type} />
                      {s.full_title || s.title}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      {s.user || "Unknown"}
                      {s.device ? ` · ${s.device}` : ""}
                      {` · ${s.connection_name}`}
                      {s.bitrate_kbps > 0
                        ? ` · ${formatBitrate(s.bitrate_kbps)}`
                        : ""}
                    </p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {s.transcode && (
                      <Badge variant="outline" className="gap-1 text-[10px]">
                        <Zap className="h-3 w-3" /> Transcode
                      </Badge>
                    )}
                    {s.state === "paused" ? (
                      <PauseCircle className="h-4 w-4 text-muted-foreground" />
                    ) : (
                      <PlayCircle className="h-4 w-4 text-accent" />
                    )}
                  </div>
                </div>
                <Progress value={s.progress} className="mt-3 h-1.5" />
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function BarList({
  rows,
  emptyLabel,
}: {
  rows: { label: string; sub?: string; value: number; display: string }[];
  emptyLabel: string;
}) {
  if (rows.length === 0) {
    return <p className="py-4 text-sm text-muted-foreground">{emptyLabel}</p>;
  }
  const max = Math.max(...rows.map((r) => r.value), 1);
  return (
    <div className="space-y-2.5">
      {rows.map((r, i) => (
        <div key={i} className="space-y-1">
          <div className="flex items-baseline justify-between gap-2 text-sm">
            <span className="truncate">{r.label}</span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {r.display}
            </span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-accent/70"
              style={{ width: `${(r.value / max) * 100}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

function userRows(users: UserStat[]) {
  return users.map((u) => ({
    label: u.user,
    value: u.plays,
    display: `${u.plays} plays · ${formatWatched(u.watched_ms)}`,
  }));
}

function mediaRows(media: MediaStat[]) {
  return media.map((m) => ({
    label: m.title,
    value: m.plays,
    display: `${m.plays} plays · ${formatWatched(m.watched_ms)}`,
  }));
}

function PlaysPerDay({ days }: { days: { day: string; plays: number }[] }) {
  if (days.length === 0) {
    return (
      <p className="py-4 text-sm text-muted-foreground">
        No plays in this window.
      </p>
    );
  }
  const max = Math.max(...days.map((d) => d.plays), 1);
  return (
    <div className="flex h-32 items-end gap-1">
      {days.map((d) => (
        <div
          key={d.day}
          className="flex flex-1 flex-col items-center gap-1"
          title={`${d.day}: ${d.plays}`}
        >
          <div className="flex w-full flex-1 items-end">
            <div
              className="w-full rounded-t bg-accent/70"
              style={{
                height: `${(d.plays / max) * 100}%`,
                minHeight: d.plays > 0 ? 2 : 0,
              }}
            />
          </div>
          <span className="text-[9px] text-muted-foreground">
            {d.day.slice(5)}
          </span>
        </div>
      ))}
    </div>
  );
}

export function AnalyticsPage() {
  const { setHeader } = usePageHeader();
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const [windowDays, setWindowDays] = React.useState(30);
  React.useEffect(() => setHeader({ title: "Analytics" }), [setHeader]);

  const stats = useAnalyticsStats(windowDays);
  const history = useAnalyticsHistory(50);

  if (!isAdmin) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 p-12 text-center text-muted-foreground">
        <ShieldAlert className="h-8 w-8" />
        <p>You need admin access to view analytics.</p>
      </div>
    );
  }

  const s = stats.data;

  return (
    <div className="space-y-6 p-6">
      <ActiveStreams />

      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Watch reports</h2>
          <p className="text-sm text-muted-foreground">
            Aggregated playback activity over the selected window.
          </p>
        </div>
        <Select
          value={String(windowDays)}
          onValueChange={(v) => setWindowDays(Number(v))}
        >
          <SelectTrigger className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="7">Last 7 days</SelectItem>
            <SelectItem value="30">Last 30 days</SelectItem>
            <SelectItem value="90">Last 90 days</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <StatCard
          icon={<PlayCircle className="h-5 w-5" />}
          label="Total plays"
          value={s ? String(s.totals.plays) : "—"}
        />
        <StatCard
          icon={<Users className="h-5 w-5" />}
          label="Unique viewers"
          value={s ? String(s.totals.unique_users) : "—"}
        />
        <StatCard
          icon={<Clock className="h-5 w-5" />}
          label="Watch time"
          value={s ? formatWatched(s.totals.watched_ms) : "—"}
        />
        <StatCard
          icon={<Zap className="h-5 w-5" />}
          label="Transcoded plays"
          value={s ? String(s.totals.transcode_plays) : "—"}
        />
        <StatCard
          icon={<MonitorPlay className="h-5 w-5" />}
          label="Direct plays"
          value={s ? String(s.totals.direct_plays) : "—"}
        />
        <StatCard
          icon={<Activity className="h-5 w-5" />}
          label="Avg session bitrate"
          value={s ? formatBitrate(s.totals.avg_bitrate_kbps) : "—"}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Plays per day</CardTitle>
        </CardHeader>
        <CardContent>
          <PlaysPerDay days={s?.plays_per_day ?? []} />
        </CardContent>
      </Card>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Users className="h-4 w-4" /> Top viewers
            </CardTitle>
          </CardHeader>
          <CardContent>
            <BarList
              rows={userRows(s?.top_users ?? [])}
              emptyLabel="No activity yet."
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Film className="h-4 w-4" /> Most watched
            </CardTitle>
          </CardHeader>
          <CardContent>
            <BarList
              rows={mediaRows(s?.top_media ?? [])}
              emptyLabel="No activity yet."
            />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Recent history</CardTitle>
          <CardDescription>
            The latest playback sessions across all servers.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {history.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : !history.data || history.data.length === 0 ? (
            <EmptyState
              icon={<Activity />}
              title="No watch history yet"
              description="Once people start watching, their sessions are recorded here."
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Title</TableHead>
                  <TableHead>User</TableHead>
                  <TableHead>Started</TableHead>
                  <TableHead>Watched</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {history.data.map((h) => (
                  <TableRow key={h.id}>
                    <TableCell className="flex items-center gap-1.5">
                      <MediaTypeIcon type={h.media_type} />
                      <span className="truncate">{h.full_title}</span>
                    </TableCell>
                    <TableCell>{h.user || "—"}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {h.started_at
                        ? new Date(h.started_at).toLocaleString()
                        : "—"}
                    </TableCell>
                    <TableCell>{formatWatched(h.watched_ms)}</TableCell>
                    <TableCell>
                      <Badge variant={h.ended_at ? "secondary" : "default"}>
                        {h.ended_at ? "Finished" : "Playing"}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
