import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppearanceProvider } from "@/hooks/use-appearance";
import { AppearanceSettings } from "@/components/appearance-settings";
import { ThemeToggle } from "@/components/theme-toggle";
import { PALETTES, hsl, paletteVariables } from "@/lib/palette";
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
    if (init?.method === "POST") {
      return Response.json({
        url: `/api/v1/appearance/background?v=${"a".repeat(64)}`,
      });
    }
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
    await user.click(screen.getByRole("button", { name: "Ocean" }));
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
      expect(document.documentElement.style.getPropertyValue("--primary")).toBe(
        hsl(PALETTES.Ocean.primary),
      );
    });
    expect(stored).toEqual({
      theme: "dark",
      accent: "default",
      palette: PALETTES.Ocean,
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
    await userEvent.click(screen.getByText("Customize individual colors"));
    expect(screen.getByLabelText("Background image URL")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Dark" })).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /toggle theme/i }),
    ).toBeDisabled();
    expect(screen.getByLabelText("Primary hex")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Upload image" })).toBeDisabled();
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
      palette: PALETTES.Ember,
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
    expect(document.documentElement.style.getPropertyValue("--primary")).toBe(
      "",
    );
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

  it("previews custom color pickers and hex values without applying until Save", async () => {
    const user = userEvent.setup();
    renderAppearance();
    await user.click(await screen.findByText("Customize individual colors"));
    const primary = await screen.findByLabelText("Primary hex");
    await user.clear(primary);
    await user.type(primary, "#e65d29");
    fireEvent.change(screen.getByLabelText("Secondary color"), {
      target: { value: "#663399" },
    });
    fireEvent.change(screen.getByLabelText("Accent color"), {
      target: { value: "#ffee00" },
    });
    fireEvent.change(screen.getByLabelText("Surface tint color"), {
      target: { value: "#39271f" },
    });
    expect(
      screen
        .getByRole("region", { name: "Palette preview" })
        .style.getPropertyValue("--primary"),
    ).toBe(hsl("#e65d29"));
    expect(document.documentElement.style.getPropertyValue("--primary")).toBe(
      "",
    );
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    await waitFor(() => expect(stored.palette?.primary).toBe("#e65d29"));
    expect(stored.palette).toEqual({
      primary: "#e65d29",
      secondary: "#663399",
      accent: "#ffee00",
      background: "#39271f",
    });
    expect(document.documentElement.style.getPropertyValue("--primary")).toBe(
      hsl("#e65d29"),
    );
  });

  it("blocks saving invalid colors after blur until every field is corrected", async () => {
    const user = userEvent.setup();
    renderAppearance();
    await user.click(await screen.findByText("Customize individual colors"));
    await user.click(screen.getByRole("button", { name: "Dark" }));
    const primary = screen.getByLabelText("Primary hex");
    const secondary = screen.getByLabelText("Secondary hex");
    await user.clear(primary);
    await user.type(primary, "#zzzzzz");
    await user.clear(secondary);
    await user.tab();
    expect(primary).toHaveValue("#zzzzzz");
    expect(primary).toHaveAttribute("aria-invalid", "true");
    expect(secondary).toHaveValue("");
    const save = screen.getByRole("button", { name: "Save appearance" });
    expect(save).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent(
      /correct the invalid color/i,
    );
    fireEvent.submit(save.closest("form")!);
    expect(
      fetchMock.mock.calls.some(([, init]) => init?.method === "PUT"),
    ).toBe(false);
    await user.clear(primary);
    await user.type(primary, "#e65d29");
    expect(save).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Secondary color"), {
      target: { value: "#663399" },
    });
    expect(secondary).toHaveValue("#663399");
    expect(save).toBeEnabled();
    await user.click(save);
    await waitFor(() => expect(stored.palette?.primary).toBe("#e65d29"));
    expect(stored.palette?.secondary).toBe("#663399");
    expect(stored.theme).toBe("dark");
  });

  it.each(["Ocean", "Reset to defaults", "Discard changes"])(
    "clears invalid color drafts when choosing %s",
    async (action) => {
      const user = userEvent.setup();
      renderAppearance();
      await user.click(await screen.findByText("Customize individual colors"));
      await user.clear(screen.getByLabelText("Primary hex"));
      expect(
        screen.getByRole("button", { name: "Save appearance" }),
      ).toBeDisabled();
      await user.click(screen.getByRole("button", { name: action }));
      expect(screen.getByLabelText("Primary hex")).toHaveValue(
        action === "Ocean" ? PALETTES.Ocean.primary : PALETTES.Loom.primary,
      );
      expect(screen.getByLabelText("Primary hex")).toHaveAttribute(
        "aria-invalid",
        "false",
      );
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      const save = screen.getByRole("button", { name: "Save appearance" });
      if (action === "Ocean") expect(save).toBeEnabled();
      else expect(save).toBeDisabled();
    },
  );

  it("uploads an image and saves the returned Loom URL", async () => {
    const user = userEvent.setup();
    renderAppearance();
    await user.click(
      await screen.findByRole("button", { name: "Upload image" }),
    );
    const file = new File(["png"], "background.png", { type: "image/png" });
    await user.upload(screen.getByLabelText("Choose background image"), file);
    const imageUrl = `/api/v1/appearance/background?v=${"a".repeat(64)}`;
    await waitFor(() =>
      expect(document.querySelector("img")).toHaveAttribute("src", imageUrl),
    );
    const request = fetchMock.mock.calls.find(
      ([, init]) => init?.method === "POST",
    );
    expect(request?.[1]?.body).toBeInstanceOf(FormData);
    expect((request?.[1]?.body as FormData).get("image")).toBeInstanceOf(File);
    expect(stored.backgroundUrl).toBe("");
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    await waitFor(() => expect(stored.backgroundUrl).toBe(imageUrl));
    await user.click(screen.getByRole("button", { name: "Remove background" }));
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    await waitFor(() => expect(stored.backgroundUrl).toBe(""));
  });

  it("keeps detailed color controls optional and discards a draft without saving", async () => {
    const user = userEvent.setup();
    renderAppearance();
    const customize = await screen.findByText("Customize individual colors");
    expect(customize.closest("details")).not.toHaveAttribute("open");
    await user.click(screen.getByRole("button", { name: "Ocean" }));
    expect(screen.getByRole("status")).toHaveTextContent(/unsaved changes/i);
    await user.click(customize);
    expect(screen.getByLabelText("Primary hex")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Dark" }));
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    expect(screen.getByRole("button", { name: "System" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Loom" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(
      screen.getByRole("button", { name: "Save appearance" }),
    ).toBeDisabled();
    expect(
      fetchMock.mock.calls.some(([, init]) => init?.method === "PUT"),
    ).toBe(false);
    expect(stored).toEqual(DEFAULT_APPEARANCE);
  });

  it("previews the device theme when switching from a saved Dark theme to System", async () => {
    stored = { ...DEFAULT_APPEARANCE, theme: "dark", palette: PALETTES.Ocean };
    const user = userEvent.setup();
    renderAppearance();
    await user.click(await screen.findByRole("button", { name: "System" }));
    expect(
      screen
        .getByRole("region", { name: "Palette preview" })
        .style.getPropertyValue("--background"),
    ).toBe(paletteVariables(PALETTES.Ocean, "light")["--background"]);
    expect(document.documentElement.dataset.theme).toBe("dark");
  });

  it("explains when a pending uploaded background must be uploaded again", async () => {
    const user = userEvent.setup();
    renderAppearance();
    await user.click(await screen.findByRole("button", { name: "Ocean" }));
    fetchMock.mockResolvedValueOnce(
      Response.json(
        {
          error: "Uploaded background is no longer available; upload it again.",
        },
        { status: 400 },
      ),
    );
    await user.click(screen.getByRole("button", { name: "Save appearance" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /upload it again/i,
    );
    expect(stored).toEqual(DEFAULT_APPEARANCE);
  });

  it("reports failed uploads without changing the saved background", async () => {
    const user = userEvent.setup();
    renderAppearance();
    await user.click(
      await screen.findByRole("button", { name: "Upload image" }),
    );
    fetchMock.mockResolvedValueOnce(
      Response.json(
        { error: "Image dimensions exceed the supported limit." },
        { status: 400 },
      ),
    );
    await user.upload(
      screen.getByLabelText("Choose background image"),
      new File(["png"], "bad.png", { type: "image/png" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /image dimensions exceed/i,
    );
    expect(stored.backgroundUrl).toBe("");
    expect(
      screen.getByRole("button", { name: "Save appearance" }),
    ).toBeDisabled();
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
