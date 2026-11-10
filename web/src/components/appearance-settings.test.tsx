import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppearanceProvider } from "@/hooks/use-appearance";
import { AppearanceSettings } from "@/components/appearance-settings";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  DEFAULT_APPEARANCE,
  backgroundUrlError,
  type Appearance,
} from "@/lib/appearance-api";

const auth = vi.hoisted(() => ({ role: "admin" }));
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ isAuthenticated: true, user: { role: auth.role } }),
}));

let stored: Appearance;
let failSave: boolean;
let failLoad: boolean;
const fetchMock = vi.fn<typeof fetch>();

function renderAppearance() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <AppearanceProvider>
        <ThemeToggle />
        <AppearanceSettings />
      </AppearanceProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  auth.role = "admin";
  stored = { ...DEFAULT_APPEARANCE };
  failSave = false;
  failLoad = false;
  fetchMock.mockReset();
  fetchMock.mockImplementation(async (_input, init) => {
    if (init?.method === "PUT") {
      if (failSave) return new Response("Save failed", { status: 500 });
      stored = JSON.parse(String(init.body));
    } else if (failLoad) {
      return new Response("Load failed", { status: 500 });
    }
    return Response.json(stored);
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => vi.unstubAllGlobals());

describe("global appearance settings", () => {
  it("uses the instance theme instead of an old browser-local override", async () => {
    localStorage.setItem("loom.theme", "amoled");
    stored.theme = "light";
    renderAppearance();
    await screen.findByRole("button", { name: "Save appearance" });
    expect(document.documentElement.dataset.theme).toBe("light");
  });

  it("saves theme, accent, image and dimming globally and restores on remount", async () => {
    const user = userEvent.setup();
    const app = renderAppearance();
    await screen.findByRole("button", { name: "Dark" });
    await user.click(screen.getByRole("button", { name: "Dark" }));
    await user.click(screen.getByRole("button", { name: "Blue" }));
    await user.type(
      screen.getByLabelText("Background image URL"),
      "https://example.com/image.jpg",
    );
    fireEvent.change(screen.getByLabelText(/background dimming/i), {
      target: { value: "82" },
    });
    expect(document.documentElement.dataset.theme).toBe("light");
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    await waitFor(() => {
      expect(document.documentElement.dataset.theme).toBe("dark");
      expect(document.documentElement.dataset.accent).toBe("blue");
    });
    expect(stored).toEqual({
      theme: "dark",
      accent: "blue",
      backgroundUrl: "https://example.com/image.jpg",
      backgroundDim: 82,
    });
    expect(
      document.documentElement.style.getPropertyValue(
        "--loom-background-image",
      ),
    ).toBe('url("https://example.com/image.jpg")');
    expect(localStorage.getItem("loom.theme")).toBeNull();
    app.unmount();
    renderAppearance();
    await screen.findByDisplayValue("https://example.com/image.jpg");
    expect(screen.getByRole("button", { name: "Dark" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("uses the same global theme from the header toggle", async () => {
    const user = userEvent.setup();
    renderAppearance();
    await screen.findByRole("button", { name: "Save appearance" });
    await user.click(screen.getByRole("button", { name: /toggle theme/i }));
    await user.click(screen.getByRole("menuitem", { name: "AMOLED" }));
    await waitFor(() => expect(stored.theme).toBe("amoled"));
    expect(
      await screen.findByRole("button", { name: "AMOLED" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(document.documentElement.dataset.theme).toBe("amoled");
  });

  it("does not allow non-admins to change global appearance", async () => {
    auth.role = "viewer";
    renderAppearance();
    await screen.findByText(/only admins can change these settings/i);
    expect(screen.getByLabelText("Background image URL")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Dark" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /toggle theme/i }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Save appearance" }),
    ).not.toBeInTheDocument();
  });

  it("rejects unsafe URLs and explains image loading failures", async () => {
    const user = userEvent.setup();
    renderAppearance();
    const input = await screen.findByLabelText("Background image URL");
    await user.type(input, "javascript:alert(1)");
    expect(screen.getByRole("alert")).toHaveTextContent(/http or https/i);
    expect(
      screen.getByRole("button", { name: "Save appearance" }),
    ).toBeDisabled();
    await user.clear(input);
    await user.type(input, "https://example.com/missing.jpg");
    const image = document.querySelector("img");
    expect(image).not.toBeNull();
    fireEvent.error(image!);
    expect(screen.getByRole("alert")).toHaveTextContent(/could not be loaded/i);
  });

  it("restores all defaults, removing the background and accent overrides", async () => {
    stored = {
      theme: "amoled",
      accent: "rose",
      backgroundUrl: "https://example.com/a.jpg",
      backgroundDim: 90,
    };
    const user = userEvent.setup();
    renderAppearance();
    await screen.findByDisplayValue(stored.backgroundUrl);
    await user.click(screen.getByRole("button", { name: "Reset to defaults" }));
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    await waitFor(() => expect(stored).toEqual(DEFAULT_APPEARANCE));
    expect(
      document.documentElement.style.getPropertyValue(
        "--loom-background-image",
      ),
    ).toBe("none");
    expect(
      document.documentElement.style.getPropertyValue("--personal-accent"),
    ).toBe("");
  });

  it("does not apply settings when saving fails", async () => {
    failSave = true;
    const user = userEvent.setup();
    renderAppearance();
    await screen.findByRole("button", { name: "Dark" });
    await user.click(screen.getByRole("button", { name: "Dark" }));
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /failed to save/i,
    );
    expect(stored).toEqual(DEFAULT_APPEARANCE);
    expect(document.documentElement.dataset.theme).toBe("light");
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Save appearance" }),
      ).toBeEnabled(),
    );
  });

  it("shows a retryable load error instead of editable defaults", async () => {
    failLoad = true;
    renderAppearance();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /could not load/i,
    );
    expect(
      screen.queryByRole("button", { name: "Save appearance" }),
    ).not.toBeInTheDocument();
    failLoad = false;
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(
      await screen.findByRole("button", { name: "Save appearance" }),
    ).toBeInTheDocument();
  });
});

describe("background URL validation", () => {
  it.each(["", "https://example.com/a.jpg", "http://localhost:1925/image.png"])(
    "accepts %s",
    (url) => expect(backgroundUrlError(url)).toBeUndefined(),
  );
  it.each([
    "/local.jpg",
    "file:///image.png",
    "data:image/png;base64,abc",
    "javascript:alert(1)",
    "https://user:pass@example.com/image.jpg",
    `https://example.com/${"a".repeat(2048)}`,
  ])("rejects invalid URLs", (url) =>
    expect(backgroundUrlError(url)).toBeDefined(),
  );
});
