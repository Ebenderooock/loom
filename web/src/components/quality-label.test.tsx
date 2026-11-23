import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { QualityLabel } from "./quality-label";

afterEach(() => vi.restoreAllMocks());

it("displays built-in quality without fetching custom tiers", () => {
  const fetch = vi.spyOn(globalThis, "fetch");
  render(<QualityLabel quality="webdl-1080p" />);
  expect(screen.getByText("webdl-1080p")).toBeInTheDocument();
  expect(fetch).not.toHaveBeenCalled();
});

it("resolves stored tier identity to its current name", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({
      data: [{ id: "ct_test", name: "1080p HEVC", specifications: [] }],
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <QualityLabel quality="custom-tier:ct_test" />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("1080p HEVC")).toBeInTheDocument();
  expect(screen.queryByText("custom-tier:ct_test")).not.toBeInTheDocument();
});

it("surfaces tier lookup errors instead of showing an opaque ID as a quality", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json(
      { error: { message: "Tier storage unavailable" } },
      { status: 500 },
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <QualityLabel quality="custom-tier:ct_test" />
    </QueryClientProvider>,
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Custom tier unavailable",
  );
  expect(screen.getByRole("alert")).toHaveAttribute(
    "title",
    "Tier storage unavailable",
  );
});
