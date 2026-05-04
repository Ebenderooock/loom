#!/usr/bin/env node

const { Server } = require("@modelcontextprotocol/sdk/server/index.js");
const {
  StdioServerTransport,
} = require("@modelcontextprotocol/sdk/server/stdio.js");
const {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  TextContent,
} = require("@modelcontextprotocol/sdk/types.js");
const { execSync } = require("child_process");
const path = require("path");

const server = new Server({
  name: "playwright-test-runner",
  version: "1.0.0",
});

// Define tools
server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [
    {
      name: "run_tests",
      description:
        "Run all Playwright E2E tests. Returns test results with pass/fail status.",
      inputSchema: {
        type: "object",
        properties: {
          filter: {
            type: "string",
            description:
              "Optional test name filter (e.g., 'smoke', 'auth', 'movies')",
          },
          ui: {
            type: "boolean",
            description: "Run with UI mode (default: false)",
          },
          debug: {
            type: "boolean",
            description: "Run in debug mode (default: false)",
          },
        },
        required: [],
      },
    },
    {
      name: "list_tests",
      description: "List all available Playwright test files and their tests.",
      inputSchema: {
        type: "object",
        properties: {},
        required: [],
      },
    },
    {
      name: "run_specific_test",
      description: "Run a specific test file or test name.",
      inputSchema: {
        type: "object",
        properties: {
          testFile: {
            type: "string",
            description: "Path to test file (e.g., 'smoke.spec.ts') or test name",
          },
        },
        required: ["testFile"],
      },
    },
  ],
}));

// Tool execution
server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request;
  const webDir = path.join(__dirname, "..", "web");

  try {
    if (name === "run_tests") {
      const { filter = "", ui = false, debug = false } = args;
      let cmd = `cd ${webDir} && pnpm exec playwright test`;

      if (filter) {
        cmd += ` ${filter}`;
      }
      if (ui) {
        cmd += " --ui";
      }
      if (debug) {
        cmd += " --debug";
      }

      const result = execSync(cmd, { encoding: "utf-8", stdio: "pipe" });
      return {
        content: [
          new TextContent({
            type: "text",
            text: `Playwright Tests Executed\n\n${result}`,
          }),
        ],
      };
    }

    if (name === "list_tests") {
      const e2eDir = path.join(webDir, "e2e");
      let result;
      try {
        result = execSync(`cd ${webDir} && pnpm exec playwright test --list`, {
          encoding: "utf-8",
          stdio: "pipe",
        });
      } catch (e) {
        result = e.stdout || "No tests found";
      }
      return {
        content: [
          new TextContent({
            type: "text",
            text: `Available Playwright Tests\n\n${result}`,
          }),
        ],
      };
    }

    if (name === "run_specific_test") {
      const { testFile } = args;
      const cmd = `cd ${webDir} && pnpm exec playwright test ${testFile}`;
      const result = execSync(cmd, { encoding: "utf-8", stdio: "pipe" });
      return {
        content: [
          new TextContent({
            type: "text",
            text: `Test Results for ${testFile}\n\n${result}`,
          }),
        ],
      };
    }

    throw new Error(`Unknown tool: ${name}`);
  } catch (error) {
    const errorOutput = error.stderr || error.stdout || error.message;
    return {
      content: [
        new TextContent({
          type: "text",
          text: `Error running test: ${errorOutput}`,
        }),
      ],
      isError: true,
    };
  }
});

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("Playwright MCP Server running on stdio");
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
