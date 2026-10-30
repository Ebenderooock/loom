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
import { AddToCollectionButton } from "../add-to-collection";
import type { LoomCollection } from "@/lib/collections-api";

const base = {
  overview: "",
  revision: 1,
  total: 0,
  in_library: 0,
  sources: [],
  members: [],
} satisfies Partial<LoomCollection>;
let requests: { url: string; method: string; body: unknown }[];
let client: QueryClient;
beforeEach(() => {
  requests = [];
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const url = String(input),
      method = init?.method ?? "GET";
    if (method === "GET")
      return Response.json({
        items: [
          { ...base, id: "dune", name: "Dune saga" },
          {
            ...base,
            id: "faves",
            name: "Favourites",
            members: [
              {
                id: "m",
                title: "Arrival",
                media_type: "movie",
                media_id: "arrival-2016",
                external_ids: {},
                sources: [],
                provenance: [],
                local: true,
                excluded: false,
                monitored: true,
                in_library: true,
              },
            ],
          },
        ],
      });
    const body = JSON.parse(String(init?.body));
    requests.push({ url, method, body });
    if (url === "/api/v1/collections/")
      return Response.json({ ...base, id: "new", name: body.name });
    return Response.json({ ...base, id: "x", name: "x" });
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function open() {
  render(
    <QueryClientProvider client={client}>
      <AddToCollectionButton
        iconOnly
        target={{
          media_type: "movie",
          media_id: "arrival-2016",
          title: "Arrival",
          tmdb: "329865",
        }}
      />
    </QueryClientProvider>,
  );
  return userEvent.click(
    screen.getByRole("button", { name: "Add to collection" }),
  );
}

describe("Add to collection", () => {
  it("adds the real library title to an existing collection in one click", async () => {
    await open();
    const dialog = await screen.findByRole("dialog", {
      name: "Add Arrival to a collection",
    });
    expect(
      await within(dialog).findByRole("button", { name: "In Favourites" }),
    ).toBeDisabled();
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Add to Dune saga" }),
    );
    await waitFor(() =>
      expect(requests).toEqual([
        {
          url: "/api/v1/collections/dune/members",
          method: "POST",
          body: {
            items: [{ media_type: "movie", media_id: "arrival-2016" }],
          },
        },
      ]),
    );
    expect(
      await within(dialog).findByRole("button", { name: "In Dune saga" }),
    ).toBeDisabled();
  });

  it("creates a named collection and attaches the title", async () => {
    await open();
    const dialog = await screen.findByRole("dialog");
    await userEvent.type(
      within(dialog).getByLabelText("New collection name"),
      "Villeneuve",
    );
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Create" }),
    );
    await waitFor(() =>
      expect(requests).toEqual([
        {
          url: "/api/v1/collections/",
          method: "POST",
          body: { name: "Villeneuve", overview: "" },
        },
        {
          url: "/api/v1/collections/new/members",
          method: "POST",
          body: {
            items: [{ media_type: "movie", media_id: "arrival-2016" }],
          },
        },
      ]),
    );
  });
});
