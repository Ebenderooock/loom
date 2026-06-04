import * as React from "react";
import { MoreHorizontal, Plus, Send, CheckCircle2, XCircle, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useNotifications,
  useCreateNotification,
  useUpdateNotification,
  useDeleteNotification,
  useTestNotification,
  useTestNotificationConfig,
  useNotificationHistory,
  CONNECTION_TYPES,
  EVENT_TYPES,
  TEMPLATE_VARIABLES,
  type NotificationConnection,
  type ConnectionType,
  type ConnectionSettings,
  type CreateConnectionRequest,
  type UpdateConnectionRequest,
  ApiError,
} from "@/lib/notifications-api";

type DialogState =
  | { kind: "closed" }
  | { kind: "create" }
  | { kind: "edit"; connection: NotificationConnection }
  | { kind: "delete"; connection: NotificationConnection };

function errMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError)
    return `${fallback} (HTTP ${err.status}): ${err.message}`;
  if (err instanceof Error) return `${fallback}: ${err.message}`;
  return fallback;
}

function getTypeLabel(type: ConnectionType): string {
  return CONNECTION_TYPES.find((t) => t.value === type)?.label ?? type;
}

function subscribedEvents(conn: NotificationConnection): string[] {
  const events: string[] = [];
  for (const ev of EVENT_TYPES) {
    if (conn[ev.key]) events.push(ev.label);
  }
  return events;
}

// ---------- Form ----------

interface FormState {
  name: string;
  type: ConnectionType;
  enabled: boolean;
  settings: ConnectionSettings;
  on_grab: boolean;
  on_download: boolean;
  on_upgrade: boolean;
  on_rename: boolean;
  on_delete: boolean;
  on_health_issue: boolean;
  on_application_update: boolean;
  on_playback: boolean;
}

function defaultForm(): FormState {
  return {
    name: "",
    type: "discord",
    enabled: true,
    settings: {},
    on_grab: true,
    on_download: true,
    on_upgrade: false,
    on_rename: false,
    on_delete: false,
    on_health_issue: true,
    on_application_update: false,
    on_playback: false,
  };
}

function formFromConnection(c: NotificationConnection): FormState {
  return {
    name: c.name,
    type: c.type,
    enabled: c.enabled,
    settings: { ...c.settings },
    on_grab: c.on_grab,
    on_download: c.on_download,
    on_upgrade: c.on_upgrade,
    on_rename: c.on_rename,
    on_delete: c.on_delete,
    on_health_issue: c.on_health_issue,
    on_application_update: c.on_application_update,
    on_playback: c.on_playback,
  };
}

function ConnectionForm({
  initial,
  onSubmit,
  onCancel,
  onTest,
  submitting,
  topError,
}: {
  initial?: NotificationConnection;
  onSubmit: (form: FormState) => void;
  onCancel: () => void;
  onTest?: (form: FormState) => Promise<{ ok: boolean; message?: string; error?: string }>;
  submitting: boolean;
  topError?: string;
}) {
  const [form, setForm] = React.useState<FormState>(
    initial ? formFromConnection(initial) : defaultForm(),
  );
  const [testResult, setTestResult] = React.useState<{
    ok: boolean;
    message?: string;
  } | null>(null);
  const [testing, setTesting] = React.useState(false);

  const typeMeta = CONNECTION_TYPES.find((t) => t.value === form.type);
  const fields = typeMeta?.fields ?? [];

  function updateSettings(key: string, value: string | number | boolean) {
    setForm((f) => ({
      ...f,
      settings: { ...f.settings, [key]: value },
    }));
  }

  async function handleTest() {
    if (!onTest) return;
    setTestResult(null);
    setTesting(true);
    try {
      const result = await onTest(form);
      setTestResult({
        ok: result.ok,
        message: result.ok ? (result.message ?? "Test notification sent successfully") : (result.error ?? "Test failed"),
      });
    } catch (err) {
      setTestResult({
        ok: false,
        message: err instanceof Error ? err.message : "Test failed",
      });
    } finally {
      setTesting(false);
    }
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(form);
      }}
      className="space-y-4"
    >
      {topError && (
        <div className="rounded-md border border-red-500/40 bg-red-500/10 p-2 text-sm text-red-700 dark:text-red-300">
          {topError}
        </div>
      )}

      <div className="space-y-2">
        <Label htmlFor="conn-name">Name</Label>
        <Input
          id="conn-name"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="My Discord"
          required
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="conn-type">Type</Label>
        <Select
          value={form.type}
          onValueChange={(v) =>
            setForm((f) => ({ ...f, type: v as ConnectionType, settings: {} }))
          }
        >
          <SelectTrigger id="conn-type">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CONNECTION_TYPES.map((t) => (
              <SelectItem key={t.value} value={t.value}>
                {t.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-center gap-2">
        <Checkbox
          id="conn-enabled"
          checked={form.enabled}
          onCheckedChange={(v) =>
            setForm((f) => ({ ...f, enabled: v === true }))
          }
        />
        <Label htmlFor="conn-enabled">Enabled</Label>
      </div>

      {/* Dynamic settings fields */}
      <fieldset className="space-y-3 rounded-md border border-border p-3">
        <legend className="px-1 text-sm font-medium">Connection Settings</legend>

        {fields.includes("webhook_url") && (
          <div className="space-y-1">
            <Label htmlFor="s-webhook-url">Webhook URL</Label>
            <Input
              id="s-webhook-url"
              value={form.settings.webhook_url ?? ""}
              onChange={(e) => updateSettings("webhook_url", e.target.value)}
              placeholder="https://discord.com/api/webhooks/..."
            />
          </div>
        )}
        {fields.includes("server_url") && (
          <div className="space-y-1">
            <Label htmlFor="s-server-url">Server URL</Label>
            <Input
              id="s-server-url"
              value={form.settings.server_url ?? ""}
              onChange={(e) => updateSettings("server_url", e.target.value)}
              placeholder="https://gotify.example.com"
            />
          </div>
        )}
        {fields.includes("api_key") && (
          <div className="space-y-1">
            <Label htmlFor="s-api-key">API Key</Label>
            <Input
              id="s-api-key"
              value={form.settings.api_key ?? ""}
              onChange={(e) => updateSettings("api_key", e.target.value)}
            />
          </div>
        )}
        {fields.includes("topic") && (
          <div className="space-y-1">
            <Label htmlFor="s-topic">Topic</Label>
            <Input
              id="s-topic"
              value={form.settings.topic ?? ""}
              onChange={(e) => updateSettings("topic", e.target.value)}
              placeholder="loom-notifications"
            />
          </div>
        )}
        {fields.includes("bot_token") && (
          <div className="space-y-1">
            <Label htmlFor="s-bot-token">Bot Token</Label>
            <Input
              id="s-bot-token"
              value={form.settings.bot_token ?? ""}
              onChange={(e) => updateSettings("bot_token", e.target.value)}
            />
          </div>
        )}
        {fields.includes("chat_id") && (
          <div className="space-y-1">
            <Label htmlFor="s-chat-id">Chat ID</Label>
            <Input
              id="s-chat-id"
              value={form.settings.chat_id ?? ""}
              onChange={(e) => updateSettings("chat_id", e.target.value)}
            />
          </div>
        )}
        {fields.includes("user_key") && (
          <div className="space-y-1">
            <Label htmlFor="s-user-key">User Key</Label>
            <Input
              id="s-user-key"
              value={form.settings.user_key ?? ""}
              onChange={(e) => updateSettings("user_key", e.target.value)}
            />
          </div>
        )}
        {fields.includes("host") && (
          <div className="space-y-1">
            <Label htmlFor="s-host">SMTP Host</Label>
            <Input
              id="s-host"
              value={form.settings.host ?? ""}
              onChange={(e) => updateSettings("host", e.target.value)}
              placeholder="smtp.example.com"
            />
          </div>
        )}
        {fields.includes("port") && (
          <div className="space-y-1">
            <Label htmlFor="s-port">Port</Label>
            <Input
              id="s-port"
              type="number"
              value={form.settings.port ?? 587}
              onChange={(e) =>
                updateSettings("port", parseInt(e.target.value, 10) || 587)
              }
            />
          </div>
        )}
        {fields.includes("username") && (
          <div className="space-y-1">
            <Label htmlFor="s-username">Username</Label>
            <Input
              id="s-username"
              value={form.settings.username ?? ""}
              onChange={(e) => updateSettings("username", e.target.value)}
            />
          </div>
        )}
        {fields.includes("password") && (
          <div className="space-y-1">
            <Label htmlFor="s-password">Password</Label>
            <Input
              id="s-password"
              type="password"
              value={form.settings.password ?? ""}
              onChange={(e) => updateSettings("password", e.target.value)}
            />
          </div>
        )}
        {fields.includes("from") && (
          <div className="space-y-1">
            <Label htmlFor="s-from">From</Label>
            <Input
              id="s-from"
              value={form.settings.from ?? ""}
              onChange={(e) => updateSettings("from", e.target.value)}
              placeholder="loom@example.com"
            />
          </div>
        )}
        {fields.includes("to") && (
          <div className="space-y-1">
            <Label htmlFor="s-to">To</Label>
            <Input
              id="s-to"
              value={form.settings.to ?? ""}
              onChange={(e) => updateSettings("to", e.target.value)}
              placeholder="user@example.com"
            />
          </div>
        )}
        {fields.includes("tls") && (
          <div className="flex items-center gap-2">
            <Checkbox
              id="s-tls"
              checked={form.settings.tls ?? false}
              onCheckedChange={(v) => updateSettings("tls", v === true)}
            />
            <Label htmlFor="s-tls">Use TLS</Label>
          </div>
        )}
      </fieldset>

      {/* Event subscriptions */}
      <fieldset className="space-y-2 rounded-md border border-border p-3">
        <legend className="px-1 text-sm font-medium">Events</legend>
        <div className="grid grid-cols-2 gap-2">
          {EVENT_TYPES.map((ev) => (
            <div key={ev.key} className="flex items-center gap-2">
              <Checkbox
                id={`ev-${ev.key}`}
                checked={form[ev.key]}
                onCheckedChange={(v) =>
                  setForm((f) => ({ ...f, [ev.key]: v === true }))
                }
              />
              <Label htmlFor={`ev-${ev.key}`} className="text-sm">
                {ev.label}
              </Label>
            </div>
          ))}
        </div>
      </fieldset>

      {/* Template override */}
      <fieldset className="space-y-2 rounded-md border border-border p-3">
        <legend className="px-1 text-sm font-medium">
          Message Template (optional)
        </legend>
        <textarea
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm font-mono"
          rows={3}
          value={form.settings.template_override ?? ""}
          onChange={(e) => updateSettings("template_override", e.target.value)}
          placeholder="Leave empty for default template"
        />
        <p className="text-xs text-muted-foreground">
          Available variables:{" "}
          {TEMPLATE_VARIABLES.map((v) => (
            <code
              key={v}
              className="mx-0.5 rounded bg-muted px-1 py-0.5 text-[11px]"
            >
              {v}
            </code>
          ))}
        </p>
      </fieldset>

      {testResult && (
        <div
          className={`flex items-center gap-2 rounded-md border p-3 text-sm ${
            testResult.ok
              ? "border-green-500/40 bg-green-500/10 text-green-700 dark:text-green-300"
              : "border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300"
          }`}
        >
          {testResult.ok ? (
            <CheckCircle2 className="w-4 h-4 shrink-0" />
          ) : (
            <XCircle className="w-4 h-4 shrink-0" />
          )}
          <span>{testResult.message}</span>
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        {onTest && (
          <Button
            type="button"
            variant="outline"
            onClick={handleTest}
            disabled={testing || submitting}
          >
            {testing ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Testing…
              </>
            ) : (
              <>
                <Send className="mr-2 h-4 w-4" />
                Test
              </>
            )}
          </Button>
        )}
        <Button type="submit" disabled={submitting}>
          {submitting ? "Saving…" : initial ? "Update" : "Create"}
        </Button>
      </div>
    </form>
  );
}

// ---------- Page ----------

export function NotificationsPage() {
  useSetPageHeader("Notifications");
  const connectionsQ = useNotifications();
  const historyQ = useNotificationHistory(50);
  const create = useCreateNotification();
  const update = useUpdateNotification();
  const del = useDeleteNotification();
  const test = useTestNotification();
  const testConfig = useTestNotificationConfig();

  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });
  const [topError, setTopError] = React.useState<string | undefined>();

  function close() {
    setDialog({ kind: "closed" });
    setTopError(undefined);
  }

  function formToCreateBody(form: FormState): CreateConnectionRequest {
    return {
      name: form.name,
      type: form.type,
      enabled: form.enabled,
      settings: form.settings,
      on_grab: form.on_grab,
      on_download: form.on_download,
      on_upgrade: form.on_upgrade,
      on_rename: form.on_rename,
      on_delete: form.on_delete,
      on_health_issue: form.on_health_issue,
      on_application_update: form.on_application_update,
      on_playback: form.on_playback,
    };
  }

  async function handleTestUnsaved(form: FormState) {
    return testConfig.mutateAsync(formToCreateBody(form));
  }

  async function handleTestSaved(_form: FormState, id: string) {
    try {
      await test.mutateAsync(id);
      return { ok: true, message: "Test notification sent successfully" };
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : "Test failed" };
    }
  }

  async function handleCreate(form: FormState) {
    setTopError(undefined);
    try {
      await create.mutateAsync(formToCreateBody(form));
      toast.success(`Notification "${form.name}" added.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not create notification"));
    }
  }

  async function handleUpdate(form: FormState, original: NotificationConnection) {
    setTopError(undefined);
    try {
      const body: UpdateConnectionRequest = {
        name: form.name,
        type: form.type,
        enabled: form.enabled,
        settings: form.settings,
        on_grab: form.on_grab,
        on_download: form.on_download,
        on_upgrade: form.on_upgrade,
        on_rename: form.on_rename,
        on_delete: form.on_delete,
        on_health_issue: form.on_health_issue,
        on_application_update: form.on_application_update,
        on_playback: form.on_playback,
      };
      await update.mutateAsync({ id: original.id, body });
      toast.success(`Notification "${form.name}" updated.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not update notification"));
    }
  }

  async function handleDelete(conn: NotificationConnection) {
    try {
      await del.mutateAsync(conn.id);
      toast.success(`Notification "${conn.name}" deleted.`);
      close();
    } catch (err) {
      toast.error(errMessage(err, "Could not delete notification"));
    }
  }

  async function handleTest(conn: NotificationConnection) {
    try {
      await test.mutateAsync(conn.id);
      toast.success(`Test notification sent to "${conn.name}".`);
    } catch (err) {
      toast.error(errMessage(err, "Test failed"));
    }
  }

  // Build a lookup of connection names by ID for the history tab.
  const connMap = React.useMemo(() => {
    const m = new Map<string, string>();
    for (const c of connectionsQ.data ?? []) m.set(c.id, c.name);
    return m;
  }, [connectionsQ.data]);

  return (
    <div className="space-y-6">
      <Tabs defaultValue="connections">
        <div className="flex items-center justify-between gap-4">
          <TabsList>
            <TabsTrigger value="connections">Connections</TabsTrigger>
            <TabsTrigger value="history">History</TabsTrigger>
          </TabsList>
          <Button onClick={() => setDialog({ kind: "create" })} className="gap-2">
            <Plus className="h-4 w-4" />
            Add notification
          </Button>
        </div>

        {/* ── Connections Tab ── */}
        <TabsContent value="connections" className="space-y-4">
          {connectionsQ.isError && (
            <div
              role="alert"
              className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
            >
              {errMessage(connectionsQ.error, "Could not load notifications")}
            </div>
          )}

          <div className="overflow-x-auto rounded-md border border-border">
            <table className="w-full text-sm">
              <caption className="sr-only">Notification connections</caption>
              <thead className="bg-muted/50 text-left">
                <tr>
                  <th scope="col" className="px-3 py-2">Name</th>
                  <th scope="col" className="px-3 py-2">Type</th>
                  <th scope="col" className="px-3 py-2">Enabled</th>
                  <th scope="col" className="px-3 py-2">Events</th>
                  <th scope="col" className="px-3 py-2 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {connectionsQ.isLoading && (
                  <>
                    {Array.from({ length: 2 }).map((_, i) => (
                      <tr key={i} className="border-t border-border">
                        {Array.from({ length: 5 }).map((__, j) => (
                          <td key={j} className="px-3 py-3">
                            <Skeleton className="h-4 w-24" />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </>
                )}
                {!connectionsQ.isLoading &&
                  (connectionsQ.data?.length ?? 0) === 0 && (
                    <tr>
                      <td
                        colSpan={5}
                        className="px-3 py-6 text-center text-muted-foreground"
                      >
                        No notification channels configured.
                      </td>
                    </tr>
                  )}
                {(connectionsQ.data ?? []).map((c) => (
                  <tr key={c.id} className="border-t border-border">
                    <td className="px-3 py-2">
                      <div className="font-medium">{c.name}</div>
                      <div className="text-xs text-muted-foreground">{c.id}</div>
                    </td>
                    <td className="px-3 py-2">
                      <Badge variant="secondary">{getTypeLabel(c.type)}</Badge>
                    </td>
                    <td className="px-3 py-2">{c.enabled ? "Yes" : "No"}</td>
                    <td className="px-3 py-2">
                      <div className="flex flex-wrap gap-1">
                        {subscribedEvents(c).map((ev) => (
                          <Badge key={ev} variant="outline" className="text-xs">
                            {ev}
                          </Badge>
                        ))}
                        {subscribedEvents(c).length === 0 && (
                          <span className="text-muted-foreground">None</span>
                        )}
                      </div>
                    </td>
                    <td className="px-3 py-2 text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={`Actions for ${c.name}`}
                          >
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onSelect={() =>
                              setDialog({ kind: "edit", connection: c })
                            }
                          >
                            Edit
                          </DropdownMenuItem>
                          <DropdownMenuItem onSelect={() => handleTest(c)}>
                            <Send className="mr-2 h-3 w-3" />
                            Test
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onSelect={() =>
                              setDialog({ kind: "delete", connection: c })
                            }
                            className="text-red-600 focus:text-red-600"
                          >
                            Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </TabsContent>

        {/* ── History Tab ── */}
        <TabsContent value="history" className="space-y-4">
          {historyQ.isError && (
            <div
              role="alert"
              className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
            >
              {errMessage(historyQ.error, "Could not load notification history")}
            </div>
          )}

          <div className="overflow-x-auto rounded-md border border-border">
            <table className="w-full text-sm">
              <caption className="sr-only">Notification history</caption>
              <thead className="bg-muted/50 text-left">
                <tr>
                  <th scope="col" className="px-3 py-2">Time</th>
                  <th scope="col" className="px-3 py-2">Connection</th>
                  <th scope="col" className="px-3 py-2">Event</th>
                  <th scope="col" className="px-3 py-2">Title</th>
                  <th scope="col" className="px-3 py-2">Status</th>
                  <th scope="col" className="px-3 py-2">Error</th>
                </tr>
              </thead>
              <tbody>
                {historyQ.isLoading && (
                  <>
                    {Array.from({ length: 3 }).map((_, i) => (
                      <tr key={i} className="border-t border-border">
                        {Array.from({ length: 6 }).map((__, j) => (
                          <td key={j} className="px-3 py-3">
                            <Skeleton className="h-4 w-20" />
                          </td>
                        ))}
                      </tr>
                    ))}
                  </>
                )}
                {!historyQ.isLoading &&
                  (historyQ.data?.length ?? 0) === 0 && (
                    <tr>
                      <td
                        colSpan={6}
                        className="px-3 py-6 text-center text-muted-foreground"
                      >
                        No notification history yet.
                      </td>
                    </tr>
                  )}
                {(historyQ.data ?? []).map((h) => (
                  <tr key={h.id} className="border-t border-border">
                    <td className="px-3 py-2 whitespace-nowrap text-muted-foreground">
                      {new Date(h.sent_at).toLocaleString()}
                    </td>
                    <td className="px-3 py-2">
                      {h.connection_id
                        ? connMap.get(h.connection_id) ?? h.connection_id
                        : "—"}
                    </td>
                    <td className="px-3 py-2">
                      <Badge variant="outline" className="text-xs">
                        {h.event_type}
                      </Badge>
                    </td>
                    <td className="px-3 py-2">{h.title}</td>
                    <td className="px-3 py-2">
                      <Badge
                        variant={h.success ? "default" : "destructive"}
                        className="text-xs"
                      >
                        {h.success ? "Success" : "Failed"}
                      </Badge>
                    </td>
                    <td className="px-3 py-2 text-muted-foreground">
                      {h.error_message || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </TabsContent>
      </Tabs>

      {/* Create / Edit dialog */}
      <Dialog
        open={dialog.kind === "create" || dialog.kind === "edit"}
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent className="max-w-xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {dialog.kind === "edit"
                ? "Edit notification"
                : "Add notification"}
            </DialogTitle>
            <DialogDescription>
              Configure a notification channel and choose which events trigger
              it.
            </DialogDescription>
          </DialogHeader>
          {dialog.kind === "create" && (
            <ConnectionForm
              onSubmit={handleCreate}
              onCancel={close}
              onTest={handleTestUnsaved}
              submitting={create.isPending}
              topError={topError}
            />
          )}
          {dialog.kind === "edit" && (
            <ConnectionForm
              initial={dialog.connection}
              onSubmit={(form) => handleUpdate(form, dialog.connection)}
              onCancel={close}
              onTest={(form) => handleTestSaved(form, dialog.connection.id)}
              submitting={update.isPending}
              topError={topError}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Delete confirmation */}
      <Dialog
        open={dialog.kind === "delete"}
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete notification?</DialogTitle>
            <DialogDescription>
              {dialog.kind === "delete"
                ? `Permanently remove "${dialog.connection.name}".`
                : null}
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() =>
                dialog.kind === "delete"
                  ? handleDelete(dialog.connection)
                  : null
              }
              disabled={del.isPending}
            >
              {del.isPending ? "Deleting…" : "Delete"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
