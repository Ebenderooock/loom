import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NotificationsPage } from "./notifications";

vi.mock("@/hooks/use-page-header", () => ({ useSetPageHeader: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const initialConnection = {
  id: "connection-1",
  name: "Alerts",
  type: "discord",
  enabled: true,
  settings: {},
  on_grab: true,
  on_download: true,
  on_upgrade: false,
  on_rename: false,
  on_delete: false,
  on_health_issue: true,
  on_application_update: false,
  on_update_available: true,
  on_playback: false,
  tags: [],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

let connections: Array<Partial<typeof initialConnection>>;
const fetchMock = vi.fn<typeof fetch>();
const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });

function show() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <NotificationsPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  connections = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.includes("/notifications/history")) return json([]);
    if (url.endsWith("/notifications/test")) return json({ ok: true });
    if (init?.method === "POST") return json(JSON.parse(String(init.body)));
    if (init?.method === "PUT") return json(JSON.parse(String(init.body)));
    if (url.endsWith("/notifications")) return json(connections);
    throw new Error(`Unexpected request: ${init?.method} ${url}`);
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

function requestBody(method: string, url: string) {
  const call = fetchMock.mock.calls.find(
    ([input, init]) => String(input).endsWith(url) && init?.method === method,
  );
  if (!call) throw new Error(`Missing ${method} ${url} request`);
  return JSON.parse(String(call[1]?.body));
}

describe("notification update subscriptions", () => {
  it("creates a connection with the new event disabled by default", async () => {
    const user = userEvent.setup();
    show();
    await user.click(screen.getByRole("button", { name: "Add notification" }));
    await user.type(screen.getByLabelText("Name"), "Alerts");
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(requestBody("POST", "/notifications")).toMatchObject({
        on_update_available: false,
        on_application_update: false,
      });
    });
  });

  it("defaults the new event off and sends independent choices for testing and creation", async () => {
    const user = userEvent.setup();
    show();
    await user.click(screen.getByRole("button", { name: "Add notification" }));
    const available = screen.getByRole("checkbox", {
      name: "On Verified Update Available",
    });
    const appUpdate = screen.getByRole("checkbox", { name: "On App Update" });
    expect(available).not.toBeChecked();
    expect(appUpdate).not.toBeChecked();
    await user.type(screen.getByLabelText("Name"), "Alerts");
    await user.click(available);
    expect(appUpdate).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "Test" }));
    await waitFor(() => {
      expect(requestBody("POST", "/notifications/test")).toMatchObject({
        on_update_available: true,
        on_application_update: false,
      });
    });
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(requestBody("POST", "/notifications")).toMatchObject({
        on_update_available: true,
        on_application_update: false,
      });
    });
  });

  it("shows a saved subscription and sends edited values independently", async () => {
    connections = [initialConnection];
    const user = userEvent.setup();
    show();
    const row = (await screen.findByText("Alerts")).closest("tr");
    if (!row) throw new Error("Missing connection row");
    expect(
      within(row).getByText("On Verified Update Available"),
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "Actions for Alerts" }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Edit" }));
    const available = screen.getByRole("checkbox", {
      name: "On Verified Update Available",
    });
    const appUpdate = screen.getByRole("checkbox", { name: "On App Update" });
    expect(available).toBeChecked();
    expect(appUpdate).not.toBeChecked();
    await user.click(available);
    await user.click(appUpdate);
    await user.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => {
      expect(requestBody("PUT", "/notifications/connection-1")).toMatchObject({
        on_update_available: false,
        on_application_update: true,
      });
    });
  });

  it("treats a missing field in an older response as disabled", async () => {
    const { on_update_available: _omitted, ...olderConnection } =
      initialConnection;
    connections = [olderConnection];
    const user = userEvent.setup();
    show();
    await user.click(
      await screen.findByRole("button", { name: "Actions for Alerts" }),
    );
    await user.click(screen.getByRole("menuitem", { name: "Edit" }));
    expect(
      screen.getByRole("checkbox", { name: "On Verified Update Available" }),
    ).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "Update" }));
    await waitFor(() => {
      expect(requestBody("PUT", "/notifications/connection-1")).toMatchObject({
        on_update_available: false,
      });
    });
  });
});
