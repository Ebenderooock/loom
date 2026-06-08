import * as React from "react";
import { apiFetch } from "@/lib/fetch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { TableSkeleton } from "@/components/ui/skeletons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { toast } from "sonner";
import { NamingSettings } from "@/components/movies/naming-settings";
import {
  useLibraries,
  useDeleteLibrary,
  createLibrary,
  formatBytes,
} from "@/lib/libraries-api";
import {
  Plus,
  Trash2,
  Folder,
  FolderOpen,
  Loader2,
  HardDrive,
  ChevronRight,
  ArrowUp,
  Film,
  Tv,
  Pencil,
  Copy,
  Check,
  Key,
  Shield,
  Plug,
  ExternalLink,
  Download,
  Palette,
  LayoutGrid,
  List,
  GripVertical,
  Search,
  MoreHorizontal,
} from "lucide-react";
import {
  useMediaPreferences,
  useUpdateMediaPreferences,
  useParseReleaseName,
} from "@/lib/media-info-api";
import { useQualityProfiles } from "@/lib/quality-profiles-api";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Switch } from "@/components/ui/switch";
import { useAuth } from "@/hooks/use-auth";
import { useFeatures, useSetFeature } from "@/lib/features-api";
import {
  useConnections as useConnectConnections,
  useCreateConnection as useCreateConnect,
  useUpdateConnection as useUpdateConnect,
  useDeleteConnection as useDeleteConnect,
  useTestConnection as useTestConnect,
  useTestConnectionConfig as useTestConnectConfig,
  useTraktAuthorize,
  useTraktCallback,
  useTraktRefreshToken,
  useTraktSyncWatched,
  useTraktSyncCollection,
  useTraktSyncWatchlist,
  PROVIDER_TYPES,
  type ConnectConnection,
  type ProviderType as ConnectProviderType,
  type CreateConnectRequest,
} from "@/lib/connect-api";
import {
  useRemotePathMappings,
  useCreateRemotePathMapping,
  useDeleteRemotePathMapping,
  type RemotePathMapping,
} from "@/lib/remote-paths-api";
import {
  useDownloads,
  useCreateDownload,
  usePatchDownload,
  useDeleteDownload,
  type Download as DownloadClient,
} from "@/lib/downloads-api";
import { DownloadForm } from "@/components/downloads/download-form";

// ─── Types ──────────────────────────────────────────────────────────────

// ─── Libraries Panel ─────────────────────────────────────────────────

// ─── Filesystem Browser Dialog ──────────────────────────────────────────

interface DirEntry {
  name: string;
  path: string;
}

interface BrowseResult {
  parent: string;
  current: string;
  directories: DirEntry[];
}

function FolderBrowserDialog({
  open,
  onOpenChange,
  onSelect,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (path: string) => void;
}) {
  const [currentPath, setCurrentPath] = React.useState("");
  const [dirs, setDirs] = React.useState<DirEntry[]>([]);
  const [parent, setParent] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const [error, setError] = React.useState("");
  const [manualPath, setManualPath] = React.useState("");
  const [mode, setMode] = React.useState<"browse" | "manual">("browse");

  const browse = React.useCallback(async (path: string) => {
    setLoading(true);
    setError("");
    try {
      const params = path ? `?path=${encodeURIComponent(path)}` : "";
      const res = await apiFetch(`/api/v1/filesystem${params}`);
      if (!res.ok) {
        const data = await res
          .json()
          .catch(() => ({ error: "Failed to browse" }));
        throw new Error(data.error || "Failed to browse directory");
      }
      const data: BrowseResult = await res.json();
      setCurrentPath(data.current);
      setDirs(data.directories ?? []);
      setParent(data.parent);
      setManualPath(data.current);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to browse");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    if (open) {
      setMode("browse");
      setError("");
      browse("");
    }
  }, [open, browse]);

  const handleSelect = () => {
    const path = mode === "manual" ? manualPath.trim() : currentPath;
    if (path) {
      onSelect(path);
      onOpenChange(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[80vh] flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Choose Folder</DialogTitle>
        </DialogHeader>

        {/* Mode toggle */}
        <div className="flex gap-1 rounded-lg bg-muted p-1">
          <button
            type="button"
            onClick={() => setMode("browse")}
            className={cn(
              "flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
              mode === "browse"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            Browse
          </button>
          <button
            type="button"
            onClick={() => setMode("manual")}
            className={cn(
              "flex-1 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
              mode === "manual"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            Enter Manually
          </button>
        </div>

        {mode === "manual" ? (
          <div className="space-y-3 py-2">
            <p className="text-xs text-muted-foreground">
              Enter the full path to the directory where media will be stored.
            </p>
            <Input
              placeholder="/path/to/media"
              value={manualPath}
              onChange={(e) => setManualPath(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSelect()}
              className="font-mono text-sm"
              // eslint-disable-next-line jsx-a11y/no-autofocus -- intentional: focus the manual-path input on open
              autoFocus
            />
          </div>
        ) : (
          <div className="min-h-0 flex-1 space-y-2">
            {/* Current path breadcrumb */}
            <div className="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
              <FolderOpen className="h-4 w-4 shrink-0 text-primary" />
              <span className="flex-1 truncate font-mono text-sm">
                {currentPath || "/"}
              </span>
            </div>

            {error && (
              <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}

            {loading ? (
              <div className="py-2">
                <TableSkeleton rows={4} cols={3} />
              </div>
            ) : (
              <div className="max-h-[40vh] overflow-y-auto rounded-md border border-border">
                {/* Parent directory */}
                {parent && (
                  <button
                    type="button"
                    onClick={() => browse(parent)}
                    className="flex w-full items-center gap-3 border-b border-border px-3 py-2.5 text-sm transition-colors hover:bg-accent/50"
                  >
                    <ArrowUp className="h-4 w-4 text-muted-foreground" />
                    <span className="text-muted-foreground">..</span>
                  </button>
                )}
                {dirs.length === 0 ? (
                  <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                    No subdirectories
                  </div>
                ) : (
                  dirs.map((dir) => (
                    <button
                      key={dir.path}
                      type="button"
                      onClick={() => browse(dir.path)}
                      className="flex w-full items-center gap-3 border-b border-border px-3 py-2.5 text-sm transition-colors last:border-b-0 hover:bg-accent/50"
                    >
                      <Folder className="h-4 w-4 text-primary/70" />
                      <span className="flex-1 truncate text-left">
                        {dir.name}
                      </span>
                      <ChevronRight className="h-3.5 w-3.5 text-muted-foreground/50" />
                    </button>
                  ))
                )}
              </div>
            )}
          </div>
        )}

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-border pt-2">
          <p className="max-w-[60%] truncate text-xs text-muted-foreground">
            {mode === "browse" ? currentPath : manualPath || "No path entered"}
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSelect}
              disabled={mode === "manual" ? !manualPath.trim() : !currentPath}
            >
              Select Folder
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ─── Add Library Dialog (name + type + folder picker) ──────────────

function AddLibraryDialog({
  open,
  onOpenChange,
  onAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAdded: () => void;
}) {
  const [step, setStep] = React.useState<"type" | "path">("type");
  const [mediaType, setMediaType] = React.useState<string | null>(null);
  const [showBrowser, setShowBrowser] = React.useState(false);
  const [name, setName] = React.useState("");
  const [path, setPath] = React.useState("");
  const [adding, setAdding] = React.useState(false);
  const [error, setError] = React.useState("");

  React.useEffect(() => {
    if (open) {
      setStep("type");
      setMediaType(null);
      setName("");
      setPath("");
      setError("");
    }
  }, [open]);

  const selectType = (type: string) => {
    setMediaType(type);
    setStep("path");
  };

  const addLibrary = async () => {
    const trimmedName = name.trim();
    const trimmedPath = path.trim();
    if (!trimmedName || !trimmedPath || !mediaType) return;
    setAdding(true);
    setError("");
    try {
      await createLibrary({
        name: trimmedName,
        path: trimmedPath,
        media_type: mediaType as "movie" | "series" | "music",
      });
      onAdded();
      onOpenChange(false);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to add library");
    } finally {
      setAdding(false);
    }
  };

  return (
    <>
      <Dialog open={open && !showBrowser} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {step === "type"
                ? "Add Library"
                : `Add ${mediaType === "movie" ? "Movies" : mediaType === "series" ? "TV Shows" : "Music"} Library`}
            </DialogTitle>
          </DialogHeader>

          {step === "type" ? (
            <div className="space-y-3 py-2">
              <p className="text-sm text-muted-foreground">
                What type of media will this library contain?
              </p>
              <div className="grid grid-cols-2 gap-3">
                <button
                  type="button"
                  onClick={() => selectType("movie")}
                  className="group flex flex-col items-center gap-3 rounded-lg border-2 border-border p-6 transition-all hover:border-primary hover:bg-primary/5"
                >
                  <div className="rounded-full bg-primary/10 p-3 transition-colors group-hover:bg-primary/20">
                    <Film className="h-8 w-8 text-primary" />
                  </div>
                  <div className="text-center">
                    <p className="text-sm font-medium">Movies</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      Feature films and standalone titles
                    </p>
                  </div>
                </button>
                <button
                  type="button"
                  onClick={() => selectType("series")}
                  className="group flex flex-col items-center gap-3 rounded-lg border-2 border-border p-6 transition-all hover:border-primary hover:bg-primary/5"
                >
                  <div className="rounded-full bg-primary/10 p-3 transition-colors group-hover:bg-primary/20">
                    <Tv className="h-8 w-8 text-primary" />
                  </div>
                  <div className="text-center">
                    <p className="text-sm font-medium">TV Shows</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      Series, seasons and episodes
                    </p>
                  </div>
                </button>
              </div>
            </div>
          ) : (
            <div className="space-y-4 py-2">
              <p className="text-sm text-muted-foreground">
                Give your library a name and choose where your{" "}
                {mediaType === "movie" ? "movies" : "TV shows"} are stored.
              </p>

              {error && (
                <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                  {error}
                </div>
              )}

              <Input
                placeholder="Library name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="text-sm"
                // eslint-disable-next-line jsx-a11y/no-autofocus -- intentional: focus the rename input when editing
                autoFocus
              />

              <div className="flex gap-2">
                <Input
                  placeholder="/path/to/media"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && addLibrary()}
                  className="font-mono text-sm"
                />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setShowBrowser(true)}
                  title="Browse filesystem"
                >
                  <FolderOpen className="h-4 w-4" />
                </Button>
              </div>

              <div className="flex justify-between">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setStep("type")}
                >
                  ← Back
                </Button>
                <Button
                  size="sm"
                  onClick={addLibrary}
                  disabled={adding || !name.trim() || !path.trim()}
                >
                  {adding ? (
                    <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                  ) : (
                    <Plus className="mr-1 h-4 w-4" />
                  )}
                  Add Library
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <FolderBrowserDialog
        open={showBrowser}
        onOpenChange={setShowBrowser}
        onSelect={(selected) => {
          setPath(selected);
          setShowBrowser(false);
        }}
      />
    </>
  );
}

// ─── Libraries Panel ─────────────────────────────────────────────────

function LibrariesPanel() {
  const { data: libraries = [], refetch, isLoading } = useLibraries();
  const deleteMutation = useDeleteLibrary();
  const [deletingId, setDeletingId] = React.useState<string | null>(null);
  const [error, setError] = React.useState("");
  const [dialogOpen, setDialogOpen] = React.useState(false);

  const handleDelete = async (id: string) => {
    setDeletingId(id);
    setError("");
    try {
      await deleteMutation.mutateAsync(id);
    } catch {
      setError("Failed to delete library");
    } finally {
      setDeletingId(null);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center gap-2 py-8 text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading libraries…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="mb-1 text-sm font-medium">Libraries</h3>
          <p className="text-xs text-muted-foreground">
            Libraries are the directories where Loom stores your media files.
            Each movie or show is placed in a subfolder within the library you
            assign when adding it.
          </p>
        </div>
        <Button size="sm" onClick={() => setDialogOpen(true)}>
          <Plus className="mr-1 h-4 w-4" /> Add
        </Button>
      </div>

      {error && (
        <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Library list */}
      {libraries.length === 0 ? (
        <div className="rounded-lg border border-dashed border-muted-foreground/30 py-10 text-center">
          <Folder className="mx-auto mb-3 h-10 w-10 text-muted-foreground/40" />
          <p className="text-sm text-muted-foreground">
            No libraries configured
          </p>
          <p className="mt-1 text-xs text-muted-foreground/60">
            Click <strong>Add</strong> to configure your first media library
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {libraries.map((lib) => (
            <div
              key={lib.id}
              className="group flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 transition-colors hover:border-primary/30"
            >
              <div className="flex min-w-0 items-center gap-3">
                <Folder className="h-5 w-5 shrink-0 text-primary" />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{lib.name}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">
                    {lib.path}
                  </p>
                  <div className="mt-0.5 flex items-center gap-3">
                    <Badge variant="secondary" className="text-xs capitalize">
                      {lib.media_type}
                    </Badge>
                    {lib.disk_space && lib.disk_space.free_bytes > 0 && (
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <HardDrive className="h-3 w-3" />
                        {formatBytes(lib.disk_space.free_bytes)} free
                      </span>
                    )}
                    {lib.unmapped_count > 0 && (
                      <Badge variant="secondary" className="text-xs">
                        {lib.unmapped_count} unmapped
                      </Badge>
                    )}
                  </div>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="text-destructive opacity-0 transition-opacity hover:bg-destructive/10 hover:text-destructive group-hover:opacity-100"
                onClick={() => handleDelete(lib.id)}
                disabled={deletingId === lib.id}
              >
                {deletingId === lib.id ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="h-4 w-4" />
                )}
              </Button>
            </div>
          ))}
        </div>
      )}

      <AddLibraryDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onAdded={() => refetch()}
      />
    </div>
  );
}

// ─── Media Management Panel ─────────────────────────────────────────────

export function MediaManagementPanel() {
  return (
    <div className="space-y-8">
      <LibrariesPanel />
      <NamingSettings />
      <ImportModePanel />
    </div>
  );
}

// ─── Import Mode Panel ──────────────────────────────────────────────────

function ImportModePanel() {
  const [importMode, setImportMode] = React.useState("move");
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    apiFetch("/api/v1/movies/organize/import-mode")
      .then((r) => r.json())
      .then((data) => setImportMode(data.import_mode ?? "move"))
      .catch(() => {});
  }, []);

  const handleChange = async (value: string) => {
    setImportMode(value);
    setSaving(true);
    try {
      await apiFetch("/api/v1/movies/organize/import-mode", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ import_mode: value }),
      });
      toast.success("Import mode updated");
    } catch {
      toast.error("Failed to update import mode");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Import Mode</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Controls how files are transferred from the download directory to your
          library.
        </p>
        <Select
          value={importMode}
          onValueChange={handleChange}
          disabled={saving}
        >
          <SelectTrigger className="w-64">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="move">Move (rename / copy+delete)</SelectItem>
            <SelectItem value="hardlink">
              Hardlink (fall back to move)
            </SelectItem>
            <SelectItem value="hardlink_only">
              Hardlink Only (fail if not possible)
            </SelectItem>
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}

// ─── General Panel ──────────────────────────────────────────────────────

export function GeneralPanel() {
  const [logLevel, setLogLevel] = React.useState("info");
  const [apiKey, setApiKey] = React.useState<string | null>(null);
  const [copied, setCopied] = React.useState(false);

  React.useEffect(() => {
    let cancelled = false;
    apiFetch("/api/v1/auth/api-key")
      .then((res) => (res.ok ? res.json() : null))
      .then((data: { apiKey?: string } | null) => {
        if (!cancelled && data?.apiKey) {
          setApiKey(data.apiKey);
        }
      })
      .catch(() => {
        // endpoint may not exist — leave apiKey null
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const copyApiKey = async () => {
    if (!apiKey) return;
    try {
      await navigator.clipboard.writeText(apiKey);
      setCopied(true);
      toast.success("API key copied to clipboard");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Failed to copy API key");
    }
  };

  return (
    <div className="space-y-6">
      {/* Application Info */}
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 p-6">
          <div>
            <h3 className="mb-1 text-sm font-medium text-zinc-100">
              Application
            </h3>
            <p className="text-xs text-zinc-500">
              General application settings
            </p>
          </div>

          <div className="grid gap-4">
            <div className="flex items-center justify-between">
              <div>
                <Label className="text-sm text-zinc-300">App Name</Label>
                <p className="mt-0.5 text-xs text-zinc-500">
                  Your Loom instance name
                </p>
              </div>
              <Badge
                variant="outline"
                className="border-zinc-700 font-mono text-zinc-300"
              >
                Loom
              </Badge>
            </div>

            <div className="border-t border-zinc-800" />

            <div className="flex items-center justify-between">
              <div>
                <Label className="text-sm text-zinc-300">Authentication</Label>
                <p className="mt-0.5 text-xs text-zinc-500">
                  Login is required for all API access
                </p>
              </div>
              <Badge className="border-teal-700 bg-teal-600/20 text-teal-400">
                <Shield className="mr-1 h-3 w-3" /> Enabled
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Log Level */}
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 p-6">
          <div>
            <h3 className="mb-1 text-sm font-medium text-zinc-100">Logging</h3>
            <p className="text-xs text-zinc-500">
              Control the verbosity of application logs
            </p>
          </div>

          <div className="flex items-center gap-4">
            <Label className="w-24 text-sm text-zinc-300">Log Level</Label>
            <Select value={logLevel} onValueChange={setLogLevel}>
              <SelectTrigger className="w-40 border-zinc-700 bg-zinc-900">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="debug">Debug</SelectItem>
                <SelectItem value="info">Info</SelectItem>
                <SelectItem value="warn">Warn</SelectItem>
                <SelectItem value="error">Error</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* API Key */}
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 p-6">
          <div>
            <h3 className="mb-1 flex items-center gap-2 text-sm font-medium text-zinc-100">
              <Key className="h-4 w-4 text-teal-400" /> API Key
            </h3>
            <p className="text-xs text-zinc-500">
              Use this key to authenticate external API requests
            </p>
          </div>

          <div className="flex items-center gap-2">
            <Input
              readOnly
              value={apiKey ?? ""}
              placeholder="API key will be shown here once configured"
              className="flex-1 border-zinc-700 bg-zinc-900 font-mono text-sm text-zinc-400"
            />
            <Button
              variant="outline"
              size="sm"
              onClick={copyApiKey}
              disabled={!apiKey}
              className="shrink-0 border-zinc-700 text-zinc-400"
            >
              {copied ? (
                <Check className="h-4 w-4 text-teal-400" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Download Clients Panel ─────────────────────────────────────────────

export function DownloadClientsPanel() {
  const { data: clients = [], isLoading } = useDownloads();
  const createClient = useCreateDownload();
  const patchClient = usePatchDownload();
  const deleteClient = useDeleteDownload();
  const [showAdd, setShowAdd] = React.useState(false);
  const [editClient, setEditClient] = React.useState<DownloadClient | null>(
    null,
  );

  if (isLoading) {
    return (
      <div className="flex items-center justify-center gap-2 py-8 text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading download clients…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="mb-1 text-sm font-medium text-zinc-100">
            Download Clients
          </h3>
          <p className="text-xs text-zinc-500">
            Configure torrent and usenet clients for automated downloading.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="border-zinc-700 text-zinc-300"
          onClick={() => setShowAdd(true)}
        >
          <Plus className="mr-1.5 h-3.5 w-3.5" /> Add Client
        </Button>
      </div>

      {clients.length === 0 ? (
        <Card className="border-dashed border-zinc-800 bg-zinc-900/50">
          <CardContent className="py-10 text-center">
            <Download className="mx-auto mb-3 h-10 w-10 text-zinc-700" />
            <p className="text-sm text-zinc-500">
              No download clients configured
            </p>
            <p className="mt-1 text-xs text-zinc-600">
              <button
                onClick={() => setShowAdd(true)}
                className="text-teal-500 hover:text-teal-400"
              >
                Add a download client →
              </button>{" "}
              to enable automated downloading
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {clients.map((client) => (
            <Card key={client.id} className="border-zinc-800 bg-zinc-900/50">
              <CardContent className="p-4">
                <div className="mb-2 flex items-center justify-between">
                  <h4 className="truncate text-sm font-medium text-zinc-200">
                    {client.name}
                  </h4>
                  <div className="flex items-center gap-2">
                    <Badge
                      variant="outline"
                      className={cn(
                        "text-xs",
                        client.enabled
                          ? "border-teal-700 text-teal-400"
                          : "border-zinc-700 text-zinc-500",
                      )}
                    >
                      {client.enabled ? "Enabled" : "Disabled"}
                    </Badge>
                  </div>
                </div>
                <div className="mb-3 flex items-center gap-3 text-xs text-zinc-500">
                  <span className="flex items-center gap-1">
                    <Download className="h-3 w-3" /> {client.kind || "Unknown"}
                  </span>
                  <span>Priority: {client.priority}</span>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 text-xs text-zinc-400 hover:text-zinc-200"
                    onClick={() => setEditClient(client)}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 text-xs text-red-400 hover:text-red-300"
                    onClick={() => {
                      if (confirm(`Delete "${client.name}"?`)) {
                        deleteClient.mutate(client.id);
                      }
                    }}
                  >
                    Delete
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Add Dialog */}
      <Dialog open={showAdd} onOpenChange={setShowAdd}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Add Download Client</DialogTitle>
          </DialogHeader>
          <DownloadForm
            onSubmit={(values) => {
              createClient.mutate(values, {
                onSuccess: () => setShowAdd(false),
              });
            }}
            onCancel={() => setShowAdd(false)}
          />
        </DialogContent>
      </Dialog>

      {/* Edit Dialog */}
      <Dialog
        open={!!editClient}
        onOpenChange={(open) => !open && setEditClient(null)}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit Download Client</DialogTitle>
          </DialogHeader>
          {editClient && (
            <DownloadForm
              initial={editClient}
              submitLabel="Save"
              onSubmit={(values) => {
                patchClient.mutate(
                  { id: editClient.id, patch: values },
                  { onSuccess: () => setEditClient(null) },
                );
              }}
              onCancel={() => setEditClient(null)}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Stall Detection Settings */}
      <div className="border-t border-zinc-800 pt-4">
        <h3 className="mb-1 text-sm font-medium text-zinc-100">
          Stall Detection
        </h3>
        <p className="mb-4 text-xs text-zinc-500">
          Automatically detect and handle stalled or failed downloads.
        </p>

        <div className="space-y-4">
          <Card className="border-zinc-800 bg-zinc-900/50">
            <CardContent className="space-y-4 p-4">
              <div className="flex items-center gap-3">
                <Checkbox id="check-for-stalled" defaultChecked={true} />
                <div>
                  <Label
                    htmlFor="check-for-stalled"
                    className="text-sm font-medium"
                  >
                    Enable stall detection
                  </Label>
                  <p className="text-xs text-zinc-500">
                    Monitor downloads for lack of progress and take action
                  </p>
                </div>
              </div>

              <div className="grid max-w-lg grid-cols-2 gap-4">
                <div>
                  <Label
                    htmlFor="stall-timeout"
                    className="text-xs text-zinc-400"
                  >
                    Stall Timeout (minutes)
                  </Label>
                  <Input
                    id="stall-timeout"
                    type="number"
                    defaultValue={30}
                    min={5}
                    className="mt-1 border-zinc-700 bg-zinc-900"
                  />
                </div>
                <div>
                  <Label
                    htmlFor="max-retries"
                    className="text-xs text-zinc-400"
                  >
                    Max Retries
                  </Label>
                  <Input
                    id="max-retries"
                    type="number"
                    defaultValue={3}
                    min={0}
                    className="mt-1 border-zinc-700 bg-zinc-900"
                  />
                </div>
              </div>

              <div className="max-w-xs">
                <Label htmlFor="stall-action" className="text-xs text-zinc-400">
                  Action on Stall
                </Label>
                <Select defaultValue="remove">
                  <SelectTrigger className="mt-1 border-zinc-700 bg-zinc-900">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="pause">Pause</SelectItem>
                    <SelectItem value="remove">Remove</SelectItem>
                    <SelectItem value="remove_and_blocklist">
                      Remove &amp; Blocklist
                    </SelectItem>
                    <SelectItem value="retry">Retry</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Remote Path Mappings */}
      <RemotePathMappingsSection />
    </div>
  );
}

// ─── Remote Path Mappings Section ───────────────────────────────────────

function RemotePathMappingsSection() {
  const { data: mappings = [], isLoading } = useRemotePathMappings();
  const createMapping = useCreateRemotePathMapping();
  const deleteMapping = useDeleteRemotePathMapping();
  const [clients, setClients] = React.useState<{ id: string; name: string }[]>(
    [],
  );
  const [showForm, setShowForm] = React.useState(false);
  const [formClientId, setFormClientId] = React.useState("");
  const [formRemotePath, setFormRemotePath] = React.useState("");
  const [formLocalPath, setFormLocalPath] = React.useState("");

  React.useEffect(() => {
    apiFetch("/api/v1/download-clients")
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => {
        const list = Array.isArray(data?.download_clients)
          ? data.download_clients
          : Array.isArray(data)
            ? data
            : [];
        setClients(
          list.map((c: { id: string; name: string }) => ({
            id: c.id,
            name: c.name,
          })),
        );
      })
      .catch(() => {});
  }, []);

  const clientNameMap = React.useMemo(() => {
    const map: Record<string, string> = {};
    for (const c of clients) map[c.id] = c.name;
    return map;
  }, [clients]);

  const handleCreate = () => {
    if (!formClientId || !formRemotePath || !formLocalPath) return;
    createMapping.mutate(
      {
        client_id: formClientId,
        remote_path: formRemotePath,
        local_path: formLocalPath,
      },
      {
        onSuccess: () => {
          setShowForm(false);
          setFormClientId("");
          setFormRemotePath("");
          setFormLocalPath("");
          toast.success("Remote path mapping created");
        },
        onError: (err) =>
          toast.error(
            err instanceof Error ? err.message : "Failed to create mapping",
          ),
      },
    );
  };

  return (
    <div className="border-t border-zinc-800 pt-4">
      <div className="mb-4 flex items-start justify-between">
        <div>
          <h3 className="mb-1 text-sm font-medium text-zinc-100">
            Remote Path Mappings
          </h3>
          <p className="text-xs text-zinc-500">
            Map download client paths to local paths for Docker or remote
            setups.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="border-zinc-700 text-zinc-300"
          onClick={() => setShowForm(true)}
        >
          <Plus className="mr-1.5 h-3.5 w-3.5" /> Add Mapping
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center gap-2 py-4 text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : mappings.length === 0 && !showForm ? (
        <Card className="border-dashed border-zinc-800 bg-zinc-900/50">
          <CardContent className="py-6 text-center">
            <p className="text-sm text-zinc-500">
              No remote path mappings configured
            </p>
            <p className="mt-1 text-xs text-zinc-600">
              Add mappings if your download client reports paths different from
              what Loom sees locally.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card className="border-zinc-800 bg-zinc-900/50">
          <CardContent className="p-0">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-800 text-xs text-zinc-400">
                  <th className="p-3 text-left font-medium">Client</th>
                  <th className="p-3 text-left font-medium">Remote Path</th>
                  <th className="p-3 text-left font-medium">Local Path</th>
                  <th className="w-10 p-3"></th>
                </tr>
              </thead>
              <tbody>
                {mappings.map((m: RemotePathMapping) => (
                  <tr
                    key={m.id}
                    className="border-b border-zinc-800/50 last:border-0"
                  >
                    <td className="p-3 text-zinc-200">
                      {clientNameMap[m.client_id] || m.client_id}
                    </td>
                    <td className="p-3 font-mono text-xs text-zinc-400">
                      {m.remote_path}
                    </td>
                    <td className="p-3 font-mono text-xs text-zinc-400">
                      {m.local_path}
                    </td>
                    <td className="p-3">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 text-zinc-500 hover:text-red-400"
                        onClick={() =>
                          deleteMapping.mutate(m.id, {
                            onSuccess: () => toast.success("Mapping deleted"),
                            onError: () =>
                              toast.error("Failed to delete mapping"),
                          })
                        }
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </CardContent>
        </Card>
      )}

      {/* Add Mapping Form Dialog */}
      <Dialog open={showForm} onOpenChange={setShowForm}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Add Remote Path Mapping</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div>
              <Label htmlFor="rpm-client" className="text-xs text-zinc-400">
                Download Client
              </Label>
              <Select value={formClientId} onValueChange={setFormClientId}>
                <SelectTrigger
                  id="rpm-client"
                  className="mt-1 border-zinc-700 bg-zinc-900"
                >
                  <SelectValue placeholder="Select a client" />
                </SelectTrigger>
                <SelectContent>
                  {clients.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="rpm-remote" className="text-xs text-zinc-400">
                Remote Path
              </Label>
              <Input
                id="rpm-remote"
                value={formRemotePath}
                onChange={(e) => setFormRemotePath(e.target.value)}
                placeholder="/downloads/movies/"
                className="mt-1 border-zinc-700 bg-zinc-900 font-mono text-sm"
              />
              <p className="mt-1 text-xs text-zinc-600">
                The path as reported by the download client
              </p>
            </div>
            <div>
              <Label htmlFor="rpm-local" className="text-xs text-zinc-400">
                Local Path
              </Label>
              <Input
                id="rpm-local"
                value={formLocalPath}
                onChange={(e) => setFormLocalPath(e.target.value)}
                placeholder="/media/downloads/movies/"
                className="mt-1 border-zinc-700 bg-zinc-900 font-mono text-sm"
              />
              <p className="mt-1 text-xs text-zinc-600">
                The path as seen by Loom on its filesystem
              </p>
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button variant="ghost" onClick={() => setShowForm(false)}>
                Cancel
              </Button>
              <Button
                onClick={handleCreate}
                disabled={
                  !formClientId ||
                  !formRemotePath ||
                  !formLocalPath ||
                  createMapping.isPending
                }
              >
                {createMapping.isPending ? "Saving…" : "Save Mapping"}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

// ─── Connect Panel ──────────────────────────────────────────────────────

export function ConnectPanel() {
  const { data: connections = [], isLoading } = useConnectConnections();
  const createMut = useCreateConnect();
  const updateMut = useUpdateConnect();
  const deleteMut = useDeleteConnect();
  const testMut = useTestConnect();
  const testConfigMut = useTestConnectConfig();
  const traktAuthorizeMut = useTraktAuthorize();
  const traktCallbackMut = useTraktCallback();
  const traktRefreshMut = useTraktRefreshToken();
  const traktSyncWatchedMut = useTraktSyncWatched();
  const traktSyncCollectionMut = useTraktSyncCollection();
  const traktSyncWatchlistMut = useTraktSyncWatchlist();
  const navigate = useNavigate();
  const { trakt_code } = useSearch({ strict: false });

  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<ConnectConnection | null>(null);

  // Form state
  const [formProvider, setFormProvider] =
    React.useState<ConnectProviderType>("plex");
  const [formName, setFormName] = React.useState("");
  const [formHost, setFormHost] = React.useState("");
  const [formApiKey, setFormApiKey] = React.useState("");
  const [formNotifyOnImport, setFormNotifyOnImport] = React.useState(true);
  const [formEnabled, setFormEnabled] = React.useState(true);
  const [testResult, setTestResult] = React.useState<{
    ok: boolean;
    message: string;
  } | null>(null);

  // Trakt-specific state
  const [formClientId, setFormClientId] = React.useState("");
  const [formClientSecret, setFormClientSecret] = React.useState("");
  const [traktOAuthCode, setTraktOAuthCode] = React.useState("");
  const [traktAuthStep, setTraktAuthStep] = React.useState<
    "config" | "authorize" | "code" | "connected"
  >("config");

  // Auto-populate the OAuth code when returning from Trakt redirect
  React.useEffect(() => {
    if (trakt_code) {
      setTraktOAuthCode(trakt_code);
      setTraktAuthStep("code");
      // Find the existing Trakt connection and open its edit dialog
      const traktConn = connections.find((c) => c.provider === "trakt");
      if (traktConn) {
        setEditing(traktConn);
        setFormProvider(traktConn.provider);
        setFormName(traktConn.name);
        setFormClientId(traktConn.settings.client_id ?? "");
        setFormClientSecret(traktConn.settings.client_secret ?? "");
        setFormNotifyOnImport(traktConn.notify_on_import);
        setFormEnabled(traktConn.enabled);
        setDialogOpen(true);
      }
      // Clear the search param so it doesn't re-trigger
      navigate({ to: "/settings/connect", search: {}, replace: true });
    }
  }, [trakt_code, connections, navigate]);

  const isTrakt = formProvider === "trakt";

  const openCreate = () => {
    setEditing(null);
    setFormProvider("plex");
    setFormName("");
    setFormHost("");
    setFormApiKey("");
    setFormClientId("");
    setFormClientSecret("");
    setFormNotifyOnImport(true);
    setFormEnabled(true);
    setTestResult(null);
    setTraktOAuthCode("");
    setTraktAuthStep("config");
    setDialogOpen(true);
  };

  const openEdit = (c: ConnectConnection) => {
    setEditing(c);
    setFormProvider(c.provider);
    setFormName(c.name);
    setFormHost(c.settings.host ?? "");
    setFormApiKey(c.settings.api_key ?? "");
    setFormClientId(c.settings.client_id ?? "");
    setFormClientSecret(c.settings.client_secret ?? "");
    setFormNotifyOnImport(c.notify_on_import);
    setFormEnabled(c.enabled);
    setTestResult(null);
    setTraktOAuthCode("");
    setTraktAuthStep(
      c.provider === "trakt" && c.settings.access_token
        ? "connected"
        : "config",
    );
    setDialogOpen(true);
  };

  const buildBody = (): CreateConnectRequest => ({
    name: formName,
    provider: formProvider,
    enabled: formEnabled,
    settings: isTrakt
      ? { client_id: formClientId, client_secret: formClientSecret }
      : { host: formHost, api_key: formApiKey },
    notify_on_import: formNotifyOnImport,
  });

  const handleSave = () => {
    if (editing) {
      updateMut.mutate(
        {
          id: editing.id,
          body: {
            name: formName,
            provider: formProvider,
            enabled: formEnabled,
            settings: isTrakt
              ? { client_id: formClientId, client_secret: formClientSecret }
              : { host: formHost, api_key: formApiKey },
            notify_on_import: formNotifyOnImport,
          },
        },
        {
          onSuccess: () => {
            setDialogOpen(false);
            toast.success("Connection updated");
          },
          onError: (err) => toast.error(err.message),
        },
      );
    } else {
      createMut.mutate(buildBody(), {
        onSuccess: () => {
          setDialogOpen(false);
          toast.success("Connection created");
        },
        onError: (err) => toast.error(err.message),
      });
    }
  };

  const handleSaveAndAuthorize = () => {
    const body = buildBody();
    const onCreated = (conn: ConnectConnection) => {
      setEditing(conn);
      setTraktAuthStep("authorize");
      traktAuthorizeMut.mutate(
        {
          client_id: formClientId,
          redirect_uri: `${window.location.origin}/settings/trakt/callback`,
        },
        {
          onSuccess: (res) => {
            window.open(res.authorize_url, "_blank", "noopener");
            setTraktAuthStep("code");
          },
          onError: (err) => toast.error(err.message),
        },
      );
    };

    if (editing) {
      updateMut.mutate(
        { id: editing.id, body },
        {
          onSuccess: (conn) => onCreated(conn),
          onError: (err) => toast.error(err.message),
        },
      );
    } else {
      createMut.mutate(body, {
        onSuccess: (conn) => onCreated(conn),
        onError: (err) => toast.error(err.message),
      });
    }
  };

  const handleTraktCodeSubmit = () => {
    if (!editing) return;
    traktCallbackMut.mutate(
      {
        code: traktOAuthCode,
        client_id: formClientId,
        client_secret: formClientSecret,
        redirect_uri: `${window.location.origin}/settings/trakt/callback`,
        connection_id: editing.id,
      },
      {
        onSuccess: () => {
          setTraktAuthStep("connected");
          toast.success("Trakt authorized successfully");
        },
        onError: (err) => toast.error(err.message),
      },
    );
  };

  const handleTest = () => {
    setTestResult(null);
    if (editing) {
      testMut.mutate(editing.id, {
        onSuccess: (res) => setTestResult({ ok: true, message: res.message }),
        onError: (err) => setTestResult({ ok: false, message: err.message }),
      });
    } else {
      testConfigMut.mutate(buildBody(), {
        onSuccess: (res) => setTestResult({ ok: true, message: res.message }),
        onError: (err) => setTestResult({ ok: false, message: err.message }),
      });
    }
  };

  const handleDelete = (id: string) => {
    deleteMut.mutate(id, {
      onSuccess: () => toast.success("Connection deleted"),
      onError: (err) => toast.error(err.message),
    });
  };

  const handleTraktSync = (
    type: "watched" | "collection" | "watchlist",
    connectionId: string,
  ) => {
    const mut =
      type === "watched"
        ? traktSyncWatchedMut
        : type === "collection"
          ? traktSyncCollectionMut
          : traktSyncWatchlistMut;
    mut.mutate(connectionId, {
      onSuccess: (res) =>
        toast.success(
          `Synced ${type}: ${res.movies} movies, ${res.shows} shows`,
        ),
      onError: (err) => toast.error(err.message),
    });
  };

  const providerLabel = (p: ConnectProviderType) =>
    PROVIDER_TYPES.find((t) => t.value === p)?.label ?? p;

  const isSaving = createMut.isPending || updateMut.isPending;
  const isTesting = testMut.isPending || testConfigMut.isPending;
  const isTraktAuthorizing =
    traktAuthorizeMut.isPending || traktCallbackMut.isPending || isSaving;
  const isTraktSyncing =
    traktSyncWatchedMut.isPending ||
    traktSyncCollectionMut.isPending ||
    traktSyncWatchlistMut.isPending;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="mb-1 text-sm font-medium text-zinc-100">
            Connections
          </h3>
          <p className="text-xs text-zinc-500">
            Connect Loom to media servers for library refresh on import.
          </p>
        </div>
        <Button size="sm" onClick={openCreate}>
          <Plus className="mr-1 h-4 w-4" /> Add Connection
        </Button>
      </div>

      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-zinc-500" />
            </div>
          ) : connections.length === 0 ? (
            <div className="py-8 text-center text-sm text-zinc-500">
              No connections configured. Click "Add Connection" to get started.
            </div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-800 text-xs text-zinc-400">
                  <th className="px-4 py-2 text-left font-medium">Name</th>
                  <th className="px-4 py-2 text-left font-medium">Provider</th>
                  <th className="px-4 py-2 text-left font-medium">Enabled</th>
                  <th className="px-4 py-2 text-left font-medium">
                    Notify on Import
                  </th>
                  <th className="px-4 py-2 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {connections.map((conn) => (
                  <tr
                    key={conn.id}
                    className="border-b border-zinc-800/50 last:border-0"
                  >
                    <td className="px-4 py-3 text-zinc-200">{conn.name}</td>
                    <td className="px-4 py-3">
                      <Badge
                        variant="outline"
                        className="border-zinc-700 text-xs text-zinc-300"
                      >
                        {providerLabel(conn.provider)}
                      </Badge>
                      {conn.provider === "trakt" &&
                        conn.settings.access_token && (
                          <Badge
                            variant="outline"
                            className="ml-1 border-emerald-700 text-xs text-emerald-400"
                          >
                            Connected
                          </Badge>
                        )}
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant="outline"
                        className={cn(
                          "text-xs",
                          conn.enabled
                            ? "border-emerald-700 text-emerald-400"
                            : "border-zinc-700 text-zinc-500",
                        )}
                      >
                        {conn.enabled ? "Yes" : "No"}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-zinc-400">
                      {conn.notify_on_import ? "Yes" : "No"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0"
                          >
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(conn)}>
                            <Pencil className="mr-2 h-4 w-4" /> Edit
                          </DropdownMenuItem>
                          {conn.provider === "trakt" &&
                            conn.settings.access_token && (
                              <>
                                <DropdownMenuItem
                                  disabled={isTraktSyncing}
                                  onClick={() =>
                                    handleTraktSync("watched", conn.id)
                                  }
                                >
                                  <Download className="mr-2 h-4 w-4" /> Sync
                                  Watched
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  disabled={isTraktSyncing}
                                  onClick={() =>
                                    handleTraktSync("collection", conn.id)
                                  }
                                >
                                  <Download className="mr-2 h-4 w-4" /> Sync
                                  Collection
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  disabled={isTraktSyncing}
                                  onClick={() =>
                                    handleTraktSync("watchlist", conn.id)
                                  }
                                >
                                  <Download className="mr-2 h-4 w-4" /> Sync
                                  Watchlist
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  disabled={traktRefreshMut.isPending}
                                  onClick={() =>
                                    traktRefreshMut.mutate(conn.id, {
                                      onSuccess: () =>
                                        toast.success("Token refreshed"),
                                      onError: (err) =>
                                        toast.error(err.message),
                                    })
                                  }
                                >
                                  <Key className="mr-2 h-4 w-4" /> Refresh Token
                                </DropdownMenuItem>
                              </>
                            )}
                          <DropdownMenuItem
                            className="text-red-400"
                            onClick={() => handleDelete(conn.id)}
                          >
                            <Trash2 className="mr-2 h-4 w-4" /> Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </CardContent>
      </Card>

      {/* Add / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editing ? "Edit Connection" : "Add Connection"}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-2">
            <div className="space-y-1.5">
              <Label>Provider</Label>
              <Select
                value={formProvider}
                onValueChange={(v) => setFormProvider(v as ConnectProviderType)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PROVIDER_TYPES.map((p) => (
                    <SelectItem key={p.value} value={p.value}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-zinc-500">
                {
                  PROVIDER_TYPES.find((p) => p.value === formProvider)
                    ?.description
                }
              </p>
            </div>

            <div className="space-y-1.5">
              <Label>Name</Label>
              <Input
                value={formName}
                onChange={(e) => setFormName(e.target.value)}
                placeholder={isTrakt ? "My Trakt Account" : "My Plex Server"}
              />
            </div>

            {isTrakt ? (
              <>
                <div className="space-y-1.5">
                  <Label>Client ID</Label>
                  <Input
                    value={formClientId}
                    onChange={(e) => setFormClientId(e.target.value)}
                    placeholder="Your Trakt API application Client ID"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label>Client Secret</Label>
                  <Input
                    type="password"
                    value={formClientSecret}
                    onChange={(e) => setFormClientSecret(e.target.value)}
                    placeholder="Your Trakt API application Client Secret"
                  />
                </div>

                {traktAuthStep === "code" && (
                  <div className="space-y-2 rounded-md border border-zinc-700 bg-zinc-900 p-3">
                    <p className="text-xs text-zinc-400">
                      A new tab was opened to Trakt. Authorize the app, then
                      paste the code below.
                    </p>
                    <Input
                      value={traktOAuthCode}
                      onChange={(e) => setTraktOAuthCode(e.target.value)}
                      placeholder="Paste authorization code here"
                    />
                    <Button
                      size="sm"
                      onClick={handleTraktCodeSubmit}
                      disabled={!traktOAuthCode || traktCallbackMut.isPending}
                    >
                      {traktCallbackMut.isPending && (
                        <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                      )}
                      Complete Authorization
                    </Button>
                  </div>
                )}

                {traktAuthStep === "connected" && (
                  <div className="rounded-md border border-emerald-800 bg-emerald-950/50 px-3 py-2 text-sm text-emerald-400">
                    Trakt is authorized and connected.
                  </div>
                )}
              </>
            ) : (
              <>
                <div className="space-y-1.5">
                  <Label>Host</Label>
                  <Input
                    value={formHost}
                    onChange={(e) => setFormHost(e.target.value)}
                    placeholder="http://192.168.1.10:32400"
                  />
                </div>

                <div className="space-y-1.5">
                  <Label>API Key / Token</Label>
                  <Input
                    type="password"
                    value={formApiKey}
                    onChange={(e) => setFormApiKey(e.target.value)}
                    placeholder="Plex token or Emby/Jellyfin API key"
                  />
                </div>
              </>
            )}

            <div className="flex items-center justify-between">
              <Label>Notify on Import</Label>
              <Switch
                checked={formNotifyOnImport}
                onCheckedChange={setFormNotifyOnImport}
              />
            </div>

            <div className="flex items-center justify-between">
              <Label>Enabled</Label>
              <Switch checked={formEnabled} onCheckedChange={setFormEnabled} />
            </div>

            {testResult && (
              <div
                className={cn(
                  "rounded-md px-3 py-2 text-sm",
                  testResult.ok
                    ? "border border-emerald-800 bg-emerald-950/50 text-emerald-400"
                    : "border border-red-800 bg-red-950/50 text-red-400",
                )}
              >
                {testResult.message}
              </div>
            )}

            <div className="flex justify-between pt-2">
              {isTrakt ? (
                <>
                  <div />
                  <div className="flex gap-2">
                    <Button
                      variant="ghost"
                      onClick={() => setDialogOpen(false)}
                    >
                      Cancel
                    </Button>
                    {traktAuthStep === "connected" ? (
                      <Button
                        onClick={handleSave}
                        disabled={isSaving || !formName}
                      >
                        {isSaving && (
                          <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                        )}
                        {editing ? "Update" : "Save"}
                      </Button>
                    ) : traktAuthStep === "config" ? (
                      <Button
                        onClick={handleSaveAndAuthorize}
                        disabled={
                          isTraktAuthorizing ||
                          !formName ||
                          !formClientId ||
                          !formClientSecret
                        }
                      >
                        {isTraktAuthorizing && (
                          <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                        )}
                        <ExternalLink className="mr-1 h-4 w-4" />
                        Connect to Trakt
                      </Button>
                    ) : null}
                  </div>
                </>
              ) : (
                <>
                  <Button
                    variant="outline"
                    onClick={handleTest}
                    disabled={isTesting || !formHost}
                  >
                    {isTesting ? (
                      <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                    ) : (
                      <Plug className="mr-1 h-4 w-4" />
                    )}
                    Test
                  </Button>
                  <div className="flex gap-2">
                    <Button
                      variant="ghost"
                      onClick={() => setDialogOpen(false)}
                    >
                      Cancel
                    </Button>
                    <Button
                      onClick={handleSave}
                      disabled={isSaving || !formName || !formHost}
                    >
                      {isSaving && (
                        <Loader2 className="mr-1 h-4 w-4 animate-spin" />
                      )}
                      {editing ? "Update" : "Save"}
                    </Button>
                  </div>
                </>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

// ─── Download Safety Panel ──────────────────────────────────────────────

export function DownloadSafetyPanel() {
  const [blockDangerous, setBlockDangerous] = React.useState(true);
  const [patterns, setPatterns] = React.useState<string[]>([
    "password",
    "passworded",
    "virus",
    "crack",
    "keygen",
    "patch",
  ]);
  const [newPattern, setNewPattern] = React.useState("");
  const [minSize, setMinSize] = React.useState("50");
  const [maxSize, setMaxSize] = React.useState("100000");

  const addPattern = () => {
    const p = newPattern.trim().toLowerCase();
    if (p && !patterns.includes(p)) {
      setPatterns([...patterns, p]);
      setNewPattern("");
      toast.success(`Added pattern "${p}"`);
    }
  };

  const removePattern = (idx: number) => {
    setPatterns(patterns.filter((_, i) => i !== idx));
    toast.success("Pattern removed");
  };

  return (
    <div className="space-y-6">
      <div>
        <h3 className="mb-1 text-sm font-medium text-zinc-100">
          Download Safety
        </h3>
        <p className="text-xs text-zinc-500">
          Protect against malicious or mislabeled releases before and after
          download.
        </p>
      </div>

      {/* Block dangerous extensions */}
      <Card>
        <CardContent className="pt-4">
          <div className="flex items-center gap-3">
            <Checkbox
              id="block-dangerous"
              checked={blockDangerous}
              onCheckedChange={(v) => setBlockDangerous(v === true)}
            />
            <div>
              <Label htmlFor="block-dangerous" className="text-sm font-medium">
                Block dangerous file extensions
              </Label>
              <p className="text-xs text-zinc-500">
                Reject releases containing .exe, .bat, .cmd, .msi, .scr, .pif,
                .com, .vbs, .js, .wsh, .wsf, .ps1
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Suspicious patterns */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Suspicious Patterns</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-xs text-zinc-500">
            Release names matching any of these patterns will be flagged for
            manual review.
          </p>
          <div className="flex flex-wrap gap-2">
            {patterns.map((p, i) => (
              <Badge
                key={p}
                variant="secondary"
                className="flex items-center gap-1 pl-2 pr-1"
              >
                {p}
                <button
                  type="button"
                  onClick={() => removePattern(i)}
                  className="ml-1 rounded-full p-0.5 hover:bg-zinc-700"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </Badge>
            ))}
          </div>
          <div className="flex gap-2">
            <Input
              value={newPattern}
              onChange={(e) => setNewPattern(e.target.value)}
              placeholder="Add pattern…"
              className="max-w-xs"
              onKeyDown={(e) => e.key === "Enter" && addPattern()}
            />
            <Button size="sm" onClick={addPattern}>
              <Plus className="mr-1 h-4 w-4" /> Add
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Size range */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Size Anomaly Detection</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-xs text-zinc-500">
            Flag releases outside this size range (in MB) as suspicious.
          </p>
          <div className="grid max-w-sm grid-cols-2 gap-4">
            <div>
              <Label htmlFor="min-size" className="text-xs">
                Minimum (MB)
              </Label>
              <Input
                id="min-size"
                type="number"
                value={minSize}
                onChange={(e) => setMinSize(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="max-size" className="text-xs">
                Maximum (MB)
              </Label>
              <Input
                id="max-size"
                type="number"
                value={maxSize}
                onChange={(e) => setMaxSize(e.target.value)}
              />
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button onClick={() => toast.success("Download safety settings saved")}>
          Save
        </Button>
      </div>
    </div>
  );
}

// ─── UI Panel ───────────────────────────────────────────────────────────

export function UIPanel() {
  const [theme, setTheme] = React.useState("dark");
  const [pageSize, setPageSize] = React.useState(() => {
    return localStorage.getItem("loom-page-size") || "25";
  });
  const [defaultView, setDefaultView] = React.useState(() => {
    return localStorage.getItem("loom-default-view") || "grid";
  });

  const handlePageSizeChange = (value: string) => {
    setPageSize(value);
    localStorage.setItem("loom-page-size", value);
    toast.success(`Page size set to ${value}`);
  };

  const handleDefaultViewChange = (value: string) => {
    setDefaultView(value);
    localStorage.setItem("loom-default-view", value);
    toast.success(`Default view set to ${value}`);
  };

  return (
    <div className="space-y-6">
      <div>
        <h3 className="mb-1 text-sm font-medium text-zinc-100">
          User Interface
        </h3>
        <p className="text-xs text-zinc-500">
          Customize how Loom looks and feels. These settings are stored locally
          in your browser.
        </p>
      </div>

      {/* Theme */}
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 p-6">
          <div className="flex items-center gap-3">
            <Palette className="h-4 w-4 text-teal-400" />
            <div>
              <Label className="text-sm text-zinc-200">Theme</Label>
              <p className="mt-0.5 text-xs text-zinc-500">
                Choose the appearance of the interface
              </p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Select value={theme} onValueChange={setTheme}>
              <SelectTrigger className="w-40 border-zinc-700 bg-zinc-900">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="dark">Dark</SelectItem>
                <SelectItem value="light">Light</SelectItem>
                <SelectItem value="amoled">AMOLED</SelectItem>
                <SelectItem value="system">System</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {/* Page Size */}
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 p-6">
          <div>
            <Label className="text-sm text-zinc-200">Items Per Page</Label>
            <p className="mt-0.5 text-xs text-zinc-500">
              Number of items to show in paginated lists
            </p>
          </div>
          <Select value={pageSize} onValueChange={handlePageSizeChange}>
            <SelectTrigger className="w-40 border-zinc-700 bg-zinc-900">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="10">10</SelectItem>
              <SelectItem value="25">25</SelectItem>
              <SelectItem value="50">50</SelectItem>
              <SelectItem value="100">100</SelectItem>
            </SelectContent>
          </Select>
        </CardContent>
      </Card>

      {/* Default View */}
      <Card className="border-zinc-800 bg-zinc-900/50">
        <CardContent className="space-y-4 p-6">
          <div>
            <Label className="text-sm text-zinc-200">Default View Mode</Label>
            <p className="mt-0.5 text-xs text-zinc-500">
              Default layout for the movies page
            </p>
          </div>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDefaultViewChange("grid")}
              className={cn(
                "border-zinc-700",
                defaultView === "grid"
                  ? "border-teal-700 bg-teal-600/20 text-teal-300"
                  : "text-zinc-400",
              )}
            >
              <LayoutGrid className="mr-1.5 h-4 w-4" /> Grid
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDefaultViewChange("list")}
              className={cn(
                "border-zinc-700",
                defaultView === "list"
                  ? "border-teal-700 bg-teal-600/20 text-teal-300"
                  : "text-zinc-400",
              )}
            >
              <List className="mr-1.5 h-4 w-4" /> List
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Rolling Search Panel ────────────────────────────────────────────────

interface RollingSearchConfig {
  enabled: boolean;
  intervalHours: number;
  batchSize: number;
  minResearchDays: number;
  maxSearchesPerDay: number;
}

interface RollingSearchStatus {
  running: boolean;
  lastRunAt: string | null;
  nextRunAt: string | null;
  itemsSearched: number;
  itemsInQueue: number;
  quotaUsage: Record<string, number>;
}

export function RollingSearchPanel() {
  const [config, setConfig] = React.useState<RollingSearchConfig | null>(null);
  const [status, setStatus] = React.useState<RollingSearchStatus | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const triggerTimeoutRef =
    React.useRef<ReturnType<typeof setTimeout>>(undefined);

  React.useEffect(() => {
    return () => clearTimeout(triggerTimeoutRef.current);
  }, []);

  const fetchData = React.useCallback(async () => {
    try {
      const [cfgRes, statusRes] = await Promise.all([
        apiFetch("/api/v1/rolling-search/config"),
        apiFetch("/api/v1/rolling-search/status"),
      ]);
      if (cfgRes.ok) setConfig(await cfgRes.json());
      if (statusRes.ok) setStatus(await statusRes.json());
    } catch {
      toast.error("Failed to load rolling search settings");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    fetchData();
  }, [fetchData]);

  const handleSave = async () => {
    if (!config) return;
    setSaving(true);
    try {
      const res = await apiFetch("/api/v1/rolling-search/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });
      if (!res.ok) throw new Error("save failed");
      const updated = await res.json();
      setConfig(updated);
      toast.success("Rolling search settings saved");
      fetchData();
    } catch {
      toast.error("Failed to save rolling search settings");
    } finally {
      setSaving(false);
    }
  };

  const handleTrigger = async () => {
    try {
      const res = await apiFetch("/api/v1/rolling-search/trigger", {
        method: "POST",
      });
      if (!res.ok) throw new Error("trigger failed");
      toast.success("Rolling search triggered");
      triggerTimeoutRef.current = setTimeout(fetchData, 2000);
    } catch {
      toast.error("Failed to trigger rolling search");
    }
  };

  if (loading || !config) {
    return (
      <div className="flex items-center gap-2 py-8 text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Status */}
      {status && (
        <Card className="border-zinc-800 bg-zinc-900/50">
          <CardContent className="space-y-2 pt-4 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">Status</span>
              <Badge variant={status.running ? "default" : "secondary"}>
                {status.running ? "Running" : "Stopped"}
              </Badge>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">Items in queue</span>
              <span>{status.itemsInQueue}</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">
                Items searched (session)
              </span>
              <span>{status.itemsSearched}</span>
            </div>
            {status.lastRunAt && (
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Last run</span>
                <span>{new Date(status.lastRunAt).toLocaleString()}</span>
              </div>
            )}
            {status.nextRunAt && (
              <div className="flex items-center justify-between">
                <span className="text-muted-foreground">Next run</span>
                <span>{new Date(status.nextRunAt).toLocaleString()}</span>
              </div>
            )}
            {Object.keys(status.quotaUsage).length > 0 && (
              <div className="pt-2">
                <span className="text-xs text-muted-foreground">
                  Quota usage (24 h)
                </span>
                <div className="mt-1 flex flex-wrap gap-2">
                  {Object.entries(status.quotaUsage).map(([id, count]) => (
                    <Badge key={id} variant="outline" className="text-xs">
                      {id}: {count}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Enable toggle */}
      <div className="flex items-center justify-between">
        <div>
          <Label className="text-sm font-medium">Enable Rolling Search</Label>
          <p className="text-xs text-muted-foreground">
            Continuously search for missing movies and episodes on a schedule
          </p>
        </div>
        <Checkbox
          checked={config.enabled}
          onCheckedChange={(v) =>
            setConfig((c) => (c ? { ...c, enabled: !!v } : c))
          }
        />
      </div>

      {/* Interval */}
      <div className="grid gap-1.5">
        <Label htmlFor="rs-interval">Interval (hours)</Label>
        <Input
          id="rs-interval"
          type="number"
          min={1}
          max={168}
          value={config.intervalHours}
          onChange={(e) =>
            setConfig((c) =>
              c ? { ...c, intervalHours: Number(e.target.value) || 12 } : c,
            )
          }
        />
        <p className="text-xs text-muted-foreground">
          How often the scheduler runs a search batch
        </p>
      </div>

      {/* Batch size */}
      <div className="grid gap-1.5">
        <Label htmlFor="rs-batch">Batch Size</Label>
        <Input
          id="rs-batch"
          type="number"
          min={1}
          max={100}
          value={config.batchSize}
          onChange={(e) =>
            setConfig((c) =>
              c ? { ...c, batchSize: Number(e.target.value) || 5 } : c,
            )
          }
        />
        <p className="text-xs text-muted-foreground">
          Number of items to search per run
        </p>
      </div>

      {/* Min re-search days */}
      <div className="grid gap-1.5">
        <Label htmlFor="rs-redays">Minimum re-search interval (days)</Label>
        <Input
          id="rs-redays"
          type="number"
          min={1}
          max={90}
          value={config.minResearchDays}
          onChange={(e) =>
            setConfig((c) =>
              c ? { ...c, minResearchDays: Number(e.target.value) || 7 } : c,
            )
          }
        />
        <p className="text-xs text-muted-foreground">
          Don't re-search an item within this many days
        </p>
      </div>

      {/* Max searches per day */}
      <div className="grid gap-1.5">
        <Label htmlFor="rs-maxday">Max searches per indexer per day</Label>
        <Input
          id="rs-maxday"
          type="number"
          min={1}
          max={10000}
          value={config.maxSearchesPerDay}
          onChange={(e) =>
            setConfig((c) =>
              c
                ? { ...c, maxSearchesPerDay: Number(e.target.value) || 100 }
                : c,
            )
          }
        />
        <p className="text-xs text-muted-foreground">
          Per-indexer daily quota to avoid rate-limiting
        </p>
      </div>

      {/* Actions */}
      <div className="flex gap-2 pt-2">
        <Button onClick={handleSave} disabled={saving} size="sm">
          {saving ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
          Save
        </Button>
        <Button variant="outline" size="sm" onClick={handleTrigger}>
          <Search className="mr-1.5 h-4 w-4" /> Run Now
        </Button>
      </div>
    </div>
  );
}

// ─── Settings Panels ────────────────────────────────────────────────────

// ─── Media Preferences Panel ────────────────────────────────────────────

const AUDIO_CODECS = [
  "TrueHD Atmos",
  "DTS-HD MA",
  "DTS-X",
  "TrueHD",
  "DTS-HD",
  "FLAC",
  "EAC3",
  "DTS",
  "AC3",
  "AAC",
  "OPUS",
  "MP3",
];

const SUB_LANGUAGES = [
  { code: "en", label: "English" },
  { code: "fr", label: "French" },
  { code: "de", label: "German" },
  { code: "es", label: "Spanish" },
  { code: "it", label: "Italian" },
  { code: "pt", label: "Portuguese" },
  { code: "ja", label: "Japanese" },
  { code: "ko", label: "Korean" },
  { code: "zh", label: "Chinese" },
  { code: "ru", label: "Russian" },
  { code: "ar", label: "Arabic" },
  { code: "hi", label: "Hindi" },
  { code: "nl", label: "Dutch" },
  { code: "sv", label: "Swedish" },
  { code: "no", label: "Norwegian" },
  { code: "da", label: "Danish" },
  { code: "fi", label: "Finnish" },
  { code: "pl", label: "Polish" },
  { code: "tr", label: "Turkish" },
];

export function MediaPreferencesPanel() {
  // Radix Select forbids an empty-string item value, so use a sentinel for the
  // "None" option and map it to/from the stored empty string.
  const NO_PROFILE = "__none__";
  const { data: prefs, isLoading } = useMediaPreferences();
  const updateMut = useUpdateMediaPreferences();
  const parseMut = useParseReleaseName();
  const { data: qualityProfiles } = useQualityProfiles();
  const [testName, setTestName] = React.useState("");

  const [defaultProfileId, setDefaultProfileId] = React.useState("");
  const [audioOrder, setAudioOrder] = React.useState<string[]>([]);
  const [subLangs, setSubLangs] = React.useState<string[]>([]);
  const [requireSubs, setRequireSubs] = React.useState(false);
  const [preferHDR, setPreferHDR] = React.useState(true);
  const [preferAtmos, setPreferAtmos] = React.useState(true);
  const [dirty, setDirty] = React.useState(false);

  React.useEffect(() => {
    if (prefs) {
      setDefaultProfileId(prefs.default_quality_profile_id ?? "");
      setAudioOrder(prefs.preferred_audio ?? []);
      setSubLangs(prefs.preferred_sub_languages ?? []);
      setRequireSubs(prefs.require_subtitles);
      setPreferHDR(prefs.prefer_hdr);
      setPreferAtmos(prefs.prefer_atmos);
      setDirty(false);
    }
  }, [prefs]);

  const handleSave = () => {
    updateMut.mutate(
      {
        preferred_audio: audioOrder,
        preferred_sub_languages: subLangs,
        require_subtitles: requireSubs,
        prefer_hdr: preferHDR,
        prefer_atmos: preferAtmos,
        default_quality_profile_id: defaultProfileId,
      },
      {
        onSuccess: () => {
          toast.success("Media preferences saved");
          setDirty(false);
        },
        onError: () => toast.error("Failed to save media preferences"),
      },
    );
  };

  const moveAudio = (idx: number, dir: -1 | 1) => {
    const next = [...audioOrder];
    const target = idx + dir;
    if (target < 0 || target >= next.length) return;
    [next[idx], next[target]] = [next[target]!, next[idx]!];
    setAudioOrder(next);
    setDirty(true);
  };

  const toggleAudio = (codec: string) => {
    setAudioOrder((prev) => {
      if (prev.includes(codec)) return prev.filter((c) => c !== codec);
      return [...prev, codec];
    });
    setDirty(true);
  };

  const toggleSubLang = (code: string) => {
    setSubLangs((prev) => {
      if (prev.includes(code)) return prev.filter((c) => c !== code);
      return [...prev, code];
    });
    setDirty(true);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Default Quality Profile */}
      <div className="space-y-2">
        <Label className="text-base font-medium">Default Quality Profile</Label>
        <p className="text-xs text-muted-foreground">
          Used as the default when adding new movies or TV shows.
        </p>
        <Select
          value={defaultProfileId || NO_PROFILE}
          onValueChange={(v) => {
            setDefaultProfileId(v === NO_PROFILE ? "" : v);
            setDirty(true);
          }}
        >
          <SelectTrigger>
            <SelectValue placeholder="None (use first available)" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NO_PROFILE}>None</SelectItem>
            {qualityProfiles?.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Audio Codec Priority */}
      <div className="space-y-3">
        <Label className="text-base font-medium">Audio Codec Priority</Label>
        <p className="text-xs text-muted-foreground">
          Select and reorder preferred audio codecs. Higher in the list = higher
          priority.
        </p>
        {audioOrder.length > 0 && (
          <div className="space-y-1">
            {audioOrder.map((codec, idx) => (
              <div
                key={codec}
                className="flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm"
              >
                <GripVertical className="h-4 w-4 text-muted-foreground" />
                <span className="flex-1">{codec}</span>
                <Badge variant="secondary" className="text-xs">
                  #{idx + 1}
                </Badge>
                <button
                  type="button"
                  onClick={() => moveAudio(idx, -1)}
                  disabled={idx === 0}
                  className="text-muted-foreground hover:text-foreground disabled:opacity-30"
                >
                  <ArrowUp className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => moveAudio(idx, 1)}
                  disabled={idx === audioOrder.length - 1}
                  className="rotate-180 text-muted-foreground hover:text-foreground disabled:opacity-30"
                >
                  <ArrowUp className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => toggleAudio(codec)}
                  className="text-muted-foreground hover:text-destructive"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}
        <div className="flex flex-wrap gap-1.5">
          {AUDIO_CODECS.filter((c) => !audioOrder.includes(c)).map((codec) => (
            <button
              key={codec}
              type="button"
              onClick={() => toggleAudio(codec)}
              className="rounded-md border border-dashed px-2 py-1 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-foreground"
            >
              + {codec}
            </button>
          ))}
        </div>
      </div>

      {/* Subtitle Languages */}
      <div className="space-y-3">
        <Label className="text-base font-medium">
          Preferred Subtitle Languages
        </Label>
        <div className="flex flex-wrap gap-2">
          {SUB_LANGUAGES.map((lang) => (
            <label
              key={lang.code}
              className="flex items-center gap-1.5 text-sm"
            >
              <Checkbox
                checked={subLangs.includes(lang.code)}
                onCheckedChange={() => toggleSubLang(lang.code)}
              />
              {lang.label}
            </label>
          ))}
        </div>
      </div>

      {/* Toggles */}
      <div className="space-y-3">
        <label
          htmlFor="pref-require-subs"
          className="flex items-center gap-2 text-sm"
        >
          <Checkbox
            id="pref-require-subs"
            checked={requireSubs}
            onCheckedChange={(v) => {
              setRequireSubs(!!v);
              setDirty(true);
            }}
          />
          Require subtitles (penalize releases without subs)
        </label>
        <label htmlFor="pref-hdr" className="flex items-center gap-2 text-sm">
          <Checkbox
            id="pref-hdr"
            checked={preferHDR}
            onCheckedChange={(v) => {
              setPreferHDR(!!v);
              setDirty(true);
            }}
          />
          Prefer HDR releases
        </label>
        <label htmlFor="pref-atmos" className="flex items-center gap-2 text-sm">
          <Checkbox
            id="pref-atmos"
            checked={preferAtmos}
            onCheckedChange={(v) => {
              setPreferAtmos(!!v);
              setDirty(true);
            }}
          />
          Prefer Atmos audio
        </label>
      </div>

      <Button onClick={handleSave} disabled={!dirty || updateMut.isPending}>
        {updateMut.isPending && (
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        )}
        Save Preferences
      </Button>

      {/* Release name tester */}
      <div className="space-y-3 border-t pt-4">
        <Label className="text-base font-medium">Test Release Name</Label>
        <p className="text-xs text-muted-foreground">
          Parse a release name to see detected media info.
        </p>
        <div className="flex gap-2">
          <Input
            value={testName}
            onChange={(e) => setTestName(e.target.value)}
            placeholder="e.g. Movie.2024.2160p.BluRay.TrueHD.Atmos.7.1.x265-GROUP"
            className="flex-1"
          />
          <Button
            variant="secondary"
            onClick={() => parseMut.mutate(testName)}
            disabled={!testName.trim() || parseMut.isPending}
          >
            {parseMut.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Search className="h-4 w-4" />
            )}
          </Button>
        </div>
        {parseMut.data && (
          <div className="space-y-1 rounded-md border bg-muted/50 p-3 text-sm">
            {Object.entries(parseMut.data).map(([k, v]) => (
              <div key={k} className="flex gap-2">
                <span className="w-32 shrink-0 font-medium text-muted-foreground">
                  {k}:
                </span>
                <span>
                  {Array.isArray(v) ? v.join(", ") || "—" : String(v) || "—"}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export function FeaturesPanel() {
  const { user } = useAuth();
  const { data: features, isLoading } = useFeatures();
  const setFeature = useSetFeature();
  // Admin gates mutation; no-auth mode (no user) is treated as permitted to
  // match the backend's no-op admin middleware.
  const canEdit = !user || user.role === "admin";

  const toggle = (key: string, label: string, enabled: boolean) => {
    setFeature.mutate(
      { key, enabled },
      {
        onSuccess: () =>
          toast.success(`${label} ${enabled ? "enabled" : "disabled"}`),
        onError: (e) =>
          toast.error(e instanceof Error ? e.message : "Failed to update"),
      },
    );
  };

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading features…
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Enable or disable optional platform features. Changes apply immediately.
      </p>
      {!canEdit && (
        <p className="text-xs text-muted-foreground">
          Only administrators can change feature settings.
        </p>
      )}
      <div className="divide-y rounded-md border">
        {(features ?? []).map((f) => (
          <div
            key={f.key}
            className="flex items-start justify-between gap-4 p-4"
          >
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium">{f.label}</span>
                <Badge variant="outline" className="text-[10px]">
                  {f.category}
                </Badge>
              </div>
              <p className="text-xs text-muted-foreground">{f.description}</p>
            </div>
            <Switch
              checked={f.enabled}
              disabled={!canEdit || setFeature.isPending}
              onCheckedChange={(v) => toggle(f.key, f.label, v === true)}
              aria-label={`Toggle ${f.label}`}
            />
          </div>
        ))}
        {(features ?? []).length === 0 && (
          <div className="p-4 text-sm text-muted-foreground">
            No configurable features.
          </div>
        )}
      </div>
    </div>
  );
}
