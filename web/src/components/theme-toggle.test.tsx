import { describe, expect, it, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider } from "@/hooks/use-theme";
import { ThemeToggle } from "@/components/theme-toggle";

function renderWithTheme(ui: React.ReactNode) {
  return render(<ThemeProvider>{ui}</ThemeProvider>);
}

describe("ThemeToggle", () => {
  it("renders an accessible toggle button", () => {
    renderWithTheme(<ThemeToggle />);
    expect(
      screen.getByRole("button", { name: /toggle theme/i }),
    ).toBeInTheDocument();
  });

  it("opens the menu and switches the theme on selection", async () => {
    const user = userEvent.setup();
    renderWithTheme(<ThemeToggle />);
    await user.click(screen.getByRole("button", { name: /toggle theme/i }));
    const dark = await screen.findByRole("menuitem", { name: /dark/i });
    await user.click(dark);
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(window.localStorage.getItem("loom.theme")).toBe("dark");
  });

  it("follows device changes when the global theme is System", () => {
    let onChange: (() => void) | undefined;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const query = {
      ...media,
      matches: false,
      addEventListener: vi.fn((_event: string, handler: () => void) => {
        onChange = handler;
      }),
      removeEventListener: vi.fn(),
    };
    const spy = vi.spyOn(window, "matchMedia").mockReturnValue(query);
    const app = render(
      <ThemeProvider theme="system">
        <ThemeToggle />
      </ThemeProvider>,
    );
    expect(document.documentElement.dataset.theme).toBe("light");
    act(() => {
      query.matches = true;
      onChange?.();
    });
    expect(document.documentElement.dataset.theme).toBe("dark");
    app.unmount();
    expect(query.removeEventListener).toHaveBeenCalledWith("change", onChange);
    spy.mockRestore();
  });
});
