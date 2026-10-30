import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaFilesManager } from "./media-files-manager";
import type {
  MediaFilesResponse,
  MediaFilesScope,
} from "@/lib/media-files-api";

const auth = vi.hoisted(() => ({
  user: { id: 1, role: "admin" },
  isAuthenticated: true,
}));
vi.mock("@/hooks/use-auth", () => ({ useAuth: () => auth }));

const fetchMock = vi.fn<typeof fetch>();
let inventory: MediaFilesResponse;
let client: QueryClient;

function renderManager(
  scope: MediaFilesScope = { mediaType: "movie", id: "m1" },
) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MediaFilesManager {...scope} title="Test title" />
    </QueryClientProvider>,
  );
}

async function openAndSelectAll() {
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "Manage files" }));
  await user.click(
    await screen.findByRole("checkbox", { name: "Select all files" }),
  );
  return user;
}

function deleteCalls() {
  return fetchMock.mock.calls.filter(([, init]) => init?.method === "POST");
}

beforeEach(() => {
  auth.user = { id: 1, role: "admin" };
  auth.isAuthenticated = true;
  inventory = {
    deletionMode: "recycle",
    files: [
      {
        path: "movie.mkv",
        size: 1024,
        tracked: true,
        quality: "1080p",
        format: "HEVC",
      },
      { path: "Extras/interview.mp4", size: 512, tracked: false },
      { path: "Subs/movie.en.srt", size: 0, tracked: false },
    ],
  };
  fetchMock.mockReset().mockImplementation(async (_input, init) => {
    if (init?.method === "POST") {
      const { paths } = JSON.parse(init.body as string) as { paths: string[] };
      inventory = {
        ...inventory,
        files: inventory.files.filter((file) => !paths.includes(file.path)),
      };
      return Response.json({ deleted: paths });
    }
    return Response.json(inventory);
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  client?.clear();
  vi.unstubAllGlobals();
});

describe("MediaFilesManager", () => {
  it("caps a 1,001-file inventory at 1,000 selections and keeps checked items deselectable", async () => {
    inventory.files = Array.from({ length: 1001 }, (_, index) => ({
      path: `Extras/file-${index}.mkv`,
      size: 1024,
      tracked: false,
    }));
    renderManager();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Manage files" }));
    const selectAll = await screen.findByLabelText("Select first 1,000 files");
    fireEvent.click(selectAll);
    expect(
      screen.getByText(/Select up to 1,000 files per operation/),
    ).toHaveTextContent("Selection limit reached");
    const first = screen.getByLabelText("Select Extras/file-0.mkv");
    const last = screen.getByLabelText("Select Extras/file-1000.mkv");
    expect(first).toBeChecked();
    expect(first).toBeEnabled();
    expect(last).not.toBeChecked();
    expect(last).toBeDisabled();
    fireEvent.click(first);
    expect(last).toBeEnabled();
    fireEvent.click(last);
    expect(last).toBeChecked();
    expect(first).toBeDisabled();
    const deleteButton = screen.getByText("Delete selected (1000)");
    fireEvent.click(deleteButton);
    const dialog = screen
      .getByText("Recycle selected files?")
      .closest<HTMLElement>('[role="dialog"]')!;
    expect(dialog.querySelectorAll("li")).toHaveLength(1000);
    expect(
      within(dialog).queryByText("Extras/file-0.mkv"),
    ).not.toBeInTheDocument();
    expect(within(dialog).getByText("Extras/file-1000.mkv")).toBeVisible();
    expect(deleteCalls()).toHaveLength(0);
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Recycle files" }),
    );
    await waitFor(() => expect(deleteCalls()).toHaveLength(1));
    const body = JSON.parse(deleteCalls()[0]![1]!.body as string);
    expect(body.paths).toHaveLength(1000);
    expect(new Set(body.paths).size).toBe(1000);
  }, 15000);

  it.each(["user", "viewer"])(
    "hides the entire manager and never fetches for %s",
    (role) => {
      auth.user.role = role;
      renderManager();
      expect(screen.queryByRole("button")).not.toBeInTheDocument();
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it("does not expose the manager without an authenticated session", () => {
    auth.isAuthenticated = false;
    renderManager();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows all files and metadata; confirmation lists exact paths and cancel is a no-op", async () => {
    renderManager();
    expect(fetchMock).not.toHaveBeenCalled();
    const user = await openAndSelectAll();
    expect(screen.getByText("1.0 KB")).toBeVisible();
    expect(screen.getByText("0 B")).toBeVisible();
    expect(screen.getByText("1080p")).toBeVisible();
    expect(screen.getByText("HEVC")).toBeVisible();
    expect(screen.getByText("Tracked")).toBeVisible();
    expect(screen.getAllByText("Untracked")).toHaveLength(2);
    await user.click(
      screen.getByRole("button", { name: "Delete selected (3)" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Recycle selected files?",
    });
    for (const file of inventory.files) {
      expect(within(dialog).getByText(file.path)).toBeVisible();
    }
    expect(
      within(dialog).getByText("Each file listed below will be recycled."),
    ).toBeVisible();
    expect(
      within(dialog).getByRole("button", { name: "Cancel" }),
    ).toHaveFocus();
    expect(deleteCalls()).toHaveLength(0);
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(deleteCalls()).toHaveLength(0);
    expect(
      screen.queryByRole("dialog", { name: "Recycle selected files?" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Select movie.mkv" }),
    ).toBeChecked();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Delete selected (3)" }),
      ).toHaveFocus(),
    );
  });

  it.each(["recycle", "permanent"] as const)(
    "sends confirmed %s deletion with exact URL, paths, credentials, mode and header",
    async (mode) => {
      inventory.deletionMode = mode;
      renderManager({ mediaType: "series", id: "show/id", season: 0 });
      const user = await openAndSelectAll();
      await user.click(
        screen.getByRole("button", { name: "Delete selected (3)" }),
      );
      const dialog = screen.getByRole("dialog", {
        name:
          mode === "recycle"
            ? "Recycle selected files?"
            : "Permanently delete selected files?",
      });
      if (mode === "permanent") {
        expect(
          within(dialog).getByText(/permanently deleted.*cannot be undone/),
        ).toBeVisible();
      }
      expect(deleteCalls()).toHaveLength(0);
      await user.click(
        within(dialog).getByRole("button", {
          name:
            mode === "recycle" ? "Recycle files" : "Permanently delete files",
        }),
      );
      await screen.findByText("No files found.");
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/v1/media-files/series/show%2Fid?season=0",
        expect.objectContaining({ credentials: "include" }),
      );
      expect(deleteCalls()).toEqual([
        [
          "/api/v1/media-files/series/show%2Fid/delete?season=0",
          {
            method: "POST",
            credentials: "include",
            headers: {
              "Content-Type": "application/json",
              "X-Confirm-Delete": "true",
            },
            body: JSON.stringify({
              paths: ["movie.mkv", "Extras/interview.mp4", "Subs/movie.en.srt"],
              deletionMode: mode,
            }),
          },
        ],
      ]);
      expect(
        screen.getByText(
          mode === "recycle"
            ? "3 of 3 selected files recycled."
            : "3 of 3 selected files permanently deleted.",
        ),
      ).toBeVisible();
    },
  );

  it("deletes only checked opaque identifiers, not basenames or unselected files", async () => {
    renderManager();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Manage files" }));
    await user.click(
      await screen.findByRole("checkbox", { name: "Select Subs/movie.en.srt" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Delete selected (1)" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Recycle selected files?",
    });
    expect(within(dialog).queryByText("movie.mkv")).not.toBeInTheDocument();
    await user.click(
      within(dialog).getByRole("button", { name: "Recycle files" }),
    );
    await screen.findByText("1 of 1 selected files recycled.");
    expect(JSON.parse(deleteCalls()[0]![1]!.body as string).paths).toEqual([
      "Subs/movie.en.srt",
    ]);
  });

  it("blocks repeat confirmation, cancel, close and Escape while deletion is pending", async () => {
    let finish!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => {
      finish = resolve;
    });
    const implementation = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input, init) =>
      init?.method === "POST" ? pending : implementation(input, init),
    );
    renderManager();
    const user = await openAndSelectAll();
    await user.click(
      screen.getByRole("button", { name: "Delete selected (3)" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Recycle selected files?",
    });
    await user.click(
      within(dialog).getByRole("button", { name: "Recycle files" }),
    );
    expect(
      within(dialog).getByRole("button", { name: "Deleting…" }),
    ).toBeDisabled();
    expect(
      within(dialog).getByRole("button", { name: "Cancel" }),
    ).toBeDisabled();
    expect(
      within(dialog).getByRole("button", { name: "Close" }),
    ).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(dialog).toBeVisible();
    expect(deleteCalls()).toHaveLength(1);
    finish(Response.json({ deleted: [] }));
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
  });

  it("shows runtime error.message, refetches partially deleted files, clears selection and invalidates sibling scopes", async () => {
    const implementation = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation(async (input, init) => {
      if (init?.method === "POST") {
        inventory = { ...inventory, files: inventory.files.slice(1) };
        return Response.json(
          { error: { message: "Disk unavailable" } },
          { status: 500 },
        );
      }
      return implementation(input, init);
    });
    renderManager({ mediaType: "series", id: "s1", season: 1 });
    const siblingKey = ["media-files", 1, "series", "s1", "all"];
    client.setQueryData(siblingKey, inventory);
    const user = await openAndSelectAll();
    await user.click(
      screen.getByRole("button", { name: "Delete selected (3)" }),
    );
    await user.click(screen.getByRole("button", { name: "Recycle files" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Disk unavailable",
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Some files may already have been deleted",
    );
    await waitFor(() =>
      expect(screen.queryByText("movie.mkv")).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("checkbox", { name: "Select Extras/interview.mp4" }),
    ).not.toBeChecked();
    expect(
      screen.getByRole("button", { name: "Delete selected (0)" }),
    ).toBeDisabled();
    expect(client.getQueryState(siblingKey)?.isInvalidated).toBe(true);
    expect(
      fetchMock.mock.calls.filter(([, init]) => init?.method !== "POST"),
    ).toHaveLength(2);
  });

  it("refetches after a network failure too", async () => {
    const implementation = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation((input, init) =>
      init?.method === "POST"
        ? Promise.reject(new Error("Connection lost"))
        : implementation(input, init),
    );
    renderManager();
    const user = await openAndSelectAll();
    await user.click(
      screen.getByRole("button", { name: "Delete selected (3)" }),
    );
    await user.click(screen.getByRole("button", { name: "Recycle files" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Connection lost",
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
    expect(
      screen.getByRole("button", { name: "Delete selected (0)" }),
    ).toBeDisabled();
  });

  it("offers a useful retry for a GET failure without destructive controls enabled", async () => {
    fetchMock.mockResolvedValueOnce(
      Response.json({ error: { message: "Access denied" } }, { status: 403 }),
    );
    renderManager();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Manage files" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Access denied");
    expect(
      screen.getByRole("button", { name: "Delete selected (0)" }),
    ).toBeDisabled();
    await user.click(
      screen.getByRole("button", { name: "Retry loading files" }),
    );
    expect(await screen.findByText("Extras/interview.mp4")).toBeVisible();
  });
});
