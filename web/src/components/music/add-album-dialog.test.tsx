import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AddAlbumDialog } from "@/components/music/add-album-dialog";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
const fetchMock = vi.fn<typeof fetch>();

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AddAlbumDialog open onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  );
}

describe("AddAlbumDialog", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/libraries")) {
        return json({
          data: [
            {
              id: "music-library",
              name: "Music",
              media_type: "music",
              quality_profile_id: "quality-1",
              path: "/music",
            },
          ],
        });
      }
      if (url.endsWith("/api/v1/music/audio-quality-profiles")) {
        return json([
          { id: "quality-other", name: "Compressed", items: [] },
          { id: "quality-1", name: "Lossless", items: [] },
        ]);
      }
      if (url.endsWith("/api/v1/music/metadata-profiles")) return json([]);
      if (url.includes("/api/v1/albums/lookup")) {
        return json([
          {
            mbid: "group-1",
            title: "A Shared Record",
            artists: [{ name: "First Artist" }, { name: "Second Artist" }],
            album_type: "Album",
            release_date: "2024",
            already_added: false,
          },
        ]);
      }
      if (url.endsWith("/api/v1/albums") && init?.method === "POST") {
        return json({ id: "album-1", title: "A Shared Record" });
      }
      return json([]);
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("searches and adds a multi-artist album without a primary artist", async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.type(screen.getByLabelText("Search albums"), "Shared Record");
    const match = await screen.findByRole("button", {
      name: /A Shared Record.*First Artist, Second Artist/s,
    });

    await user.click(match);
    const addButton = screen.getByRole("button", { name: "Add album" });
    await waitFor(() => expect(addButton).toBeEnabled());
    await user.click(addButton);

    await waitFor(() => {
      const request = fetchMock.mock.calls.find(
        ([url, init]) =>
          String(url).endsWith("/api/v1/albums") && init?.method === "POST",
      );
      expect(request).toBeDefined();
      expect(JSON.parse(String(request?.[1]?.body))).toEqual({
        mbid: "group-1",
        qualityProfileId: "quality-1",
        libraryId: "music-library",
        monitored: true,
      });
    });
  });

  it("reports library loading failures instead of offering empty defaults", async () => {
    const successfulFetch = fetchMock.getMockImplementation()!;
    fetchMock.mockImplementation(async (input, init) => {
      if (String(input).endsWith("/api/v1/libraries")) {
        return new Response("Library service unavailable", { status: 503 });
      }
      return successfulFetch(input, init);
    });
    renderDialog();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not load music libraries or profiles",
    );
    expect(screen.getByRole("button", { name: "Add album" })).toBeDisabled();
  });
});
