import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { TitleDialog } from "./title-experience";

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
    <div role="dialog" aria-label="Normal management">
      {target.type}:{target.id}
      <button onClick={onChanged}>Save management change</button>
      <button onClick={onClose}>Close management</button>
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it.each(["movie", "series"] as const)(
  "opens canonical %s library management instead of only a badge",
  async (media) => {
    const added = vi.fn();
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);
      if (url.startsWith("/api/v1/discover/"))
        return Response.json({
          title: {
            title: "Known title",
            tmdb_id: "42",
            overview: "Full synopsis",
          },
          credits: { cast: [], crew: [] },
        });
      if (url.startsWith("/api/v1/movies") || url.startsWith("/api/v1/series"))
        return Response.json([{ id: "actual-library-id", tmdbId: "42" }]);
      throw new Error(`Unexpected request ${url}`);
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={client}>
        <TitleDialog
          open
          onOpenChange={vi.fn()}
          identity={{ media, id: "42", title: "Known title" }}
          onAdded={added}
        />
      </QueryClientProvider>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Manage in library" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Normal management" }),
    ).toHaveTextContent(`${media}:actual-library-id`);
    await user.click(
      screen.getByRole("button", { name: "Save management change" }),
    );
    expect(added).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Close management" }));
    expect(
      await screen.findByRole("button", { name: "Manage in library" }),
    ).toBeInTheDocument();
    expect(added).not.toHaveBeenCalled();
    client.clear();
  },
);
