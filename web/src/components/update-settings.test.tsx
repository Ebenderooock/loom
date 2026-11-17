import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { UpdateSettings } from "./update-settings";

const auth = vi.hoisted(() => ({ role: "admin" }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: { role: auth.role } }),
}));

const fetchMock = vi.fn<typeof fetch>();
let settings: { channel: string; preleaseOptIn: boolean; autoInstall: boolean };
let notice: string;
let canInstall: boolean;

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <UpdateSettings />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  auth.role = "admin";
  settings = { channel: "stable", preleaseOptIn: false, autoInstall: false };
  notice = "Automatic installation unavailable: no helper or safe rollback.";
  canInstall = false;
  fetchMock.mockReset();
  fetchMock.mockImplementation(async (input, init) => {
    if (init?.method === "PUT") {
      settings = JSON.parse(String(init.body));
      return Response.json(settings);
    }
    if (String(input).endsWith("/check")) {
      return Response.json({
        currentVersion: "1.0.0",
        verified: false,
        available: false,
        message: "No verified update information is available.",
        nextCheck: "2026-11-17T01:00:00Z",
      });
    }
    return Response.json({
      settings,
      canAutoInstall: canInstall,
      verificationConfigured: canInstall,
      installationNotice: notice,
    });
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => vi.unstubAllGlobals());

describe("update settings", () => {
  it("defaults to Stable and cannot enable automatic installation", async () => {
    show();
    expect(await screen.findByLabelText("Update channel")).toHaveValue(
      "stable",
    );
    const auto = screen.getByLabelText("Automatically install native updates");
    expect(auto).toBeDisabled();
    expect(auto).not.toBeChecked();
    expect(
      screen.getByText(/publisher verification key has not been provisioned/),
    ).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1); // no background GitHub check
  });

  it("requires a separate warning acknowledgement for Prelease and persists the opt-in", async () => {
    const user = userEvent.setup();
    const app = show();
    await screen.findByLabelText("Update channel");
    await user.selectOptions(
      screen.getByLabelText("Update channel"),
      "prelease",
    );
    expect(screen.getByRole("alert")).toHaveTextContent("may cause data loss");
    expect(
      screen.getByRole("button", { name: "Save update channel" }),
    ).toBeDisabled();
    await user.click(screen.getByLabelText(/I understand the risks/));
    await user.click(
      screen.getByRole("button", { name: "Save update channel" }),
    );
    await waitFor(() =>
      expect(settings).toEqual({
        channel: "prelease",
        preleaseOptIn: true,
        autoInstall: false,
      }),
    );
    app.unmount();
    show();
    expect(await screen.findByLabelText("Update channel")).toHaveValue(
      "prelease",
    );
    expect(screen.getByLabelText(/I understand the risks/)).toBeChecked();
    await user.click(screen.getByLabelText(/I understand the risks/));
    expect(
      screen.getByRole("button", { name: "Check for updates" }),
    ).toBeDisabled();
    await user.selectOptions(screen.getByLabelText("Update channel"), "stable");
    await user.click(
      screen.getByRole("button", { name: "Save update channel" }),
    );
    await waitFor(() =>
      expect(settings).toEqual({
        channel: "stable",
        preleaseOptIn: false,
        autoInstall: false,
      }),
    );
  });

  it("shows PostgreSQL manual-update fallback and handles unverified checks", async () => {
    notice = "PostgreSQL installations are manual-update only.";
    const user = userEvent.setup();
    show();
    expect(await screen.findByText(notice)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Check for updates" }));
    expect(
      await screen.findByText("No verified update information is available."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Update available:/)).not.toBeInTheDocument();
  });

  it("does not fetch update settings for non-administrators", () => {
    auth.role = "viewer";
    show();
    expect(screen.getByText(/Only administrators/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("only saves automatic installation after an explicit opt-in on an eligible native install", async () => {
    canInstall = true;
    const user = userEvent.setup();
    show();
    const auto = await screen.findByLabelText(
      "Automatically install native updates",
    );
    expect(auto).toBeEnabled();
    expect(auto).not.toBeChecked();
    await user.click(auto);
    expect(settings.autoInstall).toBe(false);
    await user.click(
      screen.getByRole("button", { name: "Save update channel" }),
    );
    await waitFor(() => expect(settings.autoInstall).toBe(true));
  });
});
