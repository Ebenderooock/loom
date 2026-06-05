import * as React from "react";
import {
  MoreHorizontal,
  Plus,
  Trash2,
  FlaskConical,
  Download,
} from "lucide-react";
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  useCustomFormats,
  useCreateCustomFormat,
  useUpdateCustomFormat,
  useDeleteCustomFormat,
  useTestCustomFormat,
  IMPLEMENTATIONS,
  type CustomFormat,
  type Specification,
  type SpecImplementation,
  type TestResult,
} from "@/lib/custom-formats-api";

type DialogState =
  | { kind: "closed" }
  | { kind: "edit"; cf: CustomFormat }
  | { kind: "create" }
  | { kind: "test" }
  | { kind: "presets" };

const EMPTY_SPEC: Specification = {
  name: "",
  implementation: "ReleaseTitleSpec",
  negate: false,
  required: false,
  fields: { value: "" },
};

export function CustomFormatsPage() {
  useSetPageHeader("Custom Formats", "Rule-based release matching");
  const { data: formats, isLoading } = useCustomFormats();
  const deleteMut = useDeleteCustomFormat();
  const [dialog, setDialog] = React.useState<DialogState>({ kind: "closed" });

  const handleDelete = (id: string) => {
    deleteMut.mutate(id, {
      onSuccess: () => toast.success("Deleted"),
      onError: (e) => toast.error(e.message),
    });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Custom Formats</h1>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDialog({ kind: "test" })}
          >
            <FlaskConical className="mr-2 h-4 w-4" /> Test
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDialog({ kind: "presets" })}
          >
            <Download className="mr-2 h-4 w-4" /> Import Preset
          </Button>
          <Button size="sm" onClick={() => setDialog({ kind: "create" })}>
            <Plus className="mr-2 h-4 w-4" /> Add
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="h-16 w-full rounded-lg" />
          ))}
        </div>
      ) : !formats || formats.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
          <p className="text-lg font-medium">No custom formats</p>
          <p className="mt-1 text-sm">
            Create one or import a preset to get started.
          </p>
        </div>
      ) : (
        <div className="divide-y rounded-lg border">
          {formats.map((cf) => (
            <div
              key={cf.id}
              className="flex items-center justify-between px-4 py-3"
            >
              <div className="flex min-w-0 items-center gap-3">
                <span className="truncate font-medium">{cf.name}</span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {cf.specifications?.length ?? 0} spec
                  {(cf.specifications?.length ?? 0) !== 1 ? "s" : ""}
                </span>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" aria-label="Actions">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={() => setDialog({ kind: "edit", cf })}
                  >
                    Edit
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive"
                    onClick={() => handleDelete(cf.id)}
                  >
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
        </div>
      )}

      {(dialog.kind === "create" || dialog.kind === "edit") && (
        <EditDialog
          initial={dialog.kind === "edit" ? dialog.cf : undefined}
          onClose={() => setDialog({ kind: "closed" })}
        />
      )}
      {dialog.kind === "test" && (
        <TestDialog onClose={() => setDialog({ kind: "closed" })} />
      )}
      {dialog.kind === "presets" && (
        <PresetsDialog onClose={() => setDialog({ kind: "closed" })} />
      )}
    </div>
  );
}

// ---------- Edit/Create Dialog ----------

function EditDialog({
  initial,
  onClose,
}: {
  initial?: CustomFormat;
  onClose: () => void;
}) {
  const createMut = useCreateCustomFormat();
  const updateMut = useUpdateCustomFormat();
  const isEdit = !!initial;

  const [id, setId] = React.useState(initial?.id ?? "");
  const [name, setName] = React.useState(initial?.name ?? "");
  const [includeRename, setIncludeRename] = React.useState(
    initial?.include_when_renaming ?? false,
  );
  const [specs, setSpecs] = React.useState<Specification[]>(
    initial?.specifications ?? [{ ...EMPTY_SPEC }],
  );

  const addSpec = () => setSpecs((s) => [...s, { ...EMPTY_SPEC }]);
  const removeSpec = (i: number) =>
    setSpecs((s) => s.filter((_, idx) => idx !== i));
  const updateSpec = (i: number, patch: Partial<Specification>) =>
    setSpecs((s) => s.map((sp, idx) => (idx === i ? { ...sp, ...patch } : sp)));

  const handleSave = () => {
    const body: CustomFormat = {
      id: isEdit ? initial!.id : id,
      name,
      include_when_renaming: includeRename,
      specifications: specs,
    };
    if (isEdit) {
      updateMut.mutate(
        { id: initial!.id, body },
        {
          onSuccess: () => {
            toast.success("Updated");
            onClose();
          },
          onError: (e) => toast.error(e.message),
        },
      );
    } else {
      createMut.mutate(body, {
        onSuccess: () => {
          toast.success("Created");
          onClose();
        },
        onError: (e) => toast.error(e.message),
      });
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit" : "Add"} Custom Format</DialogTitle>
          <DialogDescription>Define matching rules.</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          {!isEdit && (
            <div className="grid gap-1.5">
              <Label htmlFor="cf-id">ID (slug)</Label>
              <Input
                id="cf-id"
                value={id}
                onChange={(e) => setId(e.target.value)}
                placeholder="prefer-hevc"
              />
            </div>
          )}
          <div className="grid gap-1.5">
            <Label htmlFor="cf-name">Name</Label>
            <Input
              id="cf-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Prefer x265"
            />
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="cf-rename"
              checked={includeRename}
              onCheckedChange={(v) => setIncludeRename(!!v)}
            />
            <Label htmlFor="cf-rename">Include when renaming</Label>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>Specifications</Label>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={addSpec}
              >
                <Plus className="mr-1 h-3 w-3" /> Add Spec
              </Button>
            </div>
            {specs.map((spec, i) => (
              <div key={i} className="space-y-2 rounded-md border p-3">
                <div className="flex items-center gap-2">
                  <Input
                    className="flex-1"
                    placeholder="Spec name"
                    value={spec.name}
                    onChange={(e) => updateSpec(i, { name: e.target.value })}
                  />
                  <Select
                    value={spec.implementation}
                    onValueChange={(v) =>
                      updateSpec(i, {
                        implementation: v as SpecImplementation,
                        fields: { value: "" },
                      })
                    }
                  >
                    <SelectTrigger className="w-48">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {IMPLEMENTATIONS.map((im) => (
                        <SelectItem key={im.value} value={im.value}>
                          {im.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => removeSpec(i)}
                    aria-label="Remove spec"
                  >
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </div>
                <div className="flex items-center gap-4">
                  <Input
                    placeholder={
                      IMPLEMENTATIONS.find(
                        (im) => im.value === spec.implementation,
                      )?.placeholder ?? "value"
                    }
                    value={String(spec.fields?.value ?? "")}
                    onChange={(e) =>
                      updateSpec(i, {
                        fields: { ...spec.fields, value: e.target.value },
                      })
                    }
                    className="flex-1"
                  />
                  {spec.implementation === "SizeSpec" && (
                    <Input
                      placeholder="max GB"
                      value={String(spec.fields?.max ?? "")}
                      onChange={(e) =>
                        updateSpec(i, {
                          fields: { ...spec.fields, max: e.target.value },
                        })
                      }
                      className="w-28"
                    />
                  )}
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id={`negate-${i}`}
                      checked={spec.negate}
                      onCheckedChange={(v) => updateSpec(i, { negate: !!v })}
                    />
                    <Label htmlFor={`negate-${i}`} className="text-xs">
                      Negate
                    </Label>
                  </div>
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id={`required-${i}`}
                      checked={spec.required}
                      onCheckedChange={(v) => updateSpec(i, { required: !!v })}
                    />
                    <Label htmlFor={`required-${i}`} className="text-xs">
                      Required
                    </Label>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={createMut.isPending || updateMut.isPending}
          >
            {isEdit ? "Save" : "Create"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Test Dialog ----------

function TestDialog({ onClose }: { onClose: () => void }) {
  const testMut = useTestCustomFormat();
  const [title, setTitle] = React.useState("");
  const [result, setResult] = React.useState<TestResult | null>(null);

  const handleTest = () => {
    testMut.mutate(title, {
      onSuccess: (r) => setResult(r),
      onError: (e) => toast.error(e.message),
    });
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>Test Release Title</DialogTitle>
          <DialogDescription>
            Paste a release title to see which custom formats match.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="flex gap-2">
            <Input
              placeholder="Movie.Name.2024.2160p.BluRay.x265.DTS-HD.MA-GROUP"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleTest()}
              className="flex-1"
            />
            <Button onClick={handleTest} disabled={testMut.isPending || !title}>
              Test
            </Button>
          </div>
          {result && (
            <div className="space-y-3">
              <div className="space-y-1 rounded-md border p-3 text-sm">
                <p>
                  <strong>Resolution:</strong>{" "}
                  {result.release.resolution || "—"}
                </p>
                <p>
                  <strong>Source:</strong> {result.release.source || "—"}
                </p>
                <p>
                  <strong>Codec:</strong> {result.release.codec || "—"}
                </p>
                <p>
                  <strong>Audio:</strong> {result.release.audio || "—"}
                </p>
                <p>
                  <strong>Group:</strong> {result.release.group || "—"}
                </p>
                <p>
                  <strong>Languages:</strong>{" "}
                  {result.release.languages?.join(", ") || "—"}
                </p>
              </div>
              {result.matches.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No custom formats matched.
                </p>
              ) : (
                <div className="space-y-1">
                  <p className="text-sm font-medium">Matches</p>
                  {result.matches.map((m) => (
                    <div
                      key={m.custom_format_id}
                      className="flex items-center justify-between rounded-md border px-3 py-2 text-sm"
                    >
                      <span>{m.custom_format_name}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---------- Presets Dialog ----------

const PRESETS: CustomFormat[] = [
  {
    id: "prefer-hevc",
    name: "Prefer x265/HEVC",
    include_when_renaming: false,
    specifications: [
      {
        name: "x265/HEVC",
        implementation: "CodecSpec",
        negate: false,
        required: false,
        fields: { value: "x265" },
      },
    ],
  },
  {
    id: "prefer-atmos-truehd",
    name: "Prefer Atmos/TrueHD",
    include_when_renaming: false,
    specifications: [
      {
        name: "Atmos",
        implementation: "AudioSpec",
        negate: false,
        required: false,
        fields: { value: "Atmos" },
      },
      {
        name: "TrueHD",
        implementation: "AudioSpec",
        negate: false,
        required: false,
        fields: { value: "TrueHD" },
      },
    ],
  },
  {
    id: "avoid-lq-groups",
    name: "Avoid LQ Groups",
    include_when_renaming: false,
    specifications: [
      {
        name: "LQ Group",
        implementation: "ReleaseTitleSpec",
        negate: false,
        required: false,
        fields: { value: "(?i)\\b(YIFY|YTS|EVO|SPARKS|RARBG|aXXo)\\b" },
      },
    ],
  },
  {
    id: "prefer-bluray",
    name: "Prefer BluRay",
    include_when_renaming: false,
    specifications: [
      {
        name: "BluRay",
        implementation: "SourceSpec",
        negate: false,
        required: false,
        fields: { value: "BluRay" },
      },
    ],
  },
  {
    id: "avoid-cam-ts",
    name: "Avoid CAM/TS",
    include_when_renaming: false,
    specifications: [
      {
        name: "CAM",
        implementation: "SourceSpec",
        negate: false,
        required: false,
        fields: { value: "CAM" },
      },
      {
        name: "TS",
        implementation: "SourceSpec",
        negate: false,
        required: false,
        fields: { value: "TS" },
      },
    ],
  },
];

function PresetsDialog({ onClose }: { onClose: () => void }) {
  const createMut = useCreateCustomFormat();

  const handleImport = (preset: CustomFormat) => {
    createMut.mutate(preset, {
      onSuccess: () => toast.success(`Imported "${preset.name}"`),
      onError: (e) => toast.error(e.message),
    });
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Import Preset</DialogTitle>
          <DialogDescription>
            One-click import common custom format profiles.
          </DialogDescription>
        </DialogHeader>
        <div className="divide-y rounded-md border">
          {PRESETS.map((p) => (
            <div
              key={p.id}
              className="flex items-center justify-between px-4 py-3"
            >
              <div>
                <p className="text-sm font-medium">{p.name}</p>
                <p className="text-xs text-muted-foreground">
                  {p.specifications.length} spec
                  {p.specifications.length !== 1 ? "s" : ""}
                </p>
              </div>
              <Button
                size="sm"
                variant="outline"
                onClick={() => handleImport(p)}
                disabled={createMut.isPending}
              >
                Import
              </Button>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
