// IndexersPage is the operator's main view onto configured indexers.
// It lists every indexer with a health badge, exposes Test/Edit/Delete
// row actions, and hosts the manual-search panel and add/edit dialogs.

import * as React from "react";
import { MoreHorizontal, Plus } from "lucide-react";
import { toast } from "sonner";
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
import { HealthBadge } from "@/components/indexers/health-badge";
import {
  IndexerForm,
  type IndexerFormValues,
} from "@/components/indexers/indexer-form";
import {
  toCreatePayload,
  toPatchPayload,
} from "@/components/indexers/indexer-form-adapter";
import { SearchPanel } from "@/components/indexers/search-panel";
import {
  ApiError,
  useCreateIndexer,
  useDeleteIndexer,
  useIndexers,
  usePatchIndexer,
  useProxies,
  useTestIndexer,
  type Indexer,
} from "@/lib/indexers-api";

type DialogState =
  | { kind: "closed" }
  | { kind: "create" }
  | { kind: "edit"; indexer: Indexer }
  | { kind: "search"; indexer: Indexer }
  | { kind: "delete"; indexer: Indexer };

function errMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError)
    return `${fallback} (HTTP ${err.status}): ${err.message}`;
  if (err instanceof Error) return `${fallback}: ${err.message}`;
  return fallback;
}

export function IndexersPage() {
  const indexersQ = useIndexers();
  const proxiesQ = useProxies();
  const create = useCreateIndexer();
  const patch = usePatchIndexer();
  const del = useDeleteIndexer();
  const test = useTestIndexer();

  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });
  const [topError, setTopError] = React.useState<string | undefined>();

  const proxies = React.useMemo(() => proxiesQ.data ?? [], [proxiesQ.data]);
  const proxyById = React.useMemo(() => {
    const m = new Map<string, string>();
    for (const p of proxies) m.set(p.id, p.name);
    return m;
  }, [proxies]);

  function close() {
    setDialog({ kind: "closed" });
    setTopError(undefined);
  }

  async function handleCreate(values: IndexerFormValues) {
    setTopError(undefined);
    try {
      await create.mutateAsync(toCreatePayload(values));
      toast.success(`Indexer “${values.name}” added.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not create indexer"));
    }
  }

  async function handlePatch(values: IndexerFormValues, original: Indexer) {
    setTopError(undefined);
    try {
      const body = toPatchPayload(values, original);
      if (Object.keys(body).length === 0) {
        toast.message("No changes to save.");
        close();
        return;
      }
      await patch.mutateAsync({ id: original.id, patch: body });
      toast.success(`Indexer “${values.name}” updated.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not update indexer"));
    }
  }

  async function handleDelete(indexer: Indexer) {
    try {
      await del.mutateAsync(indexer.id);
      toast.success(`Indexer “${indexer.name}” deleted.`);
      close();
    } catch (err) {
      toast.error(errMessage(err, "Could not delete indexer"));
    }
  }

  async function handleTest(indexer: Indexer) {
    try {
      const res = await test.mutateAsync(indexer.id);
      if (res.ok) {
        toast.success(`“${indexer.name}” healthy (${res.latency_ms} ms).`);
      } else {
        toast.error(
          `“${indexer.name}” failed: ${res.error ?? "unknown error"}.`,
        );
      }
    } catch (err) {
      toast.error(errMessage(err, "Test failed"));
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Indexers</h1>
          <p className="text-sm text-muted-foreground">
            Newznab- and Torznab-compatible feeds Loom uses to find releases.
          </p>
        </div>
        <Button onClick={() => setDialog({ kind: "create" })} className="gap-2">
          <Plus className="h-4 w-4" />
          Add indexer
        </Button>
      </div>

      {indexersQ.isError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {errMessage(indexersQ.error, "Could not load indexers")}
        </div>
      ) : null}

      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full text-sm">
          <caption className="sr-only">Configured indexers</caption>
          <thead className="bg-muted/50 text-left">
            <tr>
              <th scope="col" className="px-3 py-2">
                Name
              </th>
              <th scope="col" className="px-3 py-2">
                Kind
              </th>
              <th scope="col" className="px-3 py-2">
                Enabled
              </th>
              <th scope="col" className="px-3 py-2">
                Health
              </th>
              <th scope="col" className="px-3 py-2">
                Proxy
              </th>
              <th scope="col" className="px-3 py-2 text-right">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {indexersQ.isLoading ? (
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
            {!indexersQ.isLoading && (indexersQ.data?.length ?? 0) === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="px-3 py-6 text-center text-muted-foreground"
                >
                  No indexers configured. Click “Add indexer” to set one up.
                </td>
              </tr>
            ) : null}
            {(indexersQ.data ?? []).map((idx) => (
              <tr key={idx.id} className="border-t border-border">
                <td className="px-3 py-2">
                  <div className="font-medium">{idx.name}</div>
                  <div className="text-xs text-muted-foreground">{idx.id}</div>
                </td>
                <td className="px-3 py-2 text-muted-foreground">{idx.kind}</td>
                <td className="px-3 py-2">{idx.enabled ? "Yes" : "No"}</td>
                <td className="px-3 py-2">
                  <HealthBadge health={idx.health} />
                  {idx.health?.last_error ? (
                    <div className="mt-1 max-w-[24ch] truncate text-xs text-muted-foreground">
                      {idx.health.last_error}
                    </div>
                  ) : null}
                </td>
                <td className="px-3 py-2 text-muted-foreground">
                  {idx.proxy_id
                    ? (proxyById.get(idx.proxy_id) ?? idx.proxy_id)
                    : "—"}
                </td>
                <td className="px-3 py-2 text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`Actions for ${idx.name}`}
                      >
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onSelect={() =>
                          setDialog({ kind: "edit", indexer: idx })
                        }
                      >
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => handleTest(idx)}>
                        Test
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={() =>
                          setDialog({ kind: "search", indexer: idx })
                        }
                      >
                        Search…
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onSelect={() =>
                          setDialog({ kind: "delete", indexer: idx })
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
              {dialog.kind === "edit" ? "Edit indexer" : "Add indexer"}
            </DialogTitle>
            <DialogDescription>
              Configure how Loom talks to this Newznab- or Torznab-compatible
              feed.
            </DialogDescription>
          </DialogHeader>
          {dialog.kind === "create" ? (
            <IndexerForm
              proxies={proxies}
              onSubmit={(v) => handleCreate(v)}
              onCancel={close}
              submitting={create.isPending}
              topError={topError}
            />
          ) : null}
          {dialog.kind === "edit" ? (
            <IndexerForm
              initial={dialog.indexer}
              proxies={proxies}
              onSubmit={(v) => handlePatch(v, dialog.indexer)}
              onCancel={close}
              submitting={patch.isPending}
              topError={topError}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      {/* Search dialog */}
      <Dialog
        open={dialog.kind === "search"}
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>Manual search</DialogTitle>
            <DialogDescription>
              Run an ad-hoc query against a single indexer to verify it is
              returning the releases you expect.
            </DialogDescription>
          </DialogHeader>
          {dialog.kind === "search" ? (
            <SearchPanel indexer={dialog.indexer} onClose={close} />
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
            <DialogTitle>Delete indexer?</DialogTitle>
            <DialogDescription>
              {dialog.kind === "delete"
                ? `Permanently remove “${dialog.indexer.name}”. This cannot be undone.`
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
                dialog.kind === "delete" ? handleDelete(dialog.indexer) : null
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
