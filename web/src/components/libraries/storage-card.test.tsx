import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StorageCard } from "./storage-card";

const api = vi.hoisted(() => ({ useLibraries: vi.fn() }));
vi.mock("@/lib/libraries-api", () => api);
afterEach(cleanup);

function library(name: string, volume: string, free = 400, total = 1000) {
  return {
    id: name,
    name,
    size_bytes: 100,
    disk_space: {
      volume_id: volume,
      total_bytes: total,
      free_bytes: free,
      used_bytes: total - free,
    },
  };
}

describe("StorageCard", () => {
  it("shows one shared volume with accessible usage, library labels and other usage", () => {
    api.useLibraries.mockReturnValue({
      data: [library("Movies", "one"), library("TV", "one")],
    });
    render(<StorageCard />);
    expect(screen.getAllByRole("region")).toHaveLength(1);
    expect(
      screen.getByRole("img", { name: /400 B free of 1000 B, 60% used/ }),
    ).toBeInTheDocument();
    expect(screen.getByText("Movies")).toBeInTheDocument();
    expect(screen.getByText("TV")).toBeInTheDocument();
    expect(screen.getByText("Other usage")).toBeInTheDocument();
    expect(screen.queryByText(/estimated/i)).not.toBeInTheDocument();
  });

  it("counts separate volumes once and names libraries excluded from totals", () => {
    api.useLibraries.mockReturnValue({
      data: [
        library("Movies", "one"),
        library("TV", "two", 800, 2000),
        { id: "offline", name: "Offline" },
      ],
    });
    render(<StorageCard />);
    expect(screen.getAllByRole("region")).toHaveLength(2);
    expect(screen.getByText(/across 2 volumes/)).toHaveTextContent(
      "1.2 KB free of 2.9 KB",
    );
    expect(
      screen.getByText(/Storage info unavailable: Offline/),
    ).toBeInTheDocument();
  });

  it("makes NAS fallback uncertainty visible", () => {
    api.useLibraries.mockReturnValue({
      data: [library("Movies", "mount1"), library("TV", "mount2")],
    });
    render(<StorageCard />);
    expect(screen.getAllByRole("region")).toHaveLength(1);
    expect(
      screen.getByText(/Separate drives with similar readings/),
    ).toBeInTheDocument();
  });

  it("shows the low-free warning only below ten percent", () => {
    api.useLibraries.mockReturnValue({ data: [library("Movies", "one", 100)] });
    const { rerender } = render(<StorageCard />);
    expect(screen.queryByText(/Low free space/)).not.toBeInTheDocument();
    api.useLibraries.mockReturnValue({ data: [library("Movies", "one", 99)] });
    rerender(<StorageCard />);
    expect(screen.getByText(/Low free space/)).toBeInTheDocument();
  });

  it("does not turn a failed request into an empty successful state", () => {
    api.useLibraries.mockReturnValue({ isError: true });
    render(<StorageCard />);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Unable to load storage information",
    );
    expect(
      screen.queryByText("No libraries configured"),
    ).not.toBeInTheDocument();
  });
});
