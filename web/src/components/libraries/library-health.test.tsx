import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { LibraryHealth } from "./library-health";
import type { HealthReport } from "@/lib/libraries-api";

const api = vi.hoisted(() => ({
  getLibraryHealth: vi.fn(),
  runLibraryHealth: vi.fn(),
  dismissLibraryHealth: vi.fn(),
}));
vi.mock("@/lib/libraries-api", () => api);

function report(): HealthReport {
  return {
    library_id: "lib",
    root: "/media",
    status: "incomplete",
    checked_at: "2026-10-23T01:00:00Z",
    scanned_entries: 12,
    inspected_records: 3,
    entry_limit: 10000,
    warnings: [
      "Mount availability is uncertain. Absence is not confirmed deletion.",
    ],
    findings: [
      {
        id: "sample",
        kind: "sample",
        path: "/media/sample.mkv",
        related_paths: ["/media/feature.mkv"],
        confidence: "low",
        dismissed: false,
        evidence: ["Sample token in path; duration is unknown."],
        preview: {
          action: "review_selection",
          outcome: "No changes. Compare playback before manual action.",
          executable: false,
        },
      },
    ],
  };
}

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LibraryHealth library={{ id: "lib", name: "Movies" }} />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

beforeEach(() => {
  vi.resetAllMocks();
  api.getLibraryHealth.mockResolvedValue(null);
});
afterEach(cleanup);

describe("LibraryHealth", () => {
  it("does not present a missing report as a clean library", async () => {
    const user = setup();
    await user.click(
      screen.getByRole("button", { name: "Library health for Movies" }),
    );
    expect(await screen.findByText(/No health report yet/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Run health check" }),
    ).toBeEnabled();
  });

  it("shows evidence, mount uncertainty and a non-executable per-item preview", async () => {
    api.runLibraryHealth.mockResolvedValue(report());
    const user = setup();
    await user.click(
      screen.getByRole("button", { name: "Library health for Movies" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Run health check" }),
    );
    expect(await screen.findByText("Suspected sample")).toBeInTheDocument();
    expect(screen.getByText("incomplete")).toBeInTheDocument();
    expect(
      screen.getByText(/Mount availability is uncertain/),
    ).toBeInTheDocument();
    expect(screen.getByText(/Sample token in path/)).toBeInTheDocument();
    await user.click(screen.getByText("Action preview (read-only)"));
    expect(
      screen.getByText("No changes. Compare playback before manual action."),
    ).toBeVisible();
    expect(screen.getByText(/Execution is not available/)).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /delete|fix|execute/i }),
    ).not.toBeInTheDocument();
  });

  it("retains dismissals across recheck and supports restoring a finding", async () => {
    const initial = report();
    const dismissed = report();
    dismissed.findings = dismissed.findings.map((finding) => ({
      ...finding,
      dismissed: true,
    }));
    api.getLibraryHealth.mockResolvedValue(initial);
    api.dismissLibraryHealth
      .mockResolvedValueOnce(dismissed)
      .mockResolvedValueOnce(initial);
    api.runLibraryHealth.mockResolvedValue(dismissed);
    const user = setup();
    await user.click(
      screen.getByRole("button", { name: "Library health for Movies" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Dismiss finding" }),
    );
    expect(api.dismissLibraryHealth).toHaveBeenCalledWith(
      "lib",
      "sample",
      true,
    );
    expect(await screen.findByText(/No visible findings/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Recheck library" }));
    await waitFor(() => expect(api.runLibraryHealth).toHaveBeenCalledTimes(1));
    await user.click(
      screen.getByRole("checkbox", { name: "Show dismissed findings" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Restore finding" }),
    );
    expect(api.dismissLibraryHealth).toHaveBeenLastCalledWith(
      "lib",
      "sample",
      false,
    );
    expect(
      await screen.findByRole("button", { name: "Dismiss finding" }),
    ).toBeEnabled();
  });

  it("cancels the actual fetch with AbortSignal, without showing a new success", async () => {
    let signal: AbortSignal | undefined;
    api.runLibraryHealth.mockImplementation(
      (_id: string, abort: AbortSignal) => {
        signal = abort;
        return new Promise((_resolve, reject) =>
          abort.addEventListener("abort", () =>
            reject(new DOMException("Cancelled", "AbortError")),
          ),
        );
      },
    );
    const user = setup();
    await user.click(
      screen.getByRole("button", { name: "Library health for Movies" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Run health check" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Cancel check" }),
    );
    expect(signal?.aborted).toBe(true);
    expect(await screen.findByText(/Check cancelled/)).toBeInTheDocument();
    expect(screen.queryByText("complete")).not.toBeInTheDocument();
  });

  it("surfaces API errors and paginates large reports", async () => {
    const large = report();
    large.findings = Array.from({ length: 26 }, (_, i) => ({
      ...large.findings[0]!,
      id: `finding-${i}`,
      path: `/media/sample-${i}.mkv`,
    }));
    api.getLibraryHealth.mockResolvedValue(large);
    api.runLibraryHealth.mockRejectedValue(
      new Error("Library scan active; retry later"),
    );
    const user = setup();
    await user.click(
      screen.getByRole("button", { name: "Library health for Movies" }),
    );
    expect(await screen.findByText("Page 1 of 2")).toBeInTheDocument();
    expect(screen.getAllByRole("article")).toHaveLength(25);
    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(screen.getAllByRole("article")).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: "Recheck library" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Library scan active; retry later",
    );
  });
});
