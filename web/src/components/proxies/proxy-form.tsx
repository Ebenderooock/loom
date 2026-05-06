// ProxyForm collects the fields needed to create or edit a proxy.
// Each kind has its own minimal field set; we only render fields
// relevant to the selected kind to keep the form short.

import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type {
  Proxy,
  ProxyConfig,
  ProxyCreate,
  ProxyKind,
  ProxyPatch,
} from "@/lib/indexers-api";

const PROXY_KINDS: { value: ProxyKind; label: string; helper: string }[] = [
  {
    value: "http",
    label: "HTTP",
    helper: "Plain HTTP forward proxy (e.g. Squid).",
  },
  {
    value: "https",
    label: "HTTPS",
    helper: "TLS-terminated forward proxy.",
  },
  {
    value: "socks5",
    label: "SOCKS5",
    helper: "SOCKS5 proxy address as host:port.",
  },
  {
    value: "flaresolverr",
    label: "FlareSolverr",
    helper: "Cloudflare bypass service. Used for Torznab feeds gated by CF.",
  },
];

export interface ProxyFormValues {
  id?: string;
  kind: ProxyKind;
  name: string;
  enabled: boolean;
  // HTTP/HTTPS/FlareSolverr
  url: string;
  // SOCKS5
  address: string;
  username: string;
  password: string;
  // FlareSolverr
  max_timeout_sec?: number;
  session_mode?: "" | "none" | "shared";
}

export interface ProxyFormErrors {
  name?: string;
  url?: string;
  address?: string;
}

export function validateProxyForm(values: ProxyFormValues): ProxyFormErrors {
  const errors: ProxyFormErrors = {};
  if (!values.name.trim()) {
    errors.name = "Give the proxy a recognizable name.";
  }
  if (values.kind === "socks5") {
    if (!values.address.trim()) {
      errors.address = "Enter the SOCKS5 endpoint, e.g. 127.0.0.1:1080.";
    } else if (!/^.+:\d+$/.test(values.address.trim())) {
      errors.address = "Address must be host:port.";
    }
  } else {
    if (!values.url.trim()) {
      errors.url = "Enter the proxy URL, e.g. http://gateway:3128.";
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
  }
  return errors;
}

export function valuesToConfig(values: ProxyFormValues): ProxyConfig {
  switch (values.kind) {
    case "socks5": {
      const cfg: Record<string, unknown> = { address: values.address.trim() };
      if (values.username) cfg.username = values.username;
      if (values.password) cfg.password = values.password;
      return cfg as unknown as ProxyConfig;
    }
    case "flaresolverr": {
      const cfg: Record<string, unknown> = { url: values.url.trim() };
      if (typeof values.max_timeout_sec === "number") {
        cfg.max_timeout_sec = values.max_timeout_sec;
      }
      if (values.session_mode) {
        cfg.session_mode = values.session_mode;
      }
      return cfg as unknown as ProxyConfig;
    }
    case "http":
    case "https":
    default: {
      const cfg: Record<string, unknown> = { url: values.url.trim() };
      if (values.username) cfg.username = values.username;
      if (values.password) cfg.password = values.password;
      return cfg as unknown as ProxyConfig;
    }
  }
}

export function toProxyCreate(values: ProxyFormValues): ProxyCreate {
  return {
    kind: values.kind,
    name: values.name.trim(),
    enabled: values.enabled,
    config: valuesToConfig(values),
  };
}

export function toProxyPatch(values: ProxyFormValues): ProxyPatch {
  return {
    name: values.name.trim(),
    enabled: values.enabled,
    kind: values.kind,
    config: valuesToConfig(values),
  };
}

export interface ProxyFormProps {
  initial?: Proxy;
  onSubmit: (values: ProxyFormValues, isEdit: boolean) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
  topError?: string;
}

export function ProxyForm({
  initial,
  onSubmit,
  onCancel,
  submitting,
  topError,
}: ProxyFormProps) {
  const isEdit = Boolean(initial);
  const initialCfg = (initial?.config ?? {}) as Record<string, unknown>;

  const [values, setValues] = React.useState<ProxyFormValues>(() => ({
    id: initial?.id,
    kind: initial?.kind ?? "http",
    name: initial?.name ?? "",
    enabled: initial?.enabled ?? true,
    url: typeof initialCfg.url === "string" ? initialCfg.url : "",
    address:
      typeof initialCfg.address === "string" ? initialCfg.address : "",
    username:
      typeof initialCfg.username === "string" ? initialCfg.username : "",
    password:
      typeof initialCfg.password === "string" ? initialCfg.password : "",
    max_timeout_sec:
      typeof initialCfg.max_timeout_sec === "number"
        ? initialCfg.max_timeout_sec
        : undefined,
    session_mode:
      initialCfg.session_mode === "shared" ||
      initialCfg.session_mode === "none" ||
      initialCfg.session_mode === ""
        ? (initialCfg.session_mode as "" | "none" | "shared")
        : "",
  }));

  const [errors, setErrors] = React.useState<ProxyFormErrors>({});

  function update<K extends keyof ProxyFormValues>(
    key: K,
    val: ProxyFormValues[K],
  ) {
    setValues((v) => ({ ...v, [key]: val }));
  }

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const errs = validateProxyForm(values);
    setErrors(errs);
    if (Object.keys(errs).length > 0) {
      return;
    }
    await onSubmit(values, isEdit);
  }

  const kindHelper =
    PROXY_KINDS.find((k) => k.value === values.kind)?.helper ?? "";

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-4"
      aria-label={isEdit ? "Edit proxy" : "Add proxy"}
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
        <Label htmlFor="proxy-kind">Kind</Label>
        <select
          id="proxy-kind"
          value={values.kind}
          onChange={(e) => update("kind", e.target.value as ProxyKind)}
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          {PROXY_KINDS.map((k) => (
            <option key={k.value} value={k.value}>
              {k.label}
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">{kindHelper}</p>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="proxy-name">Name</Label>
        <Input
          id="proxy-name"
          value={values.name}
          onChange={(e) => update("name", e.target.value)}
          aria-invalid={Boolean(errors.name)}
          aria-describedby={errors.name ? "proxy-name-error" : undefined}
        />
        {errors.name ? (
          <p id="proxy-name-error" className="text-xs text-red-600">
            {errors.name}
          </p>
        ) : null}
      </div>

      {values.kind === "socks5" ? (
        <div className="grid gap-2">
          <Label htmlFor="proxy-address">Address</Label>
          <Input
            id="proxy-address"
            placeholder="127.0.0.1:1080"
            value={values.address}
            onChange={(e) => update("address", e.target.value)}
            aria-invalid={Boolean(errors.address)}
            aria-describedby={
              errors.address ? "proxy-address-error" : undefined
            }
          />
          {errors.address ? (
            <p id="proxy-address-error" className="text-xs text-red-600">
              {errors.address}
            </p>
          ) : null}
        </div>
      ) : (
        <div className="grid gap-2">
          <Label htmlFor="proxy-url">URL</Label>
          <Input
            id="proxy-url"
            placeholder={
              values.kind === "flaresolverr"
                ? "http://flaresolverr:8191"
                : "http://gateway:3128"
            }
            value={values.url}
            onChange={(e) => update("url", e.target.value)}
            aria-invalid={Boolean(errors.url)}
            aria-describedby={errors.url ? "proxy-url-error" : undefined}
          />
          {errors.url ? (
            <p id="proxy-url-error" className="text-xs text-red-600">
              {errors.url}
            </p>
          ) : null}
        </div>
      )}

      {values.kind !== "flaresolverr" ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="proxy-username">Username</Label>
            <Input
              id="proxy-username"
              autoComplete="off"
              value={values.username}
              onChange={(e) => update("username", e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="proxy-password">Password</Label>
            <Input
              id="proxy-password"
              type="password"
              autoComplete="off"
              value={values.password}
              onChange={(e) => update("password", e.target.value)}
            />
          </div>
        </div>
      ) : null}

      {values.kind === "flaresolverr" ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="proxy-max-timeout">Max timeout (seconds)</Label>
            <Input
              id="proxy-max-timeout"
              type="number"
              min={0}
              value={values.max_timeout_sec ?? ""}
              onChange={(e) =>
                update(
                  "max_timeout_sec",
                  e.target.value === "" ? undefined : Number(e.target.value),
                )
              }
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="proxy-session-mode">Session mode</Label>
            <select
              id="proxy-session-mode"
              value={values.session_mode ?? ""}
              onChange={(e) =>
                update(
                  "session_mode",
                  e.target.value as "" | "none" | "shared",
                )
              }
              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              <option value="">Default</option>
              <option value="none">None</option>
              <option value="shared">Shared</option>
            </select>
          </div>
        </div>
      ) : null}

      <div className="flex items-center gap-2">
        <input
          id="proxy-enabled"
          type="checkbox"
          checked={values.enabled}
          onChange={(e) => update("enabled", e.target.checked)}
          className="h-4 w-4 rounded border-input"
        />
        <Label htmlFor="proxy-enabled" className="!m-0">
          Enabled
        </Label>
      </div>

      <div className="mt-2 flex justify-end gap-2">
        {onCancel ? (
          <Button type="button" variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
        ) : null}
        <Button type="submit" disabled={submitting}>
          {submitting ? "Saving…" : isEdit ? "Save changes" : "Add proxy"}
        </Button>
      </div>
    </form>
  );
}
