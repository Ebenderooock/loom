import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SeriesPage } from "../series";
import type { Series, SeriesToolbar } from "@/components/series";
import type { ComponentProps } from "react";

const api = vi.hoisted(() => ({ fetch: vi.fn(), header: vi.fn() }));
vi.mock("@/lib/fetch", () => ({ apiFetch: api.fetch }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ isAuthenticated: true, user: { role: "admin" } }),
}));
vi.mock("@/hooks/use-page-header", () => ({
  useSetPageHeader: api.header,
}));
vi.mock("@/lib/libraries-api", () => ({
  useLibraries: () => ({ data: [] }),
}));
vi.mock("@tanstack/react-router", () => ({
  useSearch: () => ({}),
  useNavigate: () => vi.fn(),
}));
vi.mock("@/components/collections/add-to-collection", () => ({
  AddToCollectionButton: () => null,
  AddToCollectionDialog: () => null,
}));
vi.mock("@/components/series/series-library-import-dialog", () => ({
  SeriesLibraryImportDialog: () => null,
}));
vi.mock("@/components/series", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/series")>();
  return {
    ...actual,
    AddSeriesDialog: () => null,
    SeriesDetailSheet: () => null,
    // Exercise the page's actual filtering, selection, counts and bulk handlers
    // through native controls; card and list presentation remain real.
    SeriesToolbar: ({
      monitoredFilter,
      onMonitoredFilterChange,
      onToggleSelectAll,
      onBulkMonitor,
      onBulkUnmonitor,
      selectedCount,
    }: ComponentProps<typeof SeriesToolbar>) => (
      <div>
        <select
          aria-label="Monitoring filter"
          value={monitoredFilter}
          onChange={(event) => onMonitoredFilterChange(event.target.value)}
        >
          <option value="all">All</option>
          <option value="monitored">Monitored</option>
          <option value="unmonitored">Unmonitored</option>
        </select>
        <button onClick={onToggleSelectAll}>Select filtered shows</button>
        <button onClick={onBulkMonitor}>Monitor selected</button>
        <button onClick={onBulkUnmonitor}>Unmonitor selected</button>
        <span>{selectedCount} selected</span>
      </div>
    ),
  };
});

const enabled = [
  "monitored",
  "all",
  "future",
  "missing",
  "existing",
  "pilot",
  "firstSeason",
  "lastSeason",
];
const disabled = ["none", "unmonitored", "archived"];
const shows: Series[] = [...enabled, ...disabled].map((monitoringStatus) => ({
  id: monitoringStatus,
  title: `Show ${monitoringStatus}`,
  year: 2026,
  overview: "",
  genres: [],
  runtime: 45,
  rating: 0,
  network: "",
  status: "continuing",
  seriesType: "standard",
  qualityProfileId: "quality",
  libraryId: "",
  monitoringStatus,
  seasonFolder: true,
}));

beforeEach(() => {
  vi.clearAllMocks();
  api.fetch.mockReset().mockImplementation(async (url, init) => {
    if (init?.method === "PUT") return Response.json({});
    if (url === "/api/v1/series") return Response.json(shows);
    if (url === "/api/v1/quality-profiles") return Response.json([]);
    throw new Error(`Unexpected request: ${url}`);
  });
});

function cards() {
  return screen.getAllByRole("button", { name: /Show / });
}

describe("series monitoring filters, counts and bulk selection", () => {
  it("counts all enabled strategies and filters disabled statuses consistently", async () => {
    render(<SeriesPage />);
    await screen.findByRole("button", { name: /Show all/ });
    expect(api.header).toHaveBeenLastCalledWith(
      "TV Shows",
      "11 series • 8 monitored • 11 continuing",
    );
    for (const card of cards()) {
      expect(card.parentElement?.classList.contains("opacity-60")).toBe(
        disabled.some((status) => card.textContent?.includes(`Show ${status}`)),
      );
    }
    const user = userEvent.setup();
    await user.selectOptions(
      screen.getByLabelText("Monitoring filter"),
      "monitored",
    );
    expect(cards()).toHaveLength(enabled.length);
    for (const status of enabled) {
      expect(
        screen.getByRole("button", { name: new RegExp(`Show ${status}`) }),
      ).toBeInTheDocument();
    }
    await user.selectOptions(
      screen.getByLabelText("Monitoring filter"),
      "unmonitored",
    );
    expect(cards()).toHaveLength(disabled.length);
    for (const status of disabled) {
      expect(
        screen.getByRole("button", { name: new RegExp(`Show ${status}`) }),
      ).toBeInTheDocument();
    }
  });

  it.each([
    ["monitored", "Unmonitor selected", "unmonitored", enabled, 0],
    ["unmonitored", "Monitor selected", "monitored", disabled, 11],
  ] as const)(
    "bulk changes filtered %s shows without touching season selections",
    async (filter, action, status, expectedIds, monitoredCount) => {
      render(<SeriesPage />);
      await screen.findByRole("button", { name: /Show all/ });
      const user = userEvent.setup();
      await user.selectOptions(
        screen.getByLabelText("Monitoring filter"),
        filter,
      );
      await user.click(
        screen.getByRole("button", { name: "Select filtered shows" }),
      );
      expect(screen.getByText(`${expectedIds.length} selected`)).toBeVisible();
      await user.click(screen.getByRole("button", { name: action }));
      await waitFor(() =>
        expect(api.header).toHaveBeenLastCalledWith(
          "TV Shows",
          `11 series • ${monitoredCount} monitored • 11 continuing`,
        ),
      );
      const writes = api.fetch.mock.calls.filter(
        ([, init]) => init?.method === "PUT",
      );
      expect(writes).toHaveLength(expectedIds.length);
      expect(writes.map(([url]) => url).sort()).toEqual(
        expectedIds.map((id) => `/api/v1/series/${id}/monitoring`).sort(),
      );
      expect(
        writes.every(([, init]) => JSON.parse(init.body).status === status),
      ).toBe(true);
      expect(screen.getByText("0 selected")).toBeVisible();
    },
  );
});
