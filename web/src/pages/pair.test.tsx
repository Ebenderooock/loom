import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PairPage } from "./pair";
import { AuthPage } from "./auth";

const navigate = vi.fn();
const refreshAuth = vi.fn();
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => navigate }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ refreshAuth }),
}));

const fetchMock = vi.fn();
function respond(data: unknown, status = 200) {
  fetchMock.mockResolvedValueOnce({
    ok: status < 400,
    json: async () => data,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("fetch", fetchMock);
  window.history.replaceState({}, "", "/pair?code=ABCD-2345");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("device approval", () => {
  it("never approves on visiting a link and explicitly confirms exact code", async () => {
    render(<PairPage />);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Code shown on your TV")).toHaveValue(
      "ABCD-2345",
    );
    respond({
      user_code: "ABCD-2345",
      device_name: "Living room",
      status: "pending",
      expires_in: 300,
    });
    fireEvent.click(screen.getByRole("button", { name: "Review request" }));
    await screen.findByText("Living room");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    respond({ status: "approved" });
    fireEvent.click(screen.getByRole("button", { name: "Approve my TV" }));
    await screen.findByRole("status");
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({
      method: "POST",
      headers: { "X-Loom-Pairing": "1" },
      body: JSON.stringify({ user_code: "ABCD-2345", decision: "approve" }),
    });
  });

  it("shows expiry and does not offer approval for invalid requests", async () => {
    respond({ error: "code expired" }, 410);
    render(<PairPage />);
    fireEvent.click(screen.getByRole("button", { name: "Review request" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("code expired");
    expect(
      screen.queryByRole("button", { name: "Approve my TV" }),
    ).not.toBeInTheDocument();
  });

  it("denies without exposing credentials and retains the proxy prefix", async () => {
    window.history.replaceState({}, "", "/loom/pair?code=ABCD-2345");
    respond({
      user_code: "ABCD-2345",
      device_name: "TV",
      status: "pending",
      expires_in: 60,
    });
    render(<PairPage />);
    fireEvent.click(screen.getByRole("button", { name: "Review request" }));
    await screen.findByRole("button", { name: "Deny request" });
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/loom/api/v1/auth/pairing/request?code=ABCD-2345",
    );
    respond({ status: "denied" });
    fireEvent.click(screen.getByRole("button", { name: "Deny request" }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      "No API key was issued",
    );
  });

  it("retains approval URL and code through browser login", async () => {
    respond({});
    render(<AuthPage />);
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "alice" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Login" }));
    await waitFor(() => expect(refreshAuth).toHaveBeenCalled());
    expect(navigate).not.toHaveBeenCalled();
    expect(window.location.search).toBe("?code=ABCD-2345");
  });
});
