# Playwright E2E Testing & MCP Setup

This Loom project now includes Playwright for end-to-end testing and an MCP (Model Context Protocol) server for automated test execution.

## Quick Start

### Running Tests Manually

```bash
# Run all tests
cd web && pnpm test:e2e

# Run specific test file
cd web && pnpm exec playwright test movies.spec.ts

# Run with UI (interactive mode)
cd web && pnpm exec playwright test --ui

# Run in debug mode
cd web && pnpm exec playwright test --debug

# List all tests
cd web && pnpm exec playwright test --list
```

### Using the MCP for Test Execution

The Playwright MCP server is configured in `.mcp.json` and exposes three tools:

#### 1. **run_tests** - Run all tests with optional filtering
```
Tool: run_tests
Parameters:
  - filter (optional): Test name filter (e.g., "movies", "smoke", "auth")
  - ui (optional): Run with UI mode (true/false)
  - debug (optional): Run in debug mode (true/false)
```

#### 2. **list_tests** - List all available tests
```
Tool: list_tests
Parameters: (none)
```

#### 3. **run_specific_test** - Run a specific test file or test
```
Tool: run_specific_test
Parameters:
  - testFile (required): Path or name (e.g., "movies.spec.ts" or "smoke")
```

## Test Files

### `smoke.spec.ts`
Basic smoke test verifying the dashboard loads with required UI elements.

### `movies.spec.ts`
Comprehensive E2E tests for the Movies library management feature:
- Modal flow for adding libraries
- Library type selection (Movies/Series)
- Manual path entry
- Navigation and button states
- Display of empty movie list

## Configuration

**Playwright Config** (`web/playwright.config.ts`):
- Base URL: `http://localhost:5173`
- Browser: Chromium
- Test directory: `web/e2e/`
- Web server: Started automatically on port 5173

**MCP Config** (`.mcp.json`):
- Playwright MCP server at `mcp-playwright/server.js`
- Exposes test execution tools via stdio transport

## Writing New Tests

1. Create a new `.spec.ts` file in `web/e2e/`
2. Import from `@playwright/test`:
   ```typescript
   import { test, expect } from "@playwright/test";
   ```
3. Write test cases:
   ```typescript
   test("should do something", async ({ page }) => {
     await page.goto("/path");
     await expect(page.getByRole("heading")).toBeVisible();
   });
   ```
4. Run with: `pnpm exec playwright test your-test.spec.ts`

## Debugging Tests

### Open DevTools
```bash
cd web && pnpm exec playwright test --debug
```

### Check trace files
Playwright automatically saves traces on first retry:
```bash
cd web && pnpm exec playwright show-trace trace.zip
```

### Use page.pause() in tests
Add `await page.pause()` to pause execution and inspect the page in DevTools.

## Continuous Integration

Tests can be run in CI with:
```bash
# Build first
pnpm build

# Run tests (CI mode runs once, no retries)
CI=true pnpm test:e2e
```

## MCP Server Development

The MCP server runs as a Node.js process and communicates via stdio:

**Location**: `mcp-playwright/server.js`

**To extend**:
1. Add new tool definitions in `ListToolsRequestSchema` handler
2. Implement tool logic in `CallToolRequestSchema` handler
3. Return results as `TextContent` objects

**To test MCP locally**:
```bash
cd mcp-playwright && node server.js
```

Then send tool calls via stdin (JSON-RPC 2.0 format).

## Troubleshooting

### Tests fail with "net::ERR_CONNECTION_REFUSED"
- Ensure backend is running on `localhost:8989`
- Ensure frontend dev server is running on `localhost:5173`

### "Authorization" errors in tests
- Tests assume user is logged in via cookies
- Add auth setup in `test.beforeEach()` if needed
- Use `credentials: "include"` in API calls

### MCP server not found
- Verify `mcp-playwright/server.js` exists
- Check `.mcp.json` configuration
- Reload MCP with: `copilot-cli reload-mcp` (if available)

## Next Steps

- [ ] Add auth flow E2E tests
- [ ] Add dashboard E2E tests
- [ ] Add indexers management tests
- [ ] Add series/shows tests
- [ ] Add download client tests
- [ ] Set up CI/CD integration (GitHub Actions)
