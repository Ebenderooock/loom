import { Fragment, useEffect, useId, useRef, useState } from "react";
import {
  Copy,
  FolderSearch,
  History,
  Loader2,
  MoreHorizontal,
  RefreshCw,
  Server,
} from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/hooks/use-auth";
import { apiFetch } from "@/lib/fetch";
import { fetchImportHistory, type ImportRecord } from "@/lib/imports-api";
import type { ScanResult } from "@/hooks/use-library-import";
import { relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface Availability {
  folderPath: string | null;
  unavailableReason: string | null;
  rescanUnavailableReason: string | null;
  plexUnavailableReason: string | null;
}

// These title maintenance requests must never enter acquisition recovery.
async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await apiFetch(url, init, { acquisitionRecovery: false });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(
      (typeof body?.error === "string" ? body.error : body?.error?.message) ||
        `Request failed (${response.status})`,
    );
  }
  return response.json();
}

export function TitleActionsMenu({
  mediaType,
  id,
  title,
  onChanged,
}: {
  mediaType: "movie" | "series";
  id: string;
  title: string;
  onChanged: (signal: AbortSignal) => Promise<void>;
}) {
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const base = `/api/v1/${mediaType === "movie" ? "movies" : "series"}/${encodeURIComponent(id)}`;
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [pending, setPending] = useState<string | null>(null);
  const pendingRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const lifetime = useRef<AbortController | null>(null);
  const [historyOpen, setHistoryOpen] = useState(false);
  const descriptionId = useId();

  useEffect(() => {
    const controller = new AbortController();
    lifetime.current = controller;
    if (isAdmin) {
      void request<Availability>(`${base}/actions`, {
        signal: controller.signal,
      })
        .then((data) => {
          if (
            !data ||
            !(
              data.folderPath === null || typeof data.folderPath === "string"
            ) ||
            !(
              data.unavailableReason === null ||
              typeof data.unavailableReason === "string"
            ) ||
            !(
              data.rescanUnavailableReason === null ||
              typeof data.rescanUnavailableReason === "string"
            ) ||
            !(
              data.plexUnavailableReason === null ||
              typeof data.plexUnavailableReason === "string"
            )
          ) {
            throw new Error("Unexpected title actions response");
          }
          if (!controller.signal.aborted) setAvailability(data);
        })
        .catch((error: unknown) => {
          if (!controller.signal.aborted)
            setLoadError(
              error instanceof Error ? error.message : String(error),
            );
        });
    }
    return () => controller.abort();
  }, [base, isAdmin]);

  const blocked = !isAdmin
    ? "Administrator access required."
    : loadError
      ? `Actions unavailable: ${loadError}`
      : !availability
        ? "Checking action availability…"
        : pending
          ? "Wait for the current action to finish."
          : null;
  const folderBlocked =
    blocked ||
    availability?.unavailableReason ||
    (!availability?.folderPath
      ? "No verified library folder is available."
      : null);
  const rescanBlocked = blocked || availability?.rescanUnavailableReason;
  const plexBlocked = blocked || availability?.plexUnavailableReason;

  const run = async (
    action: string,
    operation: (signal: AbortSignal) => Promise<void>,
  ) => {
    if (
      blocked ||
      pendingRef.current ||
      !lifetime.current ||
      lifetime.current.signal.aborted
    )
      return;
    pendingRef.current = true;
    setPending(action);
    const signal = lifetime.current.signal;
    try {
      await operation(signal);
    } catch (error) {
      if (!signal.aborted)
        toast.error(`${action} failed`, {
          description: error instanceof Error ? error.message : String(error),
        });
    } finally {
      pendingRef.current = false;
      if (!signal.aborted) setPending(null);
    }
  };

  const sync = () => {
    if (plexBlocked) return;
    void run("Sync with Plex", async (signal) => {
      const result = await request<{ message: string; connections: number }>(
        `${base}/sync-plex`,
        { method: "POST", signal },
      );
      if (
        result.message !== "Plex library refresh requested" ||
        !Number.isInteger(result.connections) ||
        result.connections < 1
      ) {
        throw new Error("Plex did not confirm a library refresh request");
      }
      if (!signal.aborted)
        toast.success(result.message, {
          description: `${result.connections} enabled Plex connection(s). Missing media is not downloaded.`,
        });
    });
  };
  const refresh = () =>
    void run("Refresh metadata", async (signal) => {
      const result = await request<{ message: string }>(
        `${base}/refresh-metadata`,
        { method: "POST", signal },
      );
      if (result.message !== "Metadata refreshed")
        throw new Error("Metadata refresh was not confirmed");
      if (signal.aborted) return;
      await onChanged(signal);
      if (!signal.aborted) toast.success("Metadata refreshed");
    });
  const rescan = () => {
    if (rescanBlocked) return;
    void run("Rescan library files", async (signal) => {
      const result = await request<ScanResult>(`${base}/rescan-library`, {
        method: "POST",
        signal,
      });
      if (signal.aborted) return;
      if (result.errors?.length || result.status === "failed") {
        throw new Error(
          result.errors?.join("; ") || "Library file rescan failed",
        );
      }
      if (
        result.status !== "completed" ||
        !Number.isInteger(result.totalFiles) ||
        result.totalFiles < 0
      ) {
        throw new Error("Library file rescan has not confirmed completion");
      }
      await onChanged(signal);
      if (!signal.aborted)
        toast.success(
          `Registered library file check complete: ${result.totalFiles} checked`,
        );
    });
  };
  const copy = () => {
    if (folderBlocked || !availability?.folderPath) return;
    const path = availability.folderPath;
    void run("Copy folder path", async (signal) => {
      if (!navigator.clipboard?.writeText)
        throw new Error(
          "Clipboard access is unavailable. Use a secure context and allow clipboard access.",
        );
      await navigator.clipboard.writeText(path);
      if (!signal.aborted) toast.success("Folder path copied");
    });
  };
  const items = [
    {
      label: "Sync with Plex",
      icon: Server,
      reason: plexBlocked,
      description:
        "Refreshes enabled Plex servers’ libraries. Does not download missing media.",
      handler: sync,
    },
    {
      label: "Refresh metadata",
      icon: RefreshCw,
      reason: blocked,
      description:
        mediaType === "series"
          ? "Refresh title metadata and credits. Does not rebuild the episode catalog."
          : "Refresh title metadata from the metadata provider.",
      handler: refresh,
    },
    {
      label: "Rescan library files",
      icon: FolderSearch,
      reason: rescanBlocked,
      description:
        "Checks registered imported files only; does not discover or ingest new files.",
      handler: rescan,
    },
    {
      label: "View import history",
      icon: History,
      reason: pending ? "Wait for the current action to finish." : null,
      description: "View this title’s import records. Read-only; no retries.",
      handler: () => {
        if (!pending && !pendingRef.current) setHistoryOpen(true);
      },
    },
    {
      label: "Copy folder path",
      icon: Copy,
      reason: folderBlocked,
      description: "Copy the verified library folder path.",
      handler: copy,
    },
  ];

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            className="h-11 w-11 sm:h-8 sm:w-8"
            aria-label="More actions"
            aria-busy={!!pending}
            ref={triggerRef}
          >
            {pending ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          className="w-80 max-w-[calc(100vw-2rem)]"
        >
          {items.map(
            ({ label, icon: Icon, reason, description, handler }, index) => (
              <Fragment key={label}>
                {index === 3 && <DropdownMenuSeparator />}
                <DropdownMenuItem
                  aria-label={label}
                  aria-describedby={`${descriptionId}-${index}`}
                  disabled={!!reason}
                  onSelect={handler}
                  className="items-start py-2"
                >
                  <Icon
                    className="mt-0.5 h-4 w-4 shrink-0"
                    aria-hidden="true"
                  />
                  <div className="min-w-0">
                    <div className="font-medium">{label}</div>
                    <p
                      id={`${descriptionId}-${index}`}
                      className="mt-0.5 text-xs opacity-80"
                    >
                      {description}
                      {reason && <span className="mt-1 block">{reason}</span>}
                    </p>
                  </div>
                </DropdownMenuItem>
              </Fragment>
            ),
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {historyOpen && (
        <TitleImportHistory
          mediaType={mediaType}
          id={id}
          title={title}
          onOpenChange={setHistoryOpen}
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            triggerRef.current?.focus();
          }}
        />
      )}
    </>
  );
}

function TitleImportHistory({
  mediaType,
  id,
  title,
  onOpenChange,
  onCloseAutoFocus,
}: {
  mediaType: "movie" | "series";
  id: string;
  title: string;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const [rows, setRows] = useState<ImportRecord[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [offset, setOffset] = useState(0);
  const [attempt, setAttempt] = useState(0);
  const limit = 50;
  useEffect(() => {
    const controller = new AbortController();
    void fetchImportHistory(
      limit,
      offset,
      { mediaType, mediaId: id },
      controller.signal,
    )
      .then((data) => {
        if (!controller.signal.aborted) setRows(data);
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted)
          setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [mediaType, id, offset, attempt]);
  const loadPage = (next: number) => {
    setLoading(true);
    setError(null);
    setRows([]);
    setOffset(next);
  };
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[80vh] overflow-y-auto sm:max-w-2xl"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogHeader>
          <DialogTitle>Import history — {title}</DialogTitle>
          <DialogDescription>
            Import records for this {mediaType}. Viewing history does not retry
            downloads or imports.
          </DialogDescription>
        </DialogHeader>
        {loading ? (
          <p role="status">Loading import history…</p>
        ) : error ? (
          <div role="alert">
            <p>Could not load import history: {error}</p>
            <Button
              variant="outline"
              onClick={() => {
                setError(null);
                setLoading(true);
                setAttempt((v) => v + 1);
              }}
            >
              Reload history
            </Button>
          </div>
        ) : rows.length === 0 ? (
          <p>
            No import records for this title{offset ? " on this page" : ""}.
          </p>
        ) : (
          <ul className="space-y-3">
            {rows.map((row) => (
              <li key={row.id} className="rounded-md border p-3 text-sm">
                <div className="flex justify-between gap-2">
                  <span className="font-medium">{row.status}</span>
                  <time
                    dateTime={row.imported_at}
                    title={row.imported_at}
                    className="text-xs text-muted-foreground"
                  >
                    {relativeTime(row.imported_at)}
                  </time>
                </div>
                <p className="mt-1 break-all text-xs text-muted-foreground">
                  Source: {row.source_path}
                </p>
                <p className="break-all text-xs text-muted-foreground">
                  Destination: {row.dest_path || "—"}
                </p>
                <p className="text-xs text-muted-foreground">
                  Mode: {row.import_mode}
                </p>
                {row.error && (
                  <p className="mt-1 break-words text-destructive">
                    {row.error}
                  </p>
                )}
              </li>
            ))}
          </ul>
        )}
        <div className="flex items-center justify-between gap-2">
          <Button
            variant="outline"
            disabled={loading || offset === 0}
            onClick={() => loadPage(Math.max(0, offset - limit))}
          >
            Previous
          </Button>
          <span className="text-xs text-muted-foreground">
            Page {offset / limit + 1}
          </span>
          <Button
            variant="outline"
            disabled={loading || !!error || rows.length < limit}
            onClick={() => loadPage(offset + limit)}
          >
            Next
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
