import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MusicProfilesPage } from "@/pages/music-settings";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const auth = vi.hoisted(() => ({
  user: { id: 1, role: "admin" } as { id: number; role: string } | null,
}));
vi.mock("@/hooks/use-auth", () => ({ useAuth: () => auth }));
vi.mock("@/lib/custom-formats-api", () => ({
  useCustomFormats: () => ({ data: [] }),
}));

const fetchMock = vi.fn<typeof fetch>();

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MusicProfilesPage />
    </QueryClientProvider>,
  );
}

describe("MusicProfilesPage metadata source", () => {
  beforeEach(() => {
    auth.user = { id: 1, role: "admin" };
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith("/api/v1/music/metadata-source") && !init?.method) {
        return json({
          mode: "custom",
          base_url: "http://mirror.lan:5000/ws/2",
          rate_limit_ms: 0,
          auth_header_name: "X-Api-Key",
          auth_header_set: true,
          search_on_public: true,
        });
      }
      if (url.endsWith("/api/v1/music/metadata-source/test")) {
        return json({
          ok: false,
          lookup_ok: true,
          search_tested: false,
          search_ok: false,
        });
      }
      return json([]);
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("tests the source without resending the saved secret", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByLabelText("API base URL")).toHaveValue(
      "http://mirror.lan:5000/ws/2",
    );
    expect(
      screen.getByPlaceholderText("Saved — leave blank to keep"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Test connection" }));

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([u]) => String(u).endsWith("/test")),
      ).toBe(true),
    );
    const call = fetchMock.mock.calls.find(([u]) =>
      String(u).endsWith("/test"),
    );
    const body = JSON.parse(String(call?.[1]?.body));
    expect(body).toMatchObject({
      mode: "custom",
      base_url: "http://mirror.lan:5000/ws/2",
      auth_header_name: "X-Api-Key",
      search_on_public: true,
    });
    expect(body.auth_header_value).toBeUndefined();
    expect(await screen.findByText("Lookups OK")).toBeInTheDocument();
  });

  it("hides the metadata source from non-admins", async () => {
    auth.user = { id: 2, role: "user" };
    renderPage();
    expect(await screen.findByText("Music Profiles")).toBeInTheDocument();
    expect(screen.queryByText("Metadata Source")).not.toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(([u]) => String(u).includes("metadata-source")),
    ).toBe(false);
  });
});
