import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toaster } from "sonner";
import { GeneralPanel } from "../settings";
import * as apiKeys from "@/lib/api-keys-api";

vi.mock("@/lib/api-keys-api");

describe("GeneralPanel API key card", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders an existing masked key and explains that it cannot be copied", () => {
    vi.mocked(apiKeys.useAPIKeys).mockReturnValue({
      data: [
        {
          id: "key-1",
          name: "Default",
          key: "loom_abcd...",
          scopes: "*",
          created_at: "2026-07-02T00:00:00Z",
          last_used: "",
        },
      ],
      isLoading: false,
    } as any);
    vi.mocked(apiKeys.useCreateAPIKey).mockReturnValue({
      mutateAsync: vi.fn(),
      isPending: false,
    } as any);

    render(
      <>
        <GeneralPanel />
        <Toaster />
      </>,
    );

    expect(screen.getByDisplayValue("loom_abcd...")).toBeInTheDocument();
    expect(
      screen.getByText(/existing keys are masked after creation/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /generate new/i }),
    ).toBeInTheDocument();
  });

  it("shows the raw key after generation", async () => {
    const mutateAsync = vi.fn().mockResolvedValue({
      id: "key-2",
      name: "Default",
      key: "loom_super_secret_key",
      scopes: "*",
      created_at: "2026-07-02T00:00:00Z",
    });

    vi.mocked(apiKeys.useAPIKeys).mockReturnValue({
      data: [],
      isLoading: false,
    } as any);
    vi.mocked(apiKeys.useCreateAPIKey).mockReturnValue({
      mutateAsync,
      isPending: false,
    } as any);

    render(
      <>
        <GeneralPanel />
        <Toaster />
      </>,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /generate key/i }),
    );

    await waitFor(() => {
      expect(
        screen.getByDisplayValue("loom_super_secret_key"),
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText(/copy this key now\. for security, loom only shows/i),
    ).toBeInTheDocument();
    expect(mutateAsync).toHaveBeenCalledWith({ name: "Default", scopes: "*" });
  });
});
