import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AcquisitionRecovery } from "./acquisition-recovery";
import { apiFetch } from "@/lib/fetch";

function conflict(retryAllowed = true) {
  return new Response(
    JSON.stringify({
      error: {
        code: "acquisition_deferred",
        message: "This release was attempted before.",
        details: {
          history_id: "history-1",
          title: "From Zero (Deluxe) 24bit",
          status: retryAllowed ? "acquired" : "acquiring",
          retry_allowed: retryAllowed,
        },
      },
    }),
    { status: 409 },
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("in-place acquisition recovery", () => {
  it.each(["/api/v1/albums/album/grab", "/api/v1/download-clients/qbit/items"])(
    "asks before authorizing and replaying %s",
    async (path) => {
      const request = vi
        .fn()
        .mockResolvedValueOnce(conflict())
        .mockResolvedValueOnce(
          new Response(
            JSON.stringify({ results: [{ id: "history-1", ok: true }] }),
          ),
        )
        .mockResolvedValueOnce(
          new Response(JSON.stringify({ item_id: "download" }), {
            status: 202,
          }),
        );
      vi.stubGlobal("fetch", request);
      render(<AcquisitionRecovery />);
      const init = {
        method: "POST",
        body: JSON.stringify({ title: "From Zero" }),
      };
      const response = apiFetch(path, init);
      expect(await screen.findByRole("dialog")).toHaveTextContent(
        "Retry this release?",
      );
      expect(request).toHaveBeenCalledTimes(1);
      await userEvent.click(
        screen.getByRole("button", { name: "Retry release" }),
      );
      expect((await response).status).toBe(202);
      expect(request.mock.calls[1]?.[0]).toBe(
        "/api/v1/downloads/history/actions",
      );
      expect(JSON.parse(request.mock.calls[1]?.[1].body)).toEqual({
        ids: ["history-1"],
        action: "retry",
      });
      expect(request.mock.calls[2]).toEqual([
        path,
        { ...init, credentials: "include" },
      ]);
    },
  );

  it("does not authorize or resubmit when cancelled", async () => {
    const request = vi.fn().mockResolvedValue(conflict());
    vi.stubGlobal("fetch", request);
    render(<AcquisitionRecovery />);
    const response = apiFetch("/api/v1/albums/album/search", {
      method: "POST",
    });
    await screen.findByRole("dialog");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect((await response).status).toBe(409);
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("explains an in-progress admission without offering retry", async () => {
    const request = vi.fn().mockResolvedValue(conflict(false));
    vi.stubGlobal("fetch", request);
    render(<AcquisitionRecovery />);
    const response = apiFetch("/api/v1/albums/album/search", {
      method: "POST",
    });
    expect(await screen.findByRole("dialog")).toHaveTextContent(
      "Download already in progress",
    );
    expect(
      screen.queryByRole("button", { name: "Retry release" }),
    ).not.toBeInTheDocument();
    await userEvent.click(screen.getAllByRole("button", { name: "Close" })[0]!);
    await response;
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("surfaces a refused authorization without submitting again", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(conflict())
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            results: [
              { id: "history-1", ok: false, error: "Download is active." },
            ],
          }),
        ),
      );
    vi.stubGlobal("fetch", request);
    render(<AcquisitionRecovery />);
    const response = apiFetch("/api/v1/albums/album/search", {
      method: "POST",
    });
    await screen.findByRole("dialog");
    await userEvent.click(
      screen.getByRole("button", { name: "Retry release" }),
    );
    expect(await (await response).json()).toEqual({
      error: { code: "retry_refused", message: "Download is active." },
    });
    expect(request).toHaveBeenCalledTimes(2);
  });

  it("resolves pending requests on unmount without retrying", async () => {
    const request = vi.fn().mockResolvedValue(conflict());
    vi.stubGlobal("fetch", request);
    const view = render(<AcquisitionRecovery />);
    const response = apiFetch("/api/v1/albums/album/search", {
      method: "POST",
    });
    await screen.findByRole("dialog");
    view.unmount();
    await waitFor(async () => expect((await response).status).toBe(409));
    expect(request).toHaveBeenCalledTimes(1);
  });
});
