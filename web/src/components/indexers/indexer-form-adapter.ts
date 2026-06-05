// Adapters that translate the IndexerForm view-model into the
// IndexerCreate / IndexerPatch wire shapes.
//
// The PATCH adapter is the interesting one: it implements the
// "null vs unset" rule for proxy_id documented in the OpenAPI spec.

import type { Indexer, IndexerCreate, IndexerPatch } from "@/lib/indexers-api";
import type { IndexerFormValues } from "@/components/indexers/indexer-form";

function buildConfig(values: IndexerFormValues): Record<string, unknown> {
  const cfg: Record<string, unknown> = {
    url: values.url.trim(),
    api_key: values.api_key.trim(),
  };
  if (values.user_agent && values.user_agent.trim()) {
    cfg.user_agent = values.user_agent.trim();
  }
  if (values.timeout && values.timeout.trim()) {
    cfg.timeout = values.timeout.trim();
  }
  if (values.seed_ratio_limit != null) {
    cfg.seed_ratio_limit = values.seed_ratio_limit;
  }
  if (values.seed_time_limit_minutes != null) {
    cfg.seed_time_limit_minutes = values.seed_time_limit_minutes;
  }
  return cfg;
}

export function toCreatePayload(values: IndexerFormValues): IndexerCreate {
  const body: IndexerCreate = {
    kind: values.kind,
    name: values.name.trim(),
    enabled: values.enabled,
    priority: values.priority,
    config: buildConfig(values),
    categories: values.categories,
    tags: values.tags,
  };
  if (values.proxy_id) {
    body.proxy_id = values.proxy_id;
  }
  return body;
}

// toPatchPayload diffs the form values against the original indexer
// row so the PATCH only carries actually-changed fields. proxy_id
// gets the three-state treatment: omit, null, or string.
export function toPatchPayload(
  values: IndexerFormValues,
  original: Indexer,
): IndexerPatch {
  const patch: IndexerPatch = {};

  if (values.name.trim() !== original.name) {
    patch.name = values.name.trim();
  }
  if (values.enabled !== original.enabled) {
    patch.enabled = values.enabled;
  }
  if (values.priority !== original.priority) {
    patch.priority = values.priority;
  }

  const newCfg = buildConfig(values);
  const oldCfg = (original.config ?? {}) as Record<string, unknown>;
  if (!shallowEqual(newCfg, oldCfg)) {
    patch.config = newCfg;
  }

  if (!arraysEqual(values.categories, original.categories)) {
    patch.categories = values.categories;
  }
  if (!arraysEqual(values.tags, original.tags)) {
    patch.tags = values.tags;
  }

  // proxy_id semantics:
  //   form ""        → user wants no proxy
  //   form "<id>"    → user wants that proxy
  //   original.proxy_id may be undefined or "<id>"
  const wantsNone = values.proxy_id === "";
  const had = Boolean(original.proxy_id);
  if (wantsNone && had) {
    // Detach: explicit null tells the server to clear the pin.
    patch.proxy_id = null;
  } else if (!wantsNone && values.proxy_id !== (original.proxy_id ?? "")) {
    patch.proxy_id = values.proxy_id;
  }
  // else: leave proxy_id unset so the server preserves the existing value.

  return patch;
}

function arraysEqual<T>(a: T[], b: T[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}

function shallowEqual(
  a: Record<string, unknown>,
  b: Record<string, unknown>,
): boolean {
  const ak = Object.keys(a);
  const bk = Object.keys(b);
  if (ak.length !== bk.length) return false;
  for (const k of ak) {
    if (a[k] !== b[k]) return false;
  }
  return true;
}
