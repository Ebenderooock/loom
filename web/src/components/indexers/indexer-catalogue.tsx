// IndexerCatalogue is a two-step "Add indexer" flow:
//   Step 1 — browse/search 545+ bundled Cardigann definitions
//   Step 2 — configure the selected definition (or Newznab/Torznab)

import * as React from "react";
import { ArrowLeft, Globe, Lock, Plus, Search, ShieldCheck } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  IndexerForm,
  type IndexerFormValues,
} from "@/components/indexers/indexer-form";
import { toCreatePayload } from "@/components/indexers/indexer-form-adapter";
import {
  ApiError,
  useCreateIndexer,
  useDefinitions,
  type IndexerDefinition,
  type Proxy,
} from "@/lib/indexers-api";

// ---- helpers -------------------------------------------------------

function typeBadge(type_?: string) {
  switch (type_) {
    case "public":
      return (
        <Badge className="border-transparent bg-green-600/15 text-green-700 dark:text-green-400">
          <Globe className="mr-1 h-3 w-3" />
          Public
        </Badge>
      );
    case "private":
      return (
        <Badge className="border-transparent bg-red-600/15 text-red-700 dark:text-red-400">
          <Lock className="mr-1 h-3 w-3" />
          Private
        </Badge>
      );
    case "semi-private":
      return (
        <Badge className="border-transparent bg-yellow-600/15 text-yellow-700 dark:text-yellow-400">
          <ShieldCheck className="mr-1 h-3 w-3" />
          Semi-Private
        </Badge>
      );
    default:
      return null;
  }
}

function errMessage(err: unknown, fallback: string): string {
  if (err instanceof ApiError)
    return `${fallback} (HTTP ${err.status}): ${err.message}`;
  if (err instanceof Error) return `${fallback}: ${err.message}`;
  return fallback;
}

// ---- types ---------------------------------------------------------

type Selection = IndexerDefinition | "newznab" | "torznab";

interface Props {
  proxies: Proxy[];
  onCreated: () => void;
  onCancel: () => void;
}

// ---- component -----------------------------------------------------

export function IndexerCatalogue({ proxies, onCreated, onCancel }: Props) {
  const [step, setStep] = React.useState<"pick" | "configure">("pick");
  const [selected, setSelected] = React.useState<Selection | null>(null);
  const [search, setSearch] = React.useState("");
  const [typeFilter, setTypeFilter] = React.useState("all");
  const [catFilter, setCatFilter] = React.useState("all");
  const defsQ = useDefinitions();
  const create = useCreateIndexer();
  const [topError, setTopError] = React.useState<string | undefined>();

  // Collect all unique top-level categories across definitions.
  const allCategories = React.useMemo(() => {
    const set = new Set<string>();
    for (const d of defsQ.data ?? []) {
      for (const c of d.categories ?? []) set.add(c);
    }
    return Array.from(set).sort();
  }, [defsQ.data]);

  const filtered = React.useMemo(() => {
    let items = defsQ.data ?? [];
    if (search) {
      const q = search.toLowerCase();
      items = items.filter(
        (d) =>
          d.name.toLowerCase().includes(q) || d.id.toLowerCase().includes(q),
      );
    }
    if (typeFilter !== "all") {
      items = items.filter((d) => d.type === typeFilter);
    }
    if (catFilter !== "all") {
      items = items.filter((d) => d.categories?.includes(catFilter));
    }
    return items;
  }, [defsQ.data, search, typeFilter, catFilter]);

  function pick(sel: Selection) {
    setSelected(sel);
    setStep("configure");
    setTopError(undefined);
  }

  function back() {
    setStep("pick");
    setSelected(null);
    setTopError(undefined);
  }

  // Newznab / Torznab submit — reuse existing adapter
  async function handleNewznabCreate(values: IndexerFormValues) {
    setTopError(undefined);
    try {
      await create.mutateAsync(toCreatePayload(values));
      toast.success(`Indexer "${values.name}" added.`);
      onCreated();
    } catch (err) {
      setTopError(errMessage(err, "Could not create indexer"));
    }
  }

  // Cardigann submit
  async function handleCardigannCreate(
    def: IndexerDefinition,
    formData: Record<string, string>,
    name: string,
    enabled: boolean,
    priority: number,
    proxyId: string,
  ) {
    setTopError(undefined);
    try {
      const config: Record<string, unknown> = { definition_id: def.id };
      for (const [k, v] of Object.entries(formData)) {
        if (v) config[k] = v;
      }
      await create.mutateAsync({
        kind: "cardigann",
        name: name.trim(),
        enabled,
        priority,
        config,
        proxy_id: proxyId || undefined,
      });
      toast.success(`Indexer "${name}" added.`);
      onCreated();
    } catch (err) {
      setTopError(errMessage(err, "Could not create indexer"));
    }
  }

  // ---- Step 1: Catalogue picker ------------------------------------
  if (step === "pick") {
    return (
      <div className="flex flex-col gap-4">
        {/* Manual options */}
        <div className="flex gap-2">
          <Button variant="outline" className="flex-1" onClick={() => pick("newznab")}>
            + Newznab (manual URL)
          </Button>
          <Button variant="outline" className="flex-1" onClick={() => pick("torznab")}>
            + Torznab (manual URL)
          </Button>
        </div>

        <div className="relative text-center text-xs text-muted-foreground">
          <span className="bg-background px-2">or pick from bundled definitions</span>
          <div className="absolute inset-x-0 top-1/2 -z-10 h-px bg-border" />
        </div>

        {/* Search + type filter */}
        <div className="flex items-center gap-3">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search definitions…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9"
            />
          </div>
          <Tabs value={typeFilter} onValueChange={setTypeFilter}>
            <TabsList>
              <TabsTrigger value="all">All</TabsTrigger>
              <TabsTrigger value="public">Public</TabsTrigger>
              <TabsTrigger value="private">Private</TabsTrigger>
              <TabsTrigger value="semi-private">Semi-Private</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        {/* Category filter pills */}
        {allCategories.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            <Button
              size="sm"
              variant={catFilter === "all" ? "default" : "outline"}
              className="h-7 rounded-full px-3 text-xs"
              onClick={() => setCatFilter("all")}
            >
              All Categories
            </Button>
            {allCategories.map((cat) => (
              <Button
                key={cat}
                size="sm"
                variant={catFilter === cat ? "default" : "outline"}
                className="h-7 rounded-full px-3 text-xs"
                onClick={() => setCatFilter(cat)}
              >
                {cat}
              </Button>
            ))}
          </div>
        )}

        {/* Results count */}
        <p className="text-xs text-muted-foreground">
          {defsQ.isLoading
            ? "Loading definitions…"
            : `${filtered.length} definition${filtered.length === 1 ? "" : "s"}`}
        </p>

        {/* Definition table */}
        <ScrollArea className="h-[420px] rounded-md border">
          {defsQ.isLoading ? (
            <div className="space-y-2 p-3">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-9 rounded-md" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <p className="py-12 text-center text-sm text-muted-foreground">
              No definitions match your search.
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10 bg-muted/80 backdrop-blur">
                <tr className="border-b text-left text-xs font-medium text-muted-foreground">
                  <th className="px-3 py-2">Name</th>
                  <th className="px-3 py-2 hidden sm:table-cell">Type</th>
                  <th className="px-3 py-2 hidden lg:table-cell">Categories</th>
                  <th className="px-3 py-2 hidden md:table-cell">Language</th>
                  <th className="px-3 py-2 text-right w-[80px]" />
                </tr>
              </thead>
              <tbody>
                {filtered.map((def) => (
                  <tr
                    key={def.id}
                    className="border-b border-border/50 transition-colors hover:bg-muted/40"
                  >
                    <td className="px-3 py-2">
                      <div className="font-medium">{def.name}</div>
                      <div className="text-xs text-muted-foreground">{def.id}</div>
                    </td>
                    <td className="px-3 py-2 hidden sm:table-cell">
                      {typeBadge(def.type)}
                    </td>
                    <td className="px-3 py-2 hidden lg:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {(def.categories ?? []).map((cat) => (
                          <Badge key={cat} variant="secondary" className="text-[10px] px-1.5 py-0">
                            {cat}
                          </Badge>
                        ))}
                      </div>
                    </td>
                    <td className="px-3 py-2 hidden md:table-cell text-muted-foreground">
                      {def.language || "—"}
                    </td>
                    <td className="px-3 py-2 text-right">
                      <Button
                        size="sm"
                        variant="outline"
                        className="h-7 px-2.5 text-xs"
                        onClick={() => pick(def)}
                      >
                        <Plus className="mr-1 h-3 w-3" />
                        Add
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </ScrollArea>

        <div className="flex justify-end">
          <Button variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        </div>
      </div>
    );
  }

  // ---- Step 2: Configure -------------------------------------------

  // Newznab / Torznab — delegate to existing form
  if (selected === "newznab" || selected === "torznab") {
    return (
      <div className="flex flex-col gap-4">
        <Button
          variant="ghost"
          size="sm"
          className="w-fit gap-1"
          onClick={back}
        >
          <ArrowLeft className="h-4 w-4" /> Back to catalogue
        </Button>
        <IndexerForm
          proxies={proxies}
          onSubmit={(v) => handleNewznabCreate({ ...v, kind: selected })}
          onCancel={onCancel}
          submitting={create.isPending}
          topError={topError}
        />
      </div>
    );
  }

  // Cardigann definition
  if (selected && typeof selected === "object") {
    return (
      <CardigannConfigForm
        definition={selected}
        proxies={proxies}
        topError={topError}
        submitting={create.isPending}
        onBack={back}
        onCancel={onCancel}
        onSubmit={handleCardigannCreate}
      />
    );
  }

  return null;
}

// ---- Cardigann config sub-form -------------------------------------

interface CardigannConfigFormProps {
  definition: IndexerDefinition;
  proxies: Proxy[];
  topError?: string;
  submitting?: boolean;
  onBack: () => void;
  onCancel: () => void;
  onSubmit: (
    def: IndexerDefinition,
    formData: Record<string, string>,
    name: string,
    enabled: boolean,
    priority: number,
    proxyId: string,
  ) => void;
}

function CardigannConfigForm({
  definition,
  proxies,
  topError,
  submitting,
  onBack,
  onCancel,
  onSubmit,
}: CardigannConfigFormProps) {
  const [name, setName] = React.useState(definition.name);
  const [enabled, setEnabled] = React.useState(true);
  const [priority, setPriority] = React.useState(25);
  const [proxyId, setProxyId] = React.useState("");
  const [fields, setFields] = React.useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    for (const s of definition.settings ?? []) {
      init[s.name] = s.default ?? "";
    }
    return init;
  });

  function updateField(key: string, value: string) {
    setFields((prev) => ({ ...prev, [key]: value }));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    onSubmit(definition, fields, name, enabled, priority, proxyId);
  }

  const settings = definition.settings ?? [];

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4" noValidate>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="w-fit gap-1"
        onClick={onBack}
      >
        <ArrowLeft className="h-4 w-4" /> Back to catalogue
      </Button>

      <div className="flex items-center gap-2">
        <h3 className="text-lg font-semibold">{definition.name}</h3>
        {typeBadge(definition.type)}
      </div>

      {definition.description ? (
        <p className="text-sm text-muted-foreground">{definition.description}</p>
      ) : null}

      {topError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {topError}
        </div>
      ) : null}

      <div className="grid gap-2">
        <Label htmlFor="cardi-name">Name</Label>
        <Input
          id="cardi-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoComplete="off"
        />
      </div>

      {/* Dynamic settings from definition */}
      {settings.length > 0 ? (
        <div className="space-y-3">
          <p className="text-sm font-medium">Tracker credentials</p>
          {settings.map((s) => (
            <div key={s.name} className="grid gap-1">
              <Label htmlFor={`cardi-${s.name}`}>
                {s.label || s.name}
              </Label>
              <Input
                id={`cardi-${s.name}`}
                type={s.type === "password" ? "password" : "text"}
                value={fields[s.name] ?? ""}
                onChange={(e) => updateField(s.name, e.target.value)}
                placeholder={s.default || undefined}
                autoComplete="off"
              />
            </div>
          ))}
        </div>
      ) : null}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="cardi-priority">Priority (0–100)</Label>
          <Input
            id="cardi-priority"
            type="number"
            min={0}
            max={100}
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
          />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="cardi-proxy">Proxy</Label>
          <select
            id="cardi-proxy"
            value={proxyId}
            onChange={(e) => setProxyId(e.target.value)}
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          >
            <option value="">None — direct connection</option>
            {proxies.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name} ({p.kind})
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <input
          id="cardi-enabled"
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="h-4 w-4 rounded border-input"
        />
        <Label htmlFor="cardi-enabled" className="!m-0">
          Enabled
        </Label>
      </div>

      <div className="mt-2 flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? "Saving…" : "Add indexer"}
        </Button>
      </div>
    </form>
  );
}
