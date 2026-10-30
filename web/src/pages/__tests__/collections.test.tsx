import {
  cleanup,
  act,
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
      const body = (init?.body ? JSON.parse(String(init.body)) : {}) as Record<
        string,
        unknown
      >;
      requests.push({ url, method, body });
      if (method === "DELETE" && url.startsWith("/api/v1/collections/loom-1?"))
        return Response.json({ deleted: "loom-1" });
      if (url.endsWith("/bulk/add"))
        return Response.json(bulkPlan(body.execute === true));
      if (url.endsWith("/loom-1/members") && method === "POST") {
        const items = body.items as Array<Record<string, string>>;
        collection = {
          ...collection,
          members: [
            ...collection.members,
            ...items.map((item, i) =>
              member({
                id: `new-${i}`,
                title: item.media_id ?? `TMDB ${item.tmdb}`,
                local: true,
                sources: [],
              }),
            ),
          ],
        };
      }
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
    if (url === "/api/v1/quality-profiles")
      return Response.json({
        data: [
          { id: "hd", name: "HD" },
          { id: "uhd", name: "UHD" },
        ],
      });
    if (url.startsWith("/api/v1/movies?limit=1000"))
      return Response.json({
        data: [
          { id: "actual-movie", title: "A movie", tmdbId: 42 },
          { id: "lib-dune", title: "Dune", tmdbId: 438631, year: 2021 },
        ],
      });
    if (url.startsWith("/api/v1/movies/lookup"))
      return Response.json([
        { tmdbId: 438631, title: "Dune", year: 2021 },
        { tmdbId: 693134, title: "Dune: Part Two", year: 2024 },
      ]);
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
function bulkPlan(executed: boolean) {
  return {
    kind: "add",
    total: 2,
    counts: { eligible: 1, in_library: 1 },
    items: [
      {
        member_id: "s1",
        title: "A show",
        media_type: "series",
        status: executed ? "queued" : "eligible",
      },
      {
        member_id: "m1",
        title: "A movie",
        media_type: "movie",
        status: "skipped",
        reason: "in_library",
      },
    ],
    libraries: { series: [{ id: "shows", name: "Shows" }] },
    profiles: [{ id: "hd", name: "HD" }],
    settings: {
      series: {
        library_id: "shows",
        quality_profile_id: "hd",
        monitored: true,
      },
    },
    ...(executed ? { job_id: "job-1" } : {}),
  };
}
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
  it("polls canonical status Missing -> Downloading -> Available without treating membership as files", async () => {
    state.search = { id: collection.id };
    collection.members[0] = { ...collection.members[0]!, status: "missing" };
    vi.useFakeTimers();
    try {
      page();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10);
      });
      const card = screen.getByRole("button", { name: "Manage A movie" });
      expect(within(card).getByText("Missing")).toBeInTheDocument();
      for (const [status, label] of [
        ["downloading", "Downloading"],
        ["available_right_quality", "Available"],
      ] as const) {
        collection.members[0] = {
          ...collection.members[0]!,
          status,
          has_file: status === "available_right_quality",
        };
        await act(async () => {
          await vi.advanceTimersByTimeAsync(30_000);
        });
        expect(within(card).getByText(label)).toBeInTheDocument();
      }
      expect(within(card).queryByText("available")).not.toBeInTheDocument();
      expect(requests).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
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
    expect(await screen.findByText("2 of 2 in library")).toBeInTheDocument();
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
    await userEvent.click(screen.getByRole("button", { name: "Manage" }));
    const sheet = await screen.findByRole("dialog", {
      name: "Manage collection",
    });
    await userEvent.click(
      within(sheet).getByRole("button", { name: "Refresh sources" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
    );
    expect(requests[0]).toMatchObject({
      url: "/api/v1/collections/loom-1/refresh",
      method: "POST",
    });
    await userEvent.keyboard("{Escape}");
    expect(
      await screen.findByRole("heading", { name: "Mixed universe" }),
    ).toBeInTheDocument();
  });
  it("excludes locally and offers restoration without deleting the library title", async () => {
    state.search = { id: "loom-1" };
    page();
    const card = (
      await screen.findByRole("button", { name: "Manage A movie" })
    ).closest("article")!;
    await userEvent.click(
      within(card).getByRole("button", { name: "Exclude" }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Manage A movie" }),
      ).not.toBeInTheDocument(),
    );
    await userEvent.selectOptions(screen.getByLabelText("Status"), "excluded");
    expect(
      await screen.findByRole("button", { name: "Restore" }),
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
      await screen.findByRole("button", { name: "Manage" }),
    );
    await userEvent.selectOptions(
      screen.getByLabelText("Merge another collection into Mixed universe"),
      "loom-2",
    );
    const merge = screen.getByRole("button", { name: "Merge" });
    expect(merge).toBeDisabled();
    await userEvent.click(
      screen.getByLabelText("Merge Other universe into Mixed universe"),
    );
    await userEvent.click(merge);
    await waitFor(() =>
      expect(requests).toContainEqual({
        url: "/api/v1/collections/loom-1/merge",
        method: "POST",
        body: { from_id: "loom-2", revision: 3, from_revision: 2 },
      }),
    );
  });
  it("deletes only the collection after confirmation", async () => {
    state.search = { id: "loom-1" };
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Manage" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Delete collection" }),
    );
    const confirm = await screen.findByRole("dialog", {
      name: "Delete Mixed universe?",
    });
    expect(confirm).toHaveTextContent(
      "1 library title and their files stay exactly as they are",
    );
    expect(confirm).toHaveTextContent("won't be re-imported automatically");
    expect(requests).toEqual([]);
    await userEvent.click(
      within(confirm).getByRole("button", { name: "Delete collection" }),
    );
    await waitFor(() =>
      expect(requests).toEqual([
        {
          url: "/api/v1/collections/loom-1?revision=3",
          method: "DELETE",
          body: {},
        },
      ]),
    );
    expect(state.navigate).toHaveBeenCalled();
    expect(requests.some((r) => r.url.includes("/movies"))).toBe(false);
  });
  it("saves a collection quality default without touching existing members", async () => {
    state.search = { id: "loom-1" };
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Manage" }),
    );
    const select = screen.getByLabelText("Quality for new additions");
    await screen.findByRole("option", { name: "UHD" });
    await userEvent.selectOptions(select, "uhd");
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() =>
      expect(requests).toEqual([
        {
          url: "/api/v1/collections/loom-1",
          method: "PATCH",
          body: {
            name: "Mixed universe",
            overview: "Movies and shows",
            revision: 3,
            quality_profile_id: "uhd",
          },
        },
      ]),
    );
  });
  it("adds real library and TMDB titles from the picker", async () => {
    state.search = { id: "loom-1" };
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Add titles" }),
    );
    const picker = await screen.findByRole("dialog", {
      name: "Add titles to Mixed universe",
    });
    await userEvent.type(
      within(picker).getByLabelText("Search titles"),
      "dune",
    );
    const library = await within(picker).findByRole("region", {
      name: "Library results",
    });
    const tmdb = await within(picker).findByRole("region", {
      name: "TMDB results",
    });
    // TMDB's Dune is already in the library, so it only appears once.
    await within(tmdb).findByRole("checkbox", { name: /Dune: Part Two/ });
    expect(
      within(tmdb).queryByRole("checkbox", { name: /^Dune 2021/ }),
    ).toBeNull();
    await userEvent.click(
      within(library).getByRole("checkbox", { name: /^Dune/ }),
    );
    await userEvent.click(
      within(tmdb).getByRole("checkbox", { name: /Dune: Part Two/ }),
    );
    await userEvent.click(
      within(picker).getByRole("button", { name: "Add 2 titles" }),
    );
    await waitFor(() =>
      expect(requests).toContainEqual({
        url: "/api/v1/collections/loom-1/members",
        method: "POST",
        body: {
          items: [
            { media_type: "movie", media_id: "lib-dune" },
            {
              media_type: "movie",
              title: "Dune: Part Two",
              year: 2024,
              external_ids: { tmdb: "693134" },
            },
          ],
        },
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: "Add titles to Mixed universe" }),
      ).not.toBeInTheDocument(),
    );
  });
  it("previews bulk add for a selection and only adds after confirming", async () => {
    state.search = { id: "loom-1" };
    page();
    await userEvent.click(
      await screen.findByRole("button", { name: "Select" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Select A show" }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Select A movie" }),
    );
    const bar = screen.getByRole("toolbar", { name: "Selected titles" });
    expect(bar).toHaveTextContent("2 selected");
    await userEvent.click(
      within(bar).getByRole("button", { name: "Add missing" }),
    );
    const sheet = await screen.findByRole("dialog", { name: /Add missing/ });
    expect(await within(sheet).findByText("A show")).toBeInTheDocument();
    expect(within(sheet).getByText(/Already in library/i)).toBeInTheDocument();
    await waitFor(() => expect(requests.length).toBeGreaterThan(0));
    expect(requests.every((r) => r.body.execute !== true)).toBe(true);
    expect(requests[0]?.body).toMatchObject({ member_ids: ["s1", "m1"] });
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
      screen.queryByRole("button", { name: "Manage" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Add titles" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Exclude" }),
    ).not.toBeInTheDocument();
  });
});
