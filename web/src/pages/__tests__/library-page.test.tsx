import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { LibraryPage } from "../library";
import * as librariesApi from "@/lib/libraries-api";

vi.mock("@/lib/libraries-api", async () => {
  const actual = await vi.importActual<typeof librariesApi>(
    "@/lib/libraries-api",
  );
  return {
    ...actual,
    useLibraries: vi.fn(),
    useScanLibrary: vi.fn(),
  };
});

describe("LibraryPage unmapped cleanup", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("does not render unmapped count surfaces", () => {
    vi.mocked(librariesApi.useLibraries).mockReturnValue({
      data: [
        {
          id: "lib-1",
          name: "Movies",
          path: "/mnt/movies",
          media_type: "movie",
          monitor_on_add: true,
          quality_profile_id: "default",
          unmonitor_on_delete: false,
          auto_archive_watched: false,
          auto_archive_days_after_watch: 0,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
          accessible: true,
          disk_space: { total_bytes: 1000, used_bytes: 200, free_bytes: 800 },
          file_count: 42,
        },
      ],
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(librariesApi.useScanLibrary).mockReturnValue({
      mutate: vi.fn(),
    } as any);

    render(<LibraryPage />);

    expect(screen.getByText("42 files")).toBeInTheDocument();
    expect(screen.queryByText(/unmapped/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/unmapped folders/i)).not.toBeInTheDocument();
  });
});
