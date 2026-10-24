import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { MediaRequest } from "@/lib/requests-api";
import { RequestsPage } from "../requests";

const state = vi.hoisted(() => ({
  rows: [] as MediaRequest[],
  create: vi.fn(),
  setHeader: vi.fn(),
}));

vi.mock("@/hooks/use-page-header", () => ({
  usePageHeader: () => ({ setHeader: state.setHeader }),
}));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { role: "user" } }),
}));
vi.mock("@/lib/features-api", () => ({
  useFeatureEnabled: () => false,
}));
vi.mock("@/lib/requests-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/requests-api")>()),
  useMyRequests: () => ({ data: state.rows, isLoading: false }),
  useCreateRequest: () => ({ mutateAsync: state.create, isPending: false }),
  useQuotaStatus: () => ({ data: undefined, isLoading: false }),
}));

const history: MediaRequest = {
  id: "original",
  user_id: "1",
  username: "alice",
  media_type: "movie",
  tmdb_id: "603",
  title: "The Matrix",
  year: 1999,
  status: "available",
  availability: "missing",
  availability_reason: "Verified storage; file is missing",
  media_server_availability: "unknown",
  monitored: false,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
  fulfilled_at: "2026-10-01T00:00:00Z",
};

describe("Request fulfillment history", () => {
  beforeEach(() => {
    state.rows = [{ ...history }];
    state.create.mockReset().mockResolvedValue({ id: "replacement" });
  });

  it("shows fulfillment separately from current presence, monitoring and server observations", () => {
    render(<RequestsPage />);
    expect(screen.getByText("Fulfilled")).toBeVisible();
    expect(screen.getByText("Currently missing")).toBeVisible();
    expect(screen.getByText("Unmonitored")).toBeVisible();
    expect(screen.getByText("Media server: unknown")).toBeVisible();
    expect(screen.getByText(/Verified storage; file is missing/)).toBeVisible();
  });

  it("submits an explicit history-linked request without reopening history", async () => {
    render(<RequestsPage />);
    fireEvent.click(screen.getByRole("button", { name: "Re-request" }));
    await waitFor(() =>
      expect(state.create).toHaveBeenCalledWith(
        expect.objectContaining({ re_request_of: "original", tmdb_id: "603" }),
      ),
    );
    expect(screen.getByText("Fulfilled")).toBeVisible();
  });

  it.each(["unknown", "present"] as const)(
    "does not offer re-acquisition for %s observations",
    (availability) => {
      state.rows = [{ ...history, availability }];
      render(<RequestsPage />);
      expect(
        screen.queryByRole("button", { name: "Re-request" }),
      ).not.toBeInTheDocument();
      expect(screen.getByText("Fulfilled")).toBeVisible();
    },
  );
});
