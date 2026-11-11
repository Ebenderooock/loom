import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DownloadsPage } from "../downloads";
import { apiFetch } from "@/lib/fetch";

const state = vi.hoisted(() => ({
  activity: {
    items: [] as Record<string, unknown>[],
    errors: {} as Record<string, string>,
  },
  pending: null as null | (() => void),
}));

vi.mock("@/hooks/use-page-header", () => ({ useSetPageHeader: () => {} }));

vi.mock("@/lib/fetch", () => ({
  apiFetch: vi.fn(async (url: string) => {
    if (url === "/api/v1/activity") {
      if (state.pending) {
        await new Promise<void>((resolve) => {
          state.pending = resolve;
        });
      }
      return { ok: true, json: async () => state.activity };
    }
    if (url.startsWith("/api/v1/download-clients")) {
      return { ok: true, json: async () => [] };
    }
    return { ok: true, json: async () => ({}) };
  }),
}));

const activityCalls = () =>
  vi.mocked(apiFetch).mock.calls.filter(([u]) => u === "/api/v1/activity")
    .length;

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <DownloadsPage />
    </QueryClientProvider>,
  );
}

describe("DownloadsPage active downloads", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
    state.activity = { items: [], errors: {} };
    state.pending = null;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows unreachable download clients instead of an empty queue", async () => {
    state.activity.errors = {
      "deluge-nas": "connection refused (retrying in 20s)",
    };
    renderPage();

    const alert = await screen.findByTestId("download-client-errors");
    expect(alert).toHaveTextContent("A download client is unreachable");
    expect(alert).toHaveTextContent(
      "deluge-nas: connection refused (retrying in 20s)",
    );
    expect(
      screen.getByText("No downloads from reachable clients"),
    ).toBeVisible();
  });

  it("does not stack activity requests while one is pending", async () => {
    vi.useFakeTimers();
    state.pending = () => {};
    renderPage();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(activityCalls()).toBe(1);

    await act(async () => {
      state.pending?.();
      state.pending = null;
      await vi.advanceTimersByTimeAsync(2_000);
    });
    expect(activityCalls()).toBe(2);
  });
});
