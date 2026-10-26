import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiFetch } from "./fetch";
import { evaluateResults } from "./indexers-api";

vi.mock("./fetch", () => ({ apiFetch: vi.fn() }));

beforeEach(() => vi.mocked(apiFetch).mockReset());

describe("evaluateResults", () => {
  it("preserves production metadata and forwards unsaved policy only to evaluation", async () => {
    vi.mocked(apiFetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          total: 1,
          passed: 1,
          results: [],
          preview: true,
        }),
      ),
    );
    const policy = {
      items: "[]",
      cutoff: "",
      upgrade_allowed: false,
      min_format_score: 100,
      format_items: [],
    };
    await evaluateResults({
      quality_profile_id: "assigned",
      runtime: 30,
      alternate_titles: ["Localized"],
      policy_preview: policy,
      results: [
        {
          title: "Example",
          indexer_id: "one",
          link: "https://example.test/release",
          size_bytes: 1000,
          seeders: 3,
          leechers: 2,
          internal: true,
          scene: true,
        },
      ],
    });
    expect(apiFetch).toHaveBeenCalledWith(
      "/api/v1/autosearch/evaluate",
      expect.objectContaining({ method: "POST" }),
    );
    const body = JSON.parse(
      String(vi.mocked(apiFetch).mock.calls[0]?.[1]?.body),
    );
    expect(body.policy_preview).toEqual(policy);
    expect(body.runtime).toBe(30);
    expect(body.alternate_titles).toEqual(["Localized"]);
    expect(body.results[0]).toEqual(
      expect.objectContaining({
        size: 1000,
        seeders: 3,
        peers: 5,
        internal: true,
        scene: true,
      }),
    );
    expect(body.results[0].size_bytes).toBeUndefined();
  });

  it("shows the engine error rather than a success-shaped empty evaluation", async () => {
    vi.mocked(apiFetch).mockResolvedValue(
      new Response(JSON.stringify({ error: "invalid policy preview" }), {
        status: 400,
      }),
    );
    await expect(
      evaluateResults({ quality_profile_id: "assigned", results: [] }),
    ).rejects.toThrow("invalid policy preview");
  });
});
