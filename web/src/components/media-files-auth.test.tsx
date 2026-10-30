import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, useAuth } from "@/hooks/use-auth";
import { MediaFilesManager } from "./media-files-manager";

function Subject() {
  const { isAuthenticated, user } = useAuth();
  return (
    <>
      <p>Auth: {isAuthenticated ? user?.role : "unauthenticated"}</p>
      <MediaFilesManager mediaType="movie" id="m1" title="Test movie" />
    </>
  );
}

afterEach(() => vi.unstubAllGlobals());

describe("Media manager uses the real AuthProvider status model", () => {
  it("accepts a server-issued admin identity without requiring a positive user ID or session token", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async (input) =>
      String(input) === "/api/v1/auth/status"
        ? Response.json({
            setup_required: false,
            is_authenticated: true,
            user: { id: 0, username: "admin", role: "admin" },
          })
        : Response.json({ files: [], deletionMode: "recycle" }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const client = new QueryClient();
    render(
      <QueryClientProvider client={client}>
        <AuthProvider>
          <Subject />
        </AuthProvider>
      </QueryClientProvider>,
    );
    await screen.findByText("Auth: admin");
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Manage files" }));
    expect(await screen.findByText("No files found.")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/media-files/movie/m1",
      expect.objectContaining({ credentials: "include" }),
    );
    client.clear();
  });

  it.each([
    { is_authenticated: false },
    { is_authenticated: true },
    {
      is_authenticated: true,
      user: { id: 2, username: "viewer", role: "user" },
    },
  ])(
    "does not infer admin access from missing/ordinary identity: %j",
    async (status) => {
      const fetchMock = vi
        .fn<typeof fetch>()
        .mockResolvedValue(Response.json({ setup_required: false, ...status }));
      vi.stubGlobal("fetch", fetchMock);
      render(
        <AuthProvider>
          <Subject />
        </AuthProvider>,
      );
      await waitFor(() => expect(screen.getByText(/^Auth:/)).toBeVisible());
      expect(
        screen.queryByRole("button", { name: "Manage files" }),
      ).not.toBeInTheDocument();
      expect(fetchMock).toHaveBeenCalledTimes(1);
    },
  );
});
