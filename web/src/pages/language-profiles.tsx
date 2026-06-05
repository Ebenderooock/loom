import * as React from "react";
import { GripVertical, MoreHorizontal, Plus, Languages } from "lucide-react";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
  useLanguages,
  useLanguageProfiles,
  useCreateLanguageProfile,
  useUpdateLanguageProfile,
  useDeleteLanguageProfile,
  type LanguagePriority,
  type LanguageProfile,
} from "@/lib/language-profiles-api";

type DialogState =
  | { kind: "closed" }
  | { kind: "create" }
  | { kind: "edit"; profile: LanguageProfile };

export function LanguageProfilesPage() {
  useSetPageHeader(
    "Language Profiles",
    "Manage language preferences for media",
  );
  const { data: profiles, isLoading } = useLanguageProfiles();
  const deleteMutation = useDeleteLanguageProfile();
  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });

  const handleDelete = (id: string) => {
    deleteMutation.mutate(id, {
      onSuccess: () => toast.success("Profile deleted"),
      onError: (e) => toast.error(e.message),
    });
  };

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Language Profiles</h1>
          <p className="text-sm text-muted-foreground">
            Define language priorities for release matching
          </p>
        </div>
        <Button onClick={() => setDialog({ kind: "create" })}>
          <Plus className="mr-2 h-4 w-4" />
          Add Profile
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-20 w-full rounded-lg" />
          ))}
        </div>
      ) : !profiles?.length ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-center">
          <Languages className="mb-4 h-12 w-12 text-muted-foreground" />
          <h2 className="text-lg font-semibold">No language profiles yet</h2>
          <p className="mb-4 text-sm text-muted-foreground">
            Create a profile to set language priorities for release matching.
          </p>
          <Button onClick={() => setDialog({ kind: "create" })}>
            <Plus className="mr-2 h-4 w-4" />
            Create Profile
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {profiles.map((p) => (
            <div
              key={p.id}
              className="flex items-center justify-between rounded-lg border bg-card p-4"
            >
              <div>
                <div className="font-medium">{p.name}</div>
                <div className="mt-1 flex flex-wrap gap-1">
                  {p.languages
                    .filter((lp) => lp.allowed)
                    .sort((a, b) => a.priority - b.priority)
                    .map((lp) => (
                      <Badge key={lp.language.code} variant="secondary">
                        {lp.language.name}
                      </Badge>
                    ))}
                </div>
                <div className="mt-1 text-xs text-muted-foreground">
                  Cutoff: {p.cutoff_language.toUpperCase()} &middot;{" "}
                  {p.upgrade_allowed ? "Upgrades allowed" : "No upgrades"}
                </div>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={() => setDialog({ kind: "edit", profile: p })}
                  >
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => handleDelete(p.id)}
                  >
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
        </div>
      )}

      <ProfileDialog
        state={dialog}
        onClose={() => setDialog({ kind: "closed" })}
      />
    </div>
  );
}

// ---------- Dialog ----------

function ProfileDialog({
  state,
  onClose,
}: {
  state: DialogState;
  onClose: () => void;
}) {
  const { data: allLanguages } = useLanguages();
  const createMutation = useCreateLanguageProfile();
  const updateMutation = useUpdateLanguageProfile();

  const isEdit = state.kind === "edit";
  const existing = isEdit ? state.profile : null;

  const [name, setName] = React.useState("");
  const [cutoff, setCutoff] = React.useState("en");
  const [upgradeAllowed, setUpgradeAllowed] = React.useState(true);
  const [priorities, setPriorities] = React.useState<LanguagePriority[]>([]);
  const [dragIdx, setDragIdx] = React.useState<number | null>(null);

  React.useEffect(() => {
    if (state.kind === "closed") return;
    if (isEdit && existing) {
      setName(existing.name);
      setCutoff(existing.cutoff_language);
      setUpgradeAllowed(existing.upgrade_allowed);
      setPriorities(
        [...existing.languages].sort((a, b) => a.priority - b.priority),
      );
    } else {
      setName("");
      setCutoff("en");
      setUpgradeAllowed(true);
      // Default: all languages, English allowed & priority 1, rest not allowed
      if (allLanguages) {
        setPriorities(
          allLanguages.map((lang, i) => ({
            language: lang,
            allowed: lang.code === "en",
            priority: i + 1,
          })),
        );
      }
    }
  }, [state.kind, allLanguages, existing, isEdit]);

  const handleSave = () => {
    const numbered = priorities.map((p, i) => ({ ...p, priority: i + 1 }));
    if (isEdit && existing) {
      updateMutation.mutate(
        {
          id: existing.id,
          req: {
            name,
            languages: numbered,
            cutoff_language: cutoff,
            upgrade_allowed: upgradeAllowed,
          },
        },
        {
          onSuccess: () => {
            toast.success("Profile updated");
            onClose();
          },
          onError: (e) => toast.error(e.message),
        },
      );
    } else {
      createMutation.mutate(
        {
          name,
          languages: numbered,
          cutoff_language: cutoff,
          upgrade_allowed: upgradeAllowed,
        },
        {
          onSuccess: () => {
            toast.success("Profile created");
            onClose();
          },
          onError: (e) => toast.error(e.message),
        },
      );
    }
  };

  const toggleAllowed = (code: string) => {
    setPriorities((prev) =>
      prev.map((p) =>
        p.language.code === code ? { ...p, allowed: !p.allowed } : p,
      ),
    );
  };

  const handleDragStart = (idx: number) => setDragIdx(idx);
  const handleDragOver = (e: React.DragEvent, idx: number) => {
    e.preventDefault();
    if (dragIdx === null || dragIdx === idx) return;
    setPriorities((prev) => {
      const items = [...prev];
      const [moved] = items.splice(dragIdx, 1);
      items.splice(idx, 0, moved!);
      return items;
    });
    setDragIdx(idx);
  };
  const handleDragEnd = () => setDragIdx(null);

  const allowedLangs = priorities.filter((p) => p.allowed);

  return (
    <Dialog open={state.kind !== "closed"} onOpenChange={() => onClose()}>
      <DialogContent className="max-h-[85vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? "Edit Language Profile" : "New Language Profile"}
          </DialogTitle>
          <DialogDescription>
            Drag languages to set priority order. Check languages to allow.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="profile-name">Name</Label>
            <Input
              id="profile-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. French MULTi"
            />
          </div>

          <div className="space-y-2">
            <Label>Languages (drag to reorder priority)</Label>
            <div className="max-h-64 space-y-1 overflow-y-auto rounded-md border p-2">
              {priorities.map((lp, idx) => (
                <div
                  key={lp.language.code}
                  draggable
                  onDragStart={() => handleDragStart(idx)}
                  onDragOver={(e) => handleDragOver(e, idx)}
                  onDragEnd={handleDragEnd}
                  className={`flex items-center gap-2 rounded px-2 py-1.5 text-sm ${
                    dragIdx === idx ? "bg-accent" : "hover:bg-accent/50"
                  } cursor-grab active:cursor-grabbing`}
                >
                  <GripVertical className="h-4 w-4 text-muted-foreground" />
                  <Checkbox
                    checked={lp.allowed}
                    onCheckedChange={() => toggleAllowed(lp.language.code)}
                  />
                  <span className="flex-1">
                    {lp.language.name}
                    <span className="ml-1 text-xs text-muted-foreground">
                      ({lp.language.native_name})
                    </span>
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {lp.language.code.toUpperCase()}
                  </span>
                </div>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="cutoff">Cutoff Language</Label>
            <Select value={cutoff} onValueChange={setCutoff}>
              <SelectTrigger id="cutoff">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {allowedLangs.map((lp) => (
                  <SelectItem key={lp.language.code} value={lp.language.code}>
                    {lp.language.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Stop upgrading once this language is reached.
            </p>
          </div>

          <div className="flex items-center gap-2">
            <Switch
              id="upgrade"
              checked={upgradeAllowed}
              onCheckedChange={setUpgradeAllowed}
            />
            <Label htmlFor="upgrade">Allow language upgrades</Label>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button
              onClick={handleSave}
              disabled={
                !name || createMutation.isPending || updateMutation.isPending
              }
            >
              {isEdit ? "Save Changes" : "Create Profile"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
