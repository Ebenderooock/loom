import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReleaseSearchDialog } from "./release-search-dialog";

const mutateAsync = vi.fn(async () => ({ client_id: "qb", item_id: "x" }));

vi.mock("@/lib/indexers-api", () => ({
  streamSearch: (
    _params: unknown,
    cb: {
      onIndexerResult: (
        id: string,
        name: string,
        results: unknown[],
        count: number,
        elapsed: number,
      ) => void;
      onDone: () => void;
    },
  ) => {
    queueMicrotask(() => {
      cb.onIndexerResult(
        "ix",
        "Indexer",
        [
          { indexer_id: "ix", title: "Bad.Release", link: "l1" },
          { indexer_id: "ix", title: "Good.Release", link: "l2" },
        ],
        2,
        10,
      );
      cb.onDone();
    });
    return new AbortController();
  },
}));

vi.mock("./policy-preview", () => ({ PolicyPreview: () => null }));

vi.mock("@/lib/downloads-api", () => ({
  ApiError: class extends Error {},
  checkBlocklist: vi.fn(async (releases: { title: string }[]) =>
    releases.map((r) => r.title === "Bad.Release"),
  ),
  useDownloads: () => ({
    data: [{ id: "qb", name: "qBit", enabled: true }],
  }),
  useGrabRelease: () => ({ mutateAsync }),
}));

describe("ReleaseSearchDialog blocklist", () => {
  beforeEach(() => mutateAsync.mockClear());

  it("hides blocklisted releases until Ignore blocklist is enabled", async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ReleaseSearchDialog
          open
          onOpenChange={() => {}}
          title="Movie"
          mediaType="movie"
          autoSearch
        />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Good.Release")).toBeInTheDocument();
    await screen.findByText(/1 blocklisted release hidden/);
    expect(screen.queryByText("Bad.Release")).not.toBeInTheDocument();

    await userEvent.click(screen.getByLabelText("Ignore blocklist"));

    expect(await screen.findByText("Bad.Release")).toBeInTheDocument();
    expect(screen.getByText("Blocklisted")).toBeInTheDocument();

    const grabButtons = screen.getAllByTitle("Grab via qBit");
    await userEvent.click(grabButtons[0]!);
    await waitFor(() => expect(mutateAsync).toHaveBeenCalled());
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Bad.Release",
        ignore_blocklist: true,
      }),
    );
  });
});
