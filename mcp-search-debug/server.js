#!/usr/bin/env node
// Loom MCP Server — full control and observability for the Loom media manager.

const { Server } = require("@modelcontextprotocol/sdk/server/index.js");
const { StdioServerTransport } = require("@modelcontextprotocol/sdk/server/stdio.js");
const { CallToolRequestSchema, ListToolsRequestSchema } = require("@modelcontextprotocol/sdk/types.js");

const LOOM_BASE_URL = process.env.LOOM_URL || "https://loom.media.deroock.co.za";
const LOOM_API_KEY  = process.env.LOOM_API_KEY || "";

async function loomFetch(path, options = {}) {
  const url = `${LOOM_BASE_URL}${path}`;
  const headers = { "Content-Type": "application/json" };
  if (LOOM_API_KEY) headers["X-Api-Key"] = LOOM_API_KEY;
  const res = await fetch(url, { ...options, headers });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`Loom API ${res.status}: ${body.slice(0, 300)}`);
  }
  const text = await res.text();
  if (!text) return null;
  try { return JSON.parse(text); } catch { return text; }
}

function mb(b) { return b ? (b/1024/1024).toFixed(1)+" MB" : "?"; }
function gb(b) { return b ? (b/1024/1024/1024).toFixed(2)+" GB" : "?"; }
function ago(iso) {
  if (!iso) return "?";
  const d = (Date.now() - new Date(iso).getTime()) / 1000;
  if (d < 60) return `${Math.round(d)}s ago`;
  if (d < 3600) return `${Math.round(d/60)}m ago`;
  if (d < 86400) return `${Math.round(d/3600)}h ago`;
  return `${Math.round(d/86400)}d ago`;
}

const TOOLS = [
  { name:"get_system_status", description:"Get Loom version and health. Use to verify connectivity first.", inputSchema:{type:"object",properties:{}} },
  { name:"list_series", description:"List all TV series with episode stats (total/downloaded/missing). Add with_missing:true to only show series needing attention.", inputSchema:{type:"object",properties:{monitored_only:{type:"boolean"},with_missing:{type:"boolean"}}} },
  { name:"get_series", description:"Get full details of a single series including IDs, quality profile, and episode stats.", inputSchema:{type:"object",properties:{id:{type:"string",description:"Series ID e.g. from-2022"}},required:["id"]} },
  { name:"list_series_episodes", description:"List episodes for a series optionally filtered by season. Shows file status and air dates. Use missing_only:true to find what to download.", inputSchema:{type:"object",properties:{series_id:{type:"string"},season:{type:"number"},missing_only:{type:"boolean"}},required:["series_id"]} },
  { name:"list_movies", description:"List all movies with file/quality status.", inputSchema:{type:"object",properties:{monitored_only:{type:"boolean"},missing_only:{type:"boolean"}}} },
  { name:"list_indexers", description:"List indexers with health, latency, and supported capabilities.", inputSchema:{type:"object",properties:{}} },
  { name:"list_quality_profiles", description:"List quality profiles. Get IDs needed for trigger_search.", inputSchema:{type:"object",properties:{}} },
  {
    name:"trigger_search",
    description:"Trigger a search+grab for a specific episode or movie. Searches indexers, evaluates results, grabs best match. May take 30-120s for slow indexers.",
    inputSchema:{type:"object",properties:{
      media_type:{type:"string",enum:["episode","series","movie"]},
      media_id:{type:"string",description:"Series/movie ID e.g. from-2022"},
      title:{type:"string"},
      year:{type:"number"},
      season:{type:"number"},
      episode:{type:"number"},
      quality_profile_id:{type:"string"},
      tvdb_id:{type:"string"},
      tmdb_id:{type:"string"},
      imdb_id:{type:"string"},
    },required:["media_type","media_id","title","quality_profile_id"]},
  },
  {
    name:"search_missing_episodes",
    description:"Find missing monitored episodes for a series and trigger a search for each. Use limit to control how many (default 5, max 20).",
    inputSchema:{type:"object",properties:{
      series_id:{type:"string"},
      season:{type:"number"},
      quality_profile_id:{type:"string"},
      limit:{type:"number"},
    },required:["series_id"]},
  },
  { name:"list_workflows", description:"List recent download/import workflows. Filter by status: pending/active/done/failed/cancelled.", inputSchema:{type:"object",properties:{status:{type:"string"},limit:{type:"number"}}} },
  { name:"get_workflow", description:"Get full details and event history for a workflow.", inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]} },
  { name:"cancel_workflow", description:"Cancel a running workflow.", inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]} },
  { name:"retry_workflow", description:"Retry a failed workflow.", inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]} },
  {
    name:"list_search_logs",
    description:"List search debug log entries. Filter by outcome/media_type/media_id/title. Shows what was searched and why it succeeded or failed.",
    inputSchema:{type:"object",properties:{
      outcome:{type:"string",description:"grabbed|no_results|all_rejected|grab_failed|already_grabbed"},
      media_type:{type:"string"},
      media_id:{type:"string"},
      title:{type:"string",description:"Title substring filter"},
      limit:{type:"number"},
    }},
  },
  { name:"get_search_log", description:"Get full search details: tiers/queries sent to indexers, every result, rejection reasons, quality scores, what was grabbed.", inputSchema:{type:"object",properties:{id:{type:"string"}},required:["id"]} },
  { name:"search_debug_stats", description:"Aggregate search stats for last 7 days: outcomes, top reject reasons.", inputSchema:{type:"object",properties:{}} },
];

const server = new Server({ name:"loom-mcp", version:"2.0.0" });

server.setRequestHandler(ListToolsRequestSchema, async () => ({ tools: TOOLS }));

server.setRequestHandler(CallToolRequestSchema, async (req) => {
  const { name, arguments: args } = req.params;
  try {
    return { content:[{ type:"text", text: await handleTool(name, args||{}) }] };
  } catch(err) {
    return { content:[{ type:"text", text:`❌ ${err.message}\nURL: ${LOOM_BASE_URL}` }], isError:true };
  }
});

async function handleTool(name, args) {
  switch(name) {

  case "get_system_status": {
    const s = await loomFetch("/api/v1/system/status");
    let o = `# Loom System\n\n**Version:** ${s.version}  **Commit:** ${s.commit}\n**Build:** ${s.buildDate}\n**DB:** ${s.engine}\n`;
    const h = await loomFetch("/api/v1/system/health").catch(()=>null);
    if (h) {
      o += "\n## Health\n";
      const checks = Array.isArray(h) ? h : Object.values(h);
      for (const c of checks) {
        const icon = c.status==="ok"?"✅":c.status==="warning"?"⚠️":"❌";
        o += `${icon} **${c.name||c.check}**: ${c.status}${c.message?" — "+c.message:""}\n`;
      }
    }
    return o;
  }

  case "list_series": {
    const data = await loomFetch("/api/v1/series");
    let series = data?.data ?? data;
    if (args.monitored_only) series = series.filter(s=>s.monitored!==false);
    if (args.with_missing)   series = series.filter(s=>(s.episodeStats?.missingEpisodes??0)>0);
    series = series.sort((a,b)=>(b.episodeStats?.missingEpisodes??0)-(a.episodeStats?.missingEpisodes??0));
    let o = `# Series (${series.length})\n\n${"ID".padEnd(30)} ${"Title".padEnd(38)} Miss  Tot   DL\n${"-".repeat(82)}\n`;
    for (const s of series) {
      const st = s.episodeStats||{};
      o += `${(s.id||"").padEnd(30)} ${(s.title||"").slice(0,36).padEnd(38)} ${String(st.missingEpisodes??"?").padStart(4)}  ${String(st.totalEpisodes??"?").padStart(3)}  ${String(st.downloadedEpisodes??"?").padStart(3)}\n`;
    }
    return o;
  }

  case "get_series": {
    const s = await loomFetch(`/api/v1/series/${args.id}`);
    const st = s.episodeStats||{};
    return `# ${s.title} (${s.year??'?'})\n\n**ID:** ${s.id}\n**TVDB:** ${s.tvdbId??'not set'} | **TMDB:** ${s.tmdbId??'not set'} | **IMDB:** ${s.imdbId??'not set'}\n**Quality Profile:** ${s.qualityProfileId}\n**Monitoring:** ${s.monitoringStatus}\n**Status:** ${s.status}\n**Network:** ${s.network??'?'}\n\n## Episode Stats\n- Total: ${st.totalEpisodes} | Aired: ${st.airedEpisodes} | Downloaded: ${st.downloadedEpisodes}\n- Monitored: ${st.monitoredEpisodes} | **Missing: ${st.missingEpisodes}**\n`;
  }

  case "list_series_episodes": {
    const s = await loomFetch(`/api/v1/series/${args.series_id}`);
    let seasonNums;
    if (args.season) {
      seasonNums = [args.season];
    } else {
      const ss = await loomFetch(`/api/v1/series/${args.series_id}/seasons`);
      seasonNums = (ss||[]).map(x=>x.seasonNumber).filter(n=>n>0).sort((a,b)=>a-b);
    }
    let o = `# ${s.title} — Episodes\n\n`;
    let totalMissing = 0;
    for (const sn of seasonNums) {
      const data = await loomFetch(`/api/v1/series/${args.series_id}/seasons/${sn}/episodes`);
      const eps = data?.data ?? data;
      let filtered = args.missing_only ? eps.filter(e=>!e.hasFile) : eps;
      if (!filtered.length) continue;
      const miss = eps.filter(e=>!e.hasFile&&e.monitored).length;
      totalMissing += miss;
      o += `## Season ${sn}  (${miss} missing monitored)\n`;
      for (const e of filtered) {
        const icon = e.hasFile?"✅":(e.monitored?"⬇️ ":"⏸ ");
        o += `  ${icon} E${String(e.episodeNumber??'?').padStart(2,'0')} — ${(e.title||'?').slice(0,45).padEnd(45)}  ${e.airDate?`aired ${e.airDate}`:'not aired'}\n`;
      }
      o += "\n";
    }
    o += `**Total missing monitored: ${totalMissing}**\n`;
    return o;
  }

  case "list_movies": {
    const data = await loomFetch("/api/v1/movies");
    let movies = data?.data ?? data;
    if (args.monitored_only) movies = movies.filter(m=>m.monitored!==false);
    if (args.missing_only)   movies = movies.filter(m=>!m.hasFile);
    movies = movies.sort((a,b)=>(a.title||"").localeCompare(b.title||""));
    let o = `# Movies (${movies.length})\n\n`;
    for (const m of movies) {
      const icon = m.hasFile?"✅":(m.monitored?"⬇️ ":"⏸ ");
      o += `${icon} ${m.title} (${m.year??'?'})  ${m.movieFile?.quality?.quality?.name??''}  [${m.id}]\n`;
    }
    return o;
  }

  case "list_indexers": {
    const data = await loomFetch("/api/v1/indexers");
    const indexers = data?.indexers ?? data?.data ?? data;
    let o = `# Indexers (${indexers.length})\n\n`;
    for (const idx of indexers) {
      const h = idx.health||{};
      const icon = h.status==="ok"?"✅":h.status==="degraded"?"⚠️":"❌";
      o += `${icon} **${idx.name}** (${idx.id})\n`;
      o += `   Enabled: ${idx.enabled} | Priority: ${idx.priority} | Kind: ${idx.kind}\n`;
      o += `   Health: ${h.status??'unknown'} | Latency: ${h.latency_ms??'?'}ms`;
      if (h.last_checked_at) o += ` | Last checked: ${ago(h.last_checked_at)}`;
      o += "\n";
      if (h.error_message) o += `   ⚠️ ${h.error_message}\n`;
      const caps = idx.capabilities;
      if (caps) {
        const modes = Object.entries(caps.search_modes||{}).map(([m,i])=>`${m}(${i.available?"✓":"✗"})`);
        if (modes.length) o += `   Modes: ${modes.join(", ")}\n`;
        if (caps.supported_params?.length) o += `   Params: ${caps.supported_params.join(", ")}\n`;
      }
      o += "\n";
    }
    return o;
  }

  case "list_quality_profiles": {
    const data = await loomFetch("/api/v1/quality-profiles");
    const profiles = data?.data ?? data;
    let o = `# Quality Profiles (${profiles.length})\n\n`;
    for (const p of profiles) {
      o += `**${p.name}**  id=${p.id}\n`;
      o += `  Upgrade: ${p.upgradeAllowed} | MinFormatScore: ${p.minFormatScore??0}\n`;
      try {
        const items = typeof p.items==="string" ? JSON.parse(p.items) : p.items;
        const allowed = (items||[]).filter(i=>i.allowed).map(i=>i.name||i.id).join(", ");
        if (allowed) o += `  Allowed: ${allowed}\n`;
      } catch {}
      o += "\n";
    }
    return o;
  }

  case "trigger_search": {
    const payload = {
      media_type:args.media_type, media_id:args.media_id, title:args.title,
      quality_profile_id:args.quality_profile_id,
    };
    if (args.year)    payload.year    = args.year;
    if (args.season)  payload.season  = args.season;
    if (args.episode) payload.episode = args.episode;
    if (args.tvdb_id) payload.tvdb_id = args.tvdb_id;
    if (args.tmdb_id) payload.tmdb_id = args.tmdb_id;
    if (args.imdb_id) payload.imdb_id = args.imdb_id;

    const result = await loomFetch("/api/v1/autosearch", { method:"POST", body:JSON.stringify(payload) });
    if (!result) return "⚠️ Empty response";

    let o = `# Search: ${args.title}`;
    if (args.season)  o += ` S${String(args.season).padStart(2,"0")}`;
    if (args.episode) o += `E${String(args.episode).padStart(2,"0")}`;
    o += "\n\n";

    if (result.grabbed) {
      const g = result.grabbed;
      o += `✅ **GRABBED:** ${g.title}\n   Quality: ${g.quality_tier} | Score: ${g.composite_score?.toFixed(1)} | Size: ${gb(g.size)}\n   Indexer: ${g.indexer_id} | Client: ${g.client_id}\n`;
    } else {
      o += `❌ **NOT GRABBED** — ${result.reason||"unknown"}\n`;
    }
    o += `\nConsidered: ${result.considered} | Rejected: ${result.rejected}\n`;
    if (result.top_rejects?.length) {
      o += "\nTop reject reasons:\n";
      for (const r of result.top_rejects.slice(0,8)) o += `  - ${r.reason}: ${r.count}\n`;
    }
    return o;
  }

  case "search_missing_episodes": {
    const series = await loomFetch(`/api/v1/series/${args.series_id}`);
    const qpId = args.quality_profile_id || series.qualityProfileId;
    if (!qpId) return "❌ No quality profile; pass quality_profile_id explicitly.";
    const limit = Math.min(args.limit||5, 20);
    const ss = await loomFetch(`/api/v1/series/${args.series_id}/seasons`);
    const seasonNums = (ss||[]).map(s=>s.seasonNumber).filter(n=>n>0&&(!args.season||n===args.season));

    const missing = [];
    for (const sn of seasonNums) {
      if (missing.length>=limit) break;
      const data = await loomFetch(`/api/v1/series/${args.series_id}/seasons/${sn}/episodes`);
      const eps = data?.data ?? data;
      for (const e of eps) {
        if (!e.hasFile && e.monitored) missing.push({season:sn,episode:e.episodeNumber});
        if (missing.length>=limit) break;
      }
    }
    if (!missing.length) return `✅ No missing monitored episodes for ${series.title}.`;

    let o = `# Searching Missing: ${series.title}\n\nSearching ${missing.length} episodes...\n\n`;
    const base = {
      media_type:"episode", media_id:args.series_id, title:series.title,
      year:series.year, quality_profile_id:qpId,
    };
    if (series.tvdbId) base.tvdb_id = String(series.tvdbId);
    if (series.tmdbId) base.tmdb_id = String(series.tmdbId);
    if (series.imdbId) base.imdb_id = series.imdbId;

    for (const ep of missing) {
      let status;
      try {
        const r = await loomFetch("/api/v1/autosearch",{method:"POST",body:JSON.stringify({...base,season:ep.season,episode:ep.episode})});
        status = r?.grabbed ? `✅ GRABBED: ${r.grabbed.title.slice(0,60)}` : `❌ ${r?.reason||"?"} (${r?.considered}→${r?.rejected} rejected)`;
      } catch(err) { status = `⚠️ ${err.message.slice(0,80)}`; }
      o += `S${String(ep.season).padStart(2,'0')}E${String(ep.episode).padStart(2,'0')} — ${status}\n`;
    }
    return o;
  }

  case "list_workflows": {
    const data = await loomFetch("/api/v1/workflows");
    let wfs = Array.isArray(data) ? data : (data?.data ?? []);
    if (args.status) wfs = wfs.filter(w=>w.status===args.status);
    wfs = wfs.slice(0, args.limit||20);
    let o = `# Workflows (${wfs.length})\n\n`;
    for (const w of wfs) {
      const icon = {done:"✅",active:"⏳",failed:"❌",cancelled:"🚫",pending:"🔄"}[w.status]??"❓";
      o += `${icon} **${(w.id||'').slice(0,8)}…**  ${w.mediaType??'?'} — ${w.mediaTitle||w.downloadTitle||'?'}\n`;
      o += `   ${w.status} | created ${ago(w.createdAt)} | updated ${ago(w.updatedAt)}\n`;
      if (w.error) o += `   ❌ ${w.error}\n`;
      o += "\n";
    }
    return o;
  }

  case "get_workflow": {
    const w = await loomFetch(`/api/v1/workflows/${args.id}`);
    const events = await loomFetch(`/api/v1/workflows/${args.id}/events`).catch(()=>[]);
    let o = `# Workflow ${args.id}\n\n**Status:** ${w.status}\n**Media:** ${w.mediaType??'?'} — ${w.mediaTitle||w.downloadTitle||'?'}\n**Created:** ${ago(w.createdAt)} | **Updated:** ${ago(w.updatedAt)}\n`;
    if (w.error) o += `**Error:** ${w.error}\n`;
    if (events?.length) {
      o += `\n## Events\n`;
      for (const e of events) {
        o += `  ${ago(e.createdAt||e.timestamp)} — ${e.type||e.event}: ${e.message||''}\n`;
        if (e.data) { const d = typeof e.data==="string"?e.data:JSON.stringify(e.data); o += `    ${d.slice(0,200)}\n`; }
      }
    }
    return o;
  }

  case "cancel_workflow": {
    await loomFetch(`/api/v1/workflows/${args.id}/cancel`,{method:"POST"});
    return `✅ Workflow ${args.id} cancelled.`;
  }

  case "retry_workflow": {
    await loomFetch(`/api/v1/workflows/${args.id}/retry`,{method:"POST"});
    return `✅ Workflow ${args.id} retried.`;
  }

  case "list_search_logs": {
    const params = new URLSearchParams();
    if (args.outcome)    params.set("outcome",args.outcome);
    if (args.media_type) params.set("media_type",args.media_type);
    if (args.media_id)   params.set("media_id",args.media_id);
    params.set("limit", String(args.limit||20));
    const data = await loomFetch(`/api/v1/search-debug?${params}`);
    let entries = data?.entries ?? [];
    if (args.title) { const t=args.title.toLowerCase(); entries=entries.filter(e=>e.title?.toLowerCase().includes(t)); }

    let o = `# Search Logs (${entries.length} of ${data.total??'?'})\n\n`;
    for (const e of entries) {
      const icon = e.outcome==="grabbed"?"✅":e.outcome==="no_results"?"🔍":"❌";
      const ep = e.season>0 ? ` S${String(e.season).padStart(2,'0')}${e.episode>0?`E${String(e.episode).padStart(2,'0')}`:""}` : "";
      o += `${icon} [${(e.outcome||'').toUpperCase()}] ${e.title}${ep}  ${ago(e.created_at)}\n`;
      o += `   id=${e.id}  results=${e.total_results??'?'}  rejected=${e.total_rejected??'?'}  ${e.duration_ms??'?'}ms\n`;
      if (e.grabbed_title) o += `   Grabbed: ${e.grabbed_title}\n`;
      if (e.error_message) o += `   ❌ ${e.error_message}\n`;
      o += "\n";
    }
    return o;
  }

  case "get_search_log": {
    const d = await loomFetch(`/api/v1/search-debug/${args.id}`);
    let o = `# Search Log: ${d.title}`;
    if (d.season>0) o += ` S${String(d.season).padStart(2,'0')}`;
    if (d.episode>0) o += `E${String(d.episode).padStart(2,'0')}`;
    o += `\n\n**ID:** ${d.id} | **Outcome:** ${d.outcome} | **${d.duration_ms}ms**\n`;
    o += `**IDs:** TVDB=${d.tvdb_id||'—'} TMDB=${d.tmdb_id||'—'} IMDB=${d.imdb_id||'—'}\n`;
    if (d.error_message) o += `**Error:** ${d.error_message}\n`;
    o += "\n";

    if (d.tiers?.length) {
      o += `## Query Tiers\n`;
      for (const t of d.tiers) {
        o += `\n### Tier ${t.tier_index}${t.stopped_here?" ← STOPPED":""}  results=${t.result_count} accepted=${t.accepted_count} rejected=${t.rejected_count}\n`;
        for (const q of t.queries||[]) {
          let qs = `mode=${q.mode}`;
          if (q.term) qs += ` q="${q.term}"`;
          if (q.tvdb_id) qs += ` tvdb=${q.tvdb_id}`;
          if (q.tmdb_id) qs += ` tmdb=${q.tmdb_id}`;
          if (q.season) qs += ` S${q.season}`;
          if (q.episode) qs += `E${q.episode}`;
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
          const s = r.seeders!=null?` [${r.seeders}s]`:"";
          o += `   - ${r.title.slice(0,80)} (${mb(r.size)}${s})\n`;
        }
        if ((ir.results?.length??0)>12) o += `   … and ${ir.results.length-12} more\n`;
      }
      o += "\n";
    }

    if (d.evaluation?.length) {
      const accepted = d.evaluation.filter(e=>!e.rejected);
      const rejected = d.evaluation.filter(e=>e.rejected);
      o += `## Evaluation (${d.evaluation.length} results)\n`;

      if (accepted.length) {
        o += `\n### ✅ Accepted (${accepted.length})\n`;
        for (const e of accepted.slice(0,20)) {
          o += `  ✓ ${e.title.slice(0,80)}\n    ${e.quality_name||'?'} tier=${e.quality_tier} score=${e.composite_score?.toFixed(0)} size=${mb(e.size)}${e.seeders!=null?` [${e.seeders}s]`:""}\n`;
        }
      }

      if (rejected.length) {
        o += `\n### ❌ Rejected (${rejected.length})\n`;
        const byReason = {};
        for (const e of rejected) (byReason[e.reject_reason||"unknown"]=byReason[e.reject_reason||"unknown"]||[]).push(e);
        for (const [reason,items] of Object.entries(byReason).sort((a,b)=>b[1].length-a[1].length)) {
          o += `  **${reason}** (${items.length}):\n`;
          for (const e of items.slice(0,4)) o += `    ✗ ${(e.title||'').slice(0,70)}\n`;
          if (items.length>4) o += `    … and ${items.length-4} more\n`;
        }
      }

      if (d.grabbed_title) o += `\n### 🎯 Grabbed\n  ${d.grabbed_title}\n`;
    }
    return o;
  }

  case "search_debug_stats": {
    const data = await loomFetch("/api/v1/search-debug/stats");
    let o = `# Search Stats (Last 7 Days)\n\n**Total:** ${data.total_searches}\n\n## Outcomes\n`;
    for (const [k,v] of Object.entries(data.outcome_counts||{})) {
      const pct = data.total_searches ? ((v/data.total_searches)*100).toFixed(1) : 0;
      const icon = k==="grabbed"?"✅":k==="no_results"?"🔍":"❌";
      o += `${icon} ${k}: ${v} (${pct}%)\n`;
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

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}
main().catch(console.error);
