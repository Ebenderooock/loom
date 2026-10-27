import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderOpen, Loader2, Trash2 } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";
import {
  deleteMediaFiles,
  getMediaFiles,
  type DeletionMode,
  type MediaFilesScope,
} from "@/lib/media-files-api";
import { formatBytes } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

type Props = MediaFilesScope & { title: string; onChanged?: () => void };
type Confirmation = { paths: string[]; deletionMode: DeletionMode };
const MAX_SELECTED_FILES = 1000;

export function MediaFilesManager(props: Props) {
  const { user, isAuthenticated } = useAuth();
  // GET is admin-only too: do not mount queries or expose cached files otherwise.
  if (!isAuthenticated || user?.role !== "admin") return null;
  return (
    <AdminMediaFilesManager
      key={`${props.mediaType}:${props.id}:${props.season ?? "all"}:${user.id}`}
      {...props}
      userId={user.id}
    />
  );
}

function AdminMediaFilesManager({
  title,
  onChanged,
  userId,
  ...scope
}: Props & { userId: number }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [notice, setNotice] = useState("");
  const cancelRef = useRef<HTMLButtonElement>(null);
  const deleteTriggerRef = useRef<HTMLButtonElement>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const queryClient = useQueryClient();
  const titleKey = ["media-files", userId, scope.mediaType, scope.id];
  const inventory = useQuery({
    queryKey: [...titleKey, scope.season ?? "all"],
    queryFn: ({ signal }) => getMediaFiles(scope, signal),
    enabled: open,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
  const deletion = useMutation({
    retry: false,
    mutationFn: ({ paths, deletionMode }: Confirmation) =>
      deleteMediaFiles(scope, paths, deletionMode),
    onSuccess: ({ deleted }, { paths, deletionMode }) => {
      setNotice(
        `${deleted.length} of ${paths.length} selected files ${deletionMode === "recycle" ? "recycled" : "permanently deleted"}.`,
      );
    },
    onSettled: async () => {
      // Runtime failures can still delete some files. Never offer stale selections
      // for retry; invalidate the whole title, including other season inventories.
      setSelected([]);
      setConfirmation(null);
      await queryClient.invalidateQueries({ queryKey: titleKey });
      onChanged?.();
    },
  });
  const busy = deletion.isPending;
  const files = inventory.data?.files ?? [];
  const selectedPaths = [
    ...new Set(
      files
        .filter((file) => selected.includes(file.path))
        .map((file) => file.path),
    ),
  ];
  const selectAllPaths = [...new Set(files.map((file) => file.path))].slice(
    0,
    MAX_SELECTED_FILES,
  );
  const selectionAtLimit = selectedPaths.length >= MAX_SELECTED_FILES;
  const unavailable = busy || inventory.isFetching || inventory.isError;
  const changeOpen = (next: boolean) => {
    if (busy) return;
    setOpen(next);
    setSelected([]);
    setConfirmation(null);
    setNotice("");
    deletion.reset();
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="gap-2" disabled={busy}>
          <FolderOpen className="h-4 w-4" aria-hidden="true" />
          Manage files
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-2xl" closeDisabled={busy}>
        <DialogHeader>
          <DialogTitle ref={titleRef} tabIndex={-1}>
            Manage files — {title}
          </DialogTitle>
          <DialogDescription>
            All files, including extras and subtitles. Paths are relative to the
            title folder. Select only the files you want to remove.
          </DialogDescription>
        </DialogHeader>

        {inventory.isFetching && (
          <p role="status" className="flex items-center gap-2 text-sm">
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            Loading files…
          </p>
        )}
        {inventory.isError && (
          <div role="alert" className="space-y-2 text-sm text-destructive">
            <p>Could not load files: {inventory.error.message}</p>
            <Button
              variant="outline"
              size="sm"
              disabled={busy || inventory.isFetching}
              onClick={() => void inventory.refetch()}
            >
              Retry loading files
            </Button>
          </div>
        )}
        {deletion.isError && (
          <p role="alert" className="text-sm text-destructive">
            Could not delete files: {deletion.error.message}. Some files may
            already have been deleted. Review the refreshed list before trying
            again.
          </p>
        )}
        {notice && (
          <p role="status" className="text-sm">
            {notice}
          </p>
        )}
        {inventory.data && !inventory.isError && (
          <>
            <p className="text-sm text-muted-foreground">
              {inventory.data.deletionMode === "recycle"
                ? "Selected files will be recycled."
                : "Selected files will be permanently deleted. This cannot be undone."}
            </p>
            <p className="text-sm text-muted-foreground" role="status">
              Select up to 1,000 files per operation.{" "}
              {selectedPaths.length.toLocaleString()} selected.
              {selectionAtLimit &&
                " Selection limit reached. Deselect a file to choose another."}
            </p>
            {files.length === 0 ? (
              <p className="py-4 text-sm text-muted-foreground">
                No files found.
              </p>
            ) : (
              <fieldset disabled={unavailable} className="min-w-0 space-y-2">
                <legend className="sr-only">Files to remove</legend>
                <label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm">
                  <input
                    type="checkbox"
                    checked={
                      selectAllPaths.length > 0 &&
                      selectAllPaths.every((path) => selected.includes(path))
                    }
                    onChange={(event) =>
                      setSelected(event.target.checked ? selectAllPaths : [])
                    }
                    className="h-4 w-4 accent-primary"
                  />
                  {files.length > MAX_SELECTED_FILES
                    ? "Select first 1,000 files"
                    : "Select all files"}
                </label>
                <div className="max-h-80 space-y-2 overflow-y-auto">
                  {files.map((file) => (
                    <label
                      key={file.path}
                      className="flex cursor-pointer items-start gap-3 rounded-lg border border-border/30 bg-muted/20 p-3"
                    >
                      <input
                        type="checkbox"
                        aria-label={`Select ${file.path}`}
                        checked={selected.includes(file.path)}
                        disabled={
                          selectionAtLimit && !selected.includes(file.path)
                        }
                        onChange={(event) =>
                          setSelected((previous) =>
                            event.target.checked
                              ? [...previous, file.path]
                              : previous.filter((path) => path !== file.path),
                          )
                        }
                        className="mt-1 h-4 w-4 shrink-0 accent-primary"
                      />
                      <span className="min-w-0 space-y-1">
                        <span className="block break-all text-sm font-medium">
                          {file.path}
                        </span>
                        <span className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span>{formatBytes(file.size)}</span>
                          {file.quality && (
                            <Badge variant="outline">{file.quality}</Badge>
                          )}
                          {file.format && <span>{file.format}</span>}
                          <Badge variant="secondary">
                            {file.tracked ? "Tracked" : "Untracked"}
                          </Badge>
                        </span>
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>
            )}
          </>
        )}
        <DialogFooter className="gap-2">
          <Button
            variant="outline"
            onClick={() => changeOpen(false)}
            disabled={busy}
          >
            Close
          </Button>
          <Button
            ref={deleteTriggerRef}
            variant="destructive"
            disabled={
              unavailable || selectedPaths.length === 0 || !inventory.data
            }
            onClick={() => {
              if (!inventory.data) return;
              setConfirmation({
                paths: selectedPaths,
                deletionMode: inventory.data.deletionMode,
              });
            }}
          >
            <Trash2 className="mr-2 h-4 w-4" aria-hidden="true" />
            {busy ? "Deleting…" : `Delete selected (${selectedPaths.length})`}
          </Button>
        </DialogFooter>

        <Dialog
          open={confirmation !== null}
          onOpenChange={(next) => {
            if (!next && !busy) setConfirmation(null);
          }}
        >
          <DialogContent
            closeDisabled={busy}
            onOpenAutoFocus={(event) => {
              event.preventDefault();
              cancelRef.current?.focus();
            }}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              if (busy) titleRef.current?.focus();
              else deleteTriggerRef.current?.focus();
            }}
          >
            <DialogHeader>
              <DialogTitle>
                {confirmation?.deletionMode === "recycle"
                  ? "Recycle selected files?"
                  : "Permanently delete selected files?"}
              </DialogTitle>
              <DialogDescription>
                {confirmation?.deletionMode === "recycle"
                  ? "Each file listed below will be recycled."
                  : "Each file listed below will be permanently deleted. This cannot be undone."}
              </DialogDescription>
            </DialogHeader>
            <ul className="max-h-64 space-y-2 overflow-y-auto rounded-lg border border-destructive/20 bg-destructive/5 p-3 text-sm">
              {confirmation?.paths.map((path) => (
                <li key={path} className="break-all">
                  {path}
                </li>
              ))}
            </ul>
            <DialogFooter className="gap-2">
              <Button
                ref={cancelRef}
                variant="outline"
                disabled={busy}
                onClick={() => setConfirmation(null)}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                disabled={busy}
                onClick={() => {
                  if (confirmation && !busy) deletion.mutate(confirmation);
                }}
              >
                {busy
                  ? "Deleting…"
                  : confirmation?.deletionMode === "recycle"
                    ? "Recycle files"
                    : "Permanently delete files"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </DialogContent>
    </Dialog>
  );
}
