#!/usr/bin/env node
// Loom MCP Server — full control and observability for the Loom media manager.

const { Server } = require("@modelcontextprotocol/sdk/server/index.js");
const { StdioServerTransport } = require("@modelcontextprotocol/sdk/server/stdio.js");
const { CallToolRequestSchema, ListToolsRequestSchema } = require("@modelcontextprotocol/sdk/types.js");

const LOOM_BASE_URL = process.env.LOOM_URL || "https://loom.media.deroock.co.za";
const LOOM_API_KEY  = process.env.LOOM_API_KEY || "";

// ─── HTTP helper ───────────────────────────────────────────────────────────────

async function api(path, options = {}) {
  const url = `${LOOM_BASE_URL}${path}`;
  const headers = { "Content-Type": "application/json" };
  if (LOOM_API_KEY) headers["X-Api-Key"] = LOOM_API_KEY;
  const res = await fetch(url, { ...options, headers });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${res.status} ${res.statusText} ${path}: ${body.slice(0, 400)}`);
  }
  const text = await res.text();
  if (!text) return null;
  try { return JSON.parse(text); } catch { return text; }
}
const GET    = (p)         => api(p);
const POST   = (p, b)      => api(p, { method: "POST",   body: b !== undefined ? JSON.stringify(b) : undefined });
const PUT    = (p, b)      => api(p, { method: "PUT",    body: JSON.stringify(b) });
const PATCH  = (p, b)      => api(p, { method: "PATCH",  body: JSON.stringify(b) });
const DELETE = (p)         => api(p, { method: "DELETE" });

// ─── Formatters ────────────────────────────────────────────────────────────────

function mb(b) { return b ? (b/1024/1024).toFixed(1)+"MB" : "?"; }
function gb(b) { return b ? (b/1024/1024/1024).toFixed(2)+"GB" : "?"; }
function ago(iso) {
  if (!iso) return "?";
  const d = (Date.now() - new Date(iso).getTime()) / 1000;
  if (d < 60) return `${Math.round(d)}s ago`;
  if (d < 3600) return `${Math.round(d/60)}m ago`;
  if (d < 86400) return `${Math.round(d/3600)}h ago`;
  return `${Math.round(d/86400)}d ago`;
}
function pct(p) { return (p*100).toFixed(0)+"%"; }

// ─── Tools ────────────────────────────────────────────────────────────────────

const TOOLS = [

  // ── System ──────────────────────────────────────────────────────────────────
  { name:"system_status",
    description:"Loom version, commit, db engine, and health checks.",
    inputSchema:{type:"object",properties:{}} },

  { name:"system_logs",
    description:"Tail or search Loom application logs.",
    inputSchema:{type:"object",properties:{
      level:{type:"string",description:"error|warn|info|debug"},
      limit:{type:"number",description:"Max entries (default 50)"},
    }}},

  { name:"clear_system_logs",
    description:"Clear all stored system logs.",
    inputSchema:{type:"object",properties:{}} },

  { name:"browse_filesystem",
    description:"Browse the server filesystem — use to find library paths and check media files exist.",
    inputSchema:{type:"object",properties:{
      path:{type:"string",description:"Absolute path to browse (default: /)"},
    }}},

  // ── Series ──────────────────────────────────────────────────────────────────
  { name:"list_series",
    description:"List all TV series. Add with_missing:true to only show series that need episodes.",
    inputSchema:{type:"object",properties:{
      with_missing:{type:"boolean"},
      monitored_only:{type:"boolean"},
    }}},

  { name:"get_series",
    description:"Get full details of one series including IDs, quality profile, episode counts.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"lookup_series",
    description:"Search TMDB/TVDB for a series to get its metadata before adding. Returns candidate results.",
    inputSchema:{type:"object",properties:{q:{type:"string",description:"Title to search"}},required:["q"]}},

  { name:"add_series",
    description:"Add a new series to Loom from TMDB ID. Optionally trigger an immediate search.",
    inputSchema:{type:"object",properties:{
      tmdb_id:{type:"string",description:"TMDB ID"},
      quality_profile_id:{type:"string"},
      library_id:{type:"string"},
      monitoring_status:{type:"string",description:"all|future|missing|existing|first|latest|none (default: all)"},
      search:{type:"boolean",description:"Trigger search after adding (default: false)"},
    },required:["tmdb_id","quality_profile_id","library_id"]}},

  { name:"update_series",
    description:"Update series metadata, quality profile, or monitoring status.",
    inputSchema:{type:"object",properties:{
      id:{type:"string"},
      monitoring_status:{type:"string",description:"all|future|missing|existing|first|latest|none"},
      quality_profile_id:{type:"string"},
      library_id:{type:"string"},
      series_type:{type:"string",description:"standard|daily|anime"},
    },required:["id"]}},

  { name:"delete_series",
    description:"Remove a series from Loom (does NOT delete media files by default).",
    inputSchema:{type:"object",properties:{
      id:{type:"string"},
      delete_files:{type:"boolean",description:"Also delete media files (default: false)"},
    },required:["id"]}},

  { name:"refresh_series",
    description:"Refresh metadata for a series from TMDB/TVDB.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"set_series_monitoring",
    description:"Change monitoring status for a series (all/future/missing/existing/first/latest/none).",
    inputSchema:{type:"object",properties:{
      id:{type:"string"},
      status:{type:"string",description:"all|future|missing|existing|first|latest|none"},
    },required:["id","status"]}},

  { name:"list_series_episodes",
    description:"List episodes for a series. Filter by season and/or missing_only.",
    inputSchema:{type:"object",properties:{
      series_id:{type:"string"},
      season:{type:"number"},
      missing_only:{type:"boolean"},
    },required:["series_id"]}},

  { name:"bulk_update_series",
    description:"Update monitoring status for multiple series at once.",
    inputSchema:{type:"object",properties:{
      series_ids:{type:"array",items:{type:"string"}},
      monitoring_status:{type:"string"},
      quality_profile_id:{type:"string"},
    },required:["series_ids"]}},

  // ── Movies ──────────────────────────────────────────────────────────────────
  { name:"list_movies",
    description:"List all movies. Add missing_only:true to see what needs downloading.",
    inputSchema:{type:"object",properties:{
      missing_only:{type:"boolean"},
      monitored_only:{type:"boolean"},
    }}},

  { name:"get_movie",
    description:"Get full details of one movie.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"lookup_movie",
    description:"Search TMDB for a movie before adding.",
    inputSchema:{type:"object",properties:{q:{type:"string"}},required:["q"]}},

  { name:"add_movie",
    description:"Add a new movie to Loom.",
    inputSchema:{type:"object",properties:{
      title:{type:"string"},
      tmdb_id:{type:"string"},
      year:{type:"number"},
      quality_profile_id:{type:"string"},
      library_id:{type:"string"},
      monitoring_status:{type:"string",description:"monitored|unmonitored"},
      search:{type:"boolean"},
    },required:["title","quality_profile_id","library_id"]}},

  { name:"update_movie",
    description:"Update a movie's quality profile, monitoring, or library.",
    inputSchema:{type:"object",properties:{
      id:{type:"string"},
      monitoring_status:{type:"string"},
      quality_profile_id:{type:"string"},
      library_id:{type:"string"},
    },required:["id"]}},

  { name:"delete_movie",
    description:"Remove a movie from Loom.",
    inputSchema:{type:"object",properties:{
      id:{type:"string"},
      delete_files:{type:"boolean"},
    },required:["id"]}},

  // ── Search ──────────────────────────────────────────────────────────────────
  { name:"trigger_search",
    description:"Search indexers and grab the best result for an episode or movie. May take 30-120s.",
    inputSchema:{type:"object",properties:{
      media_type:{type:"string",enum:["episode","series","movie"]},
      media_id:{type:"string"},
      title:{type:"string"},
      year:{type:"number"},
      season:{type:"number"},
      episode:{type:"number"},
      quality_profile_id:{type:"string"},
      tvdb_id:{type:"string"},
      tmdb_id:{type:"string"},
      imdb_id:{type:"string"},
    },required:["media_type","media_id","title","quality_profile_id"]}},

  { name:"search_missing_episodes",
    description:"Find and trigger searches for all missing monitored episodes in a series. Limit controls how many to search at once.",
    inputSchema:{type:"object",properties:{
      series_id:{type:"string"},
      season:{type:"number"},
      quality_profile_id:{type:"string"},
      limit:{type:"number",description:"Max searches (default 5, max 20)"},
    },required:["series_id"]}},

  // ── Indexers ─────────────────────────────────────────────────────────────────
  { name:"list_indexers",
    description:"List all configured indexers with health, latency, and supported capabilities.",
    inputSchema:{type:"object",properties:{}}},

  { name:"get_indexer_definitions",
    description:"List all available indexer types (Jackett, Prowlarr, EZTV, etc) that can be configured.",
    inputSchema:{type:"object",properties:{}}},

  { name:"add_indexer",
    description:"Add a new indexer. Use get_indexer_definitions first to get the definition_id and required fields.",
    inputSchema:{type:"object",properties:{
      definition_id:{type:"string",description:"Indexer type ID from get_indexer_definitions"},
      name:{type:"string"},
      enabled:{type:"boolean"},
      priority:{type:"number"},
      settings:{type:"object",description:"Indexer-specific settings (api_key, base_url, etc)"},
    },required:["definition_id","name"]}},

  { name:"update_indexer",
    description:"Update an indexer configuration.",
    inputSchema:{type:"object",properties:{
      id:{type:"string"},
      name:{type:"string"},
      enabled:{type:"boolean"},
      priority:{type:"number"},
      settings:{type:"object"},
    },required:["id"]}},

  { name:"delete_indexer",
    description:"Remove an indexer.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"test_indexer",
    description:"Test connectivity and query capabilities of an indexer.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  // ── Download Clients ─────────────────────────────────────────────────────────
  { name:"list_download_clients",
    description:"List configured download clients (qBittorrent, Deluge, SABnzbd, etc) with connection status.",
    inputSchema:{type:"object",properties:{}}},

  { name:"get_download_queue",
    description:"Show current downloads in progress across all or a specific download client.",
    inputSchema:{type:"object",properties:{
      client_id:{type:"string",description:"Specific client ID (omit for all clients)"},
    }}},

  { name:"remove_download",
    description:"Remove a download item from the client queue.",
    inputSchema:{type:"object",properties:{
      client_id:{type:"string"},
      item_id:{type:"string"},
      delete_files:{type:"boolean"},
    },required:["client_id","item_id"]}},

  { name:"add_download_client",
    description:"Add a new download client.",
    inputSchema:{type:"object",properties:{
      name:{type:"string"},
      kind:{type:"string",description:"qbittorrent|deluge|rtorrent|transmission|nzbget|sabnzbd"},
      host:{type:"string"},
      port:{type:"number"},
      username:{type:"string"},
      password:{type:"string"},
      api_key:{type:"string"},
      category:{type:"string"},
      save_path:{type:"string"},
      enabled:{type:"boolean"},
    },required:["name","kind"]}},

  // ── Import Pipeline ──────────────────────────────────────────────────────────
  { name:"list_import_history",
    description:"Show what files have been imported, when, and their status.",
    inputSchema:{type:"object",properties:{limit:{type:"number"}}}},

  { name:"list_import_decisions",
    description:"Show pending import decisions — files found but not yet imported.",
    inputSchema:{type:"object",properties:{}}},

  { name:"scan_import_folder",
    description:"Manually scan a folder for media files to import.",
    inputSchema:{type:"object",properties:{
      path:{type:"string",description:"Absolute path to scan"},
    },required:["path"]}},

  // ── Libraries ────────────────────────────────────────────────────────────────
  { name:"list_libraries",
    description:"List media libraries (root folders) configured in Loom.",
    inputSchema:{type:"object",properties:{}}},

  { name:"scan_library",
    description:"Trigger a scan of a library to detect new or changed files.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  // ── Workflows ────────────────────────────────────────────────────────────────
  { name:"list_workflows",
    description:"List recent download/import workflows. Filter by status: pending/active/done/failed/cancelled.",
    inputSchema:{type:"object",properties:{
      status:{type:"string"},
      limit:{type:"number"},
    }}},

  { name:"get_workflow",
    description:"Get full details and event history for a workflow.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"cancel_workflow",
    description:"Cancel a running workflow.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"retry_workflow",
    description:"Retry a failed workflow.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  // ── Rolling Search ──────────────────────────────────────────────────────────
  { name:"rolling_search_status",
    description:"Get status and config of the background rolling search (auto-searches for missing media).",
    inputSchema:{type:"object",properties:{}}},

  { name:"trigger_rolling_search",
    description:"Immediately trigger a rolling search cycle.",
    inputSchema:{type:"object",properties:{}}},

  { name:"update_rolling_search_config",
    description:"Update rolling search settings (interval, enabled, etc).",
    inputSchema:{type:"object",properties:{
      enabled:{type:"boolean"},
      interval_minutes:{type:"number"},
      max_searches_per_run:{type:"number"},
    }}},

  // ── Calendar ─────────────────────────────────────────────────────────────────
  { name:"get_calendar",
    description:"Show upcoming episodes and movies airing in the next N days.",
    inputSchema:{type:"object",properties:{
      days:{type:"number",description:"Days ahead to look (default 7)"},
    }}},

  // ── Quality Profiles ─────────────────────────────────────────────────────────
  { name:"list_quality_profiles",
    description:"List quality profiles. Returns IDs needed for add_series/add_movie/trigger_search.",
    inputSchema:{type:"object",properties:{}}},

  // ── Custom Formats ───────────────────────────────────────────────────────────
  { name:"list_custom_formats",
    description:"List custom format definitions used for quality scoring.",
    inputSchema:{type:"object",properties:{}}},

  // ── Import Lists ──────────────────────────────────────────────────────────────
  { name:"list_import_lists",
    description:"List configured import lists (Trakt, IMDB, etc) that auto-add media.",
    inputSchema:{type:"object",properties:{}}},

  { name:"sync_import_list",
    description:"Trigger a manual sync of an import list.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  // ── Notifications ─────────────────────────────────────────────────────────────
  { name:"list_notifications",
    description:"List notification connections (Plex, Discord, Slack, etc).",
    inputSchema:{type:"object",properties:{}}},

  { name:"test_notification",
    description:"Send a test notification to verify a connection works.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  // ── Search Debug ──────────────────────────────────────────────────────────────
  { name:"list_search_logs",
    description:"List search debug logs. Filter by outcome/title/media. Use to diagnose why searches fail.",
    inputSchema:{type:"object",properties:{
      outcome:{type:"string",description:"grabbed|no_results|all_rejected|grab_failed|already_grabbed"},
      media_type:{type:"string"},
      media_id:{type:"string"},
      title:{type:"string"},
      limit:{type:"number"},
    }}},

  { name:"get_search_log",
    description:"Get full search details: query tiers, all indexer results, rejection reasons, what was grabbed.",
    inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]}},

  { name:"search_debug_stats",
    description:"Aggregate search statistics: outcomes, top reject reasons.",
    inputSchema:{type:"object",properties:{}}},

  // ── Connect (download client connections) ─────────────────────────────────────
  { name:"list_connect_connections",
    description:"List all media server connections (Plex, Emby, Jellyfin) configured in Loom.",
    inputSchema:{type:"object",properties:{}}},

];

// ─── Server setup ─────────────────────────────────────────────────────────────

const server = new Server(
  { name:"loom-mcp", version:"3.0.0" },
  { capabilities: { tools: {} } }
);

server.setRequestHandler(ListToolsRequestSchema, async () => ({ tools: TOOLS }));

server.setRequestHandler(CallToolRequestSchema, async (req) => {
  const { name, arguments: args } = req.params;
  try {
    return { content:[{ type:"text", text: await handle(name, args||{}) }] };
  } catch(err) {
    return { content:[{ type:"text", text:`❌ ${err.message}` }], isError:true };
  }
});

// ─── Handlers ────────────────────────────────────────────────────────────────

async function handle(name, a) {
  switch(name) {

  // ── System ──────────────────────────────────────────────────────────────────

  case "system_status": {
    const s = await GET("/api/v1/system/status");
    let o = `# Loom ${s.version}\n\n**Commit:** ${s.commit}  **Build:** ${s.buildDate}\n**DB:** ${s.engine}\n**URL:** ${LOOM_BASE_URL}\n`;
    const h = await GET("/api/v1/system/health").catch(()=>null);
    if (h) {
      o += "\n## Health\n";
      for (const c of (Array.isArray(h)?h:Object.values(h))) {
        const icon = c.status==="ok"?"✅":c.status==="warning"?"⚠️":"❌";
        o += `${icon} **${c.name||c.check}**: ${c.status}${c.message?" — "+c.message:""}\n`;
      }
    }
    return o;
  }

  case "system_logs": {
    const params = new URLSearchParams();
    if (a.level) params.set("level", a.level);
    params.set("limit", String(a.limit||50));
    const data = await GET(`/api/v1/system/logs?${params}`);
    const logs = data?.logs ?? data?.entries ?? (Array.isArray(data)?data:[]);
    let o = `# System Logs (${logs.length})\n\n`;
    for (const l of logs) {
      const icon = l.level==="error"?"❌":l.level==="warn"?"⚠️":"ℹ️";
      o += `${icon} [${l.level||"?"}] ${l.time||l.timestamp||""} ${l.message||l.msg||JSON.stringify(l)}\n`;
    }
    return o;
  }

  case "clear_system_logs": {
    await DELETE("/api/v1/system/logs");
    return "✅ System logs cleared.";
  }

  case "browse_filesystem": {
    const p = a.path ? encodeURIComponent(a.path) : "";
    const data = await GET(`/api/v1/filesystem${p?"?path="+p:""}`);
    const entries = data?.entries ?? data?.files ?? (Array.isArray(data)?data:[]);
    let o = `# Filesystem: ${a.path||"/"}\n\n`;
    for (const e of entries) {
      const icon = e.type==="directory"?"📁":"📄";
      o += `${icon} ${e.name||e.path}${e.type==="directory"?"/":""}  ${e.size!=null?mb(e.size):""}\n`;
    }
    return o;
  }

  // ── Series ──────────────────────────────────────────────────────────────────

  case "list_series": {
    const data = await GET("/api/v1/series");
    let series = data?.data ?? data;
    if (a.monitored_only) series = series.filter(s=>s.monitored!==false);
    if (a.with_missing)   series = series.filter(s=>(s.episodeStats?.missingEpisodes??0)>0);
    series = series.sort((a,b)=>(b.episodeStats?.missingEpisodes??0)-(a.episodeStats?.missingEpisodes??0));
    let o = `# Series (${series.length})\n\n${"ID".padEnd(32)} ${"Title".padEnd(38)} Miss  Tot  DL\n${"-".repeat(84)}\n`;
    for (const s of series) {
      const st = s.episodeStats||{};
      o += `${(s.id||"").padEnd(32)} ${(s.title||"").slice(0,36).padEnd(38)} ${String(st.missingEpisodes??"?").padStart(4)}  ${String(st.totalEpisodes??"?").padStart(3)} ${String(st.downloadedEpisodes??"?").padStart(3)}\n`;
    }
    return o;
  }

  case "get_series": {
    const s = await GET(`/api/v1/series/${a.id}`);
    const st = s.episodeStats||{};
    return `# ${s.title} (${s.year??'?'})\n\n**ID:** ${s.id}\n**TVDB:** ${s.tvdbId??'—'} | **TMDB:** ${s.tmdbId??'—'} | **IMDB:** ${s.imdbId??'—'}\n**Quality Profile:** ${s.qualityProfileId}\n**Monitoring:** ${s.monitoringStatus} | **Type:** ${s.seriesType??'?'}\n**Status:** ${s.status} | **Network:** ${s.network??'?'}\n**Library:** ${s.libraryId??'?'}\n\n## Episodes\nTotal: ${st.totalEpisodes} | Aired: ${st.airedEpisodes} | Downloaded: ${st.downloadedEpisodes}\nMonitored: ${st.monitoredEpisodes} | **Missing: ${st.missingEpisodes}**\n`;
  }

  case "lookup_series": {
    const data = await GET(`/api/v1/series/lookup?q=${encodeURIComponent(a.q)}`);
    const results = data?.data ?? data;
    let o = `# Series Lookup: "${a.q}"\n\n`;
    for (const s of (results||[]).slice(0,10)) {
      o += `**${s.title}** (${s.year??'?'})  TMDB=${s.tmdbId??'—'}  TVDB=${s.tvdbId??'—'}\n  ${(s.overview||'').slice(0,120)}\n\n`;
    }
    return o;
  }

  case "add_series": {
    const body = {
      tmdbId: a.tmdb_id,
      qualityProfileId: a.quality_profile_id,
      libraryId: a.library_id,
      monitoringStatus: a.monitoring_status||"all",
      search: a.search||false,
    };
    const s = await POST("/api/v1/series", body);
    return `✅ Added: **${s.title}** (${s.year??'?'})  id=${s.id}\nMonitoring: ${s.monitoringStatus} | Profile: ${s.qualityProfileId}`;
  }

  case "update_series": {
    const body = {};
    if (a.monitoring_status)   body.monitoringStatus   = a.monitoring_status;
    if (a.quality_profile_id)  body.qualityProfileId   = a.quality_profile_id;
    if (a.library_id)          body.libraryId          = a.library_id;
    if (a.series_type)         body.seriesType         = a.series_type;
    const s = await PUT(`/api/v1/series/${a.id}`, body);
    return `✅ Updated: **${s.title}**\nMonitoring: ${s.monitoringStatus} | Profile: ${s.qualityProfileId}`;
  }

  case "delete_series": {
    const qs = a.delete_files ? "?deleteFiles=true" : "";
    await DELETE(`/api/v1/series/${a.id}${qs}`);
    return `✅ Series ${a.id} removed${a.delete_files?" (files deleted)":""}.`;
  }

  case "refresh_series": {
    await POST(`/api/v1/series/${a.id}/refresh`);
    return `✅ Refresh triggered for ${a.id}.`;
  }

  case "set_series_monitoring": {
    const s = await PUT(`/api/v1/series/${a.id}/monitoring`, { status: a.status });
    return `✅ **${s.title}** monitoring set to **${s.monitoringStatus}**.`;
  }

  case "list_series_episodes": {
    const s = await GET(`/api/v1/series/${a.series_id}`);
    let seasonNums;
    if (a.season) {
      seasonNums = [a.season];
    } else {
      const ss = await GET(`/api/v1/series/${a.series_id}/seasons`);
      seasonNums = (ss||[]).map(x=>x.seasonNumber).filter(n=>n>0).sort((a,b)=>a-b);
    }
    let o = `# ${s.title} — Episodes\n\n`;
    let totalMissing = 0;
    for (const sn of seasonNums) {
      const data = await GET(`/api/v1/series/${a.series_id}/seasons/${sn}/episodes`);
      const eps = data?.data ?? data;
      const filtered = a.missing_only ? eps.filter(e=>!e.hasFile) : eps;
      if (!filtered.length) continue;
      const miss = eps.filter(e=>!e.hasFile&&e.monitored).length;
      totalMissing += miss;
      o += `## Season ${sn}  (${miss} missing)\n`;
      for (const e of filtered) {
        const icon = e.hasFile?"✅":(e.monitored?"⬇️ ":"⏸ ");
        o += `  ${icon} E${String(e.episodeNumber??'?').padStart(2,'0')} ${(e.title||'?').slice(0,50).padEnd(50)} ${e.airDate??'?'}\n`;
      }
      o += "\n";
    }
    o += `**Total missing monitored: ${totalMissing}**\n`;
    return o;
  }

  case "bulk_update_series": {
    const body = { ids: a.series_ids };
    if (a.monitoring_status)  body.monitoringStatus  = a.monitoring_status;
    if (a.quality_profile_id) body.qualityProfileId  = a.quality_profile_id;
    const r = await POST("/api/v1/series/bulk", body);
    const count = Array.isArray(r) ? r.length : (r?.updated??'?');
    return `✅ Bulk updated ${count} series.`;
  }

  // ── Movies ──────────────────────────────────────────────────────────────────

  case "list_movies": {
    const data = await GET("/api/v1/movies");
    let movies = data?.data ?? data;
    if (a.monitored_only) movies = movies.filter(m=>m.monitored!==false);
    if (a.missing_only)   movies = movies.filter(m=>!m.hasFile);
    movies = movies.sort((a,b)=>(a.title||"").localeCompare(b.title||""));
    let o = `# Movies (${movies.length})\n\n`;
    for (const m of movies) {
      const icon = m.hasFile?"✅":(m.monitored?"⬇️ ":"⏸ ");
      const q = m.movieFile?.quality?.quality?.name??"";
      o += `${icon} **${m.title}** (${m.year??'?'})  ${q}  [${m.id}]\n`;
    }
    return o;
  }

  case "get_movie": {
    const m = await GET(`/api/v1/movies/${a.id}`);
    return `# ${m.title} (${m.year??'?'})\n\n**ID:** ${m.id}\n**TMDB:** ${m.tmdbId??'—'} | **IMDB:** ${m.imdbId??'—'}\n**Quality Profile:** ${m.qualityProfileId}\n**Monitoring:** ${m.monitoringStatus}\n**Has File:** ${m.hasFile} | **File:** ${m.movieFile?.relativePath??'—'}\n**Size:** ${mb(m.movieFile?.size)}\n`;
  }

  case "lookup_movie": {
    const data = await GET(`/api/v1/movies/lookup?q=${encodeURIComponent(a.q)}`);
    const results = data?.data ?? data;
    let o = `# Movie Lookup: "${a.q}"\n\n`;
    for (const m of (results||[]).slice(0,10)) {
      o += `**${m.title}** (${m.year??'?'})  TMDB=${m.tmdbId??'—'}  IMDB=${m.imdbId??'—'}\n  ${(m.overview||'').slice(0,120)}\n\n`;
    }
    return o;
  }

  case "add_movie": {
    const body = {
      title: a.title, year: a.year,
      quality_profile_id: a.quality_profile_id,
      library_id: a.library_id,
      search: a.search||false,
    };
    if (a.tmdb_id) body.tmdb_id = a.tmdb_id;
    if (a.monitoring_status) body.monitoring_status = a.monitoring_status;
    const m = await POST("/api/v1/movies", body);
    return `✅ Added: **${m.title}** (${m.year??'?'})  id=${m.id}`;
  }

  case "update_movie": {
    const body = {};
    if (a.monitoring_status)  body.monitoring_status  = a.monitoring_status;
    if (a.quality_profile_id) body.quality_profile_id = a.quality_profile_id;
    if (a.library_id)         body.library_id         = a.library_id;
    const m = await PUT(`/api/v1/movies/${a.id}`, body);
    return `✅ Updated: **${m.title}**  monitoring=${m.monitoringStatus}`;
  }

  case "delete_movie": {
    const qs = a.delete_files ? "?deleteFiles=true" : "";
    await DELETE(`/api/v1/movies/${a.id}${qs}`);
    return `✅ Movie ${a.id} removed${a.delete_files?" (files deleted)":""}.`;
  }

  // ── Search ──────────────────────────────────────────────────────────────────

  case "trigger_search": {
    const body = { media_type:a.media_type, media_id:a.media_id, title:a.title, quality_profile_id:a.quality_profile_id };
    if (a.year)    body.year    = a.year;
    if (a.season)  body.season  = a.season;
    if (a.episode) body.episode = a.episode;
    if (a.tvdb_id) body.tvdb_id = a.tvdb_id;
    if (a.tmdb_id) body.tmdb_id = a.tmdb_id;
    if (a.imdb_id) body.imdb_id = a.imdb_id;
    const r = await POST("/api/v1/autosearch", body);
    if (!r) return "⚠️ Empty response";
    let o = `# Search: ${a.title}${a.season?` S${String(a.season).padStart(2,'0')}${a.episode?`E${String(a.episode).padStart(2,'0')}`:""}`:"  "}\n\n`;
    if (r.grabbed) {
      const g = r.grabbed;
      o += `✅ **GRABBED:** ${g.title}\n   Quality tier=${g.quality_tier} score=${g.composite_score?.toFixed(1)} size=${gb(g.size)}\n   Indexer: ${g.indexer_id} | Client: ${g.client_id}\n`;
    } else {
      o += `❌ **NOT GRABBED** — ${r.reason||"unknown"}\n`;
    }
    o += `\nConsidered: ${r.considered} | Rejected: ${r.rejected}\n`;
    if (r.top_rejects?.length) {
      o += "\nTop reject reasons:\n";
      for (const rj of r.top_rejects.slice(0,8)) o += `  ${rj.reason}: ${rj.count}\n`;
    }
    return o;
  }

  case "search_missing_episodes": {
    const series = await GET(`/api/v1/series/${a.series_id}`);
    const qpId = a.quality_profile_id || series.qualityProfileId;
    if (!qpId) return "❌ No quality profile; pass quality_profile_id.";
    const limit = Math.min(a.limit||5, 20);
    const ss = await GET(`/api/v1/series/${a.series_id}/seasons`);
    const seasonNums = (ss||[]).map(s=>s.seasonNumber).filter(n=>n>0&&(!a.season||n===a.season));
    const missing = [];
    for (const sn of seasonNums) {
      if (missing.length>=limit) break;
      const data = await GET(`/api/v1/series/${a.series_id}/seasons/${sn}/episodes`);
      for (const e of (data?.data??data)) {
        if (!e.hasFile&&e.monitored) missing.push({season:sn,episode:e.episodeNumber});
        if (missing.length>=limit) break;
      }
    }
    if (!missing.length) return `✅ No missing monitored episodes for ${series.title}.`;
    let o = `# Searching Missing: ${series.title}\n\nSearching ${missing.length} episodes...\n\n`;
    const base = { media_type:"episode", media_id:a.series_id, title:series.title, year:series.year, quality_profile_id:qpId };
    if (series.tvdbId) base.tvdb_id = String(series.tvdbId);
    if (series.tmdbId) base.tmdb_id = String(series.tmdbId);
    for (const ep of missing) {
      let status;
      try {
        const r = await POST("/api/v1/autosearch", {...base, season:ep.season, episode:ep.episode});
        status = r?.grabbed ? `✅ GRABBED: ${r.grabbed.title.slice(0,60)}` : `❌ ${r?.reason||"?"} (${r?.considered}→${r?.rejected})`;
      } catch(err) { status = `⚠️ ${err.message.slice(0,80)}`; }
      o += `S${String(ep.season).padStart(2,'0')}E${String(ep.episode).padStart(2,'0')} — ${status}\n`;
    }
    return o;
  }

  // ── Indexers ─────────────────────────────────────────────────────────────────

  case "list_indexers": {
    const data = await GET("/api/v1/indexers");
    const idxs = data?.indexers ?? data?.data ?? data;
    let o = `# Indexers (${idxs.length})\n\n`;
    for (const idx of idxs) {
      const h = idx.health||{};
      const icon = h.status==="ok"?"✅":h.status==="degraded"?"⚠️":"❌";
      o += `${icon} **${idx.name}** (id=${idx.id})\n`;
      o += `   Enabled: ${idx.enabled} | Priority: ${idx.priority} | Kind: ${idx.kind}\n`;
      o += `   Health: ${h.status??'?'} | Latency: ${h.latency_ms??'?'}ms`;
      if (h.last_checked_at) o += ` | Checked: ${ago(h.last_checked_at)}`;
      o += "\n";
      if (h.error_message) o += `   ⚠️ ${h.error_message}\n`;
      const caps = idx.capabilities;
      if (caps) {
        const modes = Object.entries(caps.search_modes||{}).map(([m,i])=>`${m}:${i.available?"✓":"✗"}`);
        if (modes.length) o += `   Modes: ${modes.join(" | ")}\n`;
        if (caps.supported_params?.length) o += `   Params: ${caps.supported_params.join(", ")}\n`;
      }
      o += "\n";
    }
    return o;
  }

  case "get_indexer_definitions": {
    const defs = await GET("/api/v1/indexers/definitions");
    const list = defs?.definitions ?? defs?.data ?? (Array.isArray(defs)?defs:[]);
    let o = `# Indexer Definitions (${list.length})\n\n`;
    for (const d of list) {
      o += `**${d.name}**  id=${d.id}  protocol=${d.protocol??'?'}  type=${d.type??'?'}\n  ${(d.description||'').slice(0,100)}\n\n`;
    }
    return o;
  }

  case "add_indexer": {
    const body = { definitionId: a.definition_id, name: a.name, enabled: a.enabled??true, priority: a.priority??25, settings: a.settings||{} };
    const idx = await POST("/api/v1/indexers", body);
    return `✅ Added indexer: **${idx.name}** (id=${idx.id})`;
  }

  case "update_indexer": {
    const body = {};
    if (a.name    !== undefined) body.name     = a.name;
    if (a.enabled !== undefined) body.enabled  = a.enabled;
    if (a.priority!== undefined) body.priority = a.priority;
    if (a.settings!== undefined) body.settings = a.settings;
    const idx = await PATCH(`/api/v1/indexers/${a.id}`, body);
    return `✅ Updated indexer: **${idx.name||a.id}**`;
  }

  case "delete_indexer": {
    await DELETE(`/api/v1/indexers/${a.id}`);
    return `✅ Indexer ${a.id} removed.`;
  }

  case "test_indexer": {
    const r = await POST(`/api/v1/indexers/${a.id}/test`);
    const status = r?.status ?? r?.success ?? r;
    return `Indexer test result: ${JSON.stringify(status, null, 2)}`;
  }

  // ── Download Clients ─────────────────────────────────────────────────────────

  case "list_download_clients": {
    const data = await GET("/api/v1/download-clients");
    const clients = data?.data ?? data?.clients ?? (Array.isArray(data)?data:[]);
    let o = `# Download Clients (${clients.length})\n\n`;
    for (const c of clients) {
      const h = c.health||{};
      const icon = h.status==="ok"?"✅":h.status==="degraded"?"⚠️":"❌";
      o += `${icon} **${c.name}** (id=${c.id})  kind=${c.kind}  enabled=${c.enabled}\n`;
      o += `   Health: ${h.status??'?'} | Latency: ${h.latency_ms??'?'}ms\n`;
      if (c.host) o += `   ${c.host}:${c.port??''}\n`;
      o += "\n";
    }
    return o;
  }

  case "get_download_queue": {
    let o = `# Download Queue\n\n`;
    if (a.client_id) {
      const items = await GET(`/api/v1/download-clients/${a.client_id}/items`);
      const list = Array.isArray(items)?items:(items?.items??[]);
      for (const item of list) {
        o += formatDownloadItem(item);
      }
      if (!list.length) o += "Queue is empty.\n";
    } else {
      const data = await GET("/api/v1/download-clients");
      const clients = data?.data ?? data?.clients ?? (Array.isArray(data)?data:[]);
      for (const c of clients.filter(c=>c.enabled)) {
        o += `## ${c.name}\n`;
        const items = await GET(`/api/v1/download-clients/${c.id}/items`).catch(()=>null);
        const list = items ? (Array.isArray(items)?items:(items?.items??[])) : [];
        if (!list.length) { o += "  (empty)\n\n"; continue; }
        for (const item of list) o += formatDownloadItem(item, "  ");
        o += "\n";
      }
    }
    return o;
  }

  case "remove_download": {
    const body = { clientId: a.client_id, itemId: a.item_id };
    if (a.delete_files) body.deleteFiles = true;
    await POST(`/api/v1/download-clients/${a.client_id}/remove`, body);
    return `✅ Download item ${a.item_id} removed.`;
  }

  case "add_download_client": {
    const settings = {};
    if (a.host)      settings.host     = a.host;
    if (a.port)      settings.port     = a.port;
    if (a.username)  settings.username = a.username;
    if (a.password)  settings.password = a.password;
    if (a.api_key)   settings.apiKey   = a.api_key;
    if (a.category)  settings.category = a.category;
    if (a.save_path) settings.savePath = a.save_path;
    const body = { name:a.name, kind:a.kind, enabled:a.enabled??true, settings };
    const c = await POST("/api/v1/download-clients", body);
    return `✅ Added download client: **${c.name}** (id=${c.id})`;
  }

  // ── Import Pipeline ──────────────────────────────────────────────────────────

  case "list_import_history": {
    const params = new URLSearchParams();
    params.set("limit", String(a.limit||50));
    const data = await GET(`/api/v1/imports/history?${params}`);
    const entries = data?.entries ?? data?.history ?? (Array.isArray(data)?data:[]);
    let o = `# Import History (${entries.length})\n\n`;
    for (const e of entries) {
      const icon = e.status==="imported"?"✅":e.status==="failed"?"❌":"⚠️";
      o += `${icon} ${e.sourceTitle||e.title||'?'}  ${ago(e.importedAt||e.createdAt)}\n`;
      o += `   Status: ${e.status} | Quality: ${e.quality?.quality?.name??'?'} | Size: ${mb(e.size)}\n`;
      if (e.error) o += `   ❌ ${e.error}\n`;
      o += "\n";
    }
    return o;
  }

  case "list_import_decisions": {
    const data = await GET("/api/v1/imports/decisions");
    const decisions = data?.decisions ?? data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Import Decisions (${decisions.length})\n\n`;
    for (const d of decisions) {
      const icon = d.approved?"✅":"❌";
      o += `${icon} ${d.localItem?.relativePath||d.path||'?'}\n`;
      if (!d.approved) o += `   Rejected: ${d.rejections?.map(r=>r.reason||r).join(", ")||'?'}\n`;
      o += "\n";
    }
    return o;
  }

  case "scan_import_folder": {
    const r = await POST("/api/v1/imports/scan", { path: a.path });
    const count = r?.count ?? r?.files ?? '?';
    return `✅ Scan triggered for ${a.path}. Found: ${count} files.`;
  }

  // ── Libraries ────────────────────────────────────────────────────────────────

  case "list_libraries": {
    const data = await GET("/api/v1/libraries");
    const libs = data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Libraries (${libs.length})\n\n`;
    for (const l of libs) {
      o += `**${l.name}** (id=${l.id})\n  Path: ${l.path}\n  Type: ${l.mediaType??l.type??'?'} | Free: ${gb(l.freeSpace)}\n\n`;
    }
    return o;
  }

  case "scan_library": {
    await POST(`/api/v1/libraries/${a.id}/scan`);
    return `✅ Library ${a.id} scan triggered.`;
  }

  // ── Workflows ────────────────────────────────────────────────────────────────

  case "list_workflows": {
    const data = await GET("/api/v1/workflows");
    let wfs = Array.isArray(data)?data:(data?.data??[]);
    if (a.status) wfs = wfs.filter(w=>w.status===a.status);
    wfs = wfs.slice(0,a.limit||20);
    let o = `# Workflows (${wfs.length})\n\n`;
    for (const w of wfs) {
      const icon = {done:"✅",active:"⏳",failed:"❌",cancelled:"🚫",pending:"🔄"}[w.status]??"❓";
      o += `${icon} **${(w.id||'').slice(0,8)}…** ${w.mediaType??'?'} — ${w.mediaTitle||w.downloadTitle||'?'}\n`;
      o += `   ${w.status} | created ${ago(w.createdAt)} | updated ${ago(w.updatedAt)}\n`;
      if (w.error) o += `   ❌ ${w.error}\n`;
      o += "\n";
    }
    return o;
  }

  case "get_workflow": {
    const w = await GET(`/api/v1/workflows/${a.id}`);
    const events = await GET(`/api/v1/workflows/${a.id}/events`).catch(()=>[]);
    let o = `# Workflow ${a.id}\n\n**Status:** ${w.status}\n**Media:** ${w.mediaType??'?'} — ${w.mediaTitle||w.downloadTitle||'?'}\n**Created:** ${ago(w.createdAt)} | **Updated:** ${ago(w.updatedAt)}\n`;
    if (w.error) o += `**Error:** ${w.error}\n`;
    if (events?.length) {
      o += `\n## Events (${events.length})\n`;
      for (const e of events) {
        o += `  ${ago(e.createdAt||e.timestamp)} — ${e.type||e.event}: ${e.message||''}\n`;
        if (e.data) { const d = typeof e.data==="string"?e.data:JSON.stringify(e.data); o += `    ${d.slice(0,200)}\n`; }
      }
    }
    return o;
  }

  case "cancel_workflow": {
    await POST(`/api/v1/workflows/${a.id}/cancel`);
    return `✅ Workflow ${a.id} cancelled.`;
  }

  case "retry_workflow": {
    await POST(`/api/v1/workflows/${a.id}/retry`);
    return `✅ Workflow ${a.id} retried.`;
  }

  // ── Rolling Search ──────────────────────────────────────────────────────────

  case "rolling_search_status": {
    const st = await GET("/api/v1/rolling-search/status");
    const cfg = await GET("/api/v1/rolling-search/config");
    let o = `# Rolling Search\n\n## Status\n${JSON.stringify(st, null, 2)}\n\n## Config\n${JSON.stringify(cfg, null, 2)}\n`;
    return o;
  }

  case "trigger_rolling_search": {
    await POST("/api/v1/rolling-search/trigger");
    return "✅ Rolling search triggered.";
  }

  case "update_rolling_search_config": {
    const body = {};
    if (a.enabled           !== undefined) body.enabled           = a.enabled;
    if (a.interval_minutes  !== undefined) body.intervalMinutes   = a.interval_minutes;
    if (a.max_searches_per_run !== undefined) body.maxSearchesPerRun = a.max_searches_per_run;
    const cfg = await PUT("/api/v1/rolling-search/config", body);
    return `✅ Rolling search config updated:\n${JSON.stringify(cfg, null, 2)}`;
  }

  // ── Calendar ─────────────────────────────────────────────────────────────────

  case "get_calendar": {
    const days = a.days || 7;
    const start = new Date().toISOString().slice(0,10);
    const end = new Date(Date.now() + days*86400000).toISOString().slice(0,10);
    const data = await GET(`/api/v1/calendar?start=${start}&end=${end}`);
    const items = data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Calendar (next ${days} days)\n\n`;
    for (const item of items) {
      o += `📅 **${item.title||item.seriesTitle||'?'}**`;
      if (item.episodeTitle) o += ` — ${item.episodeTitle}`;
      if (item.airDate) o += `  (${item.airDate})`;
      o += "\n";
    }
    if (!items.length) o += "Nothing scheduled.\n";
    return o;
  }

  // ── Quality Profiles ─────────────────────────────────────────────────────────

  case "list_quality_profiles": {
    const data = await GET("/api/v1/quality-profiles");
    const profiles = data?.data ?? data;
    let o = `# Quality Profiles (${profiles.length})\n\n`;
    for (const p of profiles) {
      o += `**${p.name}**  id=${p.id}  upgradeAllowed=${p.upgradeAllowed}\n`;
      try {
        const items = typeof p.items==="string"?JSON.parse(p.items):p.items;
        const allowed = (items||[]).filter(i=>i.allowed).map(i=>i.name||i.id).join(", ");
        if (allowed) o += `  Allowed: ${allowed}\n`;
      } catch {}
      o += "\n";
    }
    return o;
  }

  // ── Custom Formats ───────────────────────────────────────────────────────────

  case "list_custom_formats": {
    const data = await GET("/api/v1/custom-formats");
    const fmts = data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Custom Formats (${fmts.length})\n\n`;
    for (const f of fmts) {
      o += `**${f.name}**  id=${f.id}  score=${f.score??0}\n  ${(f.specifications||[]).map(s=>s.name).join(", ")}\n\n`;
    }
    return o;
  }

  // ── Import Lists ──────────────────────────────────────────────────────────────

  case "list_import_lists": {
    const data = await GET("/api/v1/import-lists");
    const lists = data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Import Lists (${lists.length})\n\n`;
    for (const l of lists) {
      o += `**${l.name}**  id=${l.id}  type=${l.type??'?'}  enabled=${l.enabled}\n`;
      if (l.lastSynced) o += `  Last synced: ${ago(l.lastSynced)}\n`;
      o += "\n";
    }
    return o;
  }

  case "sync_import_list": {
    await POST(`/api/v1/import-lists/${a.id}/sync`);
    return `✅ Import list ${a.id} sync triggered.`;
  }

  // ── Notifications ─────────────────────────────────────────────────────────────

  case "list_notifications": {
    const data = await GET("/api/v1/notifications");
    const conns = data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Notifications (${conns.length})\n\n`;
    for (const c of conns) {
      o += `**${c.name}**  id=${c.id}  type=${c.type??'?'}  enabled=${c.enabled}\n`;
      const events = [];
      if (c.onGrab)   events.push("grab");
      if (c.onDownload) events.push("download");
      if (c.onUpgrade) events.push("upgrade");
      if (events.length) o += `  Events: ${events.join(", ")}\n`;
      o += "\n";
    }
    return o;
  }

  case "test_notification": {
    await POST(`/api/v1/notifications/${a.id}/test`);
    return `✅ Test notification sent for ${a.id}.`;
  }

  // ── Connect (media server connections) ────────────────────────────────────────

  case "list_connect_connections": {
    const data = await GET("/api/v1/connect");
    const conns = data?.data ?? (Array.isArray(data)?data:[]);
    let o = `# Connect Connections (${conns.length})\n\n`;
    for (const c of conns) {
      o += `**${c.name}**  id=${c.id}  type=${c.type??'?'}  enabled=${c.enabled}\n`;
      if (c.host) o += `  Host: ${c.host}\n`;
      o += "\n";
    }
    return o;
  }

  // ── Search Debug ──────────────────────────────────────────────────────────────

  case "list_search_logs": {
    const params = new URLSearchParams();
    if (a.outcome)    params.set("outcome",a.outcome);
    if (a.media_type) params.set("media_type",a.media_type);
    if (a.media_id)   params.set("media_id",a.media_id);
    params.set("limit", String(a.limit||20));
    const data = await GET(`/api/v1/search-debug?${params}`);
    let entries = data?.entries??[];
    if (a.title) { const t=a.title.toLowerCase(); entries=entries.filter(e=>e.title?.toLowerCase().includes(t)); }
    let o = `# Search Logs (${entries.length} of ${data.total??'?'})\n\n`;
    for (const e of entries) {
      const icon = e.outcome==="grabbed"?"✅":e.outcome==="no_results"?"🔍":"❌";
      const ep = e.season>0?` S${String(e.season).padStart(2,'0')}${e.episode>0?`E${String(e.episode).padStart(2,'0')}`:""}` :"";
      o += `${icon} [${(e.outcome||'').toUpperCase()}] ${e.title}${ep}  ${ago(e.created_at)}\n`;
      o += `   id=${e.id}  results=${e.total_results??'?'}  rejected=${e.total_rejected??'?'}  ${e.duration_ms??'?'}ms\n`;
      if (e.grabbed_title) o += `   Grabbed: ${e.grabbed_title}\n`;
      if (e.error_message) o += `   ❌ ${e.error_message}\n`;
      o += "\n";
    }
    return o;
  }

  case "get_search_log": {
    const d = await GET(`/api/v1/search-debug/${a.id}`);
    let o = `# Search Log: ${d.title}`;
    if (d.season>0) o += ` S${String(d.season).padStart(2,'0')}`;
    if (d.episode>0) o += `E${String(d.episode).padStart(2,'0')}`;
    o += `\n\n**Outcome:** ${d.outcome} | **${d.duration_ms}ms**\n`;
    o += `**IDs:** TVDB=${d.tvdb_id||'—'} TMDB=${d.tmdb_id||'—'} IMDB=${d.imdb_id||'—'}\n`;
    if (d.error_message) o += `**Error:** ${d.error_message}\n`;
    o += "\n";
    if (d.tiers?.length) {
      o += `## Query Tiers\n`;
      for (const t of d.tiers) {
        o += `\n### Tier ${t.tier_index}${t.stopped_here?" ← STOPPED":""}`
           + `  results=${t.result_count} accepted=${t.accepted_count} rejected=${t.rejected_count}\n`;
        for (const q of t.queries||[]) {
          let qs = `mode=${q.mode}`;
          if (q.term)    qs += ` q="${q.term}"`;
          if (q.tvdb_id) qs += ` tvdb=${q.tvdb_id}`;
          if (q.tmdb_id) qs += ` tmdb=${q.tmdb_id}`;
          if (q.season)  qs += ` S${q.season}${q.episode?`E${q.episode}`:""}`;
          o += `  - ${qs}\n`;
        }
      }
      o += "\n";
    }
    if (d.indexer_results?.length) {
      o += `## Indexer Results\n`;
      for (const ir of d.indexer_results) {
        const icon = ir.status==="ok"?"✅":ir.status==="error"?"❌":"⚠️";
        o += `\n${icon} **${ir.indexer_name}** — ${ir.result_count} results, ${ir.latency_ms}ms\n`;
        if (ir.error) o += `   Error: ${ir.error}\n`;
        for (const r of (ir.results||[]).slice(0,12)) {
          o += `   - ${r.title.slice(0,80)} (${mb(r.size)}${r.seeders!=null?` [${r.seeders}s]`:""})\n`;
        }
        if ((ir.results?.length??0)>12) o += `   … +${ir.results.length-12} more\n`;
      }
      o += "\n";
    }
    if (d.evaluation?.length) {
      const accepted = d.evaluation.filter(e=>!e.rejected);
      const rejected = d.evaluation.filter(e=>e.rejected);
      o += `## Evaluation (${d.evaluation.length})\n`;
      if (accepted.length) {
        o += `\n### ✅ Accepted (${accepted.length})\n`;
        for (const e of accepted.slice(0,20)) {
          o += `  ✓ ${e.title.slice(0,80)}\n    ${e.quality_name||'?'} tier=${e.quality_tier} score=${e.composite_score?.toFixed(0)} ${mb(e.size)}${e.seeders!=null?` [${e.seeders}s]`:""}\n`;
        }
      }
      if (rejected.length) {
        o += `\n### ❌ Rejected (${rejected.length})\n`;
        const byReason = {};
        for (const e of rejected) (byReason[e.reject_reason||"?"] = byReason[e.reject_reason||"?"]||[]).push(e);
        for (const [reason,items] of Object.entries(byReason).sort((a,b)=>b[1].length-a[1].length)) {
          o += `  **${reason}** (${items.length}):\n`;
          for (const e of items.slice(0,4)) o += `    ✗ ${(e.title||'').slice(0,70)}\n`;
          if (items.length>4) o += `    … +${items.length-4} more\n`;
        }
      }
      if (d.grabbed_title) o += `\n### 🎯 Grabbed\n  ${d.grabbed_title}\n`;
    }
    return o;
  }

  case "search_debug_stats": {
    const data = await GET("/api/v1/search-debug/stats");
    let o = `# Search Stats\n\n**Total:** ${data.total_searches}\n\n## Outcomes\n`;
    for (const [k,v] of Object.entries(data.outcome_counts||{})) {
      const pct2 = data.total_searches ? ((v/data.total_searches)*100).toFixed(1) : 0;
      o += `${k==="grabbed"?"✅":k==="no_results"?"🔍":"❌"} ${k}: ${v} (${pct2}%)\n`;
    }
    if (data.top_reject_reasons?.length) {
      o += "\n## Top Reject Reasons\n";
      for (const r of data.top_reject_reasons.slice(0,10)) o += `  ${r.reason}: ${r.count}\n`;
    }
    return o;
  }

  default: throw new Error(`Unknown tool: ${name}`);
  }
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDownloadItem(item, indent="") {
  const prog = item.progress!=null ? `${pct(item.progress)} ` : "";
  const eta  = item.eta_seconds>0 ? `ETA ${Math.round(item.eta_seconds/60)}m ` : "";
  const dl   = item.download_rate>0 ? `↓${mb(item.download_rate)}/s ` : "";
  return `${indent}[${item.status||'?'}] ${(item.title||'?').slice(0,70)}\n${indent}  ${prog}${eta}${dl}${gb(item.size_bytes)} | id=${item.id}\n`;
}

// ─── Start ────────────────────────────────────────────────────────────────────

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}
main().catch(console.error);
