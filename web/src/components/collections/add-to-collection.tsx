import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FolderPlus, Layers, Loader2, Plus } from "lucide-react";
import {
  addCollectionMembers,
  collectionKeys,
  collectionRequest,
  hasTitle,
  type LoomCollection,
} from "@/lib/collections-api";
import { Button, type ButtonProps } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export interface CollectionTarget {
  media_type: "movie" | "series";
  media_id: string;
  title: string;
  tmdb?: string;
}

/** One-click membership for real library titles (admin only). */
export function AddToCollectionDialog({
  targets,
  open,
  onOpenChange,
}: {
  targets: CollectionTarget[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [added, setAdded] = useState<Set<string>>(new Set());
  const list = useQuery({
    queryKey: collectionKeys.all,
    queryFn: ({ signal }) =>
      collectionRequest<{ items: LoomCollection[] }>(
        "",
        undefined,
        "GET",
        signal,
      ),
    enabled: open,
  });
  const done = (c: LoomCollection) => {
    setAdded((prev) => new Set(prev).add(c.id));
    void qc.invalidateQueries({ queryKey: collectionKeys.all });
  };
  const items = targets.map((t) => ({
    media_type: t.media_type,
    media_id: t.media_id,
  }));
  const label =
    targets.length === 1 ? targets[0]!.title : `${targets.length} titles`;
  const add = useMutation({
    mutationFn: (c: LoomCollection) => addCollectionMembers(c, items),
    onSuccess: (saved, requested) => {
      setAdded((prev) => new Set(prev).add(requested.id));
      done(saved);
    },
  });
  const create = useMutation({
    mutationFn: async () => {
      const c = await collectionRequest<LoomCollection>("", {
        name: name.trim(),
        overview: "",
      });
      return addCollectionMembers(c, items);
    },
    onSuccess: (c) => {
      setName("");
      done(c);
    },
  });
  const error = add.error ?? create.error ?? list.error;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] min-w-0 max-w-md">
        <DialogHeader>
          <DialogTitle className="break-words">
            Add {label} to a collection
          </DialogTitle>
          <DialogDescription>
            Only membership changes. Monitoring, quality and files stay as they
            are.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <Input
            aria-label="New collection name"
            placeholder="New collection name"
            maxLength={300}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <Button
            type="submit"
            variant="outline"
            disabled={!name.trim() || create.isPending}
          >
            <FolderPlus className="mr-2 h-4 w-4" />
            Create
          </Button>
        </form>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error.message}
          </p>
        )}
        <ul
          aria-label="Collections"
          className="max-h-80 space-y-1 overflow-y-auto overflow-x-hidden"
        >
          {list.isPending && (
            <li role="status" className="text-sm text-muted-foreground">
              Loading collections…
            </li>
          )}
          {list.data?.items.length === 0 && (
            <li className="text-sm text-muted-foreground">
              No collections yet. Name one above.
            </li>
          )}
          {list.data?.items.map((c) => {
            const member =
              added.has(c.id) ||
              targets.every((t) =>
                hasTitle(c, t.media_type, {
                  media_id: t.media_id,
                  tmdb: t.tmdb,
                }),
              );
            const busy = add.isPending && add.variables?.id === c.id;
            return (
              <li
                key={c.id}
                className="flex min-w-0 items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/60"
              >
                <Layers
                  className="h-4 w-4 shrink-0 text-muted-foreground"
                  aria-hidden
                />
                <span className="min-w-0 flex-1 truncate text-sm">
                  {c.name}
                </span>
                <Button
                  size="sm"
                  variant={member ? "ghost" : "outline"}
                  disabled={member || add.isPending}
                  aria-label={member ? `In ${c.name}` : `Add to ${c.name}`}
                  onClick={() => add.mutate(c)}
                >
                  {busy ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : member ? (
                    <Check className="h-4 w-4" />
                  ) : (
                    <Plus className="h-4 w-4" />
                  )}
                  <span className="ml-1">{member ? "Added" : "Add"}</span>
                </Button>
              </li>
            );
          })}
        </ul>
      </DialogContent>
    </Dialog>
  );
}

/** Trigger button plus dialog, for detail drawers. */
export function AddToCollectionButton({
  target,
  iconOnly,
  ...props
}: { target: CollectionTarget; iconOnly?: boolean } & Omit<
  ButtonProps,
  "onClick"
>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {iconOnly ? (
        <Button
          size="icon"
          variant="ghost"
          className="h-8 w-8"
          title="Add to collection"
          aria-label="Add to collection"
          {...props}
          onClick={() => setOpen(true)}
        >
          <Layers className="h-3.5 w-3.5" />
        </Button>
      ) : (
        <Button
          variant="outline"
          size="sm"
          {...props}
          onClick={() => setOpen(true)}
        >
          <Layers className="mr-2 h-4 w-4" />
          Add to collection
        </Button>
      )}
      {open && (
        <AddToCollectionDialog targets={[target]} open onOpenChange={setOpen} />
      )}
    </>
  );
}
