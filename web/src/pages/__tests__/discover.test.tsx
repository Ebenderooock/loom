import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DiscoverPage } from "../discover";
import type { TitleIdentity } from "@/lib/title-api";

const state = vi.hoisted(() => ({
  role: "admin",
  writes: [] as string[],
  fail: false,
  dismissed: false,
}));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { id: 1, role: state.role } }),
}));
vi.mock("@tanstack/react-router", () => ({
  Link: ({
    search,
    children,
  }: {
    search: { tmdb?: number };
    children: React.ReactNode;
  }) => <a href={`/collections?tmdb=${search.tmdb}`}>{children}</a>,
}));
vi.mock("@/components/titles/title-experience", () => ({
  TitleDialog: ({
    identity,
    onOpenChange,
    requestAction,
  }: {
    identity: TitleIdentity;
    onOpenChange: (open: boolean) => void;
    requestAction?: React.ReactNode;
  }) => (
    <div role="dialog" aria-label="Shared title details">
      <p>
        {identity.media}:{identity.id}
      </p>
      {requestAction}
      <button onClick={() => onOpenChange(false)}>Close details</button>
    </div>
  ),
}));

const movie = {
  tmdb_id: 42,
  media_type: "movie",
  title: "Another adventure",
  year: 2020,
  rating: 7.5,
  popularity: 30,
  reasons: ["You have 2 movies starring this actor"],
  source: "tmdb",
  collection_id: 10,
};
let client: QueryClient;

beforeEach(() => {
  state.role = "admin";
  state.writes = [];
  state.fail = false;
  state.dismissed = false;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input);
    if (init?.method === "POST") state.writes.push(url);
    if (url === "/api/v1/discover/dismissals") {
      state.dismissed = true;
      return new Response(null, { status: 204 });
    }
    if (url.startsWith("/api/v1/discover/recommendations")) {
      if (state.fail)
        return new Response("TMDB access is not configured", { status: 503 });
      return Response.json({
        items: state.dismissed
          ? []
          : [
              {
                ...movie,
                ...(url.includes("series")
                  ? {
                      media_type: "series",
                      title: "Another show",
                      collection_id: undefined,
                    }
                  : {}),
              },
            ],
        warnings: [],
        stale: false,
        generated_at: "2026-10-28",
      });
    }
    if (url.startsWith("/api/v1/import-lists/discover"))
      return Response.json({ items: [], total: 0 });
    throw new Error(`Unexpected request ${url}`);
  });
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
});
function page() {
  render(
    <QueryClientProvider client={client}>
      <DiscoverPage />
    </QueryClientProvider>,
  );
}

describe("Discover recommendations", () => {
  it("keeps Collections out of Discover and links to the standalone source detail", async () => {
    page();
    expect(await screen.findByText("Another adventure")).toBeInTheDocument();
    expect(
      screen.queryByRole("tab", { name: "Collections" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "View collection" }),
    ).toHaveAttribute("href", "/collections?tmdb=10");
    expect(
      screen.getByText("You have 2 movies starring this actor"),
    ).toBeInTheDocument();
    expect(state.writes).toEqual([]);
  });
  it.each(["Movies", "TV shows"])(
    "opens the shared external details for %s without writing",
    async (tab) => {
      page();
      const user = userEvent.setup();
      await user.click(screen.getByRole("tab", { name: tab }));
      await user.click(
        await screen.findByRole("button", { name: "Details & add" }),
      );
      expect(await screen.findByRole("dialog")).toHaveTextContent(
        `${tab === "Movies" ? "movie" : "series"}:42`,
      );
      await user.click(screen.getByRole("button", { name: "Close details" }));
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
      expect(state.writes).toEqual([]);
    },
  );
  it("does not offer direct adds to non-admins", async () => {
    state.role = "user";
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Details & add" }),
    );
    expect(await screen.findByRole("dialog")).toHaveTextContent(
      "Ask an administrator",
    );
  });
  it("dismisses a saved suggestion and shows the empty state", async () => {
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Dismiss Another adventure" }),
    );
    expect(
      await screen.findByText(/No new suggestions yet/),
    ).toBeInTheDocument();
    expect(state.writes).toEqual(["/api/v1/discover/dismissals"]);
  });
  it("surfaces provider errors and retries without claiming an empty success", async () => {
    state.fail = true;
    page();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "TMDB access is not configured",
    );
    state.fail = false;
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Another adventure")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
  it("retains optional Import Lists discovery", async () => {
    page();
    await userEvent.click(screen.getByRole("tab", { name: "Import Lists" }));
    await waitFor(() =>
      expect(screen.getByText("Nothing to discover yet")).toBeInTheDocument(),
    );
    expect(state.writes).toEqual([]);
  });
});
