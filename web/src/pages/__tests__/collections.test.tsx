import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CollectionsPage } from "../collections";
import type { CollectionMember, LoomCollection } from "@/lib/collections-api";
import type { TitleIdentity } from "@/lib/title-api";

const state = vi.hoisted(() => ({
  search: {} as { id?: string; tmdb?: number },
  navigate: vi.fn(),
  role: "admin",
  add: vi.fn(),
}));
vi.mock("@tanstack/react-router", () => ({
  useSearch: () => state.search,
  useNavigate: () => state.navigate,
  Link: ({
    search,
    children,
  }: {
    search: { id: string };
    children: React.ReactNode;
  }) => <a href={`/collections?id=${search.id}`}>{children}</a>,
}));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { id: 1, role: state.role } }),
}));
vi.mock("@/components/library-media-sheet", () => ({
  LibraryMediaSheet: ({
    target,
    onClose,
    onChanged,
  }: {
    target: { type: string; id: string };
    onClose: () => void;
    onChanged: () => void;
  }) => (
    <div role="dialog" aria-label="Actual library management">
      {target.type}:{target.id}
      <button onClick={onChanged}>Refresh management</button>
      <button onClick={onClose}>Close management</button>
    </div>
  ),
}));
vi.mock("@/components/titles/title-experience", () => ({
  TitleDialog: ({
    identity,
    onAdded,
    onOpenChange,
  }: {
    identity: TitleIdentity;
    onAdded: () => void;
    onOpenChange: (open: boolean) => void;
  }) => (
    <div role="dialog" aria-label="Shared external details">
      {identity.media}:{identity.provider}:{identity.id}
      <button
        onClick={() => {
          state.add();
          onAdded();
        }}
      >
        Shared add form
      </button>
      <button onClick={() => onOpenChange(false)}>Close details</button>
    </div>
  ),
}));

let collection: LoomCollection;
let other: LoomCollection;
let requests: { url: string; method: string; body: Record<string, unknown> }[];
let client: QueryClient;
function member(overrides: Partial<CollectionMember>): CollectionMember {
  return {
    id: "m1",
    title: "A movie",
    media_type: "movie",
    external_ids: { tmdb: "42" },
    sources: ["tmdb:collection:1"],
    provenance: ["tmdb:collection:1"],
    local: false,
    excluded: false,
    monitored: false,
    in_library: false,
    ...overrides,
  };
}
beforeEach(() => {
  state.search = {};
  state.role = "admin";
  state.navigate.mockReset();
  state.add.mockReset();
  requests = [];
  collection = {
    id: "loom-1",
    name: "Mixed universe",
    overview: "Movies and shows",
    revision: 3,
    total: 2,
    in_library: 1,
    sources: [
      {
        provider: "tmdb",
        kind: "collection",
        external_id: "1",
        error: "Source access failed. Retry after checking credentials.",
      },
    ],
    members: [
      member({
        in_library: true,
        media_id: "actual-movie",
        library_id: "movies",
        has_file: false,
      }),
      member({ id: "s1", title: "A show", media_type: "series" }),
    ],
  };
  other = {
    ...collection,
    id: "loom-2",
    name: "Other universe",
    revision: 2,
    sources: [],
    members: [],
  };
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  state.add.mockImplementation(() => {
    collection = {
      ...collection,
      in_library: 2,
      members: collection.members.map((m) =>
        m.id === "s1"
          ? {
              ...m,
              in_library: true,
              media_id: "actual-show",
              library_id: "shows",
            }
          : m,
      ),
    };
  });
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input),
      method = init?.method ?? "GET";
    if (method !== "GET") {
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      requests.push({ url, method, body });
      if (url.endsWith("/refresh"))
        collection = {
          ...collection,
          sources: collection.sources.map((s) => ({
            ...s,
            error: undefined,
            last_sync: "2026-10-30T00:00:00Z",
          })),
        };
      if (url.endsWith("/members/m1"))
        collection = {
          ...collection,
          members: collection.members.map((m) =>
            m.id === "m1" ? { ...m, excluded: body.excluded as boolean } : m,
          ),
        };
      return Response.json(collection);
    }
    if (url === "/api/v1/libraries")
      return Response.json({
        data: [
          { id: "movies", name: "Movies", media_type: "movie" },
          { id: "shows", name: "Shows", media_type: "series" },
        ],
      });
    if (url === "/api/v1/collections/")
      return Response.json({ items: [collection, other] });
    if (url === "/api/v1/collections/scan")
      return Response.json({
        total: 1020,
        checked: 540,
        pending: 480,
        failed: 1,
        running: true,
        errors: [
          {
            media_type: "series",
            media_id: "show",
            message: "TVDB access failed; retry.",
          },
        ],
      });
    if (url === "/api/v1/collections/loom-1") return Response.json(collection);
    throw new Error(`Unexpected request ${method} ${url}`);
  });
});
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
});
function page() {
  render(
    <QueryClientProvider client={client}>
      <CollectionsPage />
    </QueryClientProvider>,
  );
}

describe("Standalone Collections", () => {
  it("shows automatic background progress and only queues an optional rescan", async () => {
    page();
    expect(
      await screen.findByText(/540 of 1020 library titles checked/),
    ).toBeInTheDocument();
    expect(requests).toEqual([]);
    await userEvent.click(screen.getByText("Discovery issues"));
    expect(screen.getByText(/TVDB access failed/)).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Rescan library" }),
    );
    await waitFor(() =>
      expect(requests).toEqual([
        { url: "/api/v1/collections/discover", method: "POST", body: {} },
      ]),
    );
  });
  it("lists persisted collections with optional library and media filters", async () => {
    page();
    expect(
      await screen.findByRole("link", { name: /Mixed universe/ }),
    ).toHaveAttribute("href", "/collections?id=loom-1");
    await userEvent.selectOptions(screen.getByLabelText("Library"), "shows");
    expect(
      screen.queryByRole("link", { name: /Mixed universe/ }),
    ).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("Library"), "");
    await userEvent.selectOptions(screen.getByLabelText("Media"), "series");
    expect(
      await screen.findByRole("link", { name: /Mixed universe/ }),
    ).toBeInTheDocument();
    expect(requests).toEqual([]);
  });
  it("opens real management for an added movie and shared details/add for a missing show", async () => {
    state.search = { id: "loom-1" };
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Manage A movie" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Actual library management" }),
    ).toHaveTextContent("movie:actual-movie");
    await userEvent.click(
      screen.getByRole("button", { name: "Close management" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "View A show" }));
    expect(
      await screen.findByRole("dialog", { name: "Shared external details" }),
    ).toHaveTextContent("series:tmdb:42");
    await userEvent.click(
      screen.getByRole("button", { name: "Shared add form" }),
    );
    expect(await screen.findByText("2 of 2 added to Loom")).toBeInTheDocument();
    expect(
      await screen.findByRole("dialog", { name: "Actual library management" }),
    ).toHaveTextContent("series:actual-show");
    await userEvent.click(
      screen.getByRole("button", { name: "Close management" }),
    );
    expect(
      screen.getByRole("heading", { name: "Mixed universe" }),
    ).toBeInTheDocument();
  });
  it("shows source failures, retries refresh, and preserves collection context", async () => {
    state.search = { id: "loom-1" };
    page();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Source access failed",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Refresh sources" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
    );
    expect(requests[0]).toMatchObject({
      url: "/api/v1/collections/loom-1/refresh",
      method: "POST",
    });
    expect(
      screen.getByRole("heading", { name: "Mixed universe" }),
    ).toBeInTheDocument();
  });
  it("excludes locally and offers restoration without deleting the library title", async () => {
    state.search = { id: "loom-1" };
    page();
    const card = (
      await screen.findByRole("button", { name: "Manage A movie" })
    ).closest("article")!;
    await userEvent.click(
      within(card).getByRole("button", { name: "Exclude locally" }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Manage A movie" }),
      ).not.toBeInTheDocument(),
    );
    await userEvent.selectOptions(screen.getByLabelText("Status"), "excluded");
    expect(
      await screen.findByRole("button", { name: "Restore member" }),
    ).toBeInTheDocument();
    expect(requests[0]).toMatchObject({
      method: "PATCH",
      body: { excluded: true },
    });
    expect(requests.some((r) => r.url.includes("/movies/"))).toBe(false);
  });
  it("requires explicit merge confirmation and sends both current revisions", async () => {
    state.search = { id: "loom-1" };
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Merge collections" }),
    );
    await userEvent.selectOptions(
      screen.getByLabelText("Merge another collection into Mixed universe"),
      "loom-2",
    );
    await userEvent.click(
      screen.getByLabelText("I confirm these collections should be one."),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Merge into Mixed universe" }),
    );
    await waitFor(() =>
      expect(requests).toContainEqual({
        url: "/api/v1/collections/loom-1/merge",
        method: "POST",
        body: { from_id: "loom-2", revision: 3, from_revision: 2 },
      }),
    );
  });
  it("creates a local collection without requiring a source", async () => {
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "New collection" }),
    );
    await userEvent.type(screen.getByLabelText("Name"), "My collection");
    await userEvent.click(
      screen.getByRole("button", { name: "Create collection" }),
    );
    await waitFor(() =>
      expect(requests[0]).toMatchObject({
        url: "/api/v1/collections/",
        body: { name: "My collection", overview: "" },
      }),
    );
    expect(state.navigate).toHaveBeenCalledWith({
      to: "/collections",
      search: { id: "loom-1" },
    });
  });
  it("does not mutate when opening an unsaved recommendation collection", async () => {
    state.search = { tmdb: 99 };
    page();
    expect(
      await screen.findByRole("heading", { name: "TMDB movie set 99" }),
    ).toBeInTheDocument();
    expect(requests).toEqual([]);
    await userEvent.click(
      screen.getByRole("button", { name: "Import collection" }),
    );
    await waitFor(() =>
      expect(requests[0]?.body).toEqual({
        source: { provider: "tmdb", kind: "collection", external_id: "99" },
      }),
    );
  });
  it("hides management actions for read-only users", async () => {
    state.role = "user";
    state.search = { id: "loom-1" };
    page();
    await screen.findByRole("heading", { name: "Mixed universe" });
    expect(
      screen.queryByRole("button", { name: "Edit collection" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Merge collections" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Exclude locally" }),
    ).not.toBeInTheDocument();
  });
});
