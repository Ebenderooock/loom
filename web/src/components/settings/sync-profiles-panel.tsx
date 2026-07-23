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
import { Switch } from "@/components/ui/switch";
import { Plus, Trash2, Pencil, Loader2 } from "lucide-react";
import { toast } from "sonner";
import {
  useSyncProfiles,
  useCreateSyncProfile,
  useUpdateSyncProfile,
  useDeleteSyncProfile,
  getSyncProfile,
  type SyncProfile,
  type SyncProfileIndexer,
  type CreateSyncProfileRequest,
  type UpdateSyncProfileRequest,
} from "@/lib/sync-profiles-api";
import { useIndexers, type Indexer } from "@/lib/indexers-api";

const APP_TYPES = [
  { value: "", label: "Any" },
  { value: "radarr", label: "Radarr" },
  { value: "sonarr", label: "Sonarr" },
] as const;

function appTypeLabel(v: string) {
  return APP_TYPES.find((t) => t.value === v)?.label ?? "Any";
}

// ─── Profile Dialog ─────────────────────────────────────────────────────

interface ProfileDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editId?: string;
  indexers: Indexer[];
}

function ProfileDialog({
  open,
  onOpenChange,
  editId,
  indexers,
}: ProfileDialogProps) {
  const [name, setName] = React.useState("");
  const [appType, setAppType] = React.useState("");
  const [enabled, setEnabled] = React.useState(true);
  const [selectedIndexers, setSelectedIndexers] = React.useState<
    Record<string, boolean>
  >({});
  const [loading, setLoading] = React.useState(false);

  const createMut = useCreateSyncProfile();
  const updateMut = useUpdateSyncProfile();

  // Load existing profile when editing.
  React.useEffect(() => {
    if (!open) return;
    if (editId) {
      setLoading(true);
      getSyncProfile(editId)
        .then((p) => {
          setName(p.name);
          setAppType(p.app_type);
          setEnabled(p.enabled);
          const sel: Record<string, boolean> = {};
          for (const idx of p.indexers ?? []) {
            sel[idx.indexer_id] = idx.enabled;
          }
          setSelectedIndexers(sel);
        })
        .catch((err: unknown) =>
          toast.error(
            `Failed to load profile: ${err instanceof Error ? err.message : "unknown"}`,
          ),
        )
        .finally(() => setLoading(false));
    } else {
      setName("");
      setAppType("");
      setEnabled(true);
      // Default: all indexers enabled
      const sel: Record<string, boolean> = {};
      for (const idx of indexers) {
        sel[idx.id] = true;
      }
      setSelectedIndexers(sel);
    }
  }, [open, editId, indexers]);

  const toggleIndexer = (id: string) => {
    setSelectedIndexers((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  const toggleAll = (checked: boolean) => {
    const sel: Record<string, boolean> = {};
    for (const idx of indexers) {
      sel[idx.id] = checked;
    }
    setSelectedIndexers(sel);
  };

  const handleSave = async () => {
    if (!name.trim()) {
      toast.error("Name is required");
      return;
    }

    const idxList: SyncProfileIndexer[] = Object.entries(selectedIndexers).map(
      ([indexer_id, en]) => ({ indexer_id, enabled: en }),
    );

    if (editId) {
      const body: UpdateSyncProfileRequest = {
        name: name.trim(),
        app_type: appType,
        enabled,
        indexers: idxList,
      };
      updateMut.mutate(
        { id: editId, body },
        {
          onSuccess: () => {
            toast.success("Sync profile updated");
            onOpenChange(false);
          },
          onError: (err) => toast.error(err.message),
        },
      );
    } else {
      const body: CreateSyncProfileRequest = {
        name: name.trim(),
        app_type: appType,
        enabled,
        indexers: idxList,
      };
      createMut.mutate(body, {
        onSuccess: () => {
          toast.success("Sync profile created");
          onOpenChange(false);
        },
        onError: (err) => toast.error(err.message),
      });
    }
  };

  const saving = createMut.isPending || updateMut.isPending;
  const allChecked =
    indexers.length > 0 && indexers.every((i) => selectedIndexers[i.id]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editId ? "Edit Sync Profile" : "New Sync Profile"}
          </DialogTitle>
        </DialogHeader>

        {loading ? (
          <div className="flex justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="sp-name">Name</Label>
              <Input
                id="sp-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Movies Only"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="sp-app-type">App Type</Label>
              <Select
                value={appType || "__any__"}
                onValueChange={(v) => setAppType(v === "__any__" ? "" : v)}
              >
                <SelectTrigger id="sp-app-type">
                  <SelectValue placeholder="Any" />
                </SelectTrigger>
                <SelectContent>
                  {APP_TYPES.map((t) => (
                    <SelectItem
                      key={t.value || "__any__"}
                      value={t.value || "__any__"}
                    >
                      {t.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center gap-2">
              <Switch
                id="sp-enabled"
                checked={enabled}
                onCheckedChange={setEnabled}
              />
              <Label htmlFor="sp-enabled">Enabled</Label>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>Indexers</Label>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => toggleAll(!allChecked)}
                >
                  {allChecked ? "Deselect All" : "Select All"}
                </Button>
              </div>
              <div className="max-h-48 space-y-1 overflow-y-auto rounded-md border p-2">
                {indexers.length === 0 && (
                  <p className="py-2 text-center text-sm text-muted-foreground">
                    No indexers configured
                  </p>
                )}
                {indexers.map((idx) => (
                  <label
                    key={idx.id}
                    className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-accent/50"
                  >
                    <Checkbox
                      checked={!!selectedIndexers[idx.id]}
                      onCheckedChange={() => toggleIndexer(idx.id)}
                    />
                    <span className="flex-1 truncate">{idx.name}</span>
                    {!idx.enabled && (
                      <Badge variant="outline" className="text-xs">
                        disabled
                      </Badge>
                    )}
                  </label>
                ))}
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button onClick={handleSave} disabled={saving}>
                {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {editId ? "Save" : "Create"}
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

// ─── Panel ──────────────────────────────────────────────────────────────

export function SyncProfilesPanel() {
  const { data: profiles = [], isLoading } = useSyncProfiles();
  const { data: indexers = [] } = useIndexers();
  const deleteMut = useDeleteSyncProfile();

  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [editId, setEditId] = React.useState<string | undefined>();
  const [deleteTarget, setDeleteTarget] = React.useState<SyncProfile | null>(
    null,
  );

  const openCreate = () => {
    setEditId(undefined);
    setDialogOpen(true);
  };

  const openEdit = (id: string) => {
    setEditId(id);
    setDialogOpen(true);
  };

  const confirmDelete = () => {
    if (!deleteTarget) return;
    deleteMut.mutate(deleteTarget.id, {
      onSuccess: () => {
        toast.success(`Deleted "${deleteTarget.name}"`);
        setDeleteTarget(null);
      },
      onError: (err) => toast.error(err.message),
    });
  };

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Sync Profiles</CardTitle>
          <Button size="sm" onClick={openCreate}>
            <Plus className="mr-1.5 h-4 w-4" />
            Add
          </Button>
        </CardHeader>
        <CardContent>
          <p className="mb-4 text-sm text-muted-foreground">
            Control which indexers are visible to each connected Radarr/Sonarr
            instance via the compatibility API. Assign a profile by appending{" "}
            <code className="rounded bg-muted px-1 text-xs">
              ?syncProfileId=PROFILE_ID
            </code>{" "}
            to the Prowlarr base URL.
          </p>

          {isLoading ? (
            <div className="flex justify-center py-6">
              <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
            </div>
          ) : profiles.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              No sync profiles yet. All indexers are visible to all connected
              apps.
            </p>
          ) : (
            <ul className="divide-y">
              {profiles.map((p) => (
                <li
                  key={p.id}
                  className="flex items-center justify-between py-3"
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">{p.name}</span>
                      <Badge variant="secondary" className="text-xs">
                        {appTypeLabel(p.app_type)}
                      </Badge>
                      {!p.enabled && (
                        <Badge variant="outline" className="text-xs">
                          disabled
                        </Badge>
                      )}
                    </div>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                      ID: {p.id}
                    </p>
                  </div>
                  <div className="flex shrink-0 gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => openEdit(p.id)}
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setDeleteTarget(p)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <ProfileDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        editId={editId}
        indexers={indexers}
      />

      {/* Delete confirmation */}
      <Dialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Sync Profile</DialogTitle>
          </DialogHeader>
          <p className="text-sm">
            Are you sure you want to delete{" "}
            <strong>{deleteTarget?.name}</strong>? Connected apps using this
            profile will see all indexers again.
          </p>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={confirmDelete}
              disabled={deleteMut.isPending}
            >
              {deleteMut.isPending && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Delete
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
