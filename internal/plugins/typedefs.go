package plugins

// PluginTypeDefs is an ambient TypeScript declaration (.d.ts) describing the
// JavaScript runtime contract exposed to plugins: the `event` object (a
// discriminated union keyed on `event.event`, so editors can narrow `data` per
// event), `env`, `console`, and the synchronous `fetch`. The frontend feeds this
// to the Monaco editor's JS language service for IntelliSense.
//
// IMPORTANT: keep the per-event `data` shapes here in sync with the
// NotificationData() maps in internal/downloads, internal/imports and
// internal/analytics. TestTypeDefsCoverAllEvents guards that every supported
// event key is represented.
const PluginTypeDefs = `// Loom plugin runtime — globals available to every plugin script.
// Provided by Loom for editor assistance; these are not importable modules.

interface LoomEventBase {
  /** Payload schema version (currently 1). */
  version: number;
  /** Internal bus topic, e.g. "downloads.queued". */
  topic: string;
  /** Human-readable subject for the event (when known). */
  title: string;
  /** RFC3339 UTC timestamp, e.g. "2026-06-07T11:49:45Z". */
  timestamp: string;
}

interface LoomPlaybackData {
  title: string;
  /** Account/user that triggered playback. */
  user: string;
  device: string;
  /** Media-server connection name. */
  server: string;
  /** Media-server provider, e.g. "plex" | "jellyfin" | "emby". */
  provider: string;
  media_type: string;
  transcode: boolean;
  bitrate_kbps: number;
  watched_ms: number;
}

/**
 * The event that triggered this plugin run. Narrow on the literal ` + "`event.event`" + `
 * to reveal the fields available on ` + "`event.data`" + ` for that event:
 *
 *     if (event.event === "import_complete") {
 *       console.log(event.data.dest_path); // typed
 *     }
 */
type LoomEvent =
  | (LoomEventBase & { event: "grab"; data: { download_id: string; client_id: string; title: string } })
  | (LoomEventBase & { event: "download_complete"; data: { download_id: string; client_id: string; title: string; category: string } })
  | (LoomEventBase & { event: "download_failed"; data: { client_id: string; title: string; error: string } })
  | (LoomEventBase & { event: "import_complete"; data: { title: string; media_type: string; media_id: string; dest_path: string } })
  | (LoomEventBase & { event: "import_failed"; data: { title: string; error: string } })
  | (LoomEventBase & { event: "playback_started"; data: LoomPlaybackData })
  | (LoomEventBase & { event: "playback_stopped"; data: LoomPlaybackData });

interface LoomFetchOptions {
  /** HTTP method. Defaults to "GET". */
  method?: string;
  headers?: Record<string, string>;
  /** Request body as a string (e.g. JSON.stringify(...)). */
  body?: string;
}

interface LoomResponse {
  status: number;
  /** True for 2xx responses. */
  ok: boolean;
  statusText: string;
  /** Response body as a string (size-capped). */
  body: string;
  headers: Record<string, string>;
}

/** The event that triggered this run. Narrow on ` + "`event.event`" + ` to type ` + "`event.data`" + `. */
declare const event: LoomEvent;

/** Environment variables configured for this plugin (the "Environment variables" field). */
declare const env: { [key: string]: string };

/** Console logging. log/info/debug write to stdout; warn/error write to stderr. Objects are rendered as JSON. */
declare const console: {
  log(...args: any[]): void;
  info(...args: any[]): void;
  debug(...args: any[]): void;
  warn(...args: any[]): void;
  error(...args: any[]): void;
};

/**
 * Synchronous HTTP client (http/https only). Returns once the response is read.
 * Throws a JavaScript error on network failure, an invalid/disallowed URL, or
 * when request/response size caps are exceeded.
 */
declare function fetch(url: string, options?: LoomFetchOptions): LoomResponse;
`
