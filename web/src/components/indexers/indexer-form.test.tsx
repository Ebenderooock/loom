import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  IndexerForm,
  validateIndexerForm,
  type IndexerFormValues,
} from "@/components/indexers/indexer-form";

afterEach(() => vi.restoreAllMocks());

function baseValues(over: Partial<IndexerFormValues> = {}): IndexerFormValues {
  return {
    kind: "newznab",
    name: "Hydra",
    enabled: true,
    priority: 25,
    url: "https://nzbhydra.example/api",
    api_key: "secret",
    categories: [2000, 5000],
    tags: [],
    proxy_id: "",
    ...over,
  };
}

describe("validateIndexerForm", () => {
  it("accepts a fully populated form", () => {
    expect(validateIndexerForm(baseValues())).toEqual({});
  });

  it("requires a name", () => {
    expect(validateIndexerForm(baseValues({ name: "  " })).name).toMatch(
      /name/i,
    );
  });

  it("requires an http(s) URL", () => {
    expect(validateIndexerForm(baseValues({ url: "" })).url).toMatch(/URL/);
    expect(validateIndexerForm(baseValues({ url: "not a url" })).url).toBe(
      "URL is not valid.",
    );
    expect(
      validateIndexerForm(baseValues({ url: "ftp://example/api" })).url,
    ).toMatch(/http/);
  });

  it("requires an API key", () => {
    expect(validateIndexerForm(baseValues({ api_key: "" })).api_key).toMatch(
      /API key/i,
    );
  });

  it("allows a blank API key when editing an existing indexer", () => {
    expect(validateIndexerForm(baseValues({ api_key: "" }), true)).toEqual({});
  });

  it("rejects out-of-range priority", () => {
    expect(validateIndexerForm(baseValues({ priority: -1 })).priority).toBe(
      "Priority must be between 0 and 100.",
    );
    expect(validateIndexerForm(baseValues({ priority: 101 })).priority).toBe(
      "Priority must be between 0 and 100.",
    );
  });

  it("rejects malformed timeout", () => {
    expect(
      validateIndexerForm(baseValues({ timeout: "30 seconds" })).timeout,
    ).toMatch(/duration/);
    expect(
      validateIndexerForm(baseValues({ timeout: "30s" })).timeout,
    ).toBeUndefined();
  });
});

describe("IndexerForm rendering", () => {
  it("tests unsaved edits with the saved ID and allows an unchanged blank key", async () => {
    const user = userEvent.setup();
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        new Response(JSON.stringify({ ok: true, latency_ms: 1 })),
      );
    const onSubmit = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <IndexerForm
          initial={{
            id: "saved",
            kind: "newznab",
            name: "Hydra",
            enabled: true,
            priority: 25,
            config: { url: "https://example.test/api" },
            categories: [],
            tags: [],
          }}
          proxies={[]}
          onSubmit={onSubmit}
        />
      </QueryClientProvider>,
    );
    await user.clear(screen.getByLabelText(/^url$/i));
    await user.type(
      screen.getByLabelText(/^url$/i),
      "https://example.test/edited",
    );
    await user.click(screen.getByRole("button", { name: /^test$/i }));
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledOnce());
    const call = fetchSpy.mock.calls[0];
    if (!call) throw new Error("Expected a connectivity test request");
    const [url, init] = call;
    expect(url).toBe("/api/v1/indexers/test");
    expect(JSON.parse(init?.body as string)).toMatchObject({
      id: "saved",
      config: { url: "https://example.test/edited", api_key: "" },
    });
    await user.click(screen.getByRole("button", { name: /save changes/i }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: "",
        url: "https://example.test/edited",
      }),
      true,
    );
  });

  it("surfaces validation errors before calling onSubmit", async () => {
    const user = userEvent.setup();
    let called = false;
    render(
      <QueryClientProvider
        client={
          new QueryClient({
            defaultOptions: {
              queries: { retry: false },
              mutations: { retry: false },
            },
          })
        }
      >
        <IndexerForm
          proxies={[]}
          onSubmit={() => {
            called = true;
          }}
        />
      </QueryClientProvider>,
    );
    // Clear the required name field then submit.
    await user.clear(screen.getByLabelText(/^name$/i));
    await user.clear(screen.getByLabelText(/^url$/i));
    await user.clear(screen.getByLabelText(/^api key$/i));
    await user.click(screen.getByRole("button", { name: /add indexer/i }));
    expect(called).toBe(false);
    expect(
      screen.getByText(/give the indexer a recognizable name/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/enter the upstream url/i)).toBeInTheDocument();
    expect(screen.getByText(/api key is required/i)).toBeInTheDocument();
  });
});
