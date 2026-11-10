import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LibraryImportDialog } from "@/components/movies/library-import-dialog";
import { SeriesLibraryImportDialog } from "@/components/series/series-library-import-dialog";
import { apiFetch } from "@/lib/fetch";
import type { Library } from "@/lib/libraries-api";

vi.mock("@/lib/fetch", () => ({ apiFetch: vi.fn() }));

const library: Library = {
  id: "library-1",
  name: "Media",
  path: "/volume1/media",
  media_type: "movie",
  monitor_on_add: true,
  quality_profile_id: "",
  unmonitor_on_delete: false,
  auto_archive_watched: false,
  auto_archive_days_after_watch: 0,
  created_at: "",
  updated_at: "",
  accessible: true,
  disk_space: { total_bytes: 0, free_bytes: 0, used_bytes: 0 },
  file_count: 0,
};

const running = {
  id: "scan-1",
  libraryId: library.id,
  rootFolderPath: library.path,
  status: "running",
  totalFiles: 3,
  matched: 1,
  unmatched: 0,
  imported: 1,
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

for (const [mediaType, Component] of [
  ["movies", LibraryImportDialog],
  ["series", SeriesLibraryImportDialog],
] as const) {
  describe(`${mediaType} existing-library import`, () => {
    beforeEach(() => {
      vi.resetAllMocks();
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    const renderDialog = (onImportComplete = vi.fn()) =>
      render(
        <Component
          open
          onOpenChange={vi.fn()}
          libraries={[{ ...library }]}
          onImportComplete={onImportComplete}
        />,
      );

    async function start() {
      await act(async () => {
        fireEvent.click(screen.getByRole("button", { name: "Start Scan" }));
      });
    }

    it("shows loading immediately, before the start request resolves", async () => {
      let resolve!: (response: Response) => void;
      vi.mocked(apiFetch).mockReturnValueOnce(
        new Promise<Response>((done) => {
          resolve = done;
        }),
      );
      renderDialog();
      await start();
      expect(screen.getByRole("status")).toHaveTextContent("Starting scan");
      vi.mocked(apiFetch).mockResolvedValueOnce(json(running));
      await act(async () => {
        resolve(json({ scanId: "scan-1" }));
      });
      expect(apiFetch).toHaveBeenNthCalledWith(
        1,
        `/api/v1/${mediaType}/scan`,
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ libraryId: library.id }),
        }),
      );
      expect(screen.getByText("Files Found")).toBeInTheDocument();
    });

    it("shows start errors without polling an undefined scan ID", async () => {
      vi.mocked(apiFetch).mockResolvedValueOnce(
        json({ error: "permission denied: /volume1/media" }, 500),
      );
      renderDialog();
      await start();
      expect(screen.getByRole("alert")).toHaveTextContent("permission denied");
      expect(screen.getByRole("button", { name: "Start Scan" })).toBeEnabled();
      expect(apiFetch).toHaveBeenCalledTimes(1);
    });

    it("rejects a missing scan ID", async () => {
      vi.mocked(apiFetch).mockResolvedValueOnce(json({}));
      renderDialog();
      await start();
      expect(screen.getByRole("alert")).toHaveTextContent("invalid scan ID");
      expect(apiFetch).toHaveBeenCalledTimes(1);
    });

    it("shows polling errors and allows retry", async () => {
      vi.mocked(apiFetch)
        .mockResolvedValueOnce(json({ scanId: "scan-1" }))
        .mockResolvedValueOnce(json(running))
        .mockResolvedValueOnce(json({ error: "scan not found" }, 404));
      renderDialog();
      await start();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500);
      });
      expect(screen.getByRole("alert")).toHaveTextContent("scan not found");
      fireEvent.click(screen.getByRole("button", { name: "Try Again" }));
      expect(screen.getByRole("button", { name: "Start Scan" })).toBeEnabled();
    });

    it("keeps running and completed scans across parent refreshes", async () => {
      const onComplete = vi.fn();
      vi.mocked(apiFetch)
        .mockResolvedValueOnce(json({ scanId: "scan-1" }))
        .mockResolvedValueOnce(json(running))
        .mockResolvedValueOnce(
          json({ ...running, status: "completed", imported: 3 }),
        )
        .mockResolvedValueOnce(json([]));
      const view = renderDialog(onComplete);
      await start();
      const refreshParent = () =>
        view.rerender(
          <Component
            open
            onOpenChange={vi.fn()}
            libraries={[{ ...library }]}
            onImportComplete={onComplete}
          />,
        );
      refreshParent();
      expect(
        screen.queryByRole("button", { name: "Start Scan" }),
      ).not.toBeInTheDocument();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1500);
      });
      refreshParent();
      expect(screen.getByText("Scan Complete")).toBeInTheDocument();
      expect(onComplete).toHaveBeenCalledTimes(1);
      expect(screen.getByText("Imported")).toBeInTheDocument();
    });

    it("displays failed scan details rather than success", async () => {
      const onComplete = vi.fn();
      vi.mocked(apiFetch)
        .mockResolvedValueOnce(json({ scanId: "scan-1" }))
        .mockResolvedValueOnce(
          json({
            ...running,
            status: "failed",
            errors: ["read /volume1/media: permission denied"],
          }),
        )
        .mockResolvedValueOnce(json([]));
      renderDialog(onComplete);
      await start();
      expect(screen.getByText("Scan Failed")).toBeInTheDocument();
      expect(
        screen.getByText(/read \/volume1\/media: permission denied/),
      ).toBeInTheDocument();
      expect(onComplete).not.toHaveBeenCalled();
    });

    it("preserves completed results when loading unmatched files fails", async () => {
      vi.mocked(apiFetch)
        .mockResolvedValueOnce(json({ scanId: "scan-1" }))
        .mockResolvedValueOnce(json({ ...running, status: "completed" }))
        .mockResolvedValueOnce(json({ error: "unmatched unavailable" }, 500));
      const onComplete = vi.fn();
      renderDialog(onComplete);
      await start();
      expect(screen.getByText("Scan Complete")).toBeInTheDocument();
      expect(screen.getByRole("alert")).toHaveTextContent(
        "unmatched unavailable",
      );
      expect(onComplete).toHaveBeenCalledOnce();
    });

    it("ignores late responses after closing and reopening", async () => {
      let resolve!: (response: Response) => void;
      vi.mocked(apiFetch).mockReturnValueOnce(
        new Promise<Response>((done) => {
          resolve = done;
        }),
      );
      const view = renderDialog();
      await start();
      view.rerender(
        <Component
          open={false}
          onOpenChange={vi.fn()}
          libraries={[library]}
          onImportComplete={vi.fn()}
        />,
      );
      view.rerender(
        <Component
          open
          onOpenChange={vi.fn()}
          libraries={[library]}
          onImportComplete={vi.fn()}
        />,
      );
      await act(async () => {
        resolve(json({ scanId: "old-scan" }));
      });
      expect(screen.getByRole("button", { name: "Start Scan" })).toBeEnabled();
      expect(apiFetch).toHaveBeenCalledTimes(1);
    });
  });
}
