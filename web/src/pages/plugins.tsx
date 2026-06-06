import * as React from "react";
import { toast } from "sonner";
import {
  Puzzle,
  Plus,
  Play,
  Trash2,
  Pencil,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Loader2,
} from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useFeatureEnabled } from "@/lib/features-api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  type Plugin,
  type PluginInput,
  type PluginRun,
  usePlugins,
  usePluginEvents,
  usePluginRuns,
  useCreatePlugin,
  useUpdatePlugin,
  useDeletePlugin,
  useTestPlugin,
} from "@/lib/plugins-api";

const JS_STARTER = `// Available globals: event, env, console, fetch
// event = { version, event, topic, title, data, timestamp }
console.log("Event:", event.event, "-", event.title);

// Example: POST to a webhook
// var res = fetch("https://example.com/hook", {
//   method: "POST",
//   headers: { "Content-Type": "application/json" },
//   body: JSON.stringify({ title: event.title }),
// });
// console.log("webhook status", res.status);
`;

const emptyForm: PluginInput = {
  name: "",
  enabled: false,
  source: JS_STARTER,
  events: [],
  env: {},
  timeout_secs: 30,
};

function toForm(p: Plugin): PluginInput {
  return {
    name: p.name,
    enabled: p.enabled,
    source: p.source ?? "",
    events: p.events,
    env: p.env ?? {},
    timeout_secs: p.timeout_secs,
  };
}

function envToText(env: Record<string, string>): string {
  return Object.entries(env)
    .map(([k, v]) => `${k}=${v}`)
    .join("\n");
}
function textToEnv(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t || !t.includes("=")) continue;
    const idx = t.indexOf("=");
    out[t.slice(0, idx).trim()] = t.slice(idx + 1);
  }
  return out;
}

export function PluginsPage() {
  useSetPageHeader("Plugins", "Run custom scripts when Loom events fire");
  const enabled = useFeatureEnabled("plugins", false);
  const { data: plugins, isLoading } = usePlugins();
  const { data: events } = usePluginEvents();
  const del = useDeletePlugin();
  const test = useTestPlugin();

  const [editing, setEditing] = React.useState<Plugin | null>(null);
  const [creating, setCreating] = React.useState(false);
  const [runsFor, setRunsFor] = React.useState<string | null>(null);

  return (
    <div className="space-y-6">
      {!enabled && (
        <Card className="border-amber-500/40 bg-amber-500/5">
          <CardContent className="flex items-start gap-3 pt-6 text-sm">
            <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-500" />
            <div>
              <p className="font-medium">Plugins are disabled.</p>
              <p className="text-muted-foreground">
                Enable the “Plugins (Custom Scripts)” feature under Settings →
                Features to let Loom run these scripts. You can still manage
                definitions here.
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="border-destructive/30 bg-destructive/5">
        <CardContent className="flex items-start gap-3 pt-6 text-sm">
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-destructive" />
          <div>
            <p className="font-medium">Security notice</p>
            <p className="text-muted-foreground">
              Plugins are JavaScript that runs in-process, inside the Loom
              server, with its privileges and network access. Execution is
              CPU-bounded and timed out, but this is not an OS sandbox — a
              runaway allocation can still pressure the server. Only configure
              plugins you fully trust, and rely on container/Kubernetes controls
              for real isolation.
            </p>
          </div>
        </CardContent>
      </Card>

      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {plugins?.length ?? 0} plugin{plugins?.length === 1 ? "" : "s"}{" "}
          configured
        </p>
        <Button onClick={() => setCreating(true)} size="sm">
          <Plus className="mr-1 h-4 w-4" /> Add Plugin
        </Button>
      </div>

      {isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : !plugins || plugins.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-10 text-center text-muted-foreground">
            <Puzzle className="h-8 w-8" />
            <p>
              No plugins yet. Add one to run a script on grab/import/playback.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {plugins.map((p) => (
            <Card key={p.id}>
              <CardHeader className="flex flex-row items-start justify-between gap-3 space-y-0">
                <div className="space-y-1">
                  <CardTitle className="flex items-center gap-2 text-base">
                    {p.name}
                    <Badge variant="outline" className="text-[10px] uppercase">
                      JavaScript
                    </Badge>
                    {p.enabled ? (
                      <Badge variant="secondary">Enabled</Badge>
                    ) : (
                      <Badge variant="outline">Disabled</Badge>
                    )}
                  </CardTitle>
                  <CardDescription className="break-all font-mono text-xs">
                    {(p.source || "").split("\n")[0]?.slice(0, 80) ||
                      "JavaScript plugin"}
                  </CardDescription>
                  <div className="flex flex-wrap gap-1 pt-1">
                    {p.events.map((e) => (
                      <Badge key={e} variant="outline" className="text-[10px]">
                        {events?.find((d) => d.key === e)?.label ?? e}
                      </Badge>
                    ))}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() =>
                      test.mutate(p.id, {
                        onSuccess: (run) => {
                          setRunsFor(p.id);
                          toast[run.success ? "success" : "error"](
                            run.success
                              ? "Test run succeeded"
                              : `Test run failed: ${run.error_msg || "error"}`,
                          );
                        },
                        onError: (e) => toast.error(String(e)),
                      })
                    }
                    disabled={test.isPending}
                  >
                    {test.isPending ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <Play className="h-4 w-4" />
                    )}
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setRunsFor(p.id)}
                  >
                    History
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setEditing(p)}
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      if (confirm(`Delete plugin "${p.name}"?`)) {
                        del.mutate(p.id, {
                          onSuccess: () => toast.success("Plugin deleted"),
                          onError: (e) => toast.error(String(e)),
                        });
                      }
                    }}
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
              </CardHeader>
            </Card>
          ))}
        </div>
      )}

      {(creating || editing) && (
        <PluginDialog
          plugin={editing}
          events={events ?? []}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      )}

      {runsFor && (
        <RunsDialog pluginId={runsFor} onClose={() => setRunsFor(null)} />
      )}
    </div>
  );
}

function PluginDialog({
  plugin,
  events,
  onClose,
}: {
  plugin: Plugin | null;
  events: { key: string; label: string }[];
  onClose: () => void;
}) {
  const create = useCreatePlugin();
  const update = useUpdatePlugin();
  const [form, setForm] = React.useState<PluginInput>(
    plugin ? toForm(plugin) : emptyForm,
  );
  const [envText, setEnvText] = React.useState(
    plugin ? envToText(plugin.env ?? {}) : "",
  );

  const toggleEvent = (key: string) =>
    setForm((f) => ({
      ...f,
      events: f.events.includes(key)
        ? f.events.filter((e) => e !== key)
        : [...f.events, key],
    }));

  const submit = () => {
    const input: PluginInput = {
      ...form,
      env: textToEnv(envText),
    };
    const opts = {
      onSuccess: () => {
        toast.success(plugin ? "Plugin updated" : "Plugin created");
        onClose();
      },
      onError: (e: unknown) => toast.error(String(e)),
    };
    if (plugin) update.mutate({ id: plugin.id, input }, opts);
    else create.mutate(input, opts);
  };

  const pending = create.isPending || update.isPending;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{plugin ? "Edit Plugin" : "Add Plugin"}</DialogTitle>
          <DialogDescription>
            JavaScript runs when the selected events fire. Globals:{" "}
            <code>event</code>, <code>env</code>, <code>console</code>,{" "}
            <code>fetch</code>.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label>Name</Label>
            <Input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="My script"
            />
          </div>

          <div className="space-y-1.5">
            <Label>Script (JavaScript)</Label>
            <textarea
              className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs"
              rows={12}
              spellCheck={false}
              value={form.source}
              onChange={(e) => setForm({ ...form, source: e.target.value })}
              placeholder={JS_STARTER}
            />
            <p className="text-xs text-muted-foreground">
              ES5.1+ (goja). <code>fetch</code> supports http/https with body
              and response size caps.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label>Events</Label>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {events.map((ev) => (
                <label key={ev.key} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={form.events.includes(ev.key)}
                    onCheckedChange={() => toggleEvent(ev.key)}
                  />
                  {ev.label}
                </label>
              ))}
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>Timeout (seconds)</Label>
            <Input
              type="number"
              min={1}
              max={300}
              value={form.timeout_secs}
              onChange={(e) =>
                setForm({ ...form, timeout_secs: Number(e.target.value) })
              }
            />
          </div>

          <div className="space-y-1.5">
            <Label>Environment variables (optional)</Label>
            <textarea
              className="w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm"
              rows={3}
              value={envText}
              onChange={(e) => setEnvText(e.target.value)}
              placeholder={"KEY=value\nANOTHER=value"}
            />
            <p className="text-xs text-muted-foreground">
              One per line. Exposed to the script as the <code>env</code> global.
            </p>
          </div>

          <div className="flex items-center justify-between rounded-md border border-border p-3">
            <div>
              <Label>Enabled</Label>
              <p className="text-xs text-muted-foreground">
                Run this plugin when its events fire.
              </p>
            </div>
            <Switch
              checked={form.enabled}
              onCheckedChange={(v) => setForm({ ...form, enabled: v })}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={pending}>
            {pending && <Loader2 className="mr-1 h-4 w-4 animate-spin" />}
            {plugin ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RunsDialog({
  pluginId,
  onClose,
}: {
  pluginId: string;
  onClose: () => void;
}) {
  const { data: runs, isLoading } = usePluginRuns(pluginId);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Run History</DialogTitle>
          <DialogDescription>Most recent executions first.</DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <Skeleton className="h-32 w-full" />
        ) : !runs || runs.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No runs recorded yet.
          </p>
        ) : (
          <div className="space-y-3">
            {runs.map((r) => (
              <RunRow key={r.id} run={r} />
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function RunRow({ run }: { run: PluginRun }) {
  const [open, setOpen] = React.useState(false);
  return (
    <div className="rounded-md border border-border p-3 text-sm">
      <button
        className="flex w-full items-center justify-between gap-2 text-left"
        onClick={() => setOpen((o) => !o)}
      >
        <span className="flex items-center gap-2">
          {run.success ? (
            <CheckCircle2 className="h-4 w-4 text-emerald-500" />
          ) : (
            <XCircle className="h-4 w-4 text-destructive" />
          )}
          <span className="font-mono text-xs">{run.topic}</span>
          <span className="text-xs text-muted-foreground">
            {run.duration_ms}ms
          </span>
        </span>
        <span className="text-xs text-muted-foreground">
          {new Date(run.started_at).toLocaleString()}
        </span>
      </button>
      {run.error_msg && (
        <p className="mt-1 text-xs text-destructive">{run.error_msg}</p>
      )}
      {open && (
        <div className="mt-2 space-y-2">
          {run.stdout && (
            <div>
              <p className="text-xs font-medium text-muted-foreground">
                stdout
              </p>
              <pre className="max-h-48 overflow-auto rounded bg-muted p-2 text-[11px]">
                {run.stdout}
              </pre>
            </div>
          )}
          {run.stderr && (
            <div>
              <p className="text-xs font-medium text-muted-foreground">
                stderr
              </p>
              <pre className="max-h-48 overflow-auto rounded bg-muted p-2 text-[11px]">
                {run.stderr}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
