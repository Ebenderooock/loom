// CardigannEditForm — edit form for Cardigann indexers that shows the
// definition-specific fields: URL selector, credential settings, proxy, etc.

import * as React from "react";
import { CheckCircle2, Globe, Loader2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useDefinitions,
  useTestIndexer,
  type Indexer,
  type IndexerDefinition,
  type IndexerPatch,
  type Proxy,
  type TestResult,
} from "@/lib/indexers-api";

interface Props {
  indexer: Indexer;
  proxies: Proxy[];
  topError?: string;
  submitting?: boolean;
  onSubmit: (patch: IndexerPatch) => void;
  onCancel: () => void;
}

export function CardigannEditForm({
  indexer,
  proxies,
  topError,
  submitting,
  onSubmit,
  onCancel,
}: Props) {
  const defsQ = useDefinitions();
  const testIndexer = useTestIndexer();

  const cfg = (indexer.config ?? {}) as Record<string, string>;
  const definitionId = cfg.definition_id || cfg.definition || "";

  // Find the matching definition for links/settings
  const definition: IndexerDefinition | undefined = React.useMemo(
    () => (defsQ.data ?? []).find((d) => d.id === definitionId),
    [defsQ.data, definitionId],
  );

  const [name, setName] = React.useState(indexer.name);
  const [enabled, setEnabled] = React.useState(indexer.enabled);
  const [priority, setPriority] = React.useState(indexer.priority);
  const [proxyId, setProxyId] = React.useState(indexer.proxy_id ?? "");
  const [selectedUrl, setSelectedUrl] = React.useState(
    cfg.url || definition?.links?.[0] || "",
  );
  const [fields, setFields] = React.useState<Record<string, string>>(() => {
    const init: Record<string, string> = {};
    // Pre-fill from the indexer's saved config
    for (const [k, v] of Object.entries(cfg)) {
      if (k !== "definition_id" && k !== "definition" && k !== "url") {
        init[k] = String(v ?? "");
      }
    }
    return init;
  });
  const [testResult, setTestResult] = React.useState<TestResult | null>(null);

  // Update selectedUrl when definition loads
  React.useEffect(() => {
    if (!selectedUrl && definition?.links?.[0]) {
      setSelectedUrl(cfg.url || definition.links[0]);
    }
  }, [definition, cfg.url, selectedUrl]);

  function updateField(key: string, value: string) {
    setFields((prev) => ({ ...prev, [key]: value }));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;

    const patch: IndexerPatch = {};

    if (name.trim() !== indexer.name) patch.name = name.trim();
    if (enabled !== indexer.enabled) patch.enabled = enabled;
    if (priority !== indexer.priority) patch.priority = priority;

    // Build new config
    const newConfig: Record<string, unknown> = { definition_id: definitionId };
    if (selectedUrl) newConfig.url = selectedUrl;
    for (const [k, v] of Object.entries(fields)) {
      if (v) newConfig[k] = v;
    }
    // Check if config changed
    const oldCfg = indexer.config ?? {};
    if (JSON.stringify(newConfig) !== JSON.stringify(oldCfg)) {
      patch.config = newConfig;
    }

    // Proxy
    const wantsNone = proxyId === "";
    const had = Boolean(indexer.proxy_id);
    if (wantsNone && had) {
      patch.proxy_id = null;
    } else if (!wantsNone && proxyId !== (indexer.proxy_id ?? "")) {
      patch.proxy_id = proxyId;
    }

    onSubmit(patch);
  }

  async function handleTest() {
    setTestResult(null);
    try {
      const res = await testIndexer.mutateAsync(indexer.id);
      setTestResult(res);
    } catch (err) {
      setTestResult({
        ok: false,
        latency_ms: 0,
        error: err instanceof Error ? err.message : "Test failed",
      });
    }
  }

  const links = definition?.links ?? [];
  const settings = definition?.settings ?? [];

  if (defsQ.isLoading) {
    return <p className="text-sm text-muted-foreground">Loading definition…</p>;
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4" noValidate>
      <div className="flex items-center gap-2">
        <h3 className="text-lg font-semibold">
          {definition?.name ?? definitionId}
        </h3>
        {definition?.type === "public" && (
          <span className="rounded bg-green-600/15 px-1.5 py-0.5 text-xs text-green-700 dark:text-green-400">
            Public
          </span>
        )}
        {definition?.type === "private" && (
          <span className="rounded bg-red-600/15 px-1.5 py-0.5 text-xs text-red-700 dark:text-red-400">
            Private
          </span>
        )}
      </div>

      {topError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {topError}
        </div>
      ) : null}

      {testResult ? (
        <div
          role="status"
          className={`flex items-center gap-2 rounded-md border p-3 text-sm ${
            testResult.ok
              ? "border-green-500/40 bg-green-500/10 text-green-700 dark:text-green-300"
              : "border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300"
          }`}
        >
          {testResult.ok ? (
            <>
              <CheckCircle2 className="h-4 w-4" />
              Connection successful ({testResult.latency_ms}ms)
            </>
          ) : (
            <>
              <XCircle className="h-4 w-4" />
              {testResult.error || "Test failed"}
            </>
          )}
        </div>
      ) : null}

      <div className="grid gap-2">
        <Label htmlFor="edit-cardi-name">Name</Label>
        <Input
          id="edit-cardi-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoComplete="off"
        />
      </div>

      {/* Base URL selector */}
      {links.length > 1 ? (
        <div className="grid gap-2">
          <Label htmlFor="edit-cardi-url">Base URL</Label>
          <select
            id="edit-cardi-url"
            value={selectedUrl}
            onChange={(e) => setSelectedUrl(e.target.value)}
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          >
            {links.map((link) => (
              <option key={link} value={link}>
                {link}
              </option>
            ))}
          </select>
          <p className="text-xs text-muted-foreground">
            Select the site mirror that works in your region.
          </p>
        </div>
      ) : links.length === 1 ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Globe className="h-4 w-4" />
          <span>{links[0]}</span>
        </div>
      ) : null}

      {/* Dynamic settings from definition */}
      {settings.length > 0 ? (
        <div className="space-y-3">
          <p className="text-sm font-medium">Tracker credentials</p>
          {settings.map((s) => (
            <div key={s.name} className="grid gap-1">
              <Label htmlFor={`edit-cardi-${s.name}`}>
                {s.label || s.name}
              </Label>
              <Input
                id={`edit-cardi-${s.name}`}
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
          <Label htmlFor="edit-cardi-priority">Priority (0–100)</Label>
          <Input
            id="edit-cardi-priority"
            type="number"
            min={0}
            max={100}
            value={priority}
            onChange={(e) => setPriority(Number(e.target.value))}
          />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="edit-cardi-proxy">Proxy</Label>
          <select
            id="edit-cardi-proxy"
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
          id="edit-cardi-enabled"
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="h-4 w-4 rounded border-input"
        />
        <Label htmlFor="edit-cardi-enabled" className="!m-0">
          Enabled
        </Label>
      </div>

      <div className="mt-2 flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={testIndexer.isPending}
          onClick={handleTest}
        >
          {testIndexer.isPending ? (
            <>
              <Loader2 className="mr-1 h-4 w-4 animate-spin" />
              Testing…
            </>
          ) : (
            "Test"
          )}
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? "Saving…" : "Save"}
        </Button>
      </div>
    </form>
  );
}
