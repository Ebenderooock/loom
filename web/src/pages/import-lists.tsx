import * as React from "react";
import {
  useImportLists,
  useCreateImportList,
  useDeleteImportList,
  useUpdateImportList,
  useSyncImportList,
  useImportListDetail,
  useExclusions,
  useCreateExclusion,
  useDeleteExclusion,
  LIST_TYPES,
  MONITOR_TYPES,
  SYNC_INTERVALS,
  type ImportList,
  type CreateImportListRequest,
  type ListType,
  type ImportListExclusion,
} from "@/lib/import-lists-api";
import { usePageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  ListPlus,
  RefreshCw,
  Trash2,
  ChevronDown,
  ChevronRight,
  Plus,
  X,
  Ban,
} from "lucide-react";

export function ImportListsPage() {
  const { setHeader } = usePageHeader();
  React.useEffect(() => setHeader({ title: "Import Lists" }), [setHeader]);

  return (
    <div className="space-y-6 p-6">
      <Tabs defaultValue="lists">
        <TabsList>
          <TabsTrigger value="lists">Lists</TabsTrigger>
          <TabsTrigger value="exclusions">Exclusions</TabsTrigger>
        </TabsList>

        <TabsContent value="lists" className="mt-4">
          <ListsTab />
        </TabsContent>
        <TabsContent value="exclusions" className="mt-4">
          <ExclusionsTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}

// ---- Lists Tab ----

function ListsTab() {
  const { data: lists, isLoading } = useImportLists();
  const [showForm, setShowForm] = React.useState(false);
  const [expandedId, setExpandedId] = React.useState<string | null>(null);

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading…</p>;
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setShowForm((v) => !v)}>
          <ListPlus className="mr-2 h-4 w-4" />
          Add List
        </Button>
      </div>

      {showForm && <AddListForm onDone={() => setShowForm(false)} />}

      {(!lists || lists.length === 0) && !showForm && (
        <p className="text-sm text-muted-foreground">
          No import lists configured. Click "Add List" to get started.
        </p>
      )}

      <div className="divide-y divide-border rounded-md border border-border">
        {lists?.map((l) => (
          <ListRow
            key={l.id}
            list={l}
            expanded={expandedId === l.id}
            onToggle={() =>
              setExpandedId(expandedId === l.id ? null : l.id)
            }
          />
        ))}
      </div>
    </div>
  );
}

function ListRow({
  list,
  expanded,
  onToggle,
}: {
  list: ImportList;
  expanded: boolean;
  onToggle: () => void;
}) {
  const deleteMut = useDeleteImportList();
  const syncMut = useSyncImportList();
  const updateMut = useUpdateImportList();

  const typeMeta = LIST_TYPES.find((t) => t.value === list.list_type);

  return (
    <div>
      <div className="flex items-center gap-3 px-4 py-3">
        <button onClick={onToggle} className="text-muted-foreground">
          {expanded ? (
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </button>

        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate">{list.name}</p>
          <p className="text-xs text-muted-foreground">
            {typeMeta?.label ?? list.list_type} · {list.media_type} ·{" "}
            {list.item_count ?? 0} items
          </p>
        </div>

        <Badge variant={list.enabled ? "default" : "secondary"}>
          {list.enabled ? "Enabled" : "Disabled"}
        </Badge>

        {list.last_sync && (
          <span className="text-xs text-muted-foreground whitespace-nowrap">
            Last sync: {new Date(list.last_sync).toLocaleString()}
          </span>
        )}

        <Button
          size="sm"
          variant="ghost"
          disabled={syncMut.isPending}
          onClick={() => syncMut.mutate(list.id)}
          title="Sync now"
        >
          <RefreshCw
            className={`h-4 w-4 ${syncMut.isPending ? "animate-spin" : ""}`}
          />
        </Button>

        <Button
          size="sm"
          variant="ghost"
          onClick={() =>
            updateMut.mutate({
              id: list.id,
              body: { enabled: !list.enabled },
            })
          }
          title={list.enabled ? "Disable" : "Enable"}
        >
          {list.enabled ? "Disable" : "Enable"}
        </Button>

        <Button
          size="sm"
          variant="ghost"
          className="text-destructive"
          onClick={() => {
            if (confirm(`Delete "${list.name}"?`)) deleteMut.mutate(list.id);
          }}
          title="Delete"
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>

      {expanded && <ListItemsPanel listId={list.id} />}
    </div>
  );
}

function ListItemsPanel({ listId }: { listId: string }) {
  const { data, isLoading } = useImportListDetail(listId);

  if (isLoading) {
    return (
      <div className="px-8 py-2 text-sm text-muted-foreground">Loading…</div>
    );
  }
  if (!data?.items?.length) {
    return (
      <div className="px-8 py-2 text-sm text-muted-foreground">
        No items fetched yet. Try syncing the list.
      </div>
    );
  }

  return (
    <div className="px-8 pb-3">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs text-muted-foreground border-b border-border">
            <th className="py-1 pr-4">Title</th>
            <th className="py-1 pr-4">Year</th>
            <th className="py-1 pr-4">IMDb</th>
            <th className="py-1 pr-4">Status</th>
            <th className="py-1">Last Seen</th>
          </tr>
        </thead>
        <tbody>
          {data.items.map((item) => (
            <tr key={item.id} className="border-b border-border/50 last:border-0">
              <td className="py-1 pr-4">{item.title}</td>
              <td className="py-1 pr-4">{item.year ?? "—"}</td>
              <td className="py-1 pr-4 font-mono text-xs">
                {item.imdb_id || "—"}
              </td>
              <td className="py-1 pr-4">
                <StatusBadge status={item.status} />
              </td>
              <td className="py-1 text-xs text-muted-foreground">
                {new Date(item.last_seen).toLocaleString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const variants: Record<string, string> = {
    pending: "bg-yellow-500/10 text-yellow-500",
    added: "bg-green-500/10 text-green-500",
    excluded: "bg-neutral-500/10 text-neutral-400",
    failed: "bg-red-500/10 text-red-500",
  };
  return (
    <span
      className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${variants[status] ?? ""}`}
    >
      {status}
    </span>
  );
}

// ---- Add List Form ----

function AddListForm({ onDone }: { onDone: () => void }) {
  const createMut = useCreateImportList();
  const [form, setForm] = React.useState<CreateImportListRequest>({
    name: "",
    list_type: "trakt_list",
    enabled: true,
    media_type: "movie",
    monitor_type: "all",
    sync_interval_minutes: 360,
    search_on_add: true,
    url: "",
    api_key: "",
    access_token: "",
  });

  const typeMeta = LIST_TYPES.find((t) => t.value === form.list_type);
  const fields = typeMeta?.fields ?? [];

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMut.mutate(form, { onSuccess: onDone });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Add Import List</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="add-list-name">Name</Label>
              <Input
                id="add-list-name"
                required
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="add-list-type">List Type</Label>
              <Select
                value={form.list_type}
                onValueChange={(val) =>
                  setForm({
                    ...form,
                    list_type: val as ListType,
                    media_type:
                      LIST_TYPES.find((t) => t.value === val)?.mediaType ??
                      "movie",
                  })
                }
              >
                <SelectTrigger id="add-list-type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LIST_TYPES.map((t) => (
                    <SelectItem key={t.value} value={t.value}>
                      {t.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="add-media-type">Media Type</Label>
              <Select
                value={form.media_type}
                onValueChange={(val) =>
                  setForm({
                    ...form,
                    media_type: val as "movie" | "series",
                  })
                }
              >
                <SelectTrigger id="add-media-type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="movie">Movie</SelectItem>
                  <SelectItem value="series">Series</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="add-monitor">Monitor</Label>
              <Select
                value={form.monitor_type}
                onValueChange={(val) =>
                  setForm({
                    ...form,
                    monitor_type: val as
                      | "all"
                      | "future"
                      | "missing"
                      | "none",
                  })
                }
              >
                <SelectTrigger id="add-monitor">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MONITOR_TYPES.map((m) => (
                    <SelectItem key={m.value} value={m.value}>
                      {m.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="add-sync-interval">Sync Interval</Label>
              <Select
                value={String(form.sync_interval_minutes)}
                onValueChange={(val) =>
                  setForm({
                    ...form,
                    sync_interval_minutes: Number(val),
                  })
                }
              >
                <SelectTrigger id="add-sync-interval">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SYNC_INTERVALS.map((s) => (
                    <SelectItem key={s.value} value={String(s.value)}>
                      {s.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {fields.includes("url") && (
              <div className="space-y-2">
                <Label htmlFor="add-url">URL</Label>
                <Input
                  id="add-url"
                  value={form.url ?? ""}
                  onChange={(e) => setForm({ ...form, url: e.target.value })}
                  placeholder="https://..."
                />
              </div>
            )}

            {fields.includes("api_key") && (
              <div className="space-y-2">
                <Label htmlFor="add-api-key">API Key</Label>
                <Input
                  id="add-api-key"
                  value={form.api_key ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, api_key: e.target.value })
                  }
                />
              </div>
            )}

            {fields.includes("access_token") && (
              <div className="space-y-2">
                <Label htmlFor="add-access-token">Access Token</Label>
                <Input
                  id="add-access-token"
                  value={form.access_token ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, access_token: e.target.value })
                  }
                />
              </div>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Switch
              id="add-search-on-add"
              checked={form.search_on_add}
              onCheckedChange={(checked) =>
                setForm({ ...form, search_on_add: checked })
              }
            />
            <Label htmlFor="add-search-on-add">Search on add</Label>
          </div>

          <div className="flex gap-2">
            <Button type="submit" size="sm" disabled={createMut.isPending}>
              <Plus className="mr-1 h-4 w-4" />
              {createMut.isPending ? "Adding…" : "Add List"}
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={onDone}>
              Cancel
            </Button>
          </div>

          {createMut.isError && (
            <p className="text-sm text-destructive">
              {(createMut.error as Error).message}
            </p>
          )}
        </form>
      </CardContent>
    </Card>
  );
}

// ---- Exclusions Tab ----

function ExclusionsTab() {
  const { data: exclusions, isLoading } = useExclusions();
  const createMut = useCreateExclusion();
  const deleteMut = useDeleteExclusion();
  const [title, setTitle] = React.useState("");
  const [imdbId, setImdbId] = React.useState("");

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    createMut.mutate(
      { title: title.trim(), imdb_id: imdbId.trim() || undefined },
      {
        onSuccess: () => {
          setTitle("");
          setImdbId("");
        },
      },
    );
  };

  return (
    <div className="space-y-4">
      <form onSubmit={handleAdd} className="flex items-end gap-3">
        <div className="space-y-2">
          <Label htmlFor="excl-title">Title</Label>
          <Input
            id="excl-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Movie or series title"
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="excl-imdb">IMDb ID (optional)</Label>
          <Input
            id="excl-imdb"
            value={imdbId}
            onChange={(e) => setImdbId(e.target.value)}
            placeholder="tt1234567"
          />
        </div>
        <Button type="submit" size="sm" disabled={createMut.isPending}>
          <Ban className="mr-1 h-4 w-4" />
          Add Exclusion
        </Button>
      </form>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : !exclusions?.length ? (
        <p className="text-sm text-muted-foreground">
          No exclusions. Items you exclude won't be re-added by any import list.
        </p>
      ) : (
        <div className="divide-y divide-border rounded-md border border-border">
          {exclusions.map((ex: ImportListExclusion) => (
            <div key={ex.id} className="flex items-center gap-3 px-4 py-2">
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium">{ex.title}</p>
                <p className="text-xs text-muted-foreground">
                  {[ex.imdb_id, ex.tmdb_id, ex.tvdb_id]
                    .filter(Boolean)
                    .join(" · ") || "No external IDs"}
                  {ex.year ? ` · ${ex.year}` : ""}
                </p>
              </div>
              <Button
                size="sm"
                variant="ghost"
                className="text-destructive"
                onClick={() => deleteMut.mutate(ex.id)}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}


