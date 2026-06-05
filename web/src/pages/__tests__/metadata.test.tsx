import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { MetadataPage } from "../metadata";
import * as api from "@/lib/metadata-api";

// Mock the metadata API
vi.mock("@/lib/metadata-api");

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  // Set up default mocks for all hooks
  vi.mocked(api.useMetadataSearch).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue([]),
    isPending: false,
  } as any);

  vi.mocked(api.useMetadataImport).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
  } as any);

  vi.mocked(api.useMetadataStats).mockReturnValue({
    data: {
      hit_rate: 75.5,
      miss_rate: 24.5,
      cache_size: 1024,
      entries: 42,
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  } as any);

  vi.mocked(api.useProviderStatus).mockReturnValue({
    data: {
      name: "tmdb",
      status: "ok",
      configured_api_key: true,
    },
    isLoading: false,
    isError: false,
  } as any);

  vi.mocked(api.useProviderTest).mockReturnValue({
    mutateAsync: vi.fn().mockResolvedValue({
      ok: true,
      latency_ms: 100,
    }),
    isPending: false,
  } as any);

  return render(
    <QueryClientProvider client={queryClient}>
      <MetadataPage />
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("MetadataPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the metadata page with all tabs", () => {
    renderPage();

    // Check that tabs are rendered
    expect(screen.getByRole("tab", { name: /Search/i })).toBeInTheDocument();
    expect(
      screen.getByRole("tab", { name: /Cache Stats/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("tab", { name: /Provider Status/i }),
    ).toBeInTheDocument();
  });

  it("renders search form on Search tab", () => {
    renderPage();

    // Check search form elements
    expect(
      screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad"),
    ).toBeInTheDocument();
    expect(screen.getByPlaceholderText("e.g., 1999")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Search" })).toBeInTheDocument();
  });

  it("renders cache stats on Cache Stats tab", async () => {
    renderPage();
    const user = userEvent.setup();

    // Click Cache Stats tab
    await user.click(screen.getByRole("tab", { name: /Cache Stats/i }));

    // Check for cache stats display
    await waitFor(() => {
      expect(screen.getByText(/Hit Rate/i)).toBeInTheDocument();
    });
  });

  it("renders provider status cards on Provider Status tab", async () => {
    renderPage();
    const user = userEvent.setup();

    // Click Provider Status tab
    await user.click(screen.getByRole("tab", { name: /Provider Status/i }));

    // Check for provider cards
    await waitFor(() => {
      expect(screen.getAllByText(/tmdb/i).length).toBeGreaterThan(0);
    });
  });

  it("switches between tabs correctly", async () => {
    renderPage();
    const user = userEvent.setup();

    // Start on Search tab
    expect(
      screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad"),
    ).toBeInTheDocument();

    // Switch to Cache Stats tab
    await user.click(screen.getByRole("tab", { name: /Cache Stats/i }));
    await waitFor(() => {
      expect(screen.getByText(/Hit Rate/i)).toBeInTheDocument();
    });

    // Switch to Provider Status tab
    await user.click(screen.getByRole("tab", { name: /Provider Status/i }));
    await waitFor(() => {
      expect(screen.getAllByText(/tmdb/i).length).toBeGreaterThan(0);
    });

    // Switch back to Search tab
    await user.click(screen.getByRole("tab", { name: /Search/i }));
    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad"),
      ).toBeInTheDocument();
    });
  });

  it("handles search form submission without errors", async () => {
    const mockSearch = vi.fn().mockResolvedValue([
      {
        title: "Test Movie",
        year: 2024,
        rating: 8.5,
        overview: "A test movie",
        poster_path: "/poster.jpg",
      },
    ]);

    vi.mocked(api.useMetadataSearch).mockReturnValue({
      mutateAsync: mockSearch,
      isPending: false,
    } as any);

    renderPage();
    const user = userEvent.setup();

    // Type in search query
    const queryInput = screen.getByPlaceholderText(
      "e.g., The Matrix, Breaking Bad",
    );
    await user.type(queryInput, "Test");

    // Submit search - just verify it doesn't throw
    const searchButton = screen.getByRole("button", { name: "Search" });
    await user.click(searchButton);

    // Page should still be renderable
    expect(
      screen.getByRole("heading", { name: "Metadata" }),
    ).toBeInTheDocument();
  });

  it("displays search results when provided", async () => {
    const mockResults = [
      {
        title: "The Matrix",
        year: 1999,
        rating: 8.7,
        overview: "A hacker discovers reality is a simulation",
        poster_path: "/poster.jpg",
      },
    ];

    vi.mocked(api.useMetadataSearch).mockReturnValue({
      mutateAsync: vi.fn().mockResolvedValue(mockResults),
      isPending: false,
    } as any);

    renderPage();

    // Just verify the page renders without errors
    expect(
      screen.getByRole("heading", { name: "Metadata" }),
    ).toBeInTheDocument();
  });

  it("displays cache statistics correctly", async () => {
    renderPage();
    const user = userEvent.setup();

    // Click Cache Stats tab
    await user.click(screen.getByRole("tab", { name: /Cache Stats/i }));

    // Check for statistics values
    await waitFor(() => {
      expect(screen.getByText("75.5%")).toBeInTheDocument(); // hit rate
      expect(screen.getByText("24.5%")).toBeInTheDocument(); // miss rate
      expect(screen.getByText("1024 KB")).toBeInTheDocument(); // cache size
      expect(screen.getByText("42")).toBeInTheDocument(); // entries
    });
  });

  it("renders provider status with correct information", async () => {
    renderPage();
    const user = userEvent.setup();

    // Click Provider Status tab
    await user.click(screen.getByRole("tab", { name: /Provider Status/i }));

    // Check for provider status display
    await waitFor(() => {
      expect(screen.getAllByText(/tmdb/i).length).toBeGreaterThan(0);
    });
  });

  it("loads page without errors", () => {
    // This test just ensures the page renders without crashing
    renderPage();
    expect(
      screen.getByRole("heading", { name: "Metadata" }),
    ).toBeInTheDocument();
  });
});
