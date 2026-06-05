import { describe, expect, it } from "vitest";
import { toPatchPayload } from "@/components/indexers/indexer-form-adapter";
import type { IndexerFormValues } from "@/components/indexers/indexer-form";
import type { Indexer } from "@/lib/indexers-api";

function origin(over: Partial<Indexer> = {}): Indexer {
  return {
    id: "hydra",
    kind: "newznab",
    name: "Hydra",
    enabled: true,
    priority: 25,
    config: { url: "https://nzbhydra.example/api", api_key: "secret" },
    categories: [2000],
    tags: [],
    ...over,
  };
}

function values(over: Partial<IndexerFormValues> = {}): IndexerFormValues {
  return {
    kind: "newznab",
    name: "Hydra",
    enabled: true,
    priority: 25,
    url: "https://nzbhydra.example/api",
    api_key: "secret",
    categories: [2000],
    tags: [],
    proxy_id: "",
    ...over,
  };
}

describe("toPatchPayload — proxy_id null vs unset", () => {
  it("omits proxy_id entirely when neither side has a proxy", () => {
    const patch = toPatchPayload(values(), origin());
    expect("proxy_id" in patch).toBe(false);
  });

  it("omits proxy_id when the user keeps the same proxy attached", () => {
    const patch = toPatchPayload(
      values({ proxy_id: "p1" }),
      origin({ proxy_id: "p1" }),
    );
    expect("proxy_id" in patch).toBe(false);
  });

  it("sends proxy_id: null to detach an existing proxy", () => {
    const patch = toPatchPayload(
      values({ proxy_id: "" }),
      origin({ proxy_id: "p1" }),
    );
    // Must be present (not undefined) AND explicitly null so the JSON
    // serializer emits it as "proxy_id": null on the wire.
    expect("proxy_id" in patch).toBe(true);
    expect(patch.proxy_id).toBeNull();
  });

  it("sends a string to attach a new proxy", () => {
    const patch = toPatchPayload(
      values({ proxy_id: "p2" }),
      origin({ proxy_id: "p1" }),
    );
    expect(patch.proxy_id).toBe("p2");
  });

  it("sends a string when attaching a proxy to an indexer that had none", () => {
    const patch = toPatchPayload(values({ proxy_id: "p1" }), origin());
    expect(patch.proxy_id).toBe("p1");
  });
});

describe("toPatchPayload — diffing", () => {
  it("omits unchanged scalar fields", () => {
    const patch = toPatchPayload(values(), origin());
    expect(patch).toEqual({});
  });

  it("includes scalar fields that changed", () => {
    const patch = toPatchPayload(
      values({ name: "Hydra Prime", priority: 10, enabled: false }),
      origin(),
    );
    expect(patch).toMatchObject({
      name: "Hydra Prime",
      priority: 10,
      enabled: false,
    });
  });

  it("includes a fresh config when fields differ", () => {
    const patch = toPatchPayload(values({ api_key: "rotated" }), origin());
    expect(patch.config).toEqual({
      url: "https://nzbhydra.example/api",
      api_key: "rotated",
    });
  });
});
