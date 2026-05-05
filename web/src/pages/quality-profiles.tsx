import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Plus, MoreVertical, Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  useQualityProfiles,
  useCreateQualityProfile,
  useUpdateQualityProfile,
  useDeleteQualityProfile,
  type QualityProfile,
  type FormatItem,
} from "@/lib/quality-profiles-api";
import { useCustomFormats } from "@/lib/custom-formats-api";

export function QualityProfilesPage() {
  const { data: profiles = [], isLoading } = useQualityProfiles();
  const deleteMut = useDeleteQualityProfile();
  const [editing, setEditing] = React.useState<QualityProfile | null>(null);
  const [creating, setCreating] = React.useState(false);

  return (
    <div className="container mx-auto py-8 space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Quality Profiles</h1>
          <p className="text-muted-foreground">
            Define quality tiers and per-profile custom format scores.
          </p>
        </div>
        <Button onClick={() => setCreating(true)}>
          <Plus className="mr-2 h-4 w-4" /> Add Profile
        </Button>
      </div>

      {isLoading ? (
        <p className="text-muted-foreground">Loading…</p>
      ) : profiles.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          No quality profiles yet. Click &quot;Add Profile&quot; to create one.
        </div>
      ) : (
        <div className="rounded-lg border divide-y">
          {profiles.map((qp) => (
            <div key={qp.id} className="flex items-center justify-between px-4 py-3">
              <div className="flex items-center gap-3 min-w-0">
                <span className="font-medium truncate">{qp.name}</span>
                <Badge variant="secondary" className="shrink-0">
                  cutoff: {qp.cutoff || "—"}
                </Badge>
                {qp.upgrade_allowed && (
                  <Badge variant="outline" className="shrink-0">upgrades</Badge>
                )}
                {qp.format_items?.length > 0 && (
                  <span className="text-xs text-muted-foreground shrink-0">
                    {qp.format_items.length} CF score{qp.format_items.length !== 1 ? "s" : ""}
                  </span>
                )}
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon"><MoreVertical className="h-4 w-4" /></Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => setEditing(qp)}>
                    <Pencil className="mr-2 h-4 w-4" /> Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() =>
                      deleteMut.mutate(qp.id, {
                        onSuccess: () => toast.success("Deleted"),
                        onError: (e) => toast.error(e.message),
                      })
                    }
                  >
                    <Trash2 className="mr-2 h-4 w-4" /> Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
        </div>
      )}

      {(creating || editing) && (
        <ProfileDialog
          initial={editing ?? undefined}
          onClose={() => { setEditing(null); setCreating(false); }}
        />
      )}
    </div>
  );
}

// ---------- Profile Dialog ----------

function ProfileDialog({
  initial,
  onClose,
}: {
  initial?: QualityProfile;
  onClose: () => void;
}) {
  const createMut = useCreateQualityProfile();
  const updateMut = useUpdateQualityProfile();
  const { data: customFormats = [] } = useCustomFormats();
  const isEdit = !!initial;

  const [name, setName] = React.useState(initial?.name ?? "");
  const [cutoff, setCutoff] = React.useState(initial?.cutoff ?? "");
  const [items, setItems] = React.useState(initial?.items ?? "");
  const [upgradeAllowed, setUpgradeAllowed] = React.useState(initial?.upgrade_allowed ?? true);
  const [formatItems, setFormatItems] = React.useState<FormatItem[]>(initial?.format_items ?? []);

  const updateFormatScore = (cfId: string, score: number) => {
    setFormatItems((prev) => {
      const existing = prev.find((fi) => fi.custom_format_id === cfId);
      if (existing) {
        return prev.map((fi) =>
          fi.custom_format_id === cfId ? { ...fi, score } : fi,
        );
      }
      return [...prev, { custom_format_id: cfId, score }];
    });
  };

  const getScore = (cfId: string) =>
    formatItems.find((fi) => fi.custom_format_id === cfId)?.score ?? 0;

  const handleSave = () => {
    if (isEdit) {
      updateMut.mutate(
        {
          id: initial!.id,
          body: { name, cutoff, items, upgrade_allowed: upgradeAllowed, format_items: formatItems },
        },
        {
          onSuccess: () => { toast.success("Updated"); onClose(); },
          onError: (e) => toast.error(e.message),
        },
      );
    } else {
      createMut.mutate(
        { name, cutoff, items, upgrade_allowed: upgradeAllowed, format_items: formatItems },
        {
          onSuccess: () => { toast.success("Created"); onClose(); },
          onError: (e) => toast.error(e.message),
        },
      );
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit" : "Add"} Quality Profile</DialogTitle>
          <DialogDescription>
            Configure quality tiers and custom format scoring.
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-1.5">
            <Label htmlFor="qp-name">Name</Label>
            <Input id="qp-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="HD-1080p" />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="qp-cutoff">Cutoff</Label>
              <Input id="qp-cutoff" value={cutoff} onChange={(e) => setCutoff(e.target.value)} placeholder="Bluray-1080p" />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="qp-items">Items (JSON)</Label>
              <Input id="qp-items" value={items} onChange={(e) => setItems(e.target.value)} placeholder='["HDTV-1080p","Bluray-1080p"]' />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="qp-upgrade"
              checked={upgradeAllowed}
              onCheckedChange={(v) => setUpgradeAllowed(!!v)}
            />
            <Label htmlFor="qp-upgrade">Allow upgrades</Label>
          </div>

          {customFormats.length > 0 && (
            <div className="space-y-2">
              <Label>Custom Format Scores</Label>
              <div className="rounded-md border divide-y max-h-60 overflow-y-auto">
                {customFormats.map((cf) => (
                  <div key={cf.id} className="flex items-center justify-between px-3 py-2">
                    <span className="text-sm">{cf.name}</span>
                    <Input
                      type="number"
                      className="w-24 h-8 text-sm"
                      value={getScore(cf.id)}
                      onChange={(e) => updateFormatScore(cf.id, parseInt(e.target.value, 10) || 0)}
                    />
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>Cancel</Button>
          <Button onClick={handleSave} disabled={!name || createMut.isPending || updateMut.isPending}>
            {isEdit ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
