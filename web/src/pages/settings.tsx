import * as React from "react";
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
import { Link } from "@tanstack/react-router";
import { toast } from "sonner";
import { NamingSettings } from "@/components/movies/naming-settings";
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
  Info,
  Bell,
  Plug,
  ExternalLink,
  Download,
  Rss,
  Palette,
  LayoutGrid,
  List,
  GripVertical,
  Search,
} from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import {
  useMediaPreferences,
  useUpdateMediaPreferences,
  useParseReleaseName,
} from "@/lib/media-info-api";

const CATEGORIES = [
  { id: "general", label: "General" },
  { id: "media-management", label: "Media Management" },
  { id: "media-preferences", label: "Media Preferences" },
  { id: "profiles", label: "Profiles" },
  { id: "indexers", label: "Indexers" },
  { id: "download-clients", label: "Download Clients" },
  { id: "download-safety", label: "Download Safety" },
  { id: "rolling-search", label: "Rolling Search" },
  { id: "notifications", label: "Notifications" },
  { id: "connect", label: "Connect" },
  { id: "ui", label: "UI" },
] as const;

type Category = (typeof CATEGORIES)[number]["id"];

// ─── Types ──────────────────────────────────────────────────────────────

interface RootFolder {
  id: string;
  path: string;
  freeSpace: number;
  unmappedCount: number;
  createdAt: string;
  updatedAt: string;
}

// ─── Root Folders Panel ─────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (bytes <= 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

// ─── Filesystem Browser Dialog ──────────────────────────────────────────

type LibraryType = "movies" | "tvshows";

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
      const res = await fetch(`/api/v1/filesystem${params}`, { credentials: "include" });
      if (!res.ok) {
        const data = await res.json().catch(() => ({ error: "Failed to browse" }));
        throw new Error(data.error || "Failed to browse directory");
      }
      const data: BrowseResult = await res.json();
      setCurrentPath(data.current);
      setDirs(data.directories);
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

// ─── Add Root Folder Dialog (type chooser + folder picker) ──────────────

function AddRootFolderDialog({
  open,
  onOpenChange,
  onAdded,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAdded: () => void;
}) {
  const [step, setStep] = React.useState<"type" | "path">("type");
  const [libraryType, setLibraryType] = React.useState<LibraryType | null>(null);
  const [showBrowser, setShowBrowser] = React.useState(false);
  const [path, setPath] = React.useState("");
  const [adding, setAdding] = React.useState(false);
  const [error, setError] = React.useState("");

  React.useEffect(() => {
    if (open) {
      setStep("type");
      setLibraryType(null);
      setPath("");
      setError("");
    }
  }, [open]);

  const selectType = (type: LibraryType) => {
    setLibraryType(type);
    setStep("path");
  };

  const addFolder = async (folderPath: string) => {
    const trimmed = folderPath.trim();
    if (!trimmed) return;
    setAdding(true);
    setError("");
    try {
      const res = await fetch("/api/v1/movies/root-folders", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: trimmed }),
      });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || "Failed to add root folder");
      }
      onAdded();
      onOpenChange(false);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to add root folder");
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
              {step === "type" ? "Add Root Folder" : `Add ${libraryType === "movies" ? "Movies" : "TV Shows"} Folder`}
            </DialogTitle>
          </DialogHeader>

          {step === "type" ? (
            <div className="space-y-3 py-2">
              <p className="text-sm text-muted-foreground">
                What type of media will this folder contain?
              </p>
              <div className="grid grid-cols-2 gap-3">
                <button
                  type="button"
                  onClick={() => selectType("movies")}
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
                  onClick={() => selectType("tvshows")}
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
                Choose where your {libraryType === "movies" ? "movies" : "TV shows"} are stored. You can type a path or browse the filesystem.
              </p>

              {error && (
                <div className="rounded-md bg-destructive/10 border border-destructive/30 px-3 py-2 text-sm text-destructive">
                  {error}
                </div>
              )}

              <div className="flex gap-2">
                <Input
                  placeholder="/path/to/media"
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && addFolder(path)}
                  className="font-mono text-sm"
                  autoFocus
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
                  onClick={() => addFolder(path)}
                  disabled={adding || !path.trim()}
                >
                  {adding ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <Plus className="h-4 w-4 mr-1" />}
                  Add Folder
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

// ─── Root Folders Panel ─────────────────────────────────────────────────

function RootFoldersPanel() {
  const [folders, setFolders] = React.useState<RootFolder[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");
  const [deletingId, setDeletingId] = React.useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = React.useState(false);

  const fetchFolders = React.useCallback(async () => {
    try {
      const res = await fetch("/api/v1/movies/root-folders", { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch root folders");
      setFolders(await res.json());
    } catch {
      setError("Failed to load root folders");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { fetchFolders(); }, [fetchFolders]);

  const deleteFolder = async (id: string) => {
    setDeletingId(id);
    setError("");
    try {
      const res = await fetch(`/api/v1/movies/root-folders/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to delete root folder");
      await fetchFolders();
    } catch {
      setError("Failed to delete root folder");
    } finally {
      setDeletingId(null);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading root folders…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-sm font-medium mb-1">Root Folders</h3>
          <p className="text-xs text-muted-foreground">
            Root folders are the directories where Loom stores your media files. Each movie or show is placed in a subfolder within the root folder you assign when adding it.
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

      {/* Folder list */}
      {folders.length === 0 ? (
        <div className="rounded-lg border border-dashed border-muted-foreground/30 py-10 text-center">
          <Folder className="h-10 w-10 mx-auto text-muted-foreground/40 mb-3" />
          <p className="text-sm text-muted-foreground">No root folders configured</p>
          <p className="text-xs text-muted-foreground/60 mt-1">
            Click <strong>Add</strong> to configure your first media folder
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {folders.map((folder) => (
            <div
              key={folder.id}
              className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 group hover:border-primary/30 transition-colors"
            >
              <div className="flex items-center gap-3 min-w-0">
                <Folder className="h-5 w-5 text-primary shrink-0" />
                <div className="min-w-0">
                  <p className="text-sm font-mono truncate">{folder.path}</p>
                  <div className="flex items-center gap-3 mt-0.5">
                    {folder.freeSpace > 0 && (
                      <span className="flex items-center gap-1 text-xs text-muted-foreground">
                        <HardDrive className="h-3 w-3" />
                        {formatBytes(folder.freeSpace)} free
                      </span>
                    )}
                    {folder.unmappedCount > 0 && (
                      <Badge variant="secondary" className="text-xs">
                        {folder.unmappedCount} unmapped
                      </Badge>
                    )}
                  </div>
                </div>
              </div>
              <Button
                variant="ghost"
                size="icon"
                className="opacity-0 group-hover:opacity-100 text-destructive hover:text-destructive hover:bg-destructive/10 transition-opacity"
                onClick={() => deleteFolder(folder.id)}
                disabled={deletingId === folder.id}
              >
                {deletingId === folder.id ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Trash2 className="h-4 w-4" />
                )}
              </Button>
            </div>
          ))}
        </div>
      )}

      <AddRootFolderDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onAdded={fetchFolders}
      />
    </div>
  );
}

// ─── Media Management Panel ─────────────────────────────────────────────

function MediaManagementPanel() {
  return (
    <div className="space-y-8">
      <RootFoldersPanel />
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
    fetch("/api/v1/movies/organize/import-mode", { credentials: "include" })
      .then((r) => r.json())
      .then((data) => setImportMode(data.import_mode ?? "move"))
      .catch(() => {});
  }, []);

  const handleChange = async (value: string) => {
    setImportMode(value);
    setSaving(true);
    try {
      await fetch("/api/v1/movies/organize/import-mode", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
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
  const [apiKey] = React.useState(() => {
    // Generate a deterministic placeholder key for display
    return "loom_api_" + Math.random().toString(36).substring(2, 18);
  });
  const [copied, setCopied] = React.useState(false);

  const copyApiKey = async () => {
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
              value={apiKey}
              className="bg-zinc-900 border-zinc-700 font-mono text-sm text-zinc-400 flex-1"
            />
            <Button
              variant="outline"
              size="sm"
              onClick={copyApiKey}
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

// ─── Profiles Panel ─────────────────────────────────────────────────────

interface QualityProfile {
  id: number;
  name: string;
  cutoff: number;
  items: QualityProfileItem[];
}

interface QualityProfileItem {
  quality_definition_id: number;
  allowed: boolean;
}

interface QualityDefinition {
  id: number;
  title: string;
  source: string;
  resolution: number;
  min_size: number;
  max_size: number;
  preferred_size: number;
}

function ProfileEditorDialog({
  open,
  onOpenChange,
  profile,
  definitions,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  profile: QualityProfile | null;
  definitions: QualityDefinition[];
  onSaved: () => void;
}) {
  const isNew = !profile;
  const [name, setName] = React.useState("");
  const [items, setItems] = React.useState<Record<number, boolean>>({});
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    if (open) {
      if (profile) {
        setName(profile.name);
        const map: Record<number, boolean> = {};
        for (const item of profile.items) {
          map[item.quality_definition_id] = item.allowed;
        }
        setItems(map);
      } else {
        setName("");
        setItems({});
      }
    }
  }, [open, profile]);

  const toggleItem = (defId: number) => {
    setItems((prev) => ({ ...prev, [defId]: !prev[defId] }));
  };

  const handleSave = async () => {
    if (!name.trim()) {
      toast.error("Profile name is required");
      return;
    }

    setSaving(true);
    try {
      const body = {
        name: name.trim(),
        cutoff: 0,
        items: definitions.map((d) => ({
          quality_definition_id: d.id,
          allowed: items[d.id] ?? false,
        })),
      };

      const url = isNew
        ? "/api/v1/movies/quality-profiles"
        : `/api/v1/movies/quality-profiles/${profile!.id}`;
      const method = isNew ? "POST" : "PUT";

      const res = await fetch(url, {
        method,
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!res.ok) throw new Error(await res.text());

      toast.success(isNew ? "Profile created" : "Profile updated");
      onSaved();
      onOpenChange(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save profile");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>{isNew ? "New Quality Profile" : "Edit Quality Profile"}</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 flex-1 overflow-hidden">
          <div className="space-y-2">
            <Label className="text-sm text-zinc-400">Profile Name</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. HD-1080p"
              className="bg-zinc-900 border-zinc-700"
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <Label className="text-sm text-zinc-400">Qualities</Label>
            <div className="overflow-y-auto max-h-[40vh] rounded-md border border-zinc-800 divide-y divide-zinc-800">
              {definitions.map((def) => (
                <label
                  key={def.id}
                  className="flex items-center gap-3 px-3 py-2.5 hover:bg-zinc-800/50 cursor-pointer"
                >
                  <Checkbox
                    checked={items[def.id] ?? false}
                    onCheckedChange={() => toggleItem(def.id)}
                  />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-zinc-200">{def.title}</p>
                    <p className="text-xs text-zinc-500">
                      {def.source} · {def.resolution}p
                    </p>
                  </div>
                </label>
              ))}
              {definitions.length === 0 && (
                <div className="px-3 py-6 text-center text-sm text-zinc-500">
                  No quality definitions available
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-2 border-t border-zinc-800">
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)} className="border-zinc-700">
            Cancel
          </Button>
          <Button size="sm" onClick={handleSave} disabled={saving} className="bg-teal-600 hover:bg-teal-700">
            {saving && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
            {isNew ? "Create" : "Save"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ProfilesPanel() {
  const [profiles, setProfiles] = React.useState<QualityProfile[]>([]);
  const [definitions, setDefinitions] = React.useState<QualityDefinition[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [editorOpen, setEditorOpen] = React.useState(false);
  const [editingProfile, setEditingProfile] = React.useState<QualityProfile | null>(null);
  const [deletingId, setDeletingId] = React.useState<number | null>(null);

  const fetchData = React.useCallback(async () => {
    try {
      const [profilesRes, defsRes] = await Promise.all([
        fetch("/api/v1/movies/quality-profiles", { credentials: "include" }),
        fetch("/api/v1/movies/quality-definitions", { credentials: "include" }),
      ]);

      if (profilesRes.ok) {
        const data = await profilesRes.json();
        setProfiles(Array.isArray(data) ? data : []);
      }
      if (defsRes.ok) {
        const data = await defsRes.json();
        setDefinitions(Array.isArray(data) ? data : []);
      }
    } catch {
      // Silent fail on initial load
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { fetchData(); }, [fetchData]);

  const deleteProfile = async (id: number) => {
    setDeletingId(id);
    try {
      const res = await fetch(`/api/v1/movies/quality-profiles/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Failed to delete profile");
      toast.success("Profile deleted");
      await fetchData();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to delete");
    } finally {
      setDeletingId(null);
    }
  };

  const openEditor = (profile: QualityProfile | null) => {
    setEditingProfile(profile);
    setEditorOpen(true);
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading profiles…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-sm font-medium text-zinc-100 mb-1">Quality Profiles</h3>
          <p className="text-xs text-zinc-500">
            Quality profiles define which qualities are acceptable when grabbing releases.
          </p>
        </div>
        <Button size="sm" onClick={() => openEditor(null)} className="bg-teal-600 hover:bg-teal-700">
          <Plus className="h-4 w-4 mr-1" /> Add Profile
        </Button>
      </div>

      {profiles.length === 0 ? (
        <Card className="bg-zinc-900/50 border-zinc-800 border-dashed">
          <CardContent className="py-10 text-center">
            <GripVertical className="h-10 w-10 mx-auto text-zinc-700 mb-3" />
            <p className="text-sm text-zinc-500">No quality profiles configured</p>
            <p className="text-xs text-zinc-600 mt-1">
              Create a profile to define acceptable quality levels for your media
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {profiles.map((profile) => {
            const allowedCount = profile.items.filter((i) => i.allowed).length;
            return (
              <Card key={profile.id} className="bg-zinc-900/50 border-zinc-800 group">
                <CardContent className="p-4 flex items-center justify-between">
                  <div className="min-w-0">
                    <h4 className="text-sm font-medium text-zinc-200">{profile.name}</h4>
                    <p className="text-xs text-zinc-500 mt-0.5">
                      {allowedCount} {allowedCount === 1 ? "quality" : "qualities"} allowed
                    </p>
                    {allowedCount > 0 && (
                      <div className="flex flex-wrap gap-1 mt-2">
                        {profile.items
                          .filter((i) => i.allowed)
                          .slice(0, 5)
                          .map((item) => {
                            const def = definitions.find((d) => d.id === item.quality_definition_id);
                            return (
                              <Badge
                                key={item.quality_definition_id}
                                variant="outline"
                                className="text-xs border-zinc-700 text-zinc-400"
                              >
                                {def?.title ?? `Quality #${item.quality_definition_id}`}
                              </Badge>
                            );
                          })}
                        {allowedCount > 5 && (
                          <Badge variant="outline" className="text-xs border-zinc-700 text-zinc-500">
                            +{allowedCount - 5} more
                          </Badge>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="opacity-0 group-hover:opacity-100 text-zinc-400 hover:text-zinc-200 transition-opacity"
                      onClick={() => openEditor(profile)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="opacity-0 group-hover:opacity-100 text-destructive hover:text-destructive hover:bg-destructive/10 transition-opacity"
                      onClick={() => deleteProfile(profile.id)}
                      disabled={deletingId === profile.id}
                    >
                      {deletingId === profile.id ? (
                        <Loader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <Trash2 className="h-4 w-4" />
                      )}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      <ProfileEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        profile={editingProfile}
        definitions={definitions}
        onSaved={fetchData}
      />
    </div>
  );
}

// ─── Indexers Panel ─────────────────────────────────────────────────────

interface IndexerSummary {
  id: number;
  name: string;
  implementation: string;
  enable: boolean;
  priority: number;
}

function IndexersPanel() {
  const [indexers, setIndexers] = React.useState<IndexerSummary[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    fetch("/api/v1/indexers", { credentials: "include" })
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => setIndexers(Array.isArray(data) ? data : []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading indexers…
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="text-sm font-medium text-zinc-100 mb-1">Indexers</h3>
          <p className="text-xs text-zinc-500">
            Overview of configured indexers. Use the full Indexers page for management.
          </p>
        </div>
        <Link to="/indexers">
          <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300">
            Manage Indexers <ExternalLink className="h-3.5 w-3.5 ml-1.5" />
          </Button>
        </Link>
      </div>

      {indexers.length === 0 ? (
        <Card className="bg-zinc-900/50 border-zinc-800 border-dashed">
          <CardContent className="py-10 text-center">
            <Rss className="h-10 w-10 mx-auto text-zinc-700 mb-3" />
            <p className="text-sm text-zinc-500">No indexers configured</p>
            <p className="text-xs text-zinc-600 mt-1">
              <Link to="/indexers" className="text-teal-500 hover:text-teal-400">
                Add indexers →
              </Link>{" "}
              to start searching for releases
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {indexers.map((indexer) => (
            <Card key={indexer.id} className="bg-zinc-900/50 border-zinc-800">
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-medium text-zinc-200 truncate">{indexer.name}</h4>
                  <Badge
                    variant="outline"
                    className={cn(
                      "text-xs",
                      indexer.enable
                        ? "border-teal-700 text-teal-400"
                        : "border-zinc-700 text-zinc-500"
                    )}
                  >
                    {indexer.enable ? "Enabled" : "Disabled"}
                  </Badge>
                </div>
                <div className="flex items-center gap-3 text-xs text-zinc-500">
                  <span className="flex items-center gap-1">
                    <Rss className="h-3 w-3" /> {indexer.implementation || "Unknown"}
                  </span>
                  <span>Priority: {indexer.priority}</span>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Download Clients Panel ─────────────────────────────────────────────

interface DownloadClientSummary {
  id: number;
  name: string;
  implementation: string;
  enable: boolean;
  priority: number;
}

function DownloadClientsPanel() {
  const [clients, setClients] = React.useState<DownloadClientSummary[]>([]);
  const [loading, setLoading] = React.useState(true);

  React.useEffect(() => {
    fetch("/api/v1/download-clients", { credentials: "include" })
      .then((r) => (r.ok ? r.json() : []))
      .then((data) => setClients(Array.isArray(data) ? data : []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
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
            Overview of configured download clients. Use the Downloads page for full management.
          </p>
        </div>
        <Link to="/downloads">
          <Button variant="outline" size="sm" className="border-zinc-700 text-zinc-300">
            Manage Download Clients <ExternalLink className="h-3.5 w-3.5 ml-1.5" />
          </Button>
        </Link>
      </div>

      {clients.length === 0 ? (
        <Card className="bg-zinc-900/50 border-zinc-800 border-dashed">
          <CardContent className="py-10 text-center">
            <Download className="h-10 w-10 mx-auto text-zinc-700 mb-3" />
            <p className="text-sm text-zinc-500">No download clients configured</p>
            <p className="text-xs text-zinc-600 mt-1">
              <Link to="/downloads" className="text-teal-500 hover:text-teal-400">
                Add a download client →
              </Link>{" "}
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
                  <Badge
                    variant="outline"
                    className={cn(
                      "text-xs",
                      client.enable
                        ? "border-teal-700 text-teal-400"
                        : "border-zinc-700 text-zinc-500"
                    )}
                  >
                    {client.enable ? "Enabled" : "Disabled"}
                  </Badge>
                </div>
                <div className="flex items-center gap-3 text-xs text-zinc-500">
                  <span className="flex items-center gap-1">
                    <Download className="h-3 w-3" /> {client.implementation || "Unknown"}
                  </span>
                  <span>Priority: {client.priority}</span>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

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
    </div>
  );
}

// ─── Notifications Panel ────────────────────────────────────────────────

function NotificationsPanel() {
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
            <div>
              <h4 className="text-sm font-medium text-zinc-200 mb-1">Coming Soon</h4>
              <p className="text-sm text-zinc-400">
                Notification connections can be configured here once the backend is ready.
                Planned notification types include:
              </p>
              <ul className="mt-3 space-y-1.5 text-sm text-zinc-500">
                <li className="flex items-center gap-2">
                  <Info className="h-3.5 w-3.5 text-zinc-600" /> Email notifications
                </li>
                <li className="flex items-center gap-2">
                  <Info className="h-3.5 w-3.5 text-zinc-600" /> Discord webhooks
                </li>
                <li className="flex items-center gap-2">
                  <Info className="h-3.5 w-3.5 text-zinc-600" /> Telegram bot messages
                </li>
                <li className="flex items-center gap-2">
                  <Info className="h-3.5 w-3.5 text-zinc-600" /> Custom webhooks
                </li>
              </ul>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

// ─── Connect Panel ──────────────────────────────────────────────────────

const PLANNED_INTEGRATIONS = [
  { name: "Trakt", description: "Sync watch history and ratings" },
  { name: "Plex", description: "Media server integration for library syncing" },
  { name: "Jellyfin", description: "Open-source media server integration" },
  { name: "Emby", description: "Media server notification and sync" },
];

function ConnectPanel() {
  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-sm font-medium text-zinc-100 mb-1">Connections</h3>
        <p className="text-xs text-zinc-500">
          Third-party integrations for syncing and notifications.
        </p>
      </div>

      <Card className="bg-zinc-900/50 border-zinc-800">
        <CardContent className="p-6">
          <div className="flex items-start gap-4 mb-6">
            <div className="rounded-full bg-teal-600/10 p-3 shrink-0">
              <Plug className="h-6 w-6 text-teal-400" />
            </div>
            <div>
              <h4 className="text-sm font-medium text-zinc-200 mb-1">Integrations</h4>
              <p className="text-sm text-zinc-400">
                Connect Loom to external services for an enhanced experience.
              </p>
            </div>
          </div>

          <div className="space-y-3">
            {PLANNED_INTEGRATIONS.map((integration) => (
              <div
                key={integration.name}
                className="flex items-center justify-between rounded-lg border border-zinc-800 px-4 py-3"
              >
                <div>
                  <h5 className="text-sm font-medium text-zinc-300">{integration.name}</h5>
                  <p className="text-xs text-zinc-500 mt-0.5">{integration.description}</p>
                </div>
                <Badge variant="outline" className="border-zinc-700 text-zinc-500 text-xs shrink-0">
                  Coming Soon
                </Badge>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
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
                <SelectItem value="light" disabled>
                  Light (coming soon)
                </SelectItem>
                <SelectItem value="system" disabled>
                  System (coming soon)
                </SelectItem>
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

  const fetchData = React.useCallback(async () => {
    try {
      const [cfgRes, statusRes] = await Promise.all([
        fetch("/api/v1/rolling-search/config"),
        fetch("/api/v1/rolling-search/status"),
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
      const res = await fetch("/api/v1/rolling-search/config", {
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
      const res = await fetch("/api/v1/rolling-search/trigger", { method: "POST" });
      if (!res.ok) throw new Error("trigger failed");
      toast.success("Rolling search triggered");
      setTimeout(fetchData, 2000);
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
    case "profiles":
      return <ProfilesPanel />;
    case "indexers":
      return <IndexersPanel />;
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
