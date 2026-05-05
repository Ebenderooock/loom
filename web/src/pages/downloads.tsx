// DownloadsPage is the operator's main view onto configured download clients.
// It lists every download client with a health badge, exposes Test/Edit/Delete
// row actions, and hosts the add/edit dialogs.

import * as React from "react";
import { MoreHorizontal, Plus } from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import { DownloadHealthBadge } from "@/components/downloads/health-badge";
import {
  DownloadForm,
  type DownloadFormValues,
} from "@/components/downloads/download-form";
import {
  ApiError,
  useCreateDownload,
  useDeleteDownload,
  useDownloads,
  usePatchDownload,
  useTestDownload,
  type Download,
  type DownloadPatch,
} from "@/lib/downloads-api";

type DialogState =
  | { kind: "closed" }
  | { kind: "create" }
  | { kind: "edit"; client: Download }
  | { kind: "delete"; client: Download };

function errMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError)
    return `${fallback} (HTTP ${err.status}): ${err.message}`;
  if (err instanceof Error) return `${fallback}: ${err.message}`;
  return fallback;
}

function toPatchPayload(
  values: DownloadFormValues,
  original: Download,
): DownloadPatch {
  const patch: DownloadPatch = {};
  if (values.name !== original.name) patch.name = values.name;
  if (values.enabled !== original.enabled) patch.enabled = values.enabled;
  if (values.priority !== original.priority) patch.priority = values.priority;
  if (values.category_default !== (original.category_default ?? ""))
    patch.category_default = values.category_default;
  if (values.save_path_default !== (original.save_path_default ?? ""))
    patch.save_path_default = values.save_path_default;
  if (values.remove_completed !== (original.remove_completed ?? false))
    patch.remove_completed = values.remove_completed;
  if (values.remove_failed !== (original.remove_failed ?? false))
    patch.remove_failed = values.remove_failed;
  return patch;
}

/**
 * DownloadsPage component for managing download clients.
 */
export function DownloadsPage() {
  useSetPageHeader("Downloads");
  const downloadsQ = useDownloads();
  const create = useCreateDownload();
  const patch = usePatchDownload();
  const del = useDeleteDownload();
  const test = useTestDownload();

  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });
  const [topError, setTopError] = React.useState<string | undefined>();

  function close() {
    setDialog({ kind: "closed" });
    setTopError(undefined);
  }

  async function handleCreate(values: DownloadFormValues) {
    setTopError(undefined);
    try {
      await create.mutateAsync({
        id: values.id,
        kind: values.kind,
        name: values.name,
        protocol: values.protocol,
        enabled: values.enabled,
        priority: values.priority,
        host: values.host,
        port: values.port,
        tls: values.tls,
        username: values.username || undefined,
        password: values.password || undefined,
        category_default: values.category_default || undefined,
        save_path_default: values.save_path_default || undefined,
        remove_completed: values.remove_completed,
        remove_failed: values.remove_failed,
      });
      toast.success(`Download client "${values.name}" added.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not create download client"));
    }
  }

  async function handlePatch(values: DownloadFormValues, original: Download) {
    setTopError(undefined);
    try {
      const body = toPatchPayload(values, original);
      if (Object.keys(body).length === 0) {
        toast.message("No changes to save.");
        close();
        return;
      }
      await patch.mutateAsync({ id: original.id, patch: body });
      toast.success(`Download client "${values.name}" updated.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not update download client"));
    }
  }

  async function handleDelete(client: Download) {
    try {
      await del.mutateAsync(client.id);
      toast.success(`Download client "${client.name}" deleted.`);
      close();
    } catch (err) {
      toast.error(errMessage(err, "Could not delete download client"));
    }
  }

  async function handleTest(client: Download) {
    try {
      const res = await test.mutateAsync(client.id);
      if (res.ok) {
        toast.success(`"${client.name}" healthy.`);
      } else {
        toast.error(
          `"${client.name}" test failed: ${res.error ?? "unknown error"}.`,
        );
      }
    } catch (err) {
      toast.error(errMessage(err, "Test failed"));
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <Button onClick={() => setDialog({ kind: "create" })} className="gap-2">
          <Plus className="h-4 w-4" />
          Add client
        </Button>
      </div>

      {downloadsQ.isError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {errMessage(downloadsQ.error, "Could not load download clients")}
        </div>
      ) : null}

      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full text-sm">
          <caption className="sr-only">Configured download clients</caption>
          <thead className="bg-muted/50 text-left">
            <tr>
              <th scope="col" className="px-3 py-2">
                Name
              </th>
              <th scope="col" className="px-3 py-2">
                Kind
              </th>
              <th scope="col" className="px-3 py-2">
                Host
              </th>
              <th scope="col" className="px-3 py-2">
                Enabled
              </th>
              <th scope="col" className="px-3 py-2">
                Health
              </th>
              <th scope="col" className="px-3 py-2 text-right">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {downloadsQ.isLoading ? (
              <>
                {Array.from({ length: 3 }).map((_, i) => (
                  <tr key={i} className="border-t border-border">
                    {Array.from({ length: 6 }).map((__, j) => (
                      <td key={j} className="px-3 py-3">
                        <Skeleton className="h-4 w-24" />
                      </td>
                    ))}
                  </tr>
                ))}
              </>
            ) : null}
            {!downloadsQ.isLoading && (downloadsQ.data?.length ?? 0) === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="px-3 py-6 text-center text-muted-foreground"
                >
                  No download clients configured. Click "Add client" to set one up.
                </td>
              </tr>
            ) : null}
            {(downloadsQ.data ?? []).map((client) => (
              <tr key={client.id} className="border-t border-border">
                <td className="px-3 py-2">
                  <div className="font-medium">{client.name}</div>
                  <div className="text-xs text-muted-foreground">{client.id}</div>
                </td>
                <td className="px-3 py-2 text-muted-foreground">{client.kind}</td>
                <td className="px-3 py-2 text-muted-foreground">
                  {client.host}:{client.port}
                </td>
                <td className="px-3 py-2">{client.enabled ? "Yes" : "No"}</td>
                <td className="px-3 py-2">
                  <DownloadHealthBadge health={client.health} />
                  {client.health?.last_error ? (
                    <div className="mt-1 max-w-[24ch] truncate text-xs text-muted-foreground">
                      {client.health.last_error}
                    </div>
                  ) : null}
                </td>
                <td className="px-3 py-2 text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`Actions for ${client.name}`}
                      >
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onSelect={() =>
                          setDialog({ kind: "edit", client })
                        }
                      >
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => handleTest(client)}>
                        Test
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onSelect={() =>
                          setDialog({ kind: "delete", client })
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

      {/* Create / edit dialog */}
      <Dialog
        open={dialog.kind === "create" || dialog.kind === "edit"}
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {dialog.kind === "edit"
                ? "Edit download client"
                : "Add download client"}
            </DialogTitle>
            <DialogDescription>
              Configure how Loom talks to your torrent or Usenet download client.
            </DialogDescription>
          </DialogHeader>
          {dialog.kind === "create" ? (
            <DownloadForm
              onSubmit={(v) => handleCreate(v)}
              onCancel={close}
              submitting={create.isPending}
              topError={topError}
            />
          ) : null}
          {dialog.kind === "edit" ? (
            <DownloadForm
              initial={dialog.client}
              onSubmit={(v) => handlePatch(v, dialog.client)}
              onCancel={close}
              submitting={patch.isPending}
              topError={topError}
            />
          ) : null}
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
            <DialogTitle>Delete download client?</DialogTitle>
            <DialogDescription>
              {dialog.kind === "delete"
                ? `Permanently remove "${dialog.client.name}". This cannot be undone.`
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
                dialog.kind === "delete" ? handleDelete(dialog.client) : null
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
