import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useSystemStatus } from "@/lib/api";
import { CalendarDays, Film, ListTodo } from "lucide-react";

export function DashboardPage() {
  const { data, isLoading, isError, error } = useSystemStatus();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          A quick look at your Loom instance.
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Card>
          <CardHeader>
            <CardTitle>System status</CardTitle>
            <CardDescription>Live from /api/v1/system/status</CardDescription>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-4 w-32" />
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-4 w-40" />
              </div>
            ) : isError ? (
              <p role="alert" className="text-sm text-destructive">
                Unable to reach backend
                {error instanceof Error ? `: ${error.message}` : null}
              </p>
            ) : data ? (
              <dl className="grid gap-2 text-sm">
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Version</dt>
                  <dd className="font-mono">{data.version || "dev"}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Commit</dt>
                  <dd className="font-mono">
                    {data.commit ? data.commit.slice(0, 7) : "—"}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Build date</dt>
                  <dd className="font-mono">{data.buildDate || "—"}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">Engine</dt>
                  <dd className="font-mono">{data.engine}</dd>
                </div>
              </dl>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Film className="h-4 w-4" /> Library
            </CardTitle>
            <CardDescription>Tracked titles across types</CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-2 gap-2 text-sm">
              <div>
                <dt className="text-muted-foreground">Movies</dt>
                <dd className="text-2xl font-semibold">—</dd>
              </div>
              <div>
                <dt className="text-muted-foreground">Series</dt>
                <dd className="text-2xl font-semibold">—</dd>
              </div>
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ListTodo className="h-4 w-4" /> Activity
            </CardTitle>
            <CardDescription>Queue &amp; recent history</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">No active grabs.</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <CalendarDays className="h-4 w-4" /> Calendar today
            </CardTitle>
            <CardDescription>Releases due in the next 24h</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Nothing scheduled today.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
