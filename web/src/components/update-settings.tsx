import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/hooks/use-auth";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface Settings {
  channel: "stable" | "prelease";
  preleaseOptIn: boolean;
  autoInstall: boolean;
}

interface Status {
  settings: Settings;
  canAutoInstall: boolean;
  installationNotice: string;
  verificationConfigured: boolean;
}

interface Notice {
  currentVersion: string;
  version?: string;
  available: boolean;
  verified: boolean;
  url: string;
  nextCheck: string;
  message?: string;
}

async function request<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const res = await apiFetch(`/api/v1/updates${path}`, {
    method,
    ...(body === undefined
      ? {}
      : {
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }),
  });
  if (!res.ok) throw new Error("Update request failed. Please try again.");
  return res.json() as Promise<T>;
}

export function UpdateSettings() {
  const { user } = useAuth();
  const admin = user?.role === "admin";
  const query = useQuery({
    queryKey: ["update-settings"],
    queryFn: () => request<Status>(""),
    enabled: admin,
  });

  if (!admin) return <p>Only administrators can manage update settings.</p>;
  if (query.isPending) return <p role="status">Loading update settings...</p>;
  if (query.error || !query.data)
    return <p role="alert">Could not load update settings.</p>;
  return (
    <UpdateForm key={JSON.stringify(query.data.settings)} status={query.data} />
  );
}

function UpdateForm({ status }: { status: Status }) {
  const qc = useQueryClient();
  const [channel, setChannel] = useState(status.settings.channel);
  const [acknowledged, setAcknowledged] = useState(
    status.settings.preleaseOptIn,
  );
  const [autoInstall, setAutoInstall] = useState(status.settings.autoInstall);
  const save = useMutation({
    mutationFn: () =>
      request<Settings>("", "PUT", {
        channel,
        preleaseOptIn: channel === "prelease" && acknowledged,
        autoInstall,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["update-settings"] }),
  });
  const check = useMutation({
    mutationFn: () => request<Notice>("/check", "POST"),
  });
  const dirty =
    channel !== status.settings.channel ||
    autoInstall !== status.settings.autoInstall ||
    (channel === "prelease" && acknowledged !== status.settings.preleaseOptIn);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Updates</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Native release notices come from GitHub Releases and require a trusted
          signed manifest. Checking sends a request to GitHub; checks are cached
          for at least one hour.
        </p>
        <div className="space-y-2">
          <label htmlFor="update-channel" className="block text-sm font-medium">
            Update channel
          </label>
          <select
            id="update-channel"
            className="rounded-md border border-input bg-background p-2 text-sm"
            value={channel}
            disabled={save.isPending}
            onChange={(event) => {
              setChannel(event.target.value as Settings["channel"]);
              setAcknowledged(false);
              check.reset();
            }}
          >
            <option value="stable">Stable (default)</option>
            <option value="prelease">Prelease — experimental</option>
          </select>
        </div>
        {channel === "prelease" && (
          <div className="space-y-2 rounded-md border border-destructive p-3">
            <p role="alert" className="text-sm">
              Warning: Prelease builds are experimental and may cause data loss
              or incompatible database changes. Returning to Stable does not
              downgrade or restore your database.
            </p>
            <label className="flex items-start gap-2 text-sm">
              <input
                type="checkbox"
                checked={acknowledged}
                disabled={save.isPending}
                onChange={(event) => setAcknowledged(event.target.checked)}
              />
              I understand the risks and explicitly opt in to Prelease updates.
            </label>
          </div>
        )}
        <Button
          disabled={
            save.isPending ||
            !dirty ||
            (channel === "prelease" && !acknowledged)
          }
          onClick={() => save.mutate()}
        >
          Save update channel
        </Button>
        {save.error && <p role="alert">{save.error.message}</p>}
        <div className="space-y-2 border-t border-border pt-4">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={autoInstall}
              disabled={!status.canAutoInstall || save.isPending}
              onChange={(event) => setAutoInstall(event.target.checked)}
            />
            Automatically install native updates
          </label>
          <p className="text-sm text-muted-foreground">
            {status.installationNotice}
          </p>
          {status.canAutoInstall && (
            <p className="text-sm text-muted-foreground">
              Opting in permits service restarts. Failed updates restore the
              previous binary, SQLite database and configuration. Keep
              independent backups.
            </p>
          )}
          {!status.verificationConfigured && (
            <p className="text-sm">
              The publisher verification key has not been provisioned. Verified
              update notices are unavailable.
            </p>
          )}
        </div>
        <Button
          variant="outline"
          disabled={check.isPending || save.isPending || dirty}
          onClick={() => check.mutate()}
        >
          {check.isPending ? "Checking..." : "Check for updates"}
        </Button>
        {check.error && <p role="alert">{check.error.message}</p>}
        {check.data && (
          <div role="status" className="space-y-1 text-sm">
            <p>Installed version: {check.data.currentVersion}</p>
            {check.data.verified && (
              <p>
                {check.data.available
                  ? "Update available"
                  : "Latest signed release"}
                : {check.data.version}
              </p>
            )}
            {check.data.message && <p>{check.data.message}</p>}
            <p>Next check: {new Date(check.data.nextCheck).toLocaleString()}</p>
          </div>
        )}
        <a
          className="block text-sm text-primary underline"
          href="https://github.com/Ebenderooock/loom/releases"
          target="_blank"
          rel="noopener noreferrer"
        >
          GitHub Releases and manual downloads
        </a>
      </CardContent>
    </Card>
  );
}
