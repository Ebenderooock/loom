import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  evaluateResults,
  type EvaluateParams,
  type EvaluateResponse,
  type SearchResult,
} from "@/lib/indexers-api";
import { PolicyPreview } from "./policy-preview";

vi.mock("@/lib/indexers-api", () => ({ evaluateResults: vi.fn() }));
vi.mock("@/lib/quality-profiles-api", () => ({
  useQualityProfiles: () => ({
    data: [
      {
        id: "assigned",
        name: "Assigned profile",
        items: '[{"id":"web","allowed":true}]',
        cutoff: "web",
        upgrade_allowed: true,
        min_format_score: 10,
        format_items: [{ format_id: "preferred", score: 50 }],
      },
    ],
    isError: false,
  }),
}));

const results: SearchResult[] = [
  {
    indexer_id: "test",
    title: "Movie.1080p.WEB-DL",
    link: "https://example.test/release",
    size_bytes: 1000,
  },
];
const context = {
  title: "Movie",
  media_type: "movie",
  quality_profile_id: "assigned",
};
const emptyResponse: EvaluateResponse = {
  total: 1,
  passed: 0,
  preview: false,
  results: [],
};

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <PolicyPreview context={context} results={results} />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByText("Explain matching / preview policy"));
}

beforeEach(() => {
  vi.mocked(evaluateResults).mockReset().mockResolvedValue(emptyResponse);
});

describe("PolicyPreview", () => {
  it("uses the actual evaluate path, previews an unsaved threshold, and resets", async () => {
    show();
    await waitFor(() => expect(evaluateResults).toHaveBeenCalled());
    expect(
      vi.mocked(evaluateResults).mock.calls[0]?.[0].policy_preview,
    ).toBeUndefined();
    fireEvent.change(
      screen.getByLabelText("Preview minimum format score (points)"),
      { target: { value: "100" } },
    );
    await waitFor(() =>
      expect(evaluateResults).toHaveBeenLastCalledWith(
        expect.objectContaining({
          policy_preview: {
            items: '[{"id":"web","allowed":true}]',
            cutoff: "web",
            upgrade_allowed: true,
            min_format_score: 100,
            format_items: [{ format_id: "preferred", score: 50 }],
          },
        }),
      ),
    );
    expect(screen.getByText(/Preview only/)).toBeInTheDocument();
    fireEvent.click(screen.getByText("Reset to assigned policy"));
    await waitFor(() =>
      expect(evaluateResults).toHaveBeenLastCalledWith(
        expect.objectContaining({
          quality_profile_id: "assigned",
        }),
      ),
    );
    expect(
      screen.getByLabelText("Preview minimum format score (points)"),
    ).toHaveValue(null);
    expect(screen.getByText(/Saved-policy dry run/)).toBeInTheDocument();
  });

  it("shows errors and suppresses stale preview output when the policy changes", async () => {
    let finish: ((value: EvaluateResponse) => void) | undefined;
    vi.mocked(evaluateResults).mockImplementation((params: EvaluateParams) => {
      if (params.policy_preview)
        return Promise.reject(new Error("preview unavailable"));
      return new Promise<EvaluateResponse>((resolve) => {
        finish = resolve;
      });
    });
    show();
    await waitFor(() => expect(evaluateResults).toHaveBeenCalled());
    fireEvent.change(
      screen.getByLabelText("Preview minimum format score (points)"),
      { target: { value: "100" } },
    );
    await screen.findByText("Evaluation failed: preview unavailable");
    finish?.({ ...emptyResponse, passed: 1 });
    await waitFor(() =>
      expect(screen.queryByText(/1 of 1 accepted/)).not.toBeInTheDocument(),
    );
  });
});
