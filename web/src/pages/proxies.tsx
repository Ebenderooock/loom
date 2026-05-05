// ProxiesPage lists configured outbound proxies and exposes the
// add/edit/delete/test actions. Credentials are masked in the table
// and only shown in the edit form.

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
import {
  ProxyForm,
  toProxyCreate,
  toProxyPatch,
  type ProxyFormValues,
} from "@/components/proxies/proxy-form";
import {
  ApiError,
  useCreateProxy,
  useDeleteProxy,
  usePatchProxy,
  useProxies,
  useTestProxy,
  type Proxy,
} from "@/lib/indexers-api";

type DialogState =
  | { kind: "closed" }
  | { kind: "create" }
  | { kind: "edit"; proxy: Proxy }
  | { kind: "delete"; proxy: Proxy };

function errMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError)
    return `${fallback} (HTTP ${err.status}): ${err.message}`;
  if (err instanceof Error) return `${fallback}: ${err.message}`;
  return fallback;
}

// Mask credentials embedded in a URL so we can show it safely in the table.
export function maskUrlCredentials(raw: string): string {
  if (!raw) return "—";
  try {
    const u = new URL(raw);
    if (u.username || u.password) {
      u.username = u.username ? "***" : "";
      u.password = u.password ? "***" : "";
    }
    return u.toString();
  } catch {
    return raw;
  }
}

function describeProxy(p: Proxy): string {
  const cfg = p.config as unknown as Record<string, unknown>;
  if (p.kind === "socks5" && typeof cfg.address === "string") {
    return cfg.username ? `${cfg.address} (auth)` : cfg.address;
  }
  if (typeof cfg.url === "string") {
    return maskUrlCredentials(cfg.url);
  }
  return "—";
}

export function ProxiesPage() {
  useSetPageHeader("Proxies");
  const proxiesQ = useProxies();
  const create = useCreateProxy();
  const patch = usePatchProxy();
  const del = useDeleteProxy();
  const test = useTestProxy();

  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });
  const [topError, setTopError] = React.useState<string | undefined>();

  function close() {
    setDialog({ kind: "closed" });
    setTopError(undefined);
  }

  async function handleCreate(values: ProxyFormValues) {
    setTopError(undefined);
    try {
      await create.mutateAsync(toProxyCreate(values));
      toast.success(`Proxy “${values.name}” added.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not create proxy"));
    }
  }

  async function handlePatch(values: ProxyFormValues, original: Proxy) {
    setTopError(undefined);
    try {
      await patch.mutateAsync({ id: original.id, patch: toProxyPatch(values) });
      toast.success(`Proxy “${values.name}” updated.`);
      close();
    } catch (err) {
      setTopError(errMessage(err, "Could not update proxy"));
    }
  }

  async function handleDelete(proxy: Proxy) {
    try {
      await del.mutateAsync(proxy.id);
      toast.success(`Proxy “${proxy.name}” deleted.`);
      close();
    } catch (err) {
      // 409 with proxy_in_use is the most common failure mode here.
      const detail =
        err instanceof ApiError && err.code === "proxy_in_use"
          ? "Detach this proxy from any indexers using it before deleting."
          : undefined;
      toast.error(
        detail ? `${errMessage(err, "Could not delete proxy")} ${detail}` : errMessage(err, "Could not delete proxy"),
      );
    }
  }

  async function handleTest(proxy: Proxy) {
    try {
      const res = await test.mutateAsync(proxy.id);
      if (res.ok) {
        toast.success(`“${proxy.name}” reachable (${res.latency_ms} ms).`);
      } else {
        toast.error(
          `“${proxy.name}” failed: ${res.error ?? `HTTP ${res.status_code ?? "?"}`}.`,
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
          Add proxy
        </Button>
      </div>

      {proxiesQ.isError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {errMessage(proxiesQ.error, "Could not load proxies")}
        </div>
      ) : null}

      <div className="overflow-x-auto rounded-md border border-border">
        <table className="w-full text-sm">
          <caption className="sr-only">Configured proxies</caption>
          <thead className="bg-muted/50 text-left">
            <tr>
              <th scope="col" className="px-3 py-2">
                Name
              </th>
              <th scope="col" className="px-3 py-2">
                Kind
              </th>
              <th scope="col" className="px-3 py-2">
                Endpoint
              </th>
              <th scope="col" className="px-3 py-2">
                Enabled
              </th>
              <th scope="col" className="px-3 py-2 text-right">
                Actions
              </th>
            </tr>
          </thead>
          <tbody>
            {proxiesQ.isLoading ? (
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
            ) : null}
            {!proxiesQ.isLoading && (proxiesQ.data?.length ?? 0) === 0 ? (
              <tr>
                <td
                  colSpan={5}
                  className="px-3 py-6 text-center text-muted-foreground"
                >
                  No proxies configured.
                </td>
              </tr>
            ) : null}
            {(proxiesQ.data ?? []).map((p) => (
              <tr key={p.id} className="border-t border-border">
                <td className="px-3 py-2">
                  <div className="font-medium">{p.name}</div>
                  <div className="text-xs text-muted-foreground">{p.id}</div>
                </td>
                <td className="px-3 py-2 text-muted-foreground">{p.kind}</td>
                <td className="px-3 py-2 font-mono text-xs">
                  {describeProxy(p)}
                </td>
                <td className="px-3 py-2">{p.enabled ? "Yes" : "No"}</td>
                <td className="px-3 py-2 text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`Actions for ${p.name}`}
                      >
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onSelect={() => setDialog({ kind: "edit", proxy: p })}
                      >
                        Edit
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => handleTest(p)}>
                        Test
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onSelect={() => setDialog({ kind: "delete", proxy: p })}
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

      <Dialog
        open={dialog.kind === "create" || dialog.kind === "edit"}
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {dialog.kind === "edit" ? "Edit proxy" : "Add proxy"}
            </DialogTitle>
            <DialogDescription>
              Outbound proxies are referenced by indexers via their ID.
            </DialogDescription>
          </DialogHeader>
          {dialog.kind === "create" ? (
            <ProxyForm
              onSubmit={(v) => handleCreate(v)}
              onCancel={close}
              submitting={create.isPending}
              topError={topError}
            />
          ) : null}
          {dialog.kind === "edit" ? (
            <ProxyForm
              initial={dialog.proxy}
              onSubmit={(v) => handlePatch(v, dialog.proxy)}
              onCancel={close}
              submitting={patch.isPending}
              topError={topError}
            />
          ) : null}
        </DialogContent>
      </Dialog>

      <Dialog
        open={dialog.kind === "delete"}
        onOpenChange={(open) => {
          if (!open) close();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete proxy?</DialogTitle>
            <DialogDescription>
              {dialog.kind === "delete"
                ? `Permanently remove “${dialog.proxy.name}”. Indexers still pinned to this proxy will block deletion.`
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
                dialog.kind === "delete" ? handleDelete(dialog.proxy) : null
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
