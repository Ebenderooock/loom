// DownloadForm collects the fields needed to create or edit a download client.
// It drives both flows through the same component to keep validation rules
// in a single place. The kind selector is read-only after creation.

import * as React from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CheckCircle2, XCircle, Loader2 } from "lucide-react";
import type {
  Download,
  DownloadKind,
  DownloadProtocol,
} from "@/lib/downloads-api";
import { useTestDownloadConfig, useTestDownload } from "@/lib/downloads-api";

const DOWNLOAD_KINDS: {
  value: DownloadKind;
  label: string;
  protocol: DownloadProtocol;
  helper: string;
}[] = [
  {
    value: "builtin/torrent",
    label: "Rain (Sidecar)",
    protocol: "torrent",
    helper:
      "Rain torrent sidecar — runs alongside Loom and is managed by the Helm chart.",
  },
  {
    value: "qbittorrent",
    label: "qBittorrent",
    protocol: "torrent",
    helper: "BitTorrent client (also supports magnet links).",
  },
  {
    value: "transmission",
    label: "Transmission",
    protocol: "torrent",
    helper: "Lightweight BitTorrent client.",
  },
  {
    value: "deluge",
    label: "Deluge",
    protocol: "torrent",
    helper: "Feature-rich BitTorrent client.",
  },
  {
    value: "sabnzbd",
    label: "SABnzbd",
    protocol: "usenet",
    helper: "Usenet binary downloader.",
  },
  {
    value: "nzbget",
    label: "NZBGet",
    protocol: "usenet",
    helper: "High-performance Usenet downloader.",
  },
];

export interface DownloadFormValues {
  id?: string;
  kind: DownloadKind;
  name: string;
  protocol: DownloadProtocol;
  enabled: boolean;
  priority: number;
  host: string;
  port: number;
  tls: boolean;
  username: string;
  password: string;
  category_default: string;
  save_path_default: string;
  remove_completed: boolean;
  remove_failed: boolean;
  config?: Record<string, unknown>;
}

export interface DownloadFormErrors {
  name?: string;
  host?: string;
  port?: string;
  priority?: string;
  download_dir?: string;
}

const BUILTIN_KINDS = new Set<string>(["builtin/torrent"]);

export function validateDownloadForm(
  values: DownloadFormValues,
): DownloadFormErrors {
  const errors: DownloadFormErrors = {};
  if (!values.name.trim()) {
    errors.name = "Give the download client a recognizable name.";
  }
  if (!BUILTIN_KINDS.has(values.kind)) {
    if (!values.host.trim()) {
      errors.host =
        "Enter the host address (e.g., localhost or 192.168.1.100).";
    }
    if (
      !Number.isFinite(values.port) ||
      values.port < 1 ||
      values.port > 65535
    ) {
      errors.port = "Port must be between 1 and 65535.";
    }
  }
  // builtin/torrent allows blank download_dir since it can be set via LOOM_TORRENT_DOWNLOAD_DIR env var
  if (
    !Number.isFinite(values.priority) ||
    values.priority < 0 ||
    values.priority > 100
  ) {
    errors.priority = "Priority must be between 0 and 100.";
  }
  return errors;
}

export interface DownloadFormProps {
  initial?: Download;
  submitLabel?: string;
  onSubmit: (values: DownloadFormValues) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
  topError?: string;
}

/**
 * DownloadForm component for creating and editing download clients.
 */
export function DownloadForm({
  initial,
  submitLabel,
  onSubmit,
  onCancel,
  submitting,
  topError,
}: DownloadFormProps) {
  const isEdit = Boolean(initial);

  const [values, setValues] = React.useState<DownloadFormValues>(() => {
    const kind = (initial?.kind as DownloadKind) ?? "builtin/torrent";
    const kindDef = DOWNLOAD_KINDS.find((k) => k.value === kind);
    const isBuiltin = BUILTIN_KINDS.has(kind);
    return {
      id: initial?.id,
      kind,
      name: initial?.name ?? "",
      protocol: initial?.protocol ?? kindDef?.protocol ?? "torrent",
      enabled: initial?.enabled ?? true,
      priority: initial?.priority ?? 25,
      host: initial?.host ?? (isBuiltin ? "" : "localhost"),
      port: initial?.port ?? (isBuiltin ? 0 : 8080),
      tls: initial?.tls ?? false,
      username: initial?.username ?? "",
      password: "",
      category_default: initial?.category_default ?? "",
      save_path_default: initial?.save_path_default ?? "",
      remove_completed: initial?.remove_completed ?? false,
      remove_failed: initial?.remove_failed ?? false,
      config:
        (initial?.config as Record<string, unknown>) ??
        (isBuiltin
          ? {
              download_dir: "",
              download_speed_limit: 0,
              upload_speed_limit: 0,
            }
          : undefined),
    };
  });

  const [errors, setErrors] = React.useState<DownloadFormErrors>({});
  const [testResult, setTestResult] = React.useState<{
    ok: boolean;
    error?: string;
  } | null>(null);

  const testConfig = useTestDownloadConfig();
  const testSaved = useTestDownload();

  async function handleTest() {
    setTestResult(null);
    const errs = validateDownloadForm(values);
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
          protocol: values.protocol,
          host: values.host,
          port: values.port,
          tls: values.tls,
          username: values.username,
          password: values.password,
          config: values.config,
          save_path_default: values.save_path_default,
        });
      }
      setTestResult(res);
    } catch (err) {
      setTestResult({
        ok: false,
        error: err instanceof Error ? err.message : "Test failed",
      });
    }
  }

  const testing = testConfig.isPending || testSaved.isPending;

  function update<K extends keyof DownloadFormValues>(
    key: K,
    val: DownloadFormValues[K],
  ) {
    setValues((v) => ({ ...v, [key]: val }));
  }

  function handleKindChange(newKind: DownloadKind) {
    const kindDef = DOWNLOAD_KINDS.find((k) => k.value === newKind);
    const isBuiltin = BUILTIN_KINDS.has(newKind);
    update("kind", newKind);
    if (kindDef) {
      update("protocol", kindDef.protocol);
    }
    if (isBuiltin) {
      update("host", "");
      update("config", {
        download_dir: "",
        download_speed_limit: 0,
        upload_speed_limit: 0,
      });
    } else {
      update("host", "localhost");
      update("config", undefined);
    }
  }

  function updateConfig(key: string, val: unknown) {
    setValues((v) => ({
      ...v,
      config: { ...v.config, [key]: val },
    }));
  }

  const isBuiltinKind = BUILTIN_KINDS.has(values.kind);

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const errs = validateDownloadForm(values);
    setErrors(errs);
    if (Object.keys(errs).length > 0) {
      return;
    }
    await onSubmit(values);
  }

  const kindHelper =
    DOWNLOAD_KINDS.find((k) => k.value === values.kind)?.helper ?? "";

  return (
    <form
      onSubmit={handleSubmit}
      className="flex flex-col gap-4"
      aria-label={isEdit ? "Edit download client" : "Add download client"}
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
        <Label htmlFor="download-kind">Kind</Label>
        <select
          id="download-kind"
          value={values.kind}
          disabled={isEdit}
          onChange={(e) => handleKindChange(e.target.value as DownloadKind)}
          className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
        >
          {DOWNLOAD_KINDS.map((k) => (
            <option key={k.value} value={k.value}>
              {k.label}
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">{kindHelper}</p>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="download-name">Name</Label>
        <Input
          id="download-name"
          value={values.name}
          onChange={(e) => update("name", e.target.value)}
          placeholder="My qBittorrent Instance"
          aria-invalid={Boolean(errors.name)}
          aria-describedby={errors.name ? "download-name-error" : undefined}
          autoComplete="off"
        />
        {errors.name ? (
          <p id="download-name-error" className="text-xs text-red-600">
            {errors.name}
          </p>
        ) : null}
      </div>

      {isBuiltinKind ? (
        <>
          <div className="rounded-md border border-zinc-800 bg-zinc-900/40 px-3 py-2 text-xs text-zinc-400">
            Rain runs as a sidecar container alongside Loom. The download
            directory and RPC connection are configured via the Helm chart and
            environment variables. Only speed limits can be adjusted at runtime
            here.
          </div>

          <div className="grid gap-2">
            <Label htmlFor="download-dir">Download directory</Label>
            <Input
              id="download-dir"
              value={(values.config?.download_dir as string) ?? ""}
              onChange={(e) => updateConfig("download_dir", e.target.value)}
              placeholder="Leave blank to use LOOM_TORRENT_DOWNLOAD_DIR env var"
            />
            {errors.download_dir ? (
              <p className="text-xs text-red-600">{errors.download_dir}</p>
            ) : null}
            <p className="text-xs text-muted-foreground">
              Override where Rain saves completed downloads. Usually set via
              Helm values.
            </p>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="dl-speed">Download speed limit (KB/s)</Label>
              <Input
                id="dl-speed"
                type="number"
                min={0}
                value={Math.round(
                  ((values.config?.download_speed_limit as number) ?? 0) / 1024,
                )}
                onChange={(e) =>
                  updateConfig(
                    "download_speed_limit",
                    Number(e.target.value) * 1024,
                  )
                }
              />
              <p className="text-xs text-muted-foreground">0 = unlimited.</p>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="ul-speed">Upload speed limit (KB/s)</Label>
              <Input
                id="ul-speed"
                type="number"
                min={0}
                value={Math.round(
                  ((values.config?.upload_speed_limit as number) ?? 0) / 1024,
                )}
                onChange={(e) =>
                  updateConfig(
                    "upload_speed_limit",
                    Number(e.target.value) * 1024,
                  )
                }
              />
              <p className="text-xs text-muted-foreground">0 = unlimited.</p>
            </div>
          </div>
        </>
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="download-host">Your host</Label>
              <Input
                id="download-host"
                value={values.host}
                onChange={(e) => update("host", e.target.value)}
                placeholder="localhost"
                aria-invalid={Boolean(errors.host)}
                aria-describedby={
                  errors.host ? "download-host-error" : undefined
                }
              />
              {errors.host ? (
                <p id="download-host-error" className="text-xs text-red-600">
                  {errors.host}
                </p>
              ) : null}
            </div>

            <div className="grid gap-2">
              <Label htmlFor="download-port">Port</Label>
              <Input
                id="download-port"
                type="number"
                min={1}
                max={65535}
                value={values.port}
                onChange={(e) => update("port", Number(e.target.value))}
                aria-invalid={Boolean(errors.port)}
                aria-describedby={
                  errors.port ? "download-port-error" : undefined
                }
              />
              {errors.port ? (
                <p id="download-port-error" className="text-xs text-red-600">
                  {errors.port}
                </p>
              ) : null}
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="download-username">Username</Label>
              <Input
                id="download-username"
                value={values.username}
                onChange={(e) => update("username", e.target.value)}
                placeholder="Optional"
                autoComplete="off"
              />
              <p className="text-xs text-muted-foreground">
                Leave blank if no authentication is required.
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="download-password">Password</Label>
              <Input
                id="download-password"
                type="password"
                value={values.password}
                onChange={(e) => update("password", e.target.value)}
                placeholder="Optional"
                autoComplete="off"
              />
              <p className="text-xs text-muted-foreground">
                Write-only; never sent back to client.
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <input
              id="download-tls"
              type="checkbox"
              checked={values.tls}
              onChange={(e) => update("tls", e.target.checked)}
              className="h-4 w-4 rounded border-input"
            />
            <Label htmlFor="download-tls" className="!m-0">
              Enable TLS
            </Label>
          </div>
        </>
      )}

      <div className="grid gap-2">
        <Label htmlFor="download-priority">Priority (0–100)</Label>
        <Input
          id="download-priority"
          type="number"
          min={0}
          max={100}
          value={values.priority}
          onChange={(e) => update("priority", Number(e.target.value))}
          aria-invalid={Boolean(errors.priority)}
          aria-describedby={
            errors.priority ? "download-priority-error" : undefined
          }
        />
        {errors.priority ? (
          <p id="download-priority-error" className="text-xs text-red-600">
            {errors.priority}
          </p>
        ) : null}
      </div>

      <div className="grid gap-2">
        <Label htmlFor="download-category">Default category</Label>
        <Input
          id="download-category"
          value={values.category_default}
          onChange={(e) => update("category_default", e.target.value)}
          placeholder="e.g., tv, movies"
        />
        <p className="text-xs text-muted-foreground">
          Default category for downloads added to this client.
        </p>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="download-path">Default save path</Label>
        <Input
          id="download-path"
          value={values.save_path_default}
          onChange={(e) => update("save_path_default", e.target.value)}
          placeholder="e.g., /downloads, C:\downloads"
        />
        <p className="text-xs text-muted-foreground">
          Default save path for downloads added to this client.
        </p>
      </div>

      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <input
            id="download-remove-completed"
            type="checkbox"
            checked={values.remove_completed}
            onChange={(e) => update("remove_completed", e.target.checked)}
            className="h-4 w-4 rounded border-input"
          />
          <Label htmlFor="download-remove-completed" className="!m-0">
            Remove completed downloads
          </Label>
        </div>
        <div className="flex items-center gap-2">
          <input
            id="download-remove-failed"
            type="checkbox"
            checked={values.remove_failed}
            onChange={(e) => update("remove_failed", e.target.checked)}
            className="h-4 w-4 rounded border-input"
          />
          <Label htmlFor="download-remove-failed" className="!m-0">
            Remove failed downloads
          </Label>
        </div>
      </div>

      <div className="flex items-center gap-2">
        <input
          id="download-enabled"
          type="checkbox"
          checked={values.enabled}
          onChange={(e) => update("enabled", e.target.checked)}
          className="h-4 w-4 rounded border-input"
        />
        <Label htmlFor="download-enabled" className="!m-0">
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
              ? "Connection successful"
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
            : (submitLabel ?? (isEdit ? "Save changes" : "Add client"))}
        </Button>
      </div>
    </form>
  );
}
