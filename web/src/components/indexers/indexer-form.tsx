// IndexerForm collects the fields needed to create or edit a Loom
// indexer. It drives both flows through the same component to keep
// validation rules in a single place. The form is uncontrolled-ish:
// each field owns its state, and validation runs on submit.
//
// Editing semantics:
//   - When `initial` is provided, the form pre-fills and submits a
//     PATCH-shaped payload via `onSubmit`.
//   - The `proxy_id` field intentionally distinguishes the three
//     PATCH states described in the OpenAPI contract:
//       * "" (No proxy) -> send `proxy_id: null`  -> detach
//       * "<id>"        -> send `proxy_id: "<id>"` -> attach
//       * unchanged from initial -> field omitted -> server ignores

import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CheckCircle2, XCircle, Loader2 } from "lucide-react";
import type { Indexer, IndexerKind, Proxy } from "@/lib/indexers-api";
import { useTestIndexerConfig, useTestIndexer } from "@/lib/indexers-api";

const INDEXER_KINDS: { value: IndexerKind; label: string; helper: string }[] = [
  {
    value: "newznab",
    label: "Newznab",
    helper: "Usenet feed (Sonarr/Radarr-compatible).",
  },
  {
    value: "torznab",
    label: "Torznab",
    helper: "BitTorrent feed (Jackett/Prowlarr style).",
  },
];

export interface IndexerFormValues {
  id?: string;
  kind: IndexerKind;
  name: string;
  enabled: boolean;
  priority: number;
  url: string;
  api_key: string;
  user_agent?: string;
  timeout?: string;
  categories: number[];
  tags: string[];
  // "" represents "no proxy"; undefined means "unchanged" (only meaningful
  // for edit submissions where we want to omit the field entirely).
  proxy_id: string;
  // Per-indexer seed policy overrides (stored in config JSON).
  // undefined means "use client default".
  seed_ratio_limit?: number;
  seed_time_limit_minutes?: number;
}

export interface IndexerFormErrors {
  name?: string;
  url?: string;
  api_key?: string;
  priority?: string;
  categories?: string;
  timeout?: string;
}

export function validateIndexerForm(
  values: IndexerFormValues,
): IndexerFormErrors {
  const errors: IndexerFormErrors = {};
  if (!values.name.trim()) {
    errors.name = "Give the indexer a recognizable name.";
  }
  if (!values.url.trim()) {
    errors.url = "Enter the upstream URL, e.g. https://nzbhydra.example/api.";
  } else {
    try {
      const u = new URL(values.url);
      if (u.protocol !== "http:" && u.protocol !== "https:") {
        errors.url = "URL must use http:// or https://.";
      }
    } catch {
      errors.url = "URL is not valid.";
    }
  }
  if (!values.api_key.trim()) {
    errors.api_key = "An API key is required to authenticate to the feed.";
  }
  if (
    !Number.isFinite(values.priority) ||
    values.priority < 0 ||
    values.priority > 100
  ) {
    errors.priority = "Priority must be between 0 and 100.";
  }
  for (const cat of values.categories) {
    if (!Number.isFinite(cat) || cat < 0) {
      errors.categories = "Categories must be non-negative integers.";
      break;
    }
  }
  if (values.timeout && !/^\d+(ms|s|m|h)$/.test(values.timeout.trim())) {
    errors.timeout =
      'Use a Go duration string, for example "30s", "2m", or "1500ms".';
  }
  return errors;
}

function parseIntList(input: string): number[] {
  return input
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n));
}

function parseTags(input: string): string[] {
  return input
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

export interface IndexerFormProps {
  initial?: Indexer;
  proxies: Proxy[];
  submitLabel?: string;
  onSubmit: (
    values: IndexerFormValues,
    isEdit: boolean,
  ) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
  topError?: string;
}

export function IndexerForm({
  initial,
  proxies,
  submitLabel,
  onSubmit,
  onCancel,
  submitting,
  topError,
}: IndexerFormProps) {
  const isEdit = Boolean(initial);
  const initialConfig = (initial?.config ?? {}) as Record<string, unknown>;

  const [values, setValues] = React.useState<IndexerFormValues>(() => ({
    id: initial?.id,
    kind: ((initial?.kind as IndexerKind) ?? "newznab") as IndexerKind,
    name: initial?.name ?? "",
    enabled: initial?.enabled ?? true,
    priority: initial?.priority ?? 25,
    url: typeof initialConfig.url === "string" ? initialConfig.url : "",
    api_key:
      typeof initialConfig.api_key === "string" ? initialConfig.api_key : "",
    user_agent:
      typeof initialConfig.user_agent === "string"
        ? initialConfig.user_agent
        : "",
    timeout:
      typeof initialConfig.timeout === "string" ? initialConfig.timeout : "",
    categories: initial?.categories ?? [],
    tags: initial?.tags ?? [],
    proxy_id: initial?.proxy_id ?? "",
    seed_ratio_limit:
      typeof initialConfig.seed_ratio_limit === "number"
        ? initialConfig.seed_ratio_limit
        : undefined,
    seed_time_limit_minutes:
      typeof initialConfig.seed_time_limit_minutes === "number"
        ? initialConfig.seed_time_limit_minutes
        : undefined,
  }));

  const [errors, setErrors] = React.useState<IndexerFormErrors>({});
  const [testResult, setTestResult] = React.useState<{
    ok: boolean;
    latency_ms: number;
    error?: string;
  } | null>(null);

  const testConfig = useTestIndexerConfig();
  const testSaved = useTestIndexer();

  async function handleTest() {
    setTestResult(null);
    const errs = validateIndexerForm(values);
    setErrors(errs);
    if (Object.keys(errs).length > 0) return;

    try {
      let res;
      if (isEdit && values.id) {
        res = await testSaved.mutateAsync(values.id);
      } else {
        res = await testConfig.mutateAsync({
          kind: values.kind,
          name: values.name || "test",
          config: {
            url: values.url,
            api_key: values.api_key,
            ...(values.user_agent ? { user_agent: values.user_agent } : {}),
            ...(values.timeout ? { timeout: values.timeout } : {}),
          },
          ...(values.proxy_id ? { proxy_id: values.proxy_id } : {}),
        });
      }
      setTestResult(res);
    } catch (err) {
      setTestResult({
        ok: false,
        latency_ms: 0,
        error: err instanceof Error ? err.message : "Test failed",
      });
    }
  }

  const testing = testConfig.isPending || testSaved.isPending;

  function update<K extends keyof IndexerFormValues>(
    key: K,
    val: IndexerFormValues[K],
  ) {
    setValues((v) => ({ ...v, [key]: val }));
  }

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const errs = validateIndexerForm(values);
    setErrors(errs);
    if (Object.keys(errs).length > 0) {
      return;
    }
    await onSubmit(values, isEdit);
  }

  const kindHelper =
    INDEXER_KINDS.find((k) => k.value === values.kind)?.helper ?? "";

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-4"
      aria-label={isEdit ? "Edit indexer" : "Add indexer"}
      noValidate
    >
      {topError ? (
        <div
          role="alert"
          className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300"
        >
          {topError}
        </div>
      ) : null}

      <div className="grid gap-2">
        <Label htmlFor="indexer-kind">Kind</Label>
        <select
          id="indexer-kind"
          value={values.kind}
          disabled={isEdit}
          onChange={(e) => update("kind", e.target.value as IndexerKind)}
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
        >
          {INDEXER_KINDS.map((k) => (
            <option key={k.value} value={k.value}>
              {k.label}
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">{kindHelper}</p>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="indexer-name">Name</Label>
        <Input
          id="indexer-name"
          value={values.name}
          onChange={(e) => update("name", e.target.value)}
          aria-invalid={Boolean(errors.name)}
          aria-describedby={errors.name ? "indexer-name-error" : undefined}
          autoComplete="off"
        />
        {errors.name ? (
          <p id="indexer-name-error" className="text-xs text-red-600">
            {errors.name}
          </p>
        ) : null}
      </div>

      <div className="grid gap-2">
        <Label htmlFor="indexer-url">URL</Label>
        <Input
          id="indexer-url"
          inputMode="url"
          placeholder="https://nzbhydra.example/api"
          value={values.url}
          onChange={(e) => update("url", e.target.value)}
          aria-invalid={Boolean(errors.url)}
          aria-describedby={errors.url ? "indexer-url-error" : undefined}
        />
        {errors.url ? (
          <p id="indexer-url-error" className="text-xs text-red-600">
            {errors.url}
          </p>
        ) : null}
      </div>

      <div className="grid gap-2">
        <Label htmlFor="indexer-api-key">API key</Label>
        <Input
          id="indexer-api-key"
          type="password"
          autoComplete="off"
          value={values.api_key}
          onChange={(e) => update("api_key", e.target.value)}
          aria-invalid={Boolean(errors.api_key)}
          aria-describedby={
            errors.api_key ? "indexer-api-key-error" : undefined
          }
        />
        {errors.api_key ? (
          <p id="indexer-api-key-error" className="text-xs text-red-600">
            {errors.api_key}
          </p>
        ) : null}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="indexer-priority">Priority (0–100)</Label>
          <Input
            id="indexer-priority"
            type="number"
            min={0}
            max={100}
            value={values.priority}
            onChange={(e) => update("priority", Number(e.target.value))}
            aria-invalid={Boolean(errors.priority)}
            aria-describedby={
              errors.priority ? "indexer-priority-error" : undefined
            }
          />
          {errors.priority ? (
            <p id="indexer-priority-error" className="text-xs text-red-600">
              {errors.priority}
            </p>
          ) : null}
        </div>

        <div className="grid gap-2">
          <Label htmlFor="indexer-timeout">Timeout</Label>
          <Input
            id="indexer-timeout"
            placeholder="30s"
            value={values.timeout ?? ""}
            onChange={(e) => update("timeout", e.target.value)}
            aria-invalid={Boolean(errors.timeout)}
            aria-describedby={
              errors.timeout ? "indexer-timeout-error" : undefined
            }
          />
          {errors.timeout ? (
            <p id="indexer-timeout-error" className="text-xs text-red-600">
              {errors.timeout}
            </p>
          ) : null}
        </div>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="indexer-categories">Categories</Label>
        <Input
          id="indexer-categories"
          placeholder="2000, 5000"
          defaultValue={values.categories.join(", ")}
          onBlur={(e) => update("categories", parseIntList(e.target.value))}
          aria-describedby="indexer-categories-help"
          aria-invalid={Boolean(errors.categories)}
        />
        <p
          id="indexer-categories-help"
          className="text-xs text-muted-foreground"
        >
          Comma-separated Newznab category IDs. Leave blank to use the
          indexer&apos;s defaults.
        </p>
        {errors.categories ? (
          <p className="text-xs text-red-600">{errors.categories}</p>
        ) : null}
      </div>

      <div className="grid gap-2">
        <Label htmlFor="indexer-tags">Tags</Label>
        <Input
          id="indexer-tags"
          placeholder="public, fast"
          defaultValue={values.tags.join(", ")}
          onBlur={(e) => update("tags", parseTags(e.target.value))}
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="indexer-proxy">Proxy</Label>
        <select
          id="indexer-proxy"
          value={values.proxy_id}
          onChange={(e) => update("proxy_id", e.target.value)}
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          <option value="">None — direct connection</option>
          {proxies.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name} ({p.kind})
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">
          Route this indexer&apos;s outbound HTTP traffic through a configured
          proxy. Detaching a proxy on an existing indexer sends a JSON-null on
          PATCH so the server clears the pin.
        </p>
      </div>

      {/* --- Seeding overrides --- */}
      <fieldset className="grid gap-4 rounded-md border border-input p-4">
        <legend className="px-2 text-sm font-medium text-muted-foreground">
          Seeding
        </legend>
        <p className="-mt-2 text-xs text-muted-foreground">
          Override the download client&apos;s default seed policy for grabs from
          this indexer. Leave empty to use the client default.
        </p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="indexer-seed-ratio">Seed Ratio Limit</Label>
            <Input
              id="indexer-seed-ratio"
              type="number"
              step={0.1}
              min={0}
              placeholder="Use client default"
              value={values.seed_ratio_limit ?? ""}
              onChange={(e) =>
                update(
                  "seed_ratio_limit",
                  e.target.value === "" ? undefined : Number(e.target.value),
                )
              }
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="indexer-seed-time">Seed Time Limit (minutes)</Label>
            <Input
              id="indexer-seed-time"
              type="number"
              min={0}
              placeholder="Use client default"
              value={values.seed_time_limit_minutes ?? ""}
              onChange={(e) =>
                update(
                  "seed_time_limit_minutes",
                  e.target.value === "" ? undefined : Number(e.target.value),
                )
              }
            />
          </div>
        </div>
      </fieldset>

      <div className="flex items-center gap-2">
        <input
          id="indexer-enabled"
          type="checkbox"
          checked={values.enabled}
          onChange={(e) => update("enabled", e.target.checked)}
          className="h-4 w-4 rounded border-input"
        />
        <Label htmlFor="indexer-enabled" className="!m-0">
          Enabled
        </Label>
      </div>

      {testResult && (
        <div
          className={`flex items-center gap-2 rounded-md border p-3 text-sm ${
            testResult.ok
              ? "border-green-500/40 bg-green-500/10 text-green-700 dark:text-green-300"
              : "border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300"
          }`}
        >
          {testResult.ok ? (
            <CheckCircle2 className="h-4 w-4 shrink-0" />
          ) : (
            <XCircle className="h-4 w-4 shrink-0" />
          )}
          <span>
            {testResult.ok
              ? `Connection successful (${testResult.latency_ms}ms)`
              : testResult.error || "Connection failed"}
          </span>
        </div>
      )}

      <div className="mt-2 flex justify-end gap-2">
        {onCancel ? (
          <Button type="button" variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        ) : null}
        <Button
          type="button"
          variant="outline"
          onClick={handleTest}
          disabled={testing || submitting}
        >
          {testing ? (
            <>
              <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />
              Testing…
            </>
          ) : (
            "Test"
          )}
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting
            ? "Saving…"
            : (submitLabel ?? (isEdit ? "Save changes" : "Add indexer"))}
        </Button>
      </div>
    </form>
  );
}
