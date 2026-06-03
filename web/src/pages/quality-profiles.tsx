import * as React from "react";
import { apiFetch } from "@/lib/fetch";
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
import { Plus, MoreVertical, Pencil, Trash2, Loader2, Layers } from "lucide-react";
import { ListSkeleton } from "@/components/ui/skeletons";
import { EmptyState } from "@/components/ui/empty-state";
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

// ---------- Quality definition types and grouping ----------

interface QualityDefinition {
  id: string;
  title: string;
  name: string;
  source: string;
  resolution: string;
  modifier?: string;
  preferred_at: number;
}

interface ParsedItem {
  id: string;
  name: string;
  preferred: boolean;
  allowed: boolean;
}

const SOURCE_GROUP_ORDER = ["TV", "Web", "WebRip", "BluRay", "DVD", "Unknown"] as const;
const SOURCE_GROUP_LABELS: Record<string, string> = {
  TV: "HDTV",
  Web: "WEB-DL",
  WebRip: "WEBRip",
  BluRay: "Blu-ray",
  DVD: "DVD",
  Unknown: "Other",
};

function groupDefinitionsBySource(definitions: QualityDefinition[]) {
  const groups: { source: string; label: string; defs: QualityDefinition[] }[] = [];
  const bySource = new Map<string, QualityDefinition[]>();

  for (const def of definitions) {
    const src = def.source || "Unknown";
    if (!bySource.has(src)) bySource.set(src, []);
    bySource.get(src)!.push(def);
  }

  for (const source of SOURCE_GROUP_ORDER) {
    const defs = bySource.get(source);
    if (defs && defs.length > 0) {
      groups.push({
        source,
        label: SOURCE_GROUP_LABELS[source] || source,
        defs: defs.sort((a, b) => a.preferred_at - b.preferred_at),
      });
      bySource.delete(source);
    }
  }

  for (const [source, defs] of bySource) {
    if (defs.length > 0) {
      groups.push({ source, label: source, defs: defs.sort((a, b) => a.preferred_at - b.preferred_at) });
    }
  }

  return groups;
}

function parseProfileItems(itemsJson: string): ParsedItem[] {
  try {
    const parsed = JSON.parse(itemsJson);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

// ---------- Page ----------

export function QualityProfilesPage() {
  const { data: profiles = [], isLoading } = useQualityProfiles();
  const deleteMut = useDeleteQualityProfile();
  const [editing, setEditing] = React.useState<QualityProfile | null>(null);
  const [creating, setCreating] = React.useState(false);

  // Fetch quality definitions for the editor and for display
  const [definitions, setDefinitions] = React.useState<QualityDefinition[]>([]);
  React.useEffect(() => {
    apiFetch("/api/v1/movies/quality-definitions")
      .then((r) => r.ok ? r.json() : [])
      .then((data) => setDefinitions(Array.isArray(data) ? data : []))
      .catch((err) => console.error("fetch failed:", err));
  }, []);

  const defById = React.useMemo(() => {
    const map = new Map<string, QualityDefinition>();
    for (const d of definitions) map.set(d.id, d);
    return map;
  }, [definitions]);

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
        <ListSkeleton rows={4} />
      ) : profiles.length === 0 ? (
        <EmptyState
          icon={<Layers />}
          title="No quality profiles yet"
          description="Quality profiles decide which releases Loom grabs and when to upgrade. Create one to get started."
        />
      ) : (
        <div className="space-y-3">
          {profiles.map((qp) => {
            const items = parseProfileItems(qp.items);
            const allowedCount = items.filter((i) => i.allowed).length;

            return (
              <div key={qp.id} className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-4 flex items-center justify-between group">
                <div className="min-w-0">
                  <h4 className="text-sm font-medium text-zinc-200">{qp.name}</h4>
                  <p className="text-xs text-zinc-500 mt-0.5">
                    {allowedCount} {allowedCount === 1 ? "quality" : "qualities"} allowed
                  </p>
                  {allowedCount > 0 && (
                    <div className="flex flex-wrap gap-1 mt-2">
                      {items.filter((i) => i.allowed).slice(0, 6).map((item) => {
                        const def = defById.get(item.id);
                        return (
                          <Badge key={item.id} variant="secondary" className="text-[10px] px-1.5 py-0 bg-zinc-800 text-zinc-300">
                            {def?.title || item.name}
                          </Badge>
                        );
                      })}
                      {allowedCount > 6 && (
                        <Badge variant="secondary" className="text-[10px] px-1.5 py-0 bg-zinc-800 text-zinc-400">
                          +{allowedCount - 6} more
                        </Badge>
                      )}
                    </div>
                  )}
                  {qp.format_items?.length > 0 && (
                    <p className="text-xs text-zinc-500 mt-1">
                      {qp.format_items.length} custom format score{qp.format_items.length !== 1 ? "s" : ""}
                    </p>
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
            );
          })}
        </div>
      )}

      {(creating || editing) && (
        <ProfileDialog
          initial={editing ?? undefined}
          definitions={definitions}
          onClose={() => { setEditing(null); setCreating(false); }}
        />
      )}
    </div>
  );
}

// ---------- Profile Dialog with grouped qualities + custom format scoring ----------

function ProfileDialog({
  initial,
  definitions,
  onClose,
}: {
  initial?: QualityProfile;
  definitions: QualityDefinition[];
  onClose: () => void;
}) {
  const createMut = useCreateQualityProfile();
  const updateMut = useUpdateQualityProfile();
  const { data: customFormats = [] } = useCustomFormats();
  const isEdit = !!initial;

  const [name, setName] = React.useState(initial?.name ?? "");
  const [cutoff, setCutoff] = React.useState(initial?.cutoff ?? "");
  const [upgradeAllowed, setUpgradeAllowed] = React.useState(initial?.upgrade_allowed ?? true);
  const [formatItems, setFormatItems] = React.useState<FormatItem[]>(initial?.format_items ?? []);

  // Parse existing items into a checked-map keyed by quality definition ID
  const [checkedItems, setCheckedItems] = React.useState<Record<string, boolean>>(() => {
    if (!initial?.items) return {};
    const items = parseProfileItems(initial.items);
    const map: Record<string, boolean> = {};
    for (const item of items) {
      if (item.allowed) map[item.id] = true;
    }
    return map;
  });

  const groups = React.useMemo(() => groupDefinitionsBySource(definitions), [definitions]);

  const toggleItem = (defId: string) => {
    setCheckedItems((prev) => ({ ...prev, [defId]: !prev[defId] }));
  };

  const toggleGroup = (defs: QualityDefinition[]) => {
    const allChecked = defs.every((d) => checkedItems[d.id]);
    setCheckedItems((prev) => {
      const next = { ...prev };
      for (const d of defs) next[d.id] = !allChecked;
      return next;
    });
  };

  const toggleAll = () => {
    const allChecked = definitions.every((d) => checkedItems[d.id]);
    setCheckedItems((prev) => {
      const next = { ...prev };
      for (const d of definitions) next[d.id] = !allChecked;
      return next;
    });
  };

  const selectedCount = definitions.filter((d) => checkedItems[d.id]).length;

  const updateFormatScore = (cfId: string, score: number) => {
    setFormatItems((prev) => {
      const existing = prev.find((fi) => fi.custom_format_id === cfId);
      if (existing) {
        return prev.map((fi) => fi.custom_format_id === cfId ? { ...fi, score } : fi);
      }
      return [...prev, { custom_format_id: cfId, score }];
    });
  };

  const getScore = (cfId: string) =>
    formatItems.find((fi) => fi.custom_format_id === cfId)?.score ?? 0;

  const handleSave = () => {
    if (!name.trim()) {
      toast.error("Profile name is required");
      return;
    }

    // Build items JSON array matching the shape autosearch expects
    const itemsArray = definitions.map((d) => ({
      id: d.id,
      name: d.name,
      preferred: d.id === cutoff,
      allowed: checkedItems[d.id] ?? false,
    }));
    const itemsJson = JSON.stringify(itemsArray);

    if (isEdit) {
      updateMut.mutate(
        {
          id: initial!.id,
          body: { name: name.trim(), cutoff, items: itemsJson, upgrade_allowed: upgradeAllowed, format_items: formatItems },
        },
        {
          onSuccess: () => { toast.success("Updated"); onClose(); },
          onError: (e) => toast.error(e.message),
        },
      );
    } else {
      createMut.mutate(
        { name: name.trim(), cutoff, items: itemsJson, upgrade_allowed: upgradeAllowed, format_items: formatItems },
        {
          onSuccess: () => { toast.success("Created"); onClose(); },
          onError: (e) => toast.error(e.message),
        },
      );
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-2xl max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit" : "Add"} Quality Profile</DialogTitle>
          <DialogDescription>
            Configure quality tiers and custom format scoring.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 flex-1 overflow-hidden">
          {/* Name */}
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

          {/* Upgrade toggle */}
          <div className="flex items-center gap-2">
            <Checkbox
              id="qp-upgrade"
              checked={upgradeAllowed}
              onCheckedChange={(v) => setUpgradeAllowed(!!v)}
            />
            <Label htmlFor="qp-upgrade">Allow upgrades</Label>
          </div>

          {/* Grouped quality selector */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-sm text-zinc-400">
                Qualities{" "}
                <span className="text-zinc-600">({selectedCount}/{definitions.length})</span>
              </Label>
              <Button
                variant="ghost"
                size="sm"
                className="text-xs text-teal-500 hover:text-teal-400 h-6 px-2"
                onClick={toggleAll}
              >
                {definitions.every((d) => checkedItems[d.id]) ? "Deselect All" : "Select All"}
              </Button>
            </div>

            <div className="overflow-y-auto max-h-[30vh] rounded-md border border-zinc-800">
              {groups.map((group) => {
                const groupAllChecked = group.defs.every((d) => checkedItems[d.id]);
                const groupSomeChecked = group.defs.some((d) => checkedItems[d.id]) && !groupAllChecked;

                return (
                  <div key={group.source}>
                    <label
                      className="flex items-center gap-3 px-3 py-2 bg-zinc-800/70 cursor-pointer sticky top-0 z-10 border-b border-zinc-700/50"
                      onClick={(e) => { e.preventDefault(); toggleGroup(group.defs); }}
                    >
                      <Checkbox
                        checked={groupAllChecked}
                        className={groupSomeChecked ? "data-[state=unchecked]:bg-teal-900/40 data-[state=unchecked]:border-teal-600" : ""}
                        onCheckedChange={() => toggleGroup(group.defs)}
                      />
                      <span className="text-sm font-medium text-zinc-200">{group.label}</span>
                      <span className="text-xs text-zinc-500 ml-auto">
                        {group.defs.filter((d) => checkedItems[d.id]).length}/{group.defs.length}
                      </span>
                    </label>

                    {group.defs.map((def) => (
                      <label
                        key={def.id}
                        className="flex items-center gap-3 px-3 py-2 pl-8 hover:bg-zinc-800/50 cursor-pointer border-b border-zinc-800/50 last:border-0"
                      >
                        <Checkbox
                          checked={checkedItems[def.id] ?? false}
                          onCheckedChange={() => toggleItem(def.id)}
                        />
                        <div className="flex-1 min-w-0 flex items-center gap-2">
                          <span className="text-sm text-zinc-200">{def.title || def.name}</span>
                          {def.modifier && (
                            <Badge variant="outline" className="text-[10px] border-amber-700/50 text-amber-500 px-1 py-0">
                              {def.modifier}
                            </Badge>
                          )}
                        </div>
                        <span className="text-xs text-zinc-500 tabular-nums">{def.resolution}</span>
                      </label>
                    ))}
                  </div>
                );
              })}
              {definitions.length === 0 && (
                <div className="px-3 py-6 text-center text-sm text-zinc-500">
                  No quality definitions available
                </div>
              )}
            </div>
          </div>

          {/* Cutoff selector */}
          {selectedCount > 0 && (
            <div className="space-y-2">
              <Label className="text-sm text-zinc-400">Cutoff</Label>
              <p className="text-xs text-zinc-600 -mt-1">
                Once this quality is reached, no further upgrades will be downloaded.
              </p>
              <select
                value={cutoff}
                onChange={(e) => setCutoff(e.target.value)}
                className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-200"
              >
                <option value="">None (always upgrade)</option>
                {definitions
                  .filter((d) => checkedItems[d.id])
                  .map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.title || d.name}
                    </option>
                  ))}
              </select>
            </div>
          )}

          {/* Custom format scores */}
          {customFormats.length > 0 && (
            <div className="space-y-2">
              <Label className="text-sm text-zinc-400">Custom Format Scores</Label>
              <div className="rounded-md border border-zinc-800 divide-y divide-zinc-800 max-h-40 overflow-y-auto">
                {customFormats.map((cf) => (
                  <div key={cf.id} className="flex items-center justify-between px-3 py-2">
                    <span className="text-sm text-zinc-300">{cf.name}</span>
                    <Input
                      type="number"
                      className="w-24 h-8 text-sm bg-zinc-900 border-zinc-700"
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
          <Button
            onClick={handleSave}
            disabled={!name || createMut.isPending || updateMut.isPending}
          >
            {(createMut.isPending || updateMut.isPending) && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
            {isEdit ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
