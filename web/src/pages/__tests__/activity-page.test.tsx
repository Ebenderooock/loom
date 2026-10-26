import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActivityPage } from "../activity";
import { apiFetch } from "@/lib/fetch";

const state = vi.hoisted(() => ({
  role: "admin",
  entries: [] as Record<string, unknown>[],
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { role: state.role } }),
}));

vi.mock("@/lib/fetch", () => ({
  apiFetch: vi.fn(async (url: string) => {
    if (url === "/api/v1/downloads/history/actions") {
      return {
        ok: true,
        json: async () => ({ results: [{ id: "failure", ok: true }] }),
      };
    }
    if (url.startsWith("/api/v1/reviews/count")) {
      return { ok: true, json: async () => ({ count: 0 }) };
    }
    if (url.startsWith("/api/v1/downloads/history")) {
      return { ok: true, json: async () => state.entries };
    }
    if (url.startsWith("/api/v1/blocklist")) {
      return { ok: true, json: async () => ({ data: [] }) };
    }
    if (url.startsWith("/api/v1/reviews")) {
      return { ok: true, json: async () => ({ data: [] }) };
    }
    return { ok: true, json: async () => ({}) };
  }),
}));

describe("ActivityPage consolidation", () => {
  beforeEach(() => {
    cleanup();
    state.role = "admin";
    state.entries = [];
    vi.clearAllMocks();
  });

  it("shows history-focused tabs without queue entry", async () => {
    render(<ActivityPage />);

    expect(await screen.findByRole("tab", { name: /History/i })).toBeVisible();
    expect(screen.getByRole("tab", { name: /Blocklist/i })).toBeVisible();
    expect(screen.getByRole("tab", { name: /Reviews/i })).toBeVisible();
    expect(
      screen.queryByRole("tab", { name: /^Queue$/i }),
    ).not.toBeInTheDocument();
  });

  it("shows durable failure details and confirms administrator retry", async () => {
    state.entries = [
      {
        id: "failure",
        title: "Release",
        status: "acquisition_failed",
        completed_at: "2026-10-23T00:00:00Z",
        acquisition_key: "acquisition:release",
        stage: "provider",
        reason: "Indexer download fetch failed.",
        attempt: 3,
        retry_eligible: false,
        is_latest: true,
        media_type: "movie",
        media_id: "movie-1",
        indexer_id: "tracker",
      },
    ];
    const user = userEvent.setup();
    render(<ActivityPage />);
    expect(
      await screen.findByText("Indexer download fetch failed."),
    ).toBeVisible();
    expect(screen.getByText("Automatic retry stopped")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Enable retry" }));
    expect(screen.getByText(/check the client queue first/i)).toBeVisible();
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Enable retry",
      }),
    );
    expect(apiFetch).toHaveBeenCalledWith(
      "/api/v1/downloads/history/actions",
      expect.objectContaining({
        method: "POST",
        body: '{"ids":["failure"],"action":"retry"}',
      }),
    );
  });

  it("does not expose mutation actions to non-admin users", async () => {
    state.role = "user";
    state.entries = [
      {
        id: "failure",
        title: "Release",
        status: "acquisition_failed",
        completed_at: "2026-10-23T00:00:00Z",
        acquisition_key: "acquisition:release",
        reason: "Client unavailable.",
        is_latest: true,
      },
    ];
    render(<ActivityPage />);
    expect(await screen.findByText("Client unavailable.")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Enable retry" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Blocklist" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Clear History" }),
    ).not.toBeInTheDocument();
  });

  it("does not offer release blocklisting for a provider search failure", async () => {
    state.entries = [
      {
        id: "failure",
        title: "Movie search",
        status: "acquisition_failed",
        completed_at: "2026-10-23T00:00:00Z",
        acquisition_key: "search:query",
        stage: "provider_search",
        reason: "Indexer search failed.",
        is_latest: true,
      },
    ];
    render(<ActivityPage />);
    expect(await screen.findByText("Indexer search failed.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Enable retry" })).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Blocklist" }),
    ).not.toBeInTheDocument();
  });
});
