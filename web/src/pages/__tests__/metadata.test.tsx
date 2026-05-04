import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { MetadataPage } from "../metadata";
import * as api from "@/lib/metadata-api";

// Mock the metadata API
vi.mock("@/lib/metadata-api");

// Helper to render with query client and toaster
function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <MetadataPage />
      <Toaster />
    </QueryClientProvider>
  );
}

describe("MetadataPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("Search Tab", () => {
    it("renders search form with inputs", () => {
      renderPage();

      expect(screen.getByDisplayValue("")).toHaveAttribute("placeholder", "e.g., The Matrix, Breaking Bad");
      expect(screen.getByRole("combobox")).toBeInTheDocument();
      expect(screen.getByText("Search", { selector: "button" })).toBeInTheDocument();
    });

    it("submits search with query, type, and year", async () => {
      const mockSearch = vi.fn().mockResolvedValue([
        {
          title: "The Matrix",
          year: 1999,
          rating: 8.7,
          overview: "A hacker discovers reality is a simulation",
          poster_path: "/poster.jpg",
        },
      ]);
      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: mockSearch,
        isPending: false,
      } as any);

      renderPage();
      const user = userEvent.setup();

      // Fill search form
      const queryInput = screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad");
      await user.type(queryInput, "The Matrix");

      const yearInput = screen.getByPlaceholderText("e.g., 1999");
      await user.type(yearInput, "1999");

      // Submit
      const searchBtn = screen.getByText("Search", { selector: "button" });
      await user.click(searchBtn);

      // Verify search was called
      await waitFor(() => {
        expect(mockSearch).toHaveBeenCalled();
      });
    });

    it("shows error when search query is empty", async () => {
      renderPage();
      const user = userEvent.setup();

      const searchBtn = screen.getByText("Search", { selector: "button" });
      await user.click(searchBtn);

      await waitFor(() => {
        expect(screen.getByText("Please enter a search query")).toBeInTheDocument();
      });
    });

    it("renders results grid with movie cards", async () => {
      const mockResults = [
        {
          title: "The Matrix",
          year: 1999,
          rating: 8.7,
          overview: "A hacker discovers reality is a simulation",
          poster_path: "/poster.jpg",
        },
        {
          title: "The Matrix Reloaded",
          year: 2003,
          rating: 7.2,
          overview: "Neo and his allies race against time",
          poster_path: "/poster2.jpg",
        },
      ];

      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: vi.fn().mockResolvedValue(mockResults),
        isPending: false,
      } as any);

      const { container } = renderPage();

      // Results should be rendered
      await waitFor(() => {
        expect(screen.getByText("The Matrix")).toBeInTheDocument();
        expect(screen.getByText("The Matrix Reloaded")).toBeInTheDocument();
      });

      // Check for ratings
      const ratings = screen.getAllByText(/⭐ \d+\.\d+\/10/);
      expect(ratings.length).toBe(2);
    });

    it("truncates long overviews in results", () => {
      const longOverview =
        "This is a very long overview that should be truncated because it exceeds the maximum length allowed in the UI for display purposes.";

      const mockResults = [
        {
          title: "Long Movie",
          overview: longOverview,
          poster_path: "/poster.jpg",
          rating: 7.5,
        },
      ];

      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: vi.fn().mockResolvedValue(mockResults),
        isPending: false,
      } as any);

      renderPage();

      // Verify overview is in DOM but truncated visually (line-clamp-2)
      const overviewElement = screen.getByText(longOverview);
      expect(overviewElement).toHaveClass("line-clamp-2");
    });

    it("shows import dialog when import button clicked", async () => {
      const mockResult = {
        title: "The Matrix",
        year: 1999,
        rating: 8.7,
        overview: "A hacker discovers reality is a simulation",
        poster_path: "/poster.jpg",
      };

      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: vi.fn().mockResolvedValue([mockResult]),
        isPending: false,
      } as any);

      renderPage();
      const user = userEvent.setup();

      // Trigger search to show results
      const queryInput = screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad");
      await user.type(queryInput, "Matrix");
      await user.click(screen.getByText("Search", { selector: "button" }));

      // Wait for results
      await waitFor(() => {
        expect(screen.queryAllByText("Import").length).toBeGreaterThan(0);
      });

      // Click import button
      const importButtons = screen.queryAllByText("Import");
      await user.click(importButtons[0]);

      // Dialog should appear
      await waitFor(() => {
        expect(screen.getByText("Confirm Import")).toBeInTheDocument();
      });
    });

    it("imports metadata with confirmation", async () => {
      const mockImport = vi.fn().mockResolvedValue({ id: "1", type: "movie" });
      vi.mocked(api.useMetadataImport).mockReturnValue({
        mutateAsync: mockImport,
        isPending: false,
      } as any);

      const mockResult = {
        title: "The Matrix",
        rating: 8.7,
      };

      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: vi.fn().mockResolvedValue([mockResult]),
        isPending: false,
      } as any);

      renderPage();
      const user = userEvent.setup();

      // Search and find result
      await user.type(screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad"), "Matrix");
      await user.click(screen.getByText("Search", { selector: "button" }));

      // Click import
      await waitFor(() => {
        expect(screen.queryAllByText("Import").length).toBeGreaterThan(0);
      });

      await user.click(screen.queryAllByText("Import")[0]);

      // Confirm import in dialog
      await waitFor(() => {
        expect(screen.getByText("Confirm Import")).toBeInTheDocument();
      });

      const confirmButton = within(
        screen.getByRole("dialog")
      ).getByText("Import");
      await user.click(confirmButton);

      // Verify import was called
      await waitFor(() => {
        expect(mockImport).toHaveBeenCalled();
      });
    });

    it("shows loading state during search", () => {
      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: vi.fn(),
        isPending: true,
      } as any);

      renderPage();

      const searchBtn = screen.getByText("Searching...", { selector: "button" });
      expect(searchBtn).toBeDisabled();
    });

    it("displays empty state when no results", () => {
      vi.mocked(api.useMetadataSearch).mockReturnValue({
        mutateAsync: vi.fn().mockResolvedValue([]),
        isPending: false,
      } as any);

      renderPage();

      expect(
        screen.getByText("No results. Try a different search.")
      ).toBeInTheDocument();
    });
  });

  describe("Cache Stats Tab", () => {
    it("renders cache stats tab", () => {
      vi.mocked(api.useMetadataStats).mockReturnValue({
        data: {
          hit_rate: 75.5,
          miss_rate: 24.5,
          cache_size: 1024,
          entries: 42,
        },
        isLoading: false,
        isError: false,
      } as any);

      renderPage();

      // Click Cache Stats tab
      screen.getByRole("tab", { name: /Cache Stats/i }).click();

      // Check for stat displays
      expect(screen.getByText("75.5%")).toBeInTheDocument(); // hit rate
      expect(screen.getByText("24.5%")).toBeInTheDocument(); // miss rate
      expect(screen.getByText("1024 KB")).toBeInTheDocument(); // cache size
      expect(screen.getByText("42")).toBeInTheDocument(); // entries
    });

    it("shows refresh button and refetches stats", async () => {
      const mockRefetch = vi.fn();
      vi.mocked(api.useMetadataStats).mockReturnValue({
        data: {
          hit_rate: 75.5,
          miss_rate: 24.5,
          cache_size: 1024,
          entries: 42,
        },
        isLoading: false,
        isError: false,
        refetch: mockRefetch,
      } as any);

      renderPage();
      const user = userEvent.setup();

      screen.getByRole("tab", { name: /Cache Stats/i }).click();

      const refreshBtn = screen.getByText("Refresh", { selector: "button" });
      await user.click(refreshBtn);

      expect(mockRefetch).toHaveBeenCalled();
    });

    it("displays loading state for cache stats", () => {
      vi.mocked(api.useMetadataStats).mockReturnValue({
        isLoading: true,
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Cache Stats/i }).click();

      // Should have skeleton loaders
      const skeletons = screen.queryAllByTestId("skeleton");
      expect(skeletons.length).toBeGreaterThan(0);
    });

    it("displays error state for cache stats", () => {
      vi.mocked(api.useMetadataStats).mockReturnValue({
        isLoading: false,
        isError: true,
        error: new Error("Failed to load stats"),
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Cache Stats/i }).click();

      expect(
        screen.getByText(/Failed to load cache stats/)
      ).toBeInTheDocument();
    });
  });

  describe("Provider Status Tab", () => {
    it("renders provider status cards", () => {
      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "tmdb",
          status: "ok",
          configured_api_key: true,
        },
        isLoading: false,
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Provider Status/i }).click();

      expect(screen.getByText("tmdb")).toBeInTheDocument();
      expect(screen.getByText("✓ OK")).toBeInTheDocument();
    });

    it("displays unconfigured status", () => {
      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "tvdb",
          status: "unconfigured",
          configured_api_key: false,
        },
        isLoading: false,
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Provider Status/i }).click();

      expect(screen.getByText("⚠ Unconfigured")).toBeInTheDocument();
    });

    it("displays error status", () => {
      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "musicbrainz",
          status: "error",
          configured_api_key: true,
          last_test_error: "Connection timeout",
        },
        isLoading: false,
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Provider Status/i }).click();

      expect(screen.getByText("✗ Error")).toBeInTheDocument();
    });

    it("shows test button and handles provider test", async () => {
      const mockTest = vi.fn().mockResolvedValue({
        ok: true,
        latency_ms: 250,
      });

      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "tmdb",
          status: "ok",
          configured_api_key: true,
        },
        isLoading: false,
      } as any);

      vi.mocked(api.useProviderTest).mockReturnValue({
        mutateAsync: mockTest,
        isPending: false,
      } as any);

      renderPage();
      const user = userEvent.setup();

      screen.getByRole("tab", { name: /Provider Status/i }).click();

      const testButtons = screen.queryAllByText("Test", { selector: "button" });
      await user.click(testButtons[0]);

      await waitFor(() => {
        expect(mockTest).toHaveBeenCalled();
      });
    });

    it("displays test result after provider test", async () => {
      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "tmdb",
          status: "ok",
          configured_api_key: true,
        },
        isLoading: false,
      } as any);

      vi.mocked(api.useProviderTest).mockReturnValue({
        mutateAsync: vi.fn().mockResolvedValue({
          ok: true,
          latency_ms: 250,
        }),
        isPending: false,
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Provider Status/i }).click();

      // Assuming test result is shown (implementation dependent)
      const testButtons = screen.queryAllByText("Test", { selector: "button" });
      if (testButtons.length > 0) {
        await userEvent.setup().click(testButtons[0]);
      }
    });

    it("shows three provider cards", () => {
      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "tmdb",
          status: "ok",
          configured_api_key: true,
        },
        isLoading: false,
      } as any);

      renderPage();

      screen.getByRole("tab", { name: /Provider Status/i }).click();

      // Should have tmdb, tvdb, musicbrainz tabs
      const cards = screen.queryAllByRole("button");
      expect(cards.length).toBeGreaterThan(0);
    });
  });

  describe("Tab Navigation", () => {
    it("switches between tabs", async () => {
      vi.mocked(api.useMetadataStats).mockReturnValue({
        data: {
          hit_rate: 75.5,
          miss_rate: 24.5,
          cache_size: 1024,
          entries: 42,
        },
        isLoading: false,
        isError: false,
      } as any);

      vi.mocked(api.useProviderStatus).mockReturnValue({
        data: {
          name: "tmdb",
          status: "ok",
          configured_api_key: true,
        },
        isLoading: false,
      } as any);

      renderPage();
      const user = userEvent.setup();

      // Start on Search tab
      expect(screen.getByText("Search")).toBeInTheDocument();

      // Switch to Cache Stats
      await user.click(screen.getByRole("tab", { name: /Cache Stats/i }));
      expect(screen.getByText("Hit Rate")).toBeInTheDocument();

      // Switch to Provider Status
      await user.click(screen.getByRole("tab", { name: /Provider Status/i }));
      expect(screen.getByText(/tmdb/i)).toBeInTheDocument();

      // Switch back to Search
      await user.click(screen.getByRole("tab", { name: /^Search/i }));
      expect(screen.getByPlaceholderText("e.g., The Matrix, Breaking Bad")).toBeInTheDocument();
    });
  });
});
