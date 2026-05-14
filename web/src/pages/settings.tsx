import * as React from "react";
import { apiFetch } from "@/lib/fetch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { useNavigate } from "@tanstack/react-router";
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
  Bell,
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
import { useSetPageHeader } from "@/hooks/use-page-header";
import {
  useMediaPreferences,
  useUpdateMediaPreferences,
  useParseReleaseName,
} from "@/lib/media-info-api";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Switch } from "@/components/ui/switch";
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
import { SyncProfilesPanel } from "@/components/settings/sync-profiles-panel";

const CATEGORIES = [
  { id: "general", label: "General" },
  { id: "media-management", label: "Media Management" },
  { id: "media-preferences", label: "Media Preferences" },

  { id: "download-clients", label: "Download Clients" },
  { id: "download-safety", label: "Download Safety" },
  { id: "rolling-search", label: "Rolling Search" },
  { id: "notifications", label: "Notifications" },
  { id: "connect", label: "Connect" },
  { id: "sync-profiles", label: "Sync Profiles" },
  { id: "ui", label: "UI" },
] as const;

type Category = (typeof CATEGORIES)[number]["id"];

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
        const data = await res.json().catch(() => ({ error: "Failed to browse" }));
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
      <DialogContent className="sm:max-w-lg max-h-[80vh] flex flex-col">
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
                : "text-muted-foreground hover:text-foreground"
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
                : "text-muted-foreground hover:text-foreground"
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
              autoFocus
            />
          </div>
        ) : (
          <div className="space-y-2 flex-1 min-h-0">
            {/* Current path breadcrumb */}
            <div className="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
              <FolderOpen className="h-4 w-4 text-primary shrink-0" />
              <span className="text-sm font-mono truncate flex-1">
                {currentPath || "/"}
              </span>
            </div>

            {error && (
              <div className="rounded-md bg-destructive/10 border border-destructive/30 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}

            {loading ? (
              <div className="flex items-center justify-center py-8 text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin mr-2" /> Loading…
              </div>
            ) : (
              <div className="overflow-y-auto max-h-[40vh] rounded-md border border-border">
                {/* Parent directory */}
                {parent && (
                  <button
                    type="button"
                    onClick={() => browse(parent)}
                    className="w-full flex items-center gap-3 px-3 py-2.5 text-sm hover:bg-accent/50 transition-colors border-b border-border"
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
                      className="w-full flex items-center gap-3 px-3 py-2.5 text-sm hover:bg-accent/50 transition-colors border-b border-border last:border-b-0"
                    >
                      <Folder className="h-4 w-4 text-primary/70" />
                      <span className="truncate flex-1 text-left">{dir.name}</span>
                      <ChevronRight className="h-3.5 w-3.5 text-muted-foreground/50" />
                    </button>
                  ))
                )}
              </div>
            )}
          </div>
        )}

        {/* Footer */}
        <div className="flex items-center justify-between pt-2 border-t border-border">
          <p className="text-xs text-muted-foreground truncate max-w-[60%]">
            {mode === "browse" ? currentPath : manualPath || "No path entered"}
          </p>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
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
              {step === "type" ? "Add Library" : `Add ${mediaType === "movie" ? "Movies" : mediaType === "series" ? "TV Shows" : "Music"} Library`}
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
                  className="flex flex-col items-center gap-3 rounded-lg border-2 border-border p-6 hover:border-primary hover:bg-primary/5 transition-all group"
                >
                  <div className="rounded-full bg-primary/10 p-3 group-hover:bg-primary/20 transition-colors">
                    <Film className="h-8 w-8 text-primary" />
                  </div>
                  <div className="text-center">
                    <p className="font-medium text-sm">Movies</p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Feature films and standalone titles
                    </p>
                  </div>
                </button>
                <button
                  type="button"
                  onClick={() => selectType("series")}
                  className="flex flex-col items-center gap-3 rounded-lg border-2 border-border p-6 hover:border-primary hover:bg-primary/5 transition-all group"
                >
                  <div className="rounded-full bg-primary/10 p-3 group-hover:bg-primary/20 transition-colors">
                    <Tv className="h-8 w-8 text-primary" />
                  </div>
                  <div className="text-center">
                    <p className="font-medium text-sm">TV Shows</p>
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Series, seasons and episodes
                    </p>
                  </div>
                </button>
              </div>
            </div>
          ) : (
            <div className="space-y-4 py-2">
              <p className="text-sm text-muted-foreground">
                Give your library a name and choose where your {mediaType === "movie" ? "movies" : "TV shows"} are stored.
              </p>

              {error && (
                <div className="rounded-md bg-destructive/10 border border-destructive/30 px-3 py-2 text-sm text-destructive">
                  {error}
                </div>
              )}

              <Input
                placeholder="Library name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="text-sm"
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
                <Button variant="ghost" size="sm" onClick={() => setStep("type")}>
                  ← Back
                </Button>
                <Button
                  size="sm"
                  onClick={addLibrary}
                  disabled={adding || !name.trim() || !path.trim()}
                >
                  {adding ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <Plus className="h-4 w-4 mr-1" />}
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
      <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading libraries…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-sm font-medium mb-1">Libraries</h3>
          <p className="text-xs text-muted-foreground">
            Libraries are the directories where Loom stores your media files. Each movie or show is placed in a subfolder within the library you assign when adding it.
          </p>
        </div>
        <Button size="sm" onClick={() => setDialogOpen(true)}>
          <Plus className="h-4 w-4 mr-1" /> Add
        </Button>
      </div>

      {error && (
        <div className="rounded-md bg-destructive/10 border border-destructive/30 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Library list */}
      {libraries.length === 0 ? (
        <div className="rounded-lg border border-dashed border-muted-foreground/30 py-10 text-center">
          <Folder className="h-10 w-10 mx-auto text-muted-foreground/40 mb-3" />
          <p className="text-sm text-muted-foreground">No libraries configured</p>
          <p className="text-xs text-muted-foreground/60 mt-1">
            Click <strong>Add</strong> to configure your first media library
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {libraries.map((lib) => (
            <div
              key={lib.id}
              className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 group hover:border-primary/30 transition-colors"
            >
              <div className="flex items-center gap-3 min-w-0">
                <Folder className="h-5 w-5 text-primary shrink-0" />
                <div className="min-w-0">
                  <p className="text-sm font-medium truncate">{lib.name}</p>
                  <p className="text-xs font-mono text-muted-foreground truncate">{lib.path}</p>
                  <div className="flex items-center gap-3 mt-0.5">
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
                className="opacity-0 group-hover:opacity-100 text-destructive hover:text-destructive hover:bg-destructive/10 transition-opacity"
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

function MediaManagementPanel() {
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
          Controls how files are transferred from the download directory to your library.
        </p>
        <Select value={importMode} onValueChange={handleChange} disabled={saving}>
          <SelectTrigger className="w-64">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="move">Move (rename / copy+delete)</SelectItem>
            <SelectItem value="hardlink">Hardlink (fall back to move)</SelectItem>
            <SelectItem value="hardlink_only">Hardlink Only (fail if not possible)</SelectItem>
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}

// ─── General Panel ──────────────────────────────────────────────────────

function GeneralPanel() {
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
    return () => { cancelled = true; };
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
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6 space-y-4">
          <div>
            <h3 className="text-sm font-medium text-zinc-100 mb-1">Application</h3>
            <p className="text-xs text-zinc-500">General application settings</p>
          </div>

          <div className="grid gap-4">
            <div className="flex items-center justify-between">
              <div>
                <Label className="text-sm text-zinc-300">App Name</Label>
                <p className="text-xs text-zinc-500 mt-0.5">Your Loom instance name</p>
              </div>
              <Badge variant="outline" className="border-zinc-700 text-zinc-300 font-mono">
                Loom
              </Badge>
            </div>

            <div className="border-t border-zinc-800" />

            <div className="flex items-center justify-between">
              <div>
                <Label className="text-sm text-zinc-300">Authentication</Label>
                <p className="text-xs text-zinc-500 mt-0.5">Login is required for all API access</p>
              </div>
              <Badge className="bg-teal-600/20 text-teal-400 border-teal-700">
                <Shield className="h-3 w-3 mr-1" /> Enabled
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Log Level */}
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6 space-y-4">
          <div>
            <h3 className="text-sm font-medium text-zinc-100 mb-1">Logging</h3>
            <p className="text-xs text-zinc-500">Control the verbosity of application logs</p>
          </div>

          <div className="flex items-center gap-4">
            <Label className="text-sm text-zinc-300 w-24">Log Level</Label>
            <Select value={logLevel} onValueChange={setLogLevel}>
              <SelectTrigger className="w-40 bg-zinc-900 border-zinc-700">
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
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6 space-y-4">
          <div>
            <h3 className="text-sm font-medium text-zinc-100 mb-1 flex items-center gap-2">
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
              className="bg-zinc-900 border-zinc-700 font-mono text-sm text-zinc-400 flex-1"
            />
            <Button
              variant="outline"
              size="sm"
              onClick={copyApiKey}
              disabled={!apiKey}
              className="border-zinc-700 text-zinc-400 shrink-0"
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

function DownloadClientsPanel() {
  const { data: clients = [], isLoading } = useDownloads();
  const createClient = useCreateDownload();
  const patchClient = usePatchDownload();
  const deleteClient = useDeleteDownload();
  const [showAdd, setShowAdd] = React.useState(false);
  const [editClient, setEditClient] = React.useState<DownloadClient | null>(null);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading download clients…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-sm font-medium text-zinc-100 mb-1">Download Clients</h3>
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
          <Plus className="h-3.5 w-3.5 mr-1.5" /> Add Client
        </Button>
      </div>

      {clients.length === 0 ? (
        <Card className="bg-zinc-900/50 border-zinc-800 border-dashed">
          <CardContent className="py-10 text-center">
            <Download className="h-10 w-10 mx-auto text-zinc-700 mb-3" />
            <p className="text-sm text-zinc-500">No download clients configured</p>
            <p className="text-xs text-zinc-600 mt-1">
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
            <Card key={client.id} className="bg-zinc-900/50 border-zinc-800">
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-medium text-zinc-200 truncate">{client.name}</h4>
                  <div className="flex items-center gap-2">
                    <Badge
                      variant="outline"
                      className={cn(
                        "text-xs",
                        client.enabled
                          ? "border-teal-700 text-teal-400"
                          : "border-zinc-700 text-zinc-500"
                      )}
                    >
                      {client.enabled ? "Enabled" : "Disabled"}
                    </Badge>
                  </div>
                </div>
                <div className="flex items-center gap-3 text-xs text-zinc-500 mb-3">
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
      <Dialog open={!!editClient} onOpenChange={(open) => !open && setEditClient(null)}>
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
                  { onSuccess: () => setEditClient(null) }
                );
              }}
              onCancel={() => setEditClient(null)}
            />
          )}
        </DialogContent>
      </Dialog>

      {/* Stall Detection Settings */}
      <div className="pt-4 border-t border-zinc-800">
        <h3 className="text-sm font-medium text-zinc-100 mb-1">Stall Detection</h3>
        <p className="text-xs text-zinc-500 mb-4">
          Automatically detect and handle stalled or failed downloads.
        </p>

        <div className="space-y-4">
          <Card className="bg-zinc-900/50 border-zinc-800">
            <CardContent className="p-4 space-y-4">
              <div className="flex items-center gap-3">
                <Checkbox
                  id="check-for-stalled"
                  defaultChecked={true}
                />
                <div>
                  <Label htmlFor="check-for-stalled" className="text-sm font-medium">
                    Enable stall detection
                  </Label>
                  <p className="text-xs text-zinc-500">
                    Monitor downloads for lack of progress and take action
                  </p>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4 max-w-lg">
                <div>
                  <Label htmlFor="stall-timeout" className="text-xs text-zinc-400">
                    Stall Timeout (minutes)
                  </Label>
                  <Input
                    id="stall-timeout"
                    type="number"
                    defaultValue={30}
                    min={5}
                    className="mt-1 bg-zinc-900 border-zinc-700"
                  />
                </div>
                <div>
                  <Label htmlFor="max-retries" className="text-xs text-zinc-400">
                    Max Retries
                  </Label>
                  <Input
                    id="max-retries"
                    type="number"
                    defaultValue={3}
                    min={0}
                    className="mt-1 bg-zinc-900 border-zinc-700"
                  />
                </div>
              </div>

              <div className="max-w-xs">
                <Label htmlFor="stall-action" className="text-xs text-zinc-400">
                  Action on Stall
                </Label>
                <Select defaultValue="remove">
                  <SelectTrigger className="mt-1 bg-zinc-900 border-zinc-700">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="pause">Pause</SelectItem>
                    <SelectItem value="remove">Remove</SelectItem>
                    <SelectItem value="remove_and_blocklist">Remove &amp; Blocklist</SelectItem>
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
  const [clients, setClients] = React.useState<{ id: string; name: string }[]>([]);
  const [showForm, setShowForm] = React.useState(false);
  const [formClientId, setFormClientId] = React.useState("");
  const [formRemotePath, setFormRemotePath] = React.useState("");
  const [formLocalPath, setFormLocalPath] = React.useState("");

  React.useEffect(() => {
    apiFetch("/api/v1/download-clients")
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => {
        const list = Array.isArray(data?.download_clients) ? data.download_clients : Array.isArray(data) ? data : [];
        setClients(list.map((c: any) => ({ id: c.id, name: c.name })));
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
      { client_id: formClientId, remote_path: formRemotePath, local_path: formLocalPath },
      {
        onSuccess: () => {
          setShowForm(false);
          setFormClientId("");
          setFormRemotePath("");
          setFormLocalPath("");
          toast.success("Remote path mapping created");
        },
        onError: (err: any) => toast.error(err.message || "Failed to create mapping"),
      }
    );
  };

  return (
    <div className="pt-4 border-t border-zinc-800">
      <div className="flex items-start justify-between mb-4">
        <div>
          <h3 className="text-sm font-medium text-zinc-100 mb-1">Remote Path Mappings</h3>
          <p className="text-xs text-zinc-500">
            Map download client paths to local paths for Docker or remote setups.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="border-zinc-700 text-zinc-300"
          onClick={() => setShowForm(true)}
        >
          <Plus className="h-3.5 w-3.5 mr-1.5" /> Add Mapping
        </Button>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 text-muted-foreground py-4 justify-center">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : mappings.length === 0 && !showForm ? (
        <Card className="bg-zinc-900/50 border-zinc-800 border-dashed">
          <CardContent className="py-6 text-center">
            <p className="text-sm text-zinc-500">No remote path mappings configured</p>
            <p className="text-xs text-zinc-600 mt-1">
              Add mappings if your download client reports paths different from what Loom sees locally.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card className="bg-zinc-900/50 border-zinc-800">
          <CardContent className="p-0">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-800 text-zinc-400 text-xs">
                  <th className="text-left p-3 font-medium">Client</th>
                  <th className="text-left p-3 font-medium">Remote Path</th>
                  <th className="text-left p-3 font-medium">Local Path</th>
                  <th className="w-10 p-3"></th>
                </tr>
              </thead>
              <tbody>
                {mappings.map((m: RemotePathMapping) => (
                  <tr key={m.id} className="border-b border-zinc-800/50 last:border-0">
                    <td className="p-3 text-zinc-200">{clientNameMap[m.client_id] || m.client_id}</td>
                    <td className="p-3 text-zinc-400 font-mono text-xs">{m.remote_path}</td>
                    <td className="p-3 text-zinc-400 font-mono text-xs">{m.local_path}</td>
                    <td className="p-3">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0 text-zinc-500 hover:text-red-400"
                        onClick={() =>
                          deleteMapping.mutate(m.id, {
                            onSuccess: () => toast.success("Mapping deleted"),
                            onError: () => toast.error("Failed to delete mapping"),
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
                <SelectTrigger id="rpm-client" className="mt-1 bg-zinc-900 border-zinc-700">
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
                className="mt-1 bg-zinc-900 border-zinc-700 font-mono text-sm"
              />
              <p className="text-xs text-zinc-600 mt-1">
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
                className="mt-1 bg-zinc-900 border-zinc-700 font-mono text-sm"
              />
              <p className="text-xs text-zinc-600 mt-1">
                The path as seen by Loom on its filesystem
              </p>
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button variant="ghost" onClick={() => setShowForm(false)}>
                Cancel
              </Button>
              <Button
                onClick={handleCreate}
                disabled={!formClientId || !formRemotePath || !formLocalPath || createMapping.isPending}
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

// ─── Notifications Panel ────────────────────────────────────────────────

function NotificationsPanel() {
  const navigate = useNavigate();
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-zinc-100 mb-1">Notifications</h3>
        <p className="text-xs text-zinc-500">
          Configure how Loom notifies you about events like grabs, downloads, and health issues.
        </p>
      </div>

      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6">
          <div className="flex items-start gap-4">
            <div className="rounded-full bg-teal-600/10 p-3 shrink-0">
              <Bell className="h-6 w-6 text-teal-400" />
            </div>
            <div className="space-y-3">
              <p className="text-sm text-zinc-400">
                Manage your notification connections — set up Discord, Slack, Telegram,
                email, webhooks, and more. Choose which events trigger each channel.
              </p>
              <Button
                variant="secondary"
                onClick={() => navigate({ to: "/notifications" })}
              >
                Manage Notifications
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Connect Panel ──────────────────────────────────────────────────────

function ConnectPanel() {
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

  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<ConnectConnection | null>(null);

  // Form state
  const [formProvider, setFormProvider] = React.useState<ConnectProviderType>("plex");
  const [formName, setFormName] = React.useState("");
  const [formHost, setFormHost] = React.useState("");
  const [formApiKey, setFormApiKey] = React.useState("");
  const [formNotifyOnImport, setFormNotifyOnImport] = React.useState(true);
  const [formEnabled, setFormEnabled] = React.useState(true);
  const [testResult, setTestResult] = React.useState<{ ok: boolean; message: string } | null>(null);

  // Trakt-specific state
  const [formClientId, setFormClientId] = React.useState("");
  const [formClientSecret, setFormClientSecret] = React.useState("");
  const [traktOAuthCode, setTraktOAuthCode] = React.useState("");
  const [traktAuthStep, setTraktAuthStep] = React.useState<"config" | "authorize" | "code" | "connected">("config");

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
      c.provider === "trakt" && c.settings.access_token ? "connected" : "config",
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
        toast.success(`Synced ${type}: ${res.movies} movies, ${res.shows} shows`),
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
          <h3 className="text-sm font-medium text-zinc-100 mb-1">Connections</h3>
          <p className="text-xs text-zinc-500">
            Connect Loom to media servers for library refresh on import.
          </p>
        </div>
        <Button size="sm" onClick={openCreate}>
          <Plus className="h-4 w-4 mr-1" /> Add Connection
        </Button>
      </div>

      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-0">
          {isLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="h-5 w-5 animate-spin text-zinc-500" />
            </div>
          ) : connections.length === 0 ? (
            <div className="text-center py-8 text-sm text-zinc-500">
              No connections configured. Click "Add Connection" to get started.
            </div>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-800 text-zinc-400 text-xs">
                  <th className="text-left px-4 py-2 font-medium">Name</th>
                  <th className="text-left px-4 py-2 font-medium">Provider</th>
                  <th className="text-left px-4 py-2 font-medium">Enabled</th>
                  <th className="text-left px-4 py-2 font-medium">Notify on Import</th>
                  <th className="text-right px-4 py-2 font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {connections.map((conn) => (
                  <tr key={conn.id} className="border-b border-zinc-800/50 last:border-0">
                    <td className="px-4 py-3 text-zinc-200">{conn.name}</td>
                    <td className="px-4 py-3">
                      <Badge variant="outline" className="border-zinc-700 text-zinc-300 text-xs">
                        {providerLabel(conn.provider)}
                      </Badge>
                      {conn.provider === "trakt" && conn.settings.access_token && (
                        <Badge variant="outline" className="ml-1 border-emerald-700 text-emerald-400 text-xs">
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
                          <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEdit(conn)}>
                            <Pencil className="h-4 w-4 mr-2" /> Edit
                          </DropdownMenuItem>
                          {conn.provider === "trakt" && conn.settings.access_token && (
                            <>
                              <DropdownMenuItem
                                disabled={isTraktSyncing}
                                onClick={() => handleTraktSync("watched", conn.id)}
                              >
                                <Download className="h-4 w-4 mr-2" /> Sync Watched
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                disabled={isTraktSyncing}
                                onClick={() => handleTraktSync("collection", conn.id)}
                              >
                                <Download className="h-4 w-4 mr-2" /> Sync Collection
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                disabled={isTraktSyncing}
                                onClick={() => handleTraktSync("watchlist", conn.id)}
                              >
                                <Download className="h-4 w-4 mr-2" /> Sync Watchlist
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                disabled={traktRefreshMut.isPending}
                                onClick={() =>
                                  traktRefreshMut.mutate(conn.id, {
                                    onSuccess: () => toast.success("Token refreshed"),
                                    onError: (err) => toast.error(err.message),
                                  })
                                }
                              >
                                <Key className="h-4 w-4 mr-2" /> Refresh Token
                              </DropdownMenuItem>
                            </>
                          )}
                          <DropdownMenuItem
                            className="text-red-400"
                            onClick={() => handleDelete(conn.id)}
                          >
                            <Trash2 className="h-4 w-4 mr-2" /> Delete
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
            <DialogTitle>{editing ? "Edit Connection" : "Add Connection"}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 pt-2">
            <div className="space-y-1.5">
              <Label>Provider</Label>
              <Select value={formProvider} onValueChange={(v) => setFormProvider(v as ConnectProviderType)}>
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
                {PROVIDER_TYPES.find((p) => p.value === formProvider)?.description}
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
                      A new tab was opened to Trakt. Authorize the app, then paste the code below.
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
                        <Loader2 className="h-4 w-4 mr-1 animate-spin" />
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
              <Switch checked={formNotifyOnImport} onCheckedChange={setFormNotifyOnImport} />
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
                    ? "bg-emerald-950/50 text-emerald-400 border border-emerald-800"
                    : "bg-red-950/50 text-red-400 border border-red-800",
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
                    <Button variant="ghost" onClick={() => setDialogOpen(false)}>
                      Cancel
                    </Button>
                    {traktAuthStep === "connected" ? (
                      <Button onClick={handleSave} disabled={isSaving || !formName}>
                        {isSaving && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
                        {editing ? "Update" : "Save"}
                      </Button>
                    ) : traktAuthStep === "config" ? (
                      <Button
                        onClick={handleSaveAndAuthorize}
                        disabled={isTraktAuthorizing || !formName || !formClientId || !formClientSecret}
                      >
                        {isTraktAuthorizing && (
                          <Loader2 className="h-4 w-4 mr-1 animate-spin" />
                        )}
                        <ExternalLink className="h-4 w-4 mr-1" />
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
                      <Loader2 className="h-4 w-4 mr-1 animate-spin" />
                    ) : (
                      <Plug className="h-4 w-4 mr-1" />
                    )}
                    Test
                  </Button>
                  <div className="flex gap-2">
                    <Button variant="ghost" onClick={() => setDialogOpen(false)}>
                      Cancel
                    </Button>
                    <Button onClick={handleSave} disabled={isSaving || !formName || !formHost}>
                      {isSaving && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
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

function DownloadSafetyPanel() {
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
        <h3 className="text-sm font-medium text-zinc-100 mb-1">
          Download Safety
        </h3>
        <p className="text-xs text-zinc-500">
          Protect against malicious or mislabeled releases before and after download.
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
            Release names matching any of these patterns will be flagged for manual review.
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
                  className="ml-1 rounded-full hover:bg-zinc-700 p-0.5"
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
              <Plus className="h-4 w-4 mr-1" /> Add
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
          <div className="grid grid-cols-2 gap-4 max-w-sm">
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
        <Button
          onClick={() => toast.success("Download safety settings saved")}
        >
          Save
        </Button>
      </div>
    </div>
  );
}

// ─── UI Panel ───────────────────────────────────────────────────────────

function UIPanel() {
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
        <h3 className="text-sm font-medium text-zinc-100 mb-1">User Interface</h3>
        <p className="text-xs text-zinc-500">
          Customize how Loom looks and feels. These settings are stored locally in your browser.
        </p>
      </div>

      {/* Theme */}
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6 space-y-4">
          <div className="flex items-center gap-3">
            <Palette className="h-4 w-4 text-teal-400" />
            <div>
              <Label className="text-sm text-zinc-200">Theme</Label>
              <p className="text-xs text-zinc-500 mt-0.5">Choose the appearance of the interface</p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Select value={theme} onValueChange={setTheme}>
              <SelectTrigger className="w-40 bg-zinc-900 border-zinc-700">
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
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6 space-y-4">
          <div>
            <Label className="text-sm text-zinc-200">Items Per Page</Label>
            <p className="text-xs text-zinc-500 mt-0.5">
              Number of items to show in paginated lists
            </p>
          </div>
          <Select value={pageSize} onValueChange={handlePageSizeChange}>
            <SelectTrigger className="w-40 bg-zinc-900 border-zinc-700">
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
      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6 space-y-4">
          <div>
            <Label className="text-sm text-zinc-200">Default View Mode</Label>
            <p className="text-xs text-zinc-500 mt-0.5">
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
                  ? "bg-teal-600/20 border-teal-700 text-teal-300"
                  : "text-zinc-400"
              )}
            >
              <LayoutGrid className="h-4 w-4 mr-1.5" /> Grid
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => handleDefaultViewChange("list")}
              className={cn(
                "border-zinc-700",
                defaultView === "list"
                  ? "bg-teal-600/20 border-teal-700 text-teal-300"
                  : "text-zinc-400"
              )}
            >
              <List className="h-4 w-4 mr-1.5" /> List
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

function RollingSearchPanel() {
  const [config, setConfig] = React.useState<RollingSearchConfig | null>(null);
  const [status, setStatus] = React.useState<RollingSearchStatus | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const triggerTimeoutRef = React.useRef<ReturnType<typeof setTimeout>>();

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
      const res = await apiFetch("/api/v1/rolling-search/trigger", { method: "POST" });
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
          <CardContent className="pt-4 space-y-2 text-sm">
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
              <span className="text-muted-foreground">Items searched (session)</span>
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
                <span className="text-muted-foreground text-xs">Quota usage (24 h)</span>
                <div className="flex flex-wrap gap-2 mt-1">
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
              c ? { ...c, intervalHours: Number(e.target.value) || 12 } : c
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
              c ? { ...c, batchSize: Number(e.target.value) || 5 } : c
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
              c ? { ...c, minResearchDays: Number(e.target.value) || 7 } : c
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
              c ? { ...c, maxSearchesPerDay: Number(e.target.value) || 100 } : c
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
          {saving ? <Loader2 className="h-4 w-4 animate-spin mr-1.5" /> : null}
          Save
        </Button>
        <Button variant="outline" size="sm" onClick={handleTrigger}>
          <Search className="h-4 w-4 mr-1.5" /> Run Now
        </Button>
      </div>
    </div>
  );
}

// ─── Settings Panels ────────────────────────────────────────────────────

// ─── Media Preferences Panel ────────────────────────────────────────────

const AUDIO_CODECS = [
  "TrueHD Atmos", "DTS-HD MA", "DTS-X", "TrueHD", "DTS-HD",
  "FLAC", "EAC3", "DTS", "AC3", "AAC", "OPUS", "MP3",
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

function MediaPreferencesPanel() {
  const { data: prefs, isLoading } = useMediaPreferences();
  const updateMut = useUpdateMediaPreferences();
  const parseMut = useParseReleaseName();
  const [testName, setTestName] = React.useState("");

  const [audioOrder, setAudioOrder] = React.useState<string[]>([]);
  const [subLangs, setSubLangs] = React.useState<string[]>([]);
  const [requireSubs, setRequireSubs] = React.useState(false);
  const [preferHDR, setPreferHDR] = React.useState(true);
  const [preferAtmos, setPreferAtmos] = React.useState(true);
  const [dirty, setDirty] = React.useState(false);

  React.useEffect(() => {
    if (prefs) {
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
      {/* Audio Codec Priority */}
      <div className="space-y-3">
        <Label className="text-base font-medium">Audio Codec Priority</Label>
        <p className="text-xs text-muted-foreground">
          Select and reorder preferred audio codecs. Higher in the list = higher priority.
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
                  className="text-muted-foreground hover:text-foreground disabled:opacity-30 rotate-180"
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
              className="rounded-md border border-dashed px-2 py-1 text-xs text-muted-foreground hover:border-primary hover:text-foreground transition-colors"
            >
              + {codec}
            </button>
          ))}
        </div>
      </div>

      {/* Subtitle Languages */}
      <div className="space-y-3">
        <Label className="text-base font-medium">Preferred Subtitle Languages</Label>
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
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={requireSubs}
            onCheckedChange={(v) => {
              setRequireSubs(!!v);
              setDirty(true);
            }}
          />
          Require subtitles (penalize releases without subs)
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={preferHDR}
            onCheckedChange={(v) => {
              setPreferHDR(!!v);
              setDirty(true);
            }}
          />
          Prefer HDR releases
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
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
          <div className="rounded-md border bg-muted/50 p-3 text-sm space-y-1">
            {Object.entries(parseMut.data).map(([k, v]) => (
              <div key={k} className="flex gap-2">
                <span className="font-medium w-32 shrink-0 text-muted-foreground">
                  {k}:
                </span>
                <span>{Array.isArray(v) ? v.join(", ") || "—" : String(v) || "—"}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function SettingsContent({ category }: { category: Category }) {
  switch (category) {
    case "general":
      return <GeneralPanel />;
    case "media-management":
      return <MediaManagementPanel />;
    case "media-preferences":
      return <MediaPreferencesPanel />;

    case "download-clients":
      return <DownloadClientsPanel />;
    case "download-safety":
      return <DownloadSafetyPanel />;
    case "rolling-search":
      return <RollingSearchPanel />;
    case "notifications":
      return <NotificationsPanel />;
    case "connect":
      return <ConnectPanel />;
    case "sync-profiles":
      return <SyncProfilesPanel />;
    case "ui":
      return <UIPanel />;
  }
}

// ─── Settings Page ──────────────────────────────────────────────────────

export function SettingsPage() {
  useSetPageHeader("Settings");
  const [active, setActive] = React.useState<Category>("general");
  const activeLabel =
    CATEGORIES.find((c) => c.id === active)?.label ?? "General";

  return (
    <div className="space-y-6">
      <div className="grid gap-6 md:grid-cols-[14rem_1fr]">
        <nav aria-label="Settings sections">
          <ul className="flex flex-col gap-1">
            {CATEGORIES.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setActive(c.id)}
                  aria-current={active === c.id ? "page" : undefined}
                  className={cn(
                    "w-full rounded-md px-3 py-2 text-left text-sm transition-colors",
                    active === c.id
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
                  )}
                >
                  {c.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>
        <Card>
          <CardHeader>
            <CardTitle>{activeLabel}</CardTitle>
          </CardHeader>
          <CardContent>
            <SettingsContent category={active} />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
