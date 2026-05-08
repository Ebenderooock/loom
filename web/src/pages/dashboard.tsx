import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useSystemStatus } from "@/lib/api";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { cn } from "@/lib/utils";
import {
  Film,
  Tv,
  Radio,
  Download,
  Plus,
  Search,
  Rss,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Rocket,
  Settings,
  HardDrive,
  FolderOpen,
  type LucideIcon,
} from "lucide-react";
import { Progress } from "@/components/ui/progress";

// ---------------------------------------------------------------------------
// Inline data-fetching hooks
// ---------------------------------------------------------------------------

function useMovies() {
  return useQuery({
    queryKey: ["dashboard", "movies"],
    queryFn: async ({ signal }) => {
      const res = await fetch("/api/v1/movies?limit=1", {
        signal,
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to fetch movies");
      return (await res.json()) as { data: unknown[]; total: number };
    },
    staleTime: 60_000,
    retry: 1,
  });
}

function useSeries() {
  return useQuery({
    queryKey: ["dashboard", "series"],
    queryFn: async ({ signal }) => {
      const res = await fetch("/api/v1/series?limit=1", {
        signal,
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to fetch series");
      return (await res.json()) as { data: unknown[]; total: number };
    },
    staleTime: 60_000,
    retry: 1,
  });
}

function useIndexers() {
  return useQuery({
    queryKey: ["dashboard", "indexers"],
    queryFn: async ({ signal }) => {
      const res = await fetch("/api/v1/indexers", {
        signal,
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to fetch indexers");
      return (await res.json()) as { data: unknown[] };
    },
    staleTime: 60_000,
    retry: 1,
  });
}

function useDownloadClients() {
  return useQuery({
    queryKey: ["dashboard", "download-clients"],
    queryFn: async ({ signal }) => {
      const res = await fetch("/api/v1/download-clients", {
        signal,
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to fetch download clients");
      return (await res.json()) as { data: unknown[] };
    },
    staleTime: 60_000,
    retry: 1,
  });
}

function useLibraries() {
  return useQuery({
    queryKey: ["dashboard", "libraries"],
    queryFn: async ({ signal }) => {
      const res = await fetch("/api/v1/libraries", {
        signal,
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to fetch libraries");
      return (await res.json()) as { data: { id: string; name: string; path: string; mediaType: string; disk_space: { total_bytes: number; free_bytes: number; used_bytes: number } }[] };
    },
    staleTime: 60_000,
    retry: 1,
  });
}

function useIndexerHealth() {
  return useQuery({
    queryKey: ["dashboard", "indexer-health"],
    queryFn: async ({ signal }) => {
      const res = await fetch("/api/v1/indexers/health", {
        signal,
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to fetch indexer health");
      return (await res.json()) as {
        data: { id: number; name: string; message: string }[];
      };
    },
    staleTime: 60_000,
    retry: 1,
  });
}

// ---------------------------------------------------------------------------
// Stat Card Component
// ---------------------------------------------------------------------------

interface StatCardProps {
  icon: LucideIcon;
  label: string;
  value: string | number;
  accent: string; // Tailwind bg class for icon circle
  iconColor: string; // Tailwind text class for icon
  loading?: boolean;
}

function StatCard({
  icon: Icon,
  label,
  value,
  accent,
  iconColor,
  loading,
}: StatCardProps) {
  return (
    <Card className="relative overflow-hidden">
      <CardContent className="flex items-center gap-4 p-5">
        <div
          className={cn(
            "flex h-12 w-12 shrink-0 items-center justify-center rounded-full",
            accent,
          )}
        >
          <Icon className={cn("h-6 w-6", iconColor)} />
        </div>
        <div className="min-w-0">
          {loading ? (
            <Skeleton className="mb-1 h-8 w-16" />
          ) : (
            <p className="text-3xl font-bold tracking-tight">{value}</p>
          )}
          <p className="text-sm text-muted-foreground">{label}</p>
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Welcome / Getting Started
// ---------------------------------------------------------------------------

function WelcomeSection() {
  const steps = [
    {
      number: 1,
      title: "Configure indexers",
      description: "Add Usenet or torrent indexers so Loom can search for releases.",
      href: "/indexers",
      icon: Radio,
    },
    {
      number: 2,
      title: "Add your first movie or series",
      description: "Search and add titles you want to track and download.",
      href: "/movies",
      icon: Film,
    },
    {
      number: 3,
      title: "Set up download clients",
      description: "Connect your download client to automate grabs.",
      href: "/settings",
      icon: Settings,
    },
  ];

  return (
    <Card className="border-primary/30 bg-gradient-to-br from-primary/5 via-card to-card">
      <CardHeader className="pb-2">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/15">
            <Rocket className="h-5 w-5 text-primary" />
          </div>
          <div>
            <CardTitle className="text-xl">Welcome to Loom</CardTitle>
            <p className="text-sm text-muted-foreground">
              Get started in a few steps
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-4">
        <div className="grid gap-4 sm:grid-cols-3">
          {steps.map((step) => (
            <Link
              key={step.number}
              to={step.href}
              className="group flex gap-3 rounded-lg border border-border/50 bg-muted/30 p-4 transition-colors hover:border-primary/40 hover:bg-muted/50"
            >
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/15 text-sm font-bold text-primary">
                {step.number}
              </div>
              <div className="min-w-0">
                <p className="font-medium leading-tight group-hover:text-primary">
                  {step.title}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {step.description}
                </p>
              </div>
            </Link>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Quick Actions
// ---------------------------------------------------------------------------

function QuickActionsCard() {
  const actions = [
    { label: "Add Movie", href: "/movies", icon: Plus },
    { label: "Add Series", href: "/series", icon: Plus },
    { label: "Search All", href: "/indexers", icon: Search },
    { label: "RSS Sync", href: "/indexers", icon: Rss },
  ];

  return (
    <Card className="flex flex-col">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Quick Actions</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col justify-center">
        <div className="grid grid-cols-2 gap-2">
          {actions.map((action) => (
            <Button
              key={action.label}
              variant="outline"
              size="sm"
              className="justify-start gap-2"
              asChild
            >
              <Link to={action.href}>
                <action.icon className="h-4 w-4" />
                {action.label}
              </Link>
            </Button>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// System Health
// ---------------------------------------------------------------------------

function SystemHealthCard() {
  const health = useIndexerHealth();
  const status = useSystemStatus();

  const issues = health.data?.data ?? [];
  const hasIssues = issues.length > 0;

  return (
    <Card className="flex flex-col">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">System Health</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-1 flex-col gap-3">
        {health.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-4 w-32" />
          </div>
        ) : hasIssues ? (
          <div className="space-y-2">
            {issues.slice(0, 3).map((issue, i) => (
              <div
                key={issue.id ?? i}
                className="flex items-start gap-2 rounded-md bg-destructive/10 px-3 py-2 text-sm"
              >
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
                <span>
                  <span className="font-medium">{issue.name}:</span>{" "}
                  {issue.message}
                </span>
              </div>
            ))}
            {issues.length > 3 && (
              <Link
                to="/indexers/health"
                className="text-xs text-muted-foreground hover:text-primary"
              >
                + {issues.length - 3} more issue{issues.length - 3 > 1 ? "s" : ""}
              </Link>
            )}
          </div>
        ) : (
          <div className="flex items-center gap-2 rounded-md bg-emerald-500/10 px-3 py-2 text-sm text-emerald-400">
            <CheckCircle2 className="h-4 w-4 shrink-0" />
            All systems operational
          </div>
        )}

        {/* Version info */}
        <div className="mt-auto border-t border-border/50 pt-3">
          {status.isLoading ? (
            <Skeleton className="h-3 w-40" />
          ) : status.data ? (
            <p className="text-xs text-muted-foreground">
              Loom{" "}
              <span className="font-mono font-medium text-foreground/70">
                {status.data.version || "dev"}
              </span>
              {status.data.commit && (
                <>
                  {" · "}
                  <span className="font-mono">
                    {status.data.commit.slice(0, 7)}
                  </span>
                </>
              )}
            </p>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Storage Overview
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function StorageCard() {
  const { data, isLoading } = useLibraries();
  const libs = data?.data ?? [];
  const withSpace = libs.filter((l) => l.disk_space && l.disk_space.total_bytes > 0);

  if (isLoading) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Storage</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4" />
        </CardContent>
      </Card>
    );
  }

  if (withSpace.length === 0) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Storage</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <FolderOpen className="h-4 w-4" />
            {libs.length === 0 ? "No libraries configured" : "Storage info unavailable"}
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Storage</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {withSpace.map((lib) => {
          const used = lib.disk_space.used_bytes;
          const total = lib.disk_space.total_bytes;
          const free = lib.disk_space.free_bytes;
          const pct = total ? Math.round((used / total) * 100) : 0;
          return (
            <div key={lib.id} className="space-y-1.5">
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium truncate">{lib.name}</span>
                <span className="text-xs text-muted-foreground shrink-0 ml-2">
                  {formatBytes(free)} free
                </span>
              </div>
              <Progress value={pct} className="h-2" />
              <p className="text-xs text-muted-foreground">
                {formatBytes(used)} / {formatBytes(total)} ({pct}%)
              </p>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Recent Activity (placeholder)
// ---------------------------------------------------------------------------

function RecentActivityCard() {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Recent Activity</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col items-center justify-center py-10 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-muted/50">
            <Clock className="h-7 w-7 text-muted-foreground/50" />
          </div>
          <p className="mt-3 text-sm font-medium text-muted-foreground">
            No recent activity
          </p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            Grabs and imports will show up here
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Dashboard Page
// ---------------------------------------------------------------------------

export function DashboardPage() {
  const movies = useMovies();
  const series = useSeries();
  const indexers = useIndexers();
  const dlClients = useDownloadClients();
  useSetPageHeader("Dashboard");

  const movieCount = movies.data?.total ?? 0;
  const seriesCount = series.data?.total ?? 0;
  const indexerCount = indexers.data?.data?.length ?? 0;
  const dlClientCount = dlClients.data?.data?.length ?? 0;

  const isLoading = movies.isLoading || series.isLoading || indexers.isLoading;
  const isFreshInstall =
    !isLoading && movieCount === 0 && seriesCount === 0;

  return (
    <div className="space-y-6">
      {/* Welcome state for fresh installs */}
      {isFreshInstall && <WelcomeSection />}

      {/* Row 1: Summary stat cards */}
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          icon={Film}
          label="Movies"
          value={movieCount}
          accent="bg-blue-500/15"
          iconColor="text-blue-400"
          loading={movies.isLoading}
        />
        <StatCard
          icon={Tv}
          label="TV Shows"
          value={seriesCount}
          accent="bg-purple-500/15"
          iconColor="text-purple-400"
          loading={series.isLoading}
        />
        <StatCard
          icon={Radio}
          label="Indexers"
          value={indexerCount}
          accent="bg-teal-500/15"
          iconColor="text-teal-400"
          loading={indexers.isLoading}
        />
        <StatCard
          icon={Download}
          label="Download Clients"
          value={dlClientCount}
          accent="bg-amber-500/15"
          iconColor="text-amber-400"
          loading={dlClients.isLoading}
        />
      </div>

      {/* Row 2: Quick Actions + System Health */}
      <div className="grid gap-4 md:grid-cols-2">
        <QuickActionsCard />
        <SystemHealthCard />
      </div>

      {/* Row 3: Storage + Recent Activity */}
      <div className="grid gap-4 md:grid-cols-2">
        <StorageCard />
        <RecentActivityCard />
      </div>
    </div>
  );
}
