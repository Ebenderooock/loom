import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReleaseSearchDialog } from "./release-search-dialog";
import { ApiError } from "@/lib/downloads-api";

const mutateAsync = vi.fn(async (_request: unknown) => ({
  client_id: "qb",
  item_id: "x",
}));
const downloadClients = vi.hoisted(() => ({
  value: [{ id: "qb", name: "qBit", enabled: true }],
}));

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
  ApiError: class extends Error {
    constructor(
      public status: number,
      message: string,
      public code?: string,
    ) {
      super(message);
    }
  },
  checkBlocklist: vi.fn(async (releases: { title: string }[]) =>
    releases.map((r) => r.title === "Bad.Release"),
  ),
  useDownloads: () => ({
    data: downloadClients.value,
  }),
  useGrabRelease: () => ({ mutateAsync }),
}));

describe("ReleaseSearchDialog blocklist", () => {
  beforeEach(() => {
    mutateAsync
      .mockReset()
      .mockResolvedValue({ client_id: "qb", item_id: "x" });
    downloadClients.value = [{ id: "qb", name: "qBit", enabled: true }];
  });

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

  it("downloads the chosen release with its media context and prevents duplicate requests", async () => {
    let finish!: (value: { client_id: string; item_id: string }) => void;
    mutateAsync.mockImplementationOnce(
      () => new Promise((resolve) => (finish = resolve)),
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ReleaseSearchDialog
          open
          onOpenChange={() => {}}
          title="Movie"
          mediaType="movie"
          movieId="movie-42"
          autoSearch
        />
      </QueryClientProvider>,
    );

    const download = await screen.findByRole("button", {
      name: "Download Good.Release",
    });
    await userEvent.dblClick(download);
    expect(mutateAsync).toHaveBeenCalledTimes(1);
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        clientId: "qb",
        torrent_url: "l2",
        title: "Good.Release",
        indexer_id: "ix",
        media_type: "movie",
        movie_id: "movie-42",
      }),
    );
    expect(mutateAsync.mock.calls[0]?.[0]).not.toHaveProperty(
      "ignore_blocklist",
    );
    expect(
      screen.getByRole("button", { name: "Sending… Good.Release" }),
    ).toBeDisabled();
    finish({ client_id: "qb", item_id: "x" });
    expect(
      await screen.findByRole("button", { name: "Sent Good.Release" }),
    ).toBeDisabled();
  });

  it("keeps server blocklist rejection visible and allows an explicit override retry", async () => {
    mutateAsync.mockRejectedValueOnce(
      new ApiError(409, "Blocked", "blocklisted"),
    );
    render(
      <QueryClientProvider client={new QueryClient()}>
        <ReleaseSearchDialog
          open
          onOpenChange={() => {}}
          title="Episode"
          mediaType="episode"
          seriesId="series-42"
          episodeIds={["episode-2"]}
          season={1}
          episode={2}
          autoSearch
        />
      </QueryClientProvider>,
    );
    const download = await screen.findByRole("button", {
      name: "Download Good.Release",
    });
    await userEvent.click(download);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "This release is blocklisted",
    );
    expect(download).toBeEnabled();
    await userEvent.click(screen.getByLabelText("Ignore blocklist"));
    await userEvent.click(download);
    expect(mutateAsync).toHaveBeenLastCalledWith(
      expect.objectContaining({
        torrent_url: "l2",
        media_type: "episode",
        series_id: "series-42",
        episode_ids: ["episode-2"],
        ignore_blocklist: true,
      }),
    );
    expect(
      await screen.findByRole("button", { name: "Sent Good.Release" }),
    ).toBeDisabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps filtering and optional indexer diagnostics available", async () => {
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
    await screen.findByRole("button", { name: "Download Good.Release" });
    const summary = screen.getByText("Indexer details").closest("details");
    expect(summary).not.toHaveAttribute("open");
    await userEvent.click(screen.getByText("Indexer details"));
    expect(summary).toHaveAttribute("open");
    await userEvent.type(
      screen.getByRole("textbox", { name: "Filter releases by name" }),
      "missing",
    );
    expect(
      screen.queryByRole("button", { name: "Download Good.Release" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("No results match the current filters."),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(
      screen.getByRole("button", { name: "Download Good.Release" }),
    ).toBeInTheDocument();
  });

  it("uses the chosen client when several clients are configured", async () => {
    downloadClients.value.push({
      id: "other",
      name: "Other client",
      enabled: true,
    });
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
    await userEvent.click(
      await screen.findByRole("button", { name: "Download Good.Release" }),
    );
    await userEvent.click(
      screen.getByRole("menuitem", { name: "Other client" }),
    );
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ clientId: "other", torrent_url: "l2" }),
    );
    expect(
      await screen.findByRole("button", { name: "Sent Good.Release" }),
    ).toBeDisabled();
  });

  it("disables downloading without clients and explains why", async () => {
    downloadClients.value = [];
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
    expect(
      await screen.findByRole("button", {
        name: "Download Good.Release — no download clients configured",
      }),
    ).toBeDisabled();
    expect(mutateAsync).not.toHaveBeenCalled();
  });

  it("shows a failed download inline and supports retrying", async () => {
    mutateAsync.mockRejectedValueOnce(new Error("Client connection refused"));
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
    const download = await screen.findByRole("button", {
      name: "Download Good.Release",
    });
    await userEvent.click(download);
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Client connection refused",
    );
    await userEvent.click(download);
    expect(mutateAsync).toHaveBeenCalledTimes(2);
    expect(
      await screen.findByRole("button", { name: "Sent Good.Release" }),
    ).toBeDisabled();
  });
});
