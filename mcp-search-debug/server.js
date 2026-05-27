#!/usr/bin/env node

const { Server } = require("@modelcontextprotocol/sdk/server/index.js");
const {
  StdioServerTransport,
} = require("@modelcontextprotocol/sdk/server/stdio.js");
const {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} = require("@modelcontextprotocol/sdk/types.js");

const LOOM_BASE_URL = process.env.LOOM_URL || "https://loom.media.deroock.co.nz";
const LOOM_API_KEY = process.env.LOOM_API_KEY || "";

const server = new Server({
  name: "loom-search-debug",
  version: "1.0.0",
});

async function loomFetch(path, options = {}) {
  const url = `${LOOM_BASE_URL}${path}`;
  const headers = { "Content-Type": "application/json" };
  if (LOOM_API_KEY) {
    headers["X-Api-Key"] = LOOM_API_KEY;
  }
  const res = await fetch(url, { ...options, headers });
  if (!res.ok) {
    throw new Error(`Loom API ${res.status}: ${await res.text()}`);
  }
  return res.json();
}

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [
    {
      name: "list_search_logs",
      description:
        "List recent search debug log entries. Shows what searches were performed, their outcomes, and summary stats. Use this to understand why searches are failing.",
      inputSchema: {
        type: "object",
        properties: {
          outcome: {
            type: "string",
            description:
              'Filter by outcome: "grabbed", "no_results", "all_rejected", "grab_failed", "already_grabbed", "profile_load_failed"',
          },
          media_type: {
            type: "string",
            description: 'Filter by media type: "movie", "series", "episode"',
          },
          media_id: {
            type: "string",
            description: "Filter by specific media ID (UUID)",
          },
          limit: {
            type: "number",
            description: "Max entries to return (default 20, max 200)",
          },
        },
      },
    },
    {
      name: "get_search_log",
      description:
        "Get full details of a single search debug log entry, including query tiers, per-indexer results, and evaluation of every result (reject reasons, quality scores, etc). Use this to diagnose why a specific search failed.",
      inputSchema: {
        type: "object",
        properties: {
          id: {
            type: "string",
            description: "The search debug log entry ID",
          },
        },
        required: ["id"],
      },
    },
    {
      name: "search_debug_stats",
      description:
        "Get aggregate search statistics for the last 7 days: total searches, outcome breakdown (grabbed vs rejected vs no results), and top reject reasons. Use this for an overview of search health.",
      inputSchema: {
        type: "object",
        properties: {},
      },
    },
    {
      name: "search_logs_for_media",
      description:
        "Find all search debug logs for a specific media item by title substring. Useful when you want to see all search attempts for a particular movie or TV show.",
      inputSchema: {
        type: "object",
        properties: {
          title: {
            type: "string",
            description: "Title or partial title to search for",
          },
          media_type: {
            type: "string",
            description: 'Filter by media type: "movie", "series", "episode"',
          },
          limit: {
            type: "number",
            description: "Max entries to return (default 20)",
          },
        },
        required: ["title"],
      },
    },
  ],
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  try {
    switch (name) {
      case "list_search_logs": {
        const params = new URLSearchParams();
        if (args?.outcome) params.set("outcome", args.outcome);
        if (args?.media_type) params.set("media_type", args.media_type);
        if (args?.media_id) params.set("media_id", args.media_id);
        params.set("limit", String(args?.limit || 20));

        const data = await loomFetch(
          `/api/v1/search-debug?${params.toString()}`
        );

        // Format for readability
        let output = `Search Debug Log (${data.total} total, showing ${data.entries.length})\n\n`;
        for (const e of data.entries) {
          const time = new Date(e.created_at).toLocaleString();
          const seasonEp =
            e.season > 0
              ? ` S${String(e.season).padStart(2, "0")}${e.episode > 0 ? `E${String(e.episode).padStart(2, "0")}` : ""}`
              : "";
          output += `[${e.outcome.toUpperCase()}] ${time} - ${e.title}${seasonEp} (${e.media_type})\n`;
          output += `  ID: ${e.id}\n`;
          output += `  Results: ${e.total_results} found, ${e.total_rejected} rejected, ${e.duration_ms}ms\n`;
          if (e.grabbed_title) output += `  Grabbed: ${e.grabbed_title}\n`;
          if (e.error_message) output += `  Error: ${e.error_message}\n`;
          output += "\n";
        }

        return { content: [{ type: "text", text: output }] };
      }

      case "get_search_log": {
        const data = await loomFetch(`/api/v1/search-debug/${args.id}`);

        let output = `# Search Debug: ${data.title}\n\n`;
        output += `**ID:** ${data.id}\n`;
        output += `**Time:** ${new Date(data.created_at).toLocaleString()}\n`;
        output += `**Media:** ${data.media_type} (${data.media_id})\n`;
        output += `**Outcome:** ${data.outcome}\n`;
        output += `**Duration:** ${data.duration_ms}ms\n`;
        if (data.error_message) output += `**Error:** ${data.error_message}\n`;
        output += "\n";

        // IDs
        output += `## External IDs\n`;
        if (data.imdb_id) output += `- IMDB: ${data.imdb_id}\n`;
        if (data.tvdb_id) output += `- TVDB: ${data.tvdb_id}\n`;
        if (data.tmdb_id) output += `- TMDB: ${data.tmdb_id}\n`;
        output += "\n";

        // Tiers
        if (data.tiers?.length) {
          output += `## Query Tiers\n\n`;
          for (const tier of data.tiers) {
            output += `### Tier ${tier.tier_index}${tier.stopped_here ? " ← STOPPED HERE" : ""}\n`;
            output += `Results: ${tier.result_count} | Accepted: ${tier.accepted_count} | Rejected: ${tier.rejected_count}\n`;
            for (const q of tier.queries || []) {
              output += `- Query: mode=${q.mode || "auto"}`;
              if (q.term) output += ` term="${q.term}"`;
              if (q.imdb_id) output += ` imdb=${q.imdb_id}`;
              if (q.tvdb_id) output += ` tvdb=${q.tvdb_id}`;
              if (q.tmdb_id) output += ` tmdb=${q.tmdb_id}`;
              if (q.season) output += ` S${q.season}`;
              if (q.episode) output += `E${q.episode}`;
              if (q.year) output += ` year=${q.year}`;
              if (q.categories?.length)
                output += ` cats=[${q.categories.join(",")}]`;
              output += "\n";
            }
            output += "\n";
          }
        }

        // Indexer results
        if (data.indexer_results?.length) {
          output += `## Indexer Results\n\n`;
          for (const ir of data.indexer_results) {
            output += `### ${ir.indexer_name} (${ir.status})\n`;
            output += `Results: ${ir.result_count} | Latency: ${ir.latency_ms}ms\n`;
            if (ir.error) output += `Error: ${ir.error}\n`;
            if (ir.results?.length) {
              output += `\nResults:\n`;
              for (const r of ir.results.slice(0, 20)) {
                const sizeMB = (r.size / 1024 / 1024).toFixed(1);
                const seeders =
                  r.seeders != null ? ` [${r.seeders} seeds]` : "";
                const flags = [
                  r.freeleech && "FL",
                  r.scene && "Scene",
                  r.internal && "Int",
                ]
                  .filter(Boolean)
                  .join(",");
                output += `  - ${r.title} (${sizeMB}MB${seeders}${flags ? ` ${flags}` : ""})\n`;
              }
              if (ir.results.length > 20)
                output += `  ... and ${ir.results.length - 20} more\n`;
            }
            output += "\n";
          }
        }

        // Evaluation
        if (data.evaluation?.length) {
          output += `## Evaluation (${data.evaluation.length} results)\n\n`;

          const accepted = data.evaluation.filter((e) => !e.rejected);
          const rejected = data.evaluation.filter((e) => e.rejected);

          if (accepted.length) {
            output += `### Accepted (${accepted.length})\n`;
            for (const e of accepted.slice(0, 20)) {
              output += `  ✓ ${e.title}\n`;
              output += `    Quality: ${e.quality_name || "?"} (tier ${e.quality_tier}) | Format: ${e.format_score} | Score: ${e.composite_score.toFixed(1)}\n`;
              if (e.parsed_source) output += `    Parsed: ${e.parsed_source}`;
              if (e.parsed_resolution)
                output += ` ${e.parsed_resolution}p`;
              if (e.parsed_source || e.parsed_resolution) output += "\n";
            }
            output += "\n";
          }

          if (rejected.length) {
            output += `### Rejected (${rejected.length})\n`;
            // Group by reason
            const byReason = {};
            for (const e of rejected) {
              const reason = e.reject_reason || "unknown";
              if (!byReason[reason]) byReason[reason] = [];
              byReason[reason].push(e);
            }
            for (const [reason, items] of Object.entries(byReason)) {
              output += `  **${reason}** (${items.length}):\n`;
              for (const e of items.slice(0, 5)) {
                output += `    ✗ ${e.title}`;
                if (e.parsed_source) output += ` [${e.parsed_source}]`;
                if (e.parsed_resolution) output += ` [${e.parsed_resolution}p]`;
                output += "\n";
              }
              if (items.length > 5)
                output += `    ... and ${items.length - 5} more\n`;
            }
          }
        }

        return { content: [{ type: "text", text: output }] };
      }

      case "search_debug_stats": {
        const data = await loomFetch("/api/v1/search-debug/stats");

        let output = `# Search Debug Stats (Last 7 Days)\n\n`;
        output += `**Total Searches:** ${data.total_searches}\n\n`;

        output += `## Outcomes\n`;
        for (const [outcome, count] of Object.entries(
          data.outcome_counts || {}
        )) {
          const pct = data.total_searches
            ? ((count / data.total_searches) * 100).toFixed(1)
            : 0;
          output += `- ${outcome}: ${count} (${pct}%)\n`;
        }

        if (data.top_reject_reasons?.length) {
          output += `\n## Top Reject Reasons\n`;
          for (const r of data.top_reject_reasons.slice(0, 10)) {
            output += `- ${r.reason}: ${r.count}\n`;
          }
        }

        return { content: [{ type: "text", text: output }] };
      }

      case "search_logs_for_media": {
        // List with filters — we'll use the list endpoint and scan for title match
        const params = new URLSearchParams();
        if (args?.media_type) params.set("media_type", args.media_type);
        params.set("limit", String(args?.limit || 50));

        const data = await loomFetch(
          `/api/v1/search-debug?${params.toString()}`
        );

        // Client-side title filter (API doesn't support title search)
        const titleLower = args.title.toLowerCase();
        const filtered = data.entries.filter((e) =>
          e.title.toLowerCase().includes(titleLower)
        );

        let output = `Search logs matching "${args.title}" (${filtered.length} found)\n\n`;
        for (const e of filtered) {
          const time = new Date(e.created_at).toLocaleString();
          output += `[${e.outcome.toUpperCase()}] ${time} - ${e.title}\n`;
          output += `  ID: ${e.id}\n`;
          output += `  Results: ${e.total_results} found, ${e.total_rejected} rejected\n`;
          if (e.grabbed_title) output += `  Grabbed: ${e.grabbed_title}\n`;
          output += "\n";
        }

        return { content: [{ type: "text", text: output }] };
      }

      default:
        return {
          content: [{ type: "text", text: `Unknown tool: ${name}` }],
          isError: true,
        };
    }
  } catch (error) {
    return {
      content: [
        {
          type: "text",
          text: `Error: ${error.message}\n\nMake sure Loom is running at ${LOOM_BASE_URL}`,
        },
      ],
      isError: true,
    };
  }
});

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
}

main().catch(console.error);
