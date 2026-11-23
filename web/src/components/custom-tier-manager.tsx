import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "sonner";
import {
  IMPLEMENTATIONS,
  useCustomTiers,
  useDeleteCustomTier,
  useSaveCustomTier,
  type CustomTier,
  type Specification,
  type SpecImplementation,
} from "@/lib/custom-formats-api";

const implementations = IMPLEMENTATIONS.filter(
  (i) => i.value !== "SizeSpec" && i.value !== "IndexerFlagSpec",
);
const emptySpec = (): Specification => ({
  name: "",
  implementation: "ResolutionSpec",
  required: false,
  negate: false,
  fields: { value: "" },
});

export function CustomTierManager({ onClose }: { onClose: () => void }) {
  const { data: tiers = [], isLoading, isError } = useCustomTiers();
  const save = useSaveCustomTier();
  const remove = useDeleteCustomTier();
  const [editing, setEditing] = React.useState<CustomTier | null>(null);
  const patchSpec = (index: number, patch: Partial<Specification>) =>
    setEditing(
      (tier) =>
        tier && {
          ...tier,
          specifications: tier.specifications.map((spec, i) =>
            i === index ? { ...spec, ...patch } : spec,
          ),
        },
    );
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Custom quality tiers</DialogTitle>
          <DialogDescription>
            Tiers control allowance and upgrade order. Add one to a quality
            profile after saving it. Formats and scores remain separate.
          </DialogDescription>
        </DialogHeader>
        {isLoading && <p>Loading tiers…</p>}
        {isError && <p role="alert">Could not load custom tiers.</p>}
        {!editing && (
          <>
            {tiers.map((tier) => (
              <div
                key={tier.id}
                className="flex items-center gap-2 rounded-md border p-3"
              >
                <span className="flex-1">{tier.name}</span>
                <Button
                  variant="outline"
                  onClick={() => setEditing(structuredClone(tier))}
                >
                  Edit {tier.name}
                </Button>
                <Button
                  variant="outline"
                  disabled={remove.isPending}
                  onClick={() => {
                    if (window.confirm(`Delete "${tier.name}"?`))
                      remove.mutate(tier.id, {
                        onError: (e) => toast.error(e.message),
                      });
                  }}
                >
                  Delete {tier.name}
                </Button>
              </div>
            ))}
            <Button
              onClick={() =>
                setEditing({ id: "", name: "", specifications: [emptySpec()] })
              }
            >
              Add custom tier
            </Button>
          </>
        )}
        {editing && (
          <div className="space-y-4">
            <div className="space-y-1">
              <Label htmlFor="tier-name">Tier name</Label>
              <Input
                id="tier-name"
                value={editing.name}
                onChange={(e) =>
                  setEditing({ ...editing, name: e.target.value })
                }
              />
            </div>
            <p className="text-sm text-muted-foreground">
              Rules of different types all match; rules of the same type match
              any unless marked required. Use Source “unknown” for an
              unspecified source.
            </p>
            {editing.specifications.map((spec, index) => (
              <fieldset key={index} className="space-y-2 rounded-md border p-3">
                <legend>Rule {index + 1}</legend>
                <Label htmlFor={`tier-type-${index}`}>Rule type</Label>
                <select
                  id={`tier-type-${index}`}
                  className="w-full rounded-md border bg-card p-2"
                  value={spec.implementation}
                  onChange={(e) =>
                    patchSpec(index, {
                      implementation: e.target.value as SpecImplementation,
                    })
                  }
                >
                  {implementations.map((i) => (
                    <option key={i.value} value={i.value}>
                      {i.label}
                    </option>
                  ))}
                </select>
                <Label htmlFor={`tier-value-${index}`}>Value</Label>
                <Input
                  id={`tier-value-${index}`}
                  value={String(spec.fields.value ?? "")}
                  onChange={(e) =>
                    patchSpec(index, { fields: { value: e.target.value } })
                  }
                />
                <div className="flex items-center gap-2">
                  <Checkbox
                    id={`tier-negate-${index}`}
                    checked={spec.negate}
                    onCheckedChange={(v) => patchSpec(index, { negate: !!v })}
                  />
                  <Label htmlFor={`tier-negate-${index}`}>Negate</Label>
                  <Checkbox
                    id={`tier-required-${index}`}
                    checked={spec.required}
                    onCheckedChange={(v) => patchSpec(index, { required: !!v })}
                  />
                  <Label htmlFor={`tier-required-${index}`}>Required</Label>
                </div>
                <Button
                  variant="outline"
                  onClick={() =>
                    setEditing({
                      ...editing,
                      specifications: editing.specifications.filter(
                        (_, i) => i !== index,
                      ),
                    })
                  }
                >
                  Remove rule {index + 1}
                </Button>
              </fieldset>
            ))}
            <Button
              variant="outline"
              onClick={() =>
                setEditing({
                  ...editing,
                  specifications: [...editing.specifications, emptySpec()],
                })
              }
            >
              Add rule
            </Button>
            <DialogFooter>
              <Button variant="outline" onClick={() => setEditing(null)}>
                Cancel
              </Button>
              <Button
                disabled={
                  save.isPending ||
                  !editing.name.trim() ||
                  !editing.specifications.length
                }
                onClick={() =>
                  save.mutate(editing, {
                    onSuccess: () => {
                      toast.success("Tier saved");
                      setEditing(null);
                    },
                    onError: (e) => toast.error(e.message),
                  })
                }
              >
                Save tier
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
