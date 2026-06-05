import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  IndexerForm,
  validateIndexerForm,
  type IndexerFormValues,
} from "@/components/indexers/indexer-form";

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
