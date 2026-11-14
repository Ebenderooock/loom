import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TitleActionsMenu } from "./title-actions-menu";
import { MovieDetailSheet } from "@/components/movies/movie-detail-sheet";
import { SeriesDetailSheet } from "@/components/series/series-detail-sheet";
import type { Movie } from "@/components/movies/types";
import type { Series } from "@/components/series/types";
import type { Library } from "@/lib/libraries-api";

const state = vi.hoisted(() => ({
  role: "admin",
  fetch: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  autoSearch: vi.fn(),
}));
vi.mock("@/lib/fetch", () => ({ apiFetch: state.fetch }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { role: state.role } }),
}));
vi.mock("sonner", () => ({
  toast: { success: state.success, error: state.error },
}));
vi.mock("@/lib/autosearch-api", () => ({ autoSearch: state.autoSearch }));
vi.mock("@/components/media-files-manager", () => ({
  MediaFilesManager: () => null,
}));
vi.mock("@/components/collections/add-to-collection", () => ({
  AddToCollectionButton: () => null,
}));
vi.mock("@/components/alt-titles", () => ({ AltTitlesSection: () => null }));
vi.mock("@/components/person-discover-dialog", () => ({
  PersonDiscoverDialog: () => null,
}));
vi.mock("@/components/search/release-search-dialog", () => ({
  ReleaseSearchDialog: () => null,
}));

const availability = {
  folderPath: "/library/actual-registered-folder",
  unavailableReason: null,
  rescanUnavailableReason: null,
  plexUnavailableReason: null,
};
const common = {
  id: "selected/id",
  title: "Selected title",
  year: 2026,
  overview: "",
  genres: [],
  runtime: 45,
  rating: 0,
  status: "continuing",
  qualityProfileId: "quality",
  libraryId: "selected-library",
  monitoringStatus: "monitored",
};
const movie: Movie = { ...common, releaseDate: "" };
const series: Series = {
  ...common,
  network: "",
  seriesType: "standard",
  seasonFolder: true,
};
const labels = [
  "Sync with Plex",
  "Refresh metadata",
  "Rescan library files",
  "View import history",
  "Copy folder path",
];
const onUpdated = vi.fn();
const onChanged = vi.fn(async () => {});
const base = (type: "movie" | "series", id = common.id) =>
  `/api/v1/${type === "movie" ? "movies" : "series"}/${encodeURIComponent(id)}`;

function response(url: string) {
  if (url.endsWith("/actions")) return Response.json(availability);
  if (url.endsWith("/sync-plex"))
    return Response.json({
      message: "Plex library refresh requested",
      connections: 2,
    });
  if (url.endsWith("/refresh-metadata"))
    return Response.json({ message: "Metadata refreshed" });
  if (url.endsWith("/rescan-library"))
    return Response.json({
      status: "completed",
      totalFiles: 1,
      matched: 1,
      imported: 0,
      errors: [],
    });
  if (url.endsWith("/credits")) return Response.json({ cast: [], crew: [] });
  if (url.includes("/imports/history?")) {
    const params = new URL(url, "http://test").searchParams;
    return Response.json({
      data: [
        {
          id: "record",
          media_type:
            params.get("media_type") === "series" ? "episode" : "movie",
          media_id:
            params.get("media_type") === "series"
              ? "episode-1"
              : params.get("media_id"),
          ...(params.get("media_type") === "series"
            ? { series_id: params.get("media_id") }
            : {}),
          source_path: "/source/file.mkv",
          dest_path: "/library/file.mkv",
          import_mode: "copy",
          status: "imported",
          imported_at: "2026-11-01",
        },
      ],
    });
  }
  if (
    url.endsWith("/seasons") ||
    url.endsWith("/history") ||
    url.includes("/movies/files/")
  )
    return Response.json([]);
  return Response.json(url.includes("/movies/") ? movie : series);
}
function sheet(type: "movie" | "series", open = true, id = common.id) {
  const props = {
    open,
    onOpenChange: vi.fn(),
    onUpdated,
    onDeleted: vi.fn(),
    profiles: [{ id: "quality", name: "HD" }],
    libraries: [
      {
        id: "other-library",
        name: "Other",
        path: "/wrong",
        media_type: "movie",
        monitor_on_add: true,
        quality_profile_id: "quality",
        unmonitor_on_delete: false,
        auto_archive_watched: false,
        auto_archive_days_after_watch: 0,
        created_at: "2026-11-01",
        updated_at: "2026-11-01",
        accessible: true,
        disk_space: { total_bytes: 1000, free_bytes: 500, used_bytes: 500 },
        file_count: 0,
      },
    ] satisfies Library[],
  };
  return type === "movie" ? (
    <MovieDetailSheet {...props} movie={{ ...movie, id }} />
  ) : (
    <SeriesDetailSheet {...props} series={{ ...series, id }} />
  );
}
async function openMenu() {
  await userEvent.click(screen.getByRole("button", { name: "More actions" }));
  return screen.findAllByRole("menuitem");
}
async function choose(label: string) {
  await waitFor(() =>
    expect(screen.getByRole("menuitem", { name: label })).not.toHaveAttribute(
      "aria-disabled",
      "true",
    ),
  );
  await userEvent.click(screen.getByRole("menuitem", { name: label }));
}
beforeEach(() => {
  vi.clearAllMocks();
  userEvent.setup();
  state.role = "admin";
  state.fetch
    .mockReset()
    .mockImplementation(async (url: string) => response(url));
});

describe.each(["movie", "series"] as const)("%s detail actions", (type) => {
  it("shows exactly five actions and invokes only title-safe endpoints", async () => {
    render(sheet(type));
    const clipboard = vi
      .spyOn(navigator.clipboard, "writeText")
      .mockResolvedValue();
    expect(screen.getByRole("button", { name: "More actions" })).toHaveClass(
      "h-11",
      "w-11",
      "sm:h-8",
      "sm:w-8",
    );
    expect(
      (await openMenu()).map((item) => item.getAttribute("aria-label")),
    ).toEqual(labels);
    expect(screen.getByRole("separator")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Checks registered imported files only; does not discover or ingest new files.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Refreshes enabled Plex servers’ libraries. Does not download missing media.",
      ),
    ).toBeInTheDocument();
    await choose("Sync with Plex");
    await waitFor(() =>
      expect(state.success).toHaveBeenCalledWith(
        "Plex library refresh requested",
        expect.anything(),
      ),
    );
    await openMenu();
    await choose("Refresh metadata");
    await waitFor(() => expect(onUpdated).toHaveBeenCalled());
    if (type === "series") {
      expect(state.fetch).toHaveBeenCalledWith(
        `${base(type)}/seasons`,
        expect.objectContaining({ signal: expect.anything() }),
      );
      expect(state.fetch).toHaveBeenCalledWith(
        `${base(type)}/credits`,
        expect.objectContaining({ signal: expect.anything() }),
      );
    }
    await openMenu();
    await choose("Rescan library files");
    await waitFor(() =>
      expect(state.success).toHaveBeenCalledWith(
        "Registered library file check complete: 1 checked",
      ),
    );
    await openMenu();
    await choose("Copy folder path");
    await waitFor(() =>
      expect(clipboard).toHaveBeenCalledWith(availability.folderPath),
    );
    await openMenu();
    await choose("View import history");
    expect(
      await screen.findByRole("dialog", {
        name: `Import history — ${common.title}`,
      }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Source: /source/file.mkv"),
    ).toBeInTheDocument();
    const historyCall = state.fetch.mock.calls.find(([url]) =>
      url.includes("/imports/history?"),
    );
    const params = new URL(historyCall![0], "http://test").searchParams;
    expect(params.get("media_type")).toBe(type);
    expect(params.get("media_id")).toBe(common.id);
    const posts = state.fetch.mock.calls.filter(
      ([, init]) => init?.method === "POST",
    );
    expect(posts.map(([url]) => url)).toEqual(
      ["sync-plex", "refresh-metadata", "rescan-library"].map(
        (endpoint) => `${base(type)}/${endpoint}`,
      ),
    );
    for (const [, init, options] of posts) {
      expect(init.body).toBeUndefined();
      expect(options).toEqual({ acquisitionRecovery: false });
    }
    expect(state.autoSearch).not.toHaveBeenCalled();
    expect(
      state.fetch.mock.calls.some(([url]) =>
        /downloads|connect|reimport|\/search|\/grab|\/rescan$|\/refresh$/.test(
          url,
        ),
      ),
    ).toBe(false);
    expect(
      screen.queryByRole("button", { name: /retry|reimport/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTitle(/Refresh metadata from/),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTitle("Rescan library folder for new files"),
    ).not.toBeInTheDocument();
  });

  it("loads only for an open sheet and aborts on close or title change", async () => {
    const { rerender } = render(sheet(type, false));
    expect(
      state.fetch.mock.calls.some(([url]) => url.endsWith("/actions")),
    ).toBe(false);
    rerender(sheet(type));
    await waitFor(() =>
      expect(
        state.fetch.mock.calls.some(([url]) => url === `${base(type)}/actions`),
      ).toBe(true),
    );
    const oldSignal = state.fetch.mock.calls.find(
      ([url]) => url === `${base(type)}/actions`,
    )![1].signal;
    rerender(sheet(type, true, "second"));
    expect(oldSignal.aborted).toBe(true);
    await waitFor(() =>
      expect(
        state.fetch.mock.calls.some(
          ([url]) => url === `${base(type, "second")}/actions`,
        ),
      ).toBe(true),
    );
    const signal = state.fetch.mock.calls.find(
      ([url]) => url === `${base(type, "second")}/actions`,
    )![1].signal;
    rerender(sheet(type, false, "second"));
    expect(signal.aborted).toBe(true);
  });

  it("does not update the new title after an old mutation finishes", async () => {
    let complete!: (value: Response) => void;
    state.fetch.mockImplementation((url: string) =>
      url.endsWith("/refresh-metadata")
        ? new Promise<Response>((resolve) => {
            complete = resolve;
          })
        : Promise.resolve(response(url)),
    );
    const { rerender } = render(sheet(type));
    await openMenu();
    await choose("Refresh metadata");
    rerender(sheet(type, true, "second"));
    await act(async () =>
      complete(Response.json({ message: "Metadata refreshed" })),
    );
    expect(onUpdated).not.toHaveBeenCalled();
    expect(state.success).not.toHaveBeenCalled();
  });
});

function menu(mediaType: "movie" | "series" = "movie") {
  return (
    <TitleActionsMenu
      mediaType={mediaType}
      id={common.id}
      title={common.title}
      onChanged={onChanged}
    />
  );
}

describe("action safeguards", () => {
  it("supports keyboard history activation, Escape and trigger focus restoration", async () => {
    const user = userEvent.setup();
    render(menu());
    await waitFor(() => expect(state.fetch).toHaveBeenCalled());
    const trigger = screen.getByRole("button", { name: "More actions" });
    trigger.focus();
    await user.keyboard("{Enter}");
    const history = await screen.findByRole("menuitem", {
      name: "View import history",
    });
    await waitFor(() =>
      expect(history).not.toHaveAttribute("aria-disabled", "true"),
    );
    history.focus();
    await user.keyboard("{Enter}");
    await screen.findByText("Source: /source/file.mkv");
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(trigger).toHaveFocus();
    expect(
      state.fetch.mock.calls.some(([, init]) => init?.method === "POST"),
    ).toBe(false);
  });
  it("paginates title history and reloads failed reads without mutation", async () => {
    let fail = false;
    state.fetch.mockImplementation(async (url: string) => {
      if (!url.includes("/imports/history?")) return response(url);
      if (fail) return Response.json({ error: "Read failed" }, { status: 500 });
      const offset = new URL(url, "http://test").searchParams.get("offset");
      return Response.json({
        data:
          offset === "0"
            ? Array.from({ length: 50 }, (_, index) => ({
                id: `record-${index}`,
                media_type: "movie",
                media_id: common.id,
                status: "imported",
                source_path: `/source/${index}`,
                dest_path: `/destination/${index}`,
                import_mode: "copy",
                imported_at: "2026-11-01",
              }))
            : [],
      });
    });
    render(menu());
    await openMenu();
    await choose("View import history");
    await screen.findByText("Source: /source/0");
    fail = true;
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Read failed");
    expect(screen.queryByText("Source: /source/0")).not.toBeInTheDocument();
    fail = false;
    await userEvent.click(
      screen.getByRole("button", { name: "Reload history" }),
    );
    await screen.findByText("No import records for this title on this page.");
    expect(screen.getByText("Page 2")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Previous" }));
    await screen.findByText("Source: /source/0");
    const reads = state.fetch.mock.calls.filter(([url]) =>
      url.includes("/imports/history?"),
    );
    expect(
      reads.map(([url]) =>
        new URL(url, "http://test").searchParams.get("offset"),
      ),
    ).toEqual(["0", "50", "50", "0"]);
    expect(
      state.fetch.mock.calls.some(([, init]) => init?.method === "POST"),
    ).toBe(false);
  });
  it.each([
    ["movie", "user"],
    ["movie", "guest"],
    ["series", "user"],
    ["series", "guest"],
  ] as const)(
    "allows %s history while disabling four admin actions (%s)",
    async (mediaType, role) => {
      state.role = role;
      render(menu(mediaType));
      for (const item of await openMenu())
        if (item.getAttribute("aria-label") !== "View import history")
          expect(item).toHaveAttribute("aria-disabled", "true");
      expect(
        screen.getAllByText("Administrator access required."),
      ).toHaveLength(4);
      expect(state.fetch).not.toHaveBeenCalled();
      await choose("View import history");
      await screen.findByText("Source: /source/file.mkv");
      expect(
        state.fetch.mock.calls.every(([url]) =>
          url.includes("/imports/history?"),
        ),
      ).toBe(true);
      expect(
        state.fetch.mock.calls.some(([, init]) => init?.method === "POST"),
      ).toBe(false);
    },
  );
  it("keeps history available while admin availability loads or fails", async () => {
    let complete!: (value: Response) => void;
    state.fetch.mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          complete = resolve;
        }),
    );
    render(menu());
    for (const item of await openMenu())
      if (item.getAttribute("aria-label") !== "View import history")
        expect(item).toHaveAttribute("aria-disabled", "true");
    expect(
      screen.getByRole("menuitem", { name: "View import history" }),
    ).not.toHaveAttribute("aria-disabled", "true");
    await act(async () =>
      complete(
        Response.json({ error: { message: "Forbidden" } }, { status: 403 }),
      ),
    );
    expect(screen.getAllByText("Actions unavailable: Forbidden")).toHaveLength(
      4,
    );
    for (const item of screen.getAllByRole("menuitem"))
      if (item.getAttribute("aria-label") !== "View import history")
        expect(item).toHaveAttribute("aria-disabled", "true");
    state.fetch.mockImplementation(async (url: string) => response(url));
    await choose("View import history");
    await screen.findByText("Source: /source/file.mkv");
  });
  it("uses separate backend reasons rather than manufacturing folders", async () => {
    state.fetch.mockResolvedValue(
      Response.json({
        ...availability,
        folderPath: null,
        unavailableReason: "No unique folder",
        rescanUnavailableReason: "No complete registered files",
        plexUnavailableReason: "Plex is disabled",
      }),
    );
    render(menu());
    await openMenu();
    expect(
      screen.getByRole("menuitem", { name: "Copy folder path" }),
    ).toHaveAccessibleDescription(expect.stringContaining("No unique folder"));
    expect(
      screen.getByRole("menuitem", { name: "Rescan library files" }),
    ).toHaveAccessibleDescription(
      expect.stringContaining("No complete registered files"),
    );
    expect(
      screen.getByRole("menuitem", { name: "Sync with Plex" }),
    ).toHaveAccessibleDescription(expect.stringContaining("Plex is disabled"));
    await choose("Refresh metadata");
  });
  it("prevents duplicate clicks and disables every action while pending", async () => {
    let complete!: (value: Response) => void;
    state.fetch.mockImplementation((url: string) =>
      url.endsWith("/sync-plex")
        ? new Promise<Response>((resolve) => {
            complete = resolve;
          })
        : Promise.resolve(response(url)),
    );
    render(menu());
    await openMenu();
    const item = screen.getByRole("menuitem", { name: "Sync with Plex" });
    await waitFor(() =>
      expect(item).not.toHaveAttribute("aria-disabled", "true"),
    );
    fireEvent.click(item);
    fireEvent.click(item);
    await openMenu();
    for (const row of screen.getAllByRole("menuitem"))
      expect(row).toHaveAttribute("aria-disabled", "true");
    expect(
      state.fetch.mock.calls.filter(([, init]) => init?.method === "POST"),
    ).toHaveLength(1);
    await act(async () =>
      complete(
        Response.json({
          message: "Plex library refresh requested",
          connections: 1,
        }),
      ),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("menuitem", { name: "Sync with Plex" }),
      ).not.toHaveAttribute("aria-disabled", "true"),
    );
  });
  it.each([
    [
      "Sync with Plex",
      "/sync-plex",
      { error: { message: "Provider refused" } },
      502,
      "Provider refused",
    ],
    [
      "Sync with Plex",
      "/sync-plex",
      { message: "Plex library refresh requested", connections: 0 },
      200,
      "Plex did not confirm",
    ],
    [
      "Refresh metadata",
      "/refresh-metadata",
      { error: { message: "Metadata unavailable" } },
      500,
      "Metadata unavailable",
    ],
    [
      "Rescan library files",
      "/rescan-library",
      {
        status: "completed",
        matched: 1,
        imported: 0,
        errors: ["File size changed"],
      },
      200,
      "File size changed",
    ],
    [
      "Rescan library files",
      "/rescan-library",
      { status: "failed", errors: [] },
      200,
      "Library file rescan failed",
    ],
    [
      "Rescan library files",
      "/rescan-library",
      { status: "running", errors: [] },
      200,
      "has not confirmed completion",
    ],
  ])(
    "reports %s failures honestly (%s, %j)",
    async (label, endpoint, body, status, message) => {
      state.fetch.mockImplementation(async (url: string) =>
        url.endsWith(endpoint)
          ? Response.json(body, { status })
          : response(url),
      );
      render(menu());
      await openMenu();
      await choose(label);
      await waitFor(() =>
        expect(state.error).toHaveBeenCalledWith(
          `${label} failed`,
          expect.objectContaining({
            description: expect.stringContaining(message),
          }),
        ),
      );
      expect(state.success).not.toHaveBeenCalled();
      expect(onChanged).not.toHaveBeenCalled();
    },
  );
  it("reports clipboard failure, not success", async () => {
    render(menu());
    await openMenu();
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(
      new Error("Clipboard denied"),
    );
    await choose("Copy folder path");
    await waitFor(() =>
      expect(state.error).toHaveBeenCalledWith("Copy folder path failed", {
        description: "Clipboard denied",
      }),
    );
    expect(state.success).not.toHaveBeenCalled();
  });
  it.each(["error", "wrong-title", "malformed"])(
    "shows history %s as an error, never an empty success",
    async (mode) => {
      state.fetch.mockImplementation(async (url: string) =>
        !url.includes("/imports/history?")
          ? response(url)
          : mode === "error"
            ? Response.json(
                { error: { message: "History unavailable" } },
                { status: 500 },
              )
            : mode === "malformed"
              ? Response.json({})
              : Response.json({
                  data: [{ media_type: "series", media_id: "other" }],
                }),
      );
      render(menu());
      await openMenu();
      await choose("View import history");
      expect(await screen.findByRole("alert")).toHaveTextContent(
        "Could not load import history",
      );
      expect(screen.queryByText(/No import records/)).not.toBeInTheDocument();
      expect(
        state.fetch.mock.calls.some(([, init]) => init?.method === "POST"),
      ).toBe(false);
      expect(state.autoSearch).not.toHaveBeenCalled();
    },
  );
});
