import * as React from "react";
import {
  FolderOpen,
  HardDrive,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Trash2,
  Pencil,
  ChevronRight,
  AlertCircle,
} from "lucide-react";
import { toast } from "sonner";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import {
  useLibraries,
  useCreateLibrary,
  useUpdateLibrary,
  useDeleteLibrary,
  useScanLibrary,
  useUnmappedFolders,
  useFilesystem,
  MEDIA_TYPES,
  formatBytes,
  getMediaTypeLabel,
  type Library,
  type MediaType,
  type CreateLibraryRequest,
  type UpdateLibraryRequest,
  type UnmappedFolder,
  ApiError,
} from "@/lib/libraries-api";

// ---------- Dialog state ----------

type DialogState =
  | { kind: "closed" }
  | { kind: "create" }
  | { kind: "edit"; library: Library }
  | { kind: "delete"; library: Library }
  | { kind: "unmapped"; library: Library };

function errMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError)
    return `${fallback} (HTTP ${err.status}): ${err.message}`;
  if (err instanceof Error) return `${fallback}: ${err.message}`;
  return fallback;
}

// ---------- Main page ----------

export function LibraryPage() {
  useSetPageHeader("Libraries");
  const { data: libraries, isLoading, error } = useLibraries();
  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });
  const scanMut = useScanLibrary();

  const [rescanningAll, setRescanningAll] = React.useState(false);

  const handleScan = (lib: Library) => {
    scanMut.mutate(lib.id, {
      onSuccess: () => toast.success(`Scan started for "${lib.name}"`),
      onError: (err) => toast.error(errMessage(err, "Scan failed")),
    });
  };

  const handleRescanAll = async () => {
    if (!libraries || libraries.length === 0) return;
    setRescanningAll(true);
    let started = 0;
    for (const lib of libraries) {
      try {
        await fetch(`/api/v1/libraries/${lib.id}/scan`, { method: "POST", credentials: "include" });
        started++;
      } catch { /* continue */ }
    }
    setRescanningAll(false);
    toast.success(`Scan started for ${started} ${started === 1 ? "library" : "libraries"}`);
  };

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="flex justify-end">
          <Skeleton className="h-9 w-32" />
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i}>
              <CardHeader>
                <Skeleton className="h-5 w-3/4" />
              </CardHeader>
              <CardContent className="space-y-3">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-3 w-1/2" />
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center gap-2 text-destructive p-4">
        <AlertCircle className="h-4 w-4" />
        <span>{errMessage(error, "Failed to load libraries")}</span>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-end gap-2">
        {libraries && libraries.length > 0 && (
          <Button variant="outline" onClick={handleRescanAll} disabled={rescanningAll}>
            <RefreshCw className={`mr-2 h-4 w-4 ${rescanningAll ? "animate-spin" : ""}`} />
            {rescanningAll ? "Scanning..." : "Rescan All"}
          </Button>
        )}
        <Button onClick={() => setDialog({ kind: "create" })}>
          <Plus className="mr-2 h-4 w-4" />
          Add Library
        </Button>
      </div>

      {libraries && libraries.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12 text-muted-foreground">
            <FolderOpen className="mb-4 h-12 w-12" />
            <p className="text-lg font-medium">No libraries configured</p>
            <p className="text-sm">Add a library to start managing your media.</p>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {libraries?.map((lib) => (
          <LibraryCard
            key={lib.id}
            library={lib}
            onScan={() => handleScan(lib)}
            onEdit={() => setDialog({ kind: "edit", library: lib })}
            onDelete={() => setDialog({ kind: "delete", library: lib })}
            onUnmapped={() => setDialog({ kind: "unmapped", library: lib })}
          />
        ))}
      </div>

      {/* Create dialog */}
      {dialog.kind === "create" && (
        <LibraryFormDialog
          open
          onClose={() => setDialog({ kind: "closed" })}
        />
      )}

      {/* Edit dialog */}
      {dialog.kind === "edit" && (
        <LibraryFormDialog
          open
          library={dialog.library}
          onClose={() => setDialog({ kind: "closed" })}
        />
      )}

      {/* Delete dialog */}
      {dialog.kind === "delete" && (
        <DeleteDialog
          open
          library={dialog.library}
          onClose={() => setDialog({ kind: "closed" })}
        />
      )}

      {/* Unmapped folders dialog */}
      {dialog.kind === "unmapped" && (
        <UnmappedDialog
          open
          library={dialog.library}
          onClose={() => setDialog({ kind: "closed" })}
        />
      )}
    </div>
  );
}

// ---------- Library card ----------

function LibraryCard({
  library,
  onScan,
  onEdit,
  onDelete,
  onUnmapped,
}: {
  library: Library;
  onScan: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onUnmapped: () => void;
}) {
  const usedPct =
    library.disk_space.total_bytes > 0
      ? (library.disk_space.used_bytes / library.disk_space.total_bytes) * 100
      : 0;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-base font-semibold">{library.name}</CardTitle>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8">
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={onScan}>
              <RefreshCw className="mr-2 h-4 w-4" />
              Scan
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onEdit}>
              <Pencil className="mr-2 h-4 w-4" />
              Edit
            </DropdownMenuItem>
            {library.unmapped_count > 0 && (
              <DropdownMenuItem onClick={onUnmapped}>
                <FolderOpen className="mr-2 h-4 w-4" />
                Unmapped Folders ({library.unmapped_count})
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onDelete} className="text-destructive">
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <FolderOpen className="h-4 w-4 shrink-0" />
          <span className="truncate">{library.path}</span>
        </div>

        <div className="flex items-center gap-2">
          <Badge variant={library.accessible ? "default" : "destructive"}>
            {library.accessible ? "Online" : "Offline"}
          </Badge>
          <Badge variant="outline">{getMediaTypeLabel(library.media_type as MediaType)}</Badge>
        </div>

        {library.accessible && library.disk_space.total_bytes > 0 && (
          <div className="space-y-1">
            <div className="flex justify-between text-xs text-muted-foreground">
              <span className="flex items-center gap-1">
                <HardDrive className="h-3 w-3" />
                {formatBytes(library.disk_space.free_bytes)} free
              </span>
              <span>{formatBytes(library.disk_space.total_bytes)} total</span>
            </div>
            <Progress value={usedPct} className="h-2" />
          </div>
        )}

        <div className="flex justify-between text-sm">
          <span>{library.file_count} files</span>
          {library.unmapped_count > 0 && (
            <button
              onClick={onUnmapped}
              className="text-yellow-500 hover:underline cursor-pointer text-xs"
            >
              {library.unmapped_count} unmapped
            </button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------- Filesystem browser ----------

function FolderBrowser({
  value,
  onChange,
}: {
  value: string;
  onChange: (path: string) => void;
}) {
  const [browsePath, setBrowsePath] = React.useState(value || "");
  const { data: fs } = useFilesystem(browsePath || undefined);

  return (
    <div className="space-y-2">
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="/path/to/media"
      />
      {fs && (
        <div className="max-h-40 overflow-y-auto rounded border bg-muted/50 text-sm">
          {fs.parent && (
            <button
              className="flex w-full items-center gap-1 px-2 py-1 hover:bg-accent text-left"
              onClick={() => {
                setBrowsePath(fs.parent);
                onChange(fs.parent);
              }}
            >
              <ChevronRight className="h-3 w-3 rotate-180" />
              <span>..</span>
            </button>
          )}
          {fs.directories?.map((dir) => (
            <button
              key={dir.path}
              className="flex w-full items-center gap-1 px-2 py-1 hover:bg-accent text-left"
              onClick={() => {
                setBrowsePath(dir.path);
                onChange(dir.path);
              }}
            >
              <FolderOpen className="h-3 w-3 shrink-0 text-muted-foreground" />
              <span className="truncate">{dir.name}</span>
            </button>
          ))}
          {fs.directories?.length === 0 && (
            <p className="px-2 py-1 text-muted-foreground">No subdirectories</p>
          )}
        </div>
      )}
    </div>
  );
}

// ---------- Create/Edit form ----------

function LibraryFormDialog({
  open,
  library,
  onClose,
}: {
  open: boolean;
  library?: Library;
  onClose: () => void;
}) {
  const isEdit = !!library;
  const createMut = useCreateLibrary();
  const updateMut = useUpdateLibrary();

  const [name, setName] = React.useState(library?.name ?? "");
  const [path, setPath] = React.useState(library?.path ?? "");
  const [mediaType, setMediaType] = React.useState<MediaType>(
    (library?.media_type as MediaType) ?? "movie"
  );
  const [unmonitorOnDelete, setUnmonitorOnDelete] = React.useState(
    library?.unmonitor_on_delete ?? false
  );
  const [autoArchiveWatched, setAutoArchiveWatched] = React.useState(
    library?.auto_archive_watched ?? false
  );
  const [autoArchiveDays, setAutoArchiveDays] = React.useState(
    library?.auto_archive_days_after_watch ?? 0
  );

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !path) return;

    if (isEdit) {
      const body: UpdateLibraryRequest = {};
      if (name !== library.name) body.name = name;
      if (path !== library.path) body.path = path;
      if (mediaType !== library.media_type) body.media_type = mediaType;
      if (unmonitorOnDelete !== library.unmonitor_on_delete)
        body.unmonitor_on_delete = unmonitorOnDelete;
      if (autoArchiveWatched !== library.auto_archive_watched)
        body.auto_archive_watched = autoArchiveWatched;
      if (autoArchiveDays !== library.auto_archive_days_after_watch)
        body.auto_archive_days_after_watch = autoArchiveDays;
      updateMut.mutate(
        { id: library.id, body },
        {
          onSuccess: () => {
            toast.success(`Library "${name}" updated`);
            onClose();
          },
          onError: (err) => toast.error(errMessage(err, "Update failed")),
        }
      );
    } else {
      const body: CreateLibraryRequest = {
        name,
        path,
        media_type: mediaType,
        unmonitor_on_delete: unmonitorOnDelete,
        auto_archive_watched: autoArchiveWatched,
        auto_archive_days_after_watch: autoArchiveDays,
      };
      createMut.mutate(body, {
        onSuccess: () => {
          toast.success(`Library "${name}" added`);
          onClose();
        },
        onError: (err) => toast.error(errMessage(err, "Create failed")),
      });
    }
  };

  const isPending = createMut.isPending || updateMut.isPending;

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit Library" : "Add Library"}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? "Update the library configuration."
              : "Add a library to monitor for media files."}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="lib-name">Name</Label>
            <Input
              id="lib-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Movies Library"
              required
            />
          </div>

          <div className="space-y-2">
            <Label>Path</Label>
            <FolderBrowser value={path} onChange={setPath} />
          </div>

          <div className="space-y-2">
            <Label htmlFor="lib-media-type">Media Type</Label>
            <Select value={mediaType} onValueChange={(v) => setMediaType(v as MediaType)}>
              <SelectTrigger id="lib-media-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {MEDIA_TYPES.map((mt) => (
                  <SelectItem key={mt.value} value={mt.value}>
                    {mt.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-3 border-t pt-3">
            <p className="text-sm font-medium text-muted-foreground">Lifecycle Settings</p>

            <div className="flex items-center justify-between">
              <Label htmlFor="lib-unmonitor-delete" className="text-sm">
                Unmonitor on delete
              </Label>
              <Switch
                id="lib-unmonitor-delete"
                checked={unmonitorOnDelete}
                onCheckedChange={setUnmonitorOnDelete}
              />
            </div>

            <div className="flex items-center justify-between">
              <Label htmlFor="lib-auto-archive" className="text-sm">
                Auto-archive watched (Trakt)
              </Label>
              <Switch
                id="lib-auto-archive"
                checked={autoArchiveWatched}
                onCheckedChange={setAutoArchiveWatched}
              />
            </div>

            {autoArchiveWatched && (
              <div className="space-y-2 pl-1">
                <Label htmlFor="lib-archive-days" className="text-sm">
                  Days after watch before archiving (0 = immediate)
                </Label>
                <Input
                  id="lib-archive-days"
                  type="number"
                  min={0}
                  value={autoArchiveDays}
                  onChange={(e) => setAutoArchiveDays(Number(e.target.value))}
                />
              </div>
            )}
          </div>

          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={isPending || !name || !path}>
              {isPending ? "Saving..." : isEdit ? "Save" : "Add Library"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Delete confirmation ----------

function DeleteDialog({
  open,
  library,
  onClose,
}: {
  open: boolean;
  library: Library;
  onClose: () => void;
}) {
  const deleteMut = useDeleteLibrary();

  const handleDelete = () => {
    deleteMut.mutate(library.id, {
      onSuccess: () => {
        toast.success(`Library "${library.name}" deleted`);
        onClose();
      },
      onError: (err) => toast.error(errMessage(err, "Delete failed")),
    });
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Library</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete &quot;{library.name}&quot;? This removes the
            library from Loom but does not delete any files on disk.
          </DialogDescription>
        </DialogHeader>
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={deleteMut.isPending}
          >
            {deleteMut.isPending ? "Deleting..." : "Delete"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Unmapped folders ----------

function UnmappedDialog({
  open,
  library,
  onClose,
}: {
  open: boolean;
  library: Library;
  onClose: () => void;
}) {
  const { data: folders, isLoading } = useUnmappedFolders(library.id);

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Unmapped Folders — {library.name}</DialogTitle>
          <DialogDescription>
            These subfolders don&apos;t match any known media in your library.
          </DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        ) : folders && folders.length > 0 ? (
          <div className="max-h-64 overflow-y-auto space-y-1">
            {folders.map((f: UnmappedFolder) => (
              <div
                key={f.path}
                className="flex items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-accent"
              >
                <FolderOpen className="h-4 w-4 shrink-0 text-yellow-500" />
                <span className="truncate">{f.name}</span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground py-4 text-center">
            All folders are mapped!
          </p>
        )}
        <div className="flex justify-end">
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
