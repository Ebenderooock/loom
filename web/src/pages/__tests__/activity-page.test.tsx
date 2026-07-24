import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ActivityPage } from "../activity";

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { role: "admin" } }),
}));

vi.mock("@/lib/fetch", () => ({
  apiFetch: vi.fn(async (url: string) => {
    if (url.startsWith("/api/v1/reviews/count")) {
      return { ok: true, json: async () => ({ count: 0 }) };
    }
    if (url.startsWith("/api/v1/downloads/history")) {
      return { ok: true, json: async () => [] };
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
  it("shows history-focused tabs without queue entry", async () => {
    render(<ActivityPage />);

    expect(await screen.findByRole("tab", { name: /History/i })).toBeVisible();
    expect(screen.getByRole("tab", { name: /Blocklist/i })).toBeVisible();
    expect(screen.getByRole("tab", { name: /Reviews/i })).toBeVisible();
    expect(
      screen.queryByRole("tab", { name: /^Queue$/i }),
    ).not.toBeInTheDocument();
  });
});
