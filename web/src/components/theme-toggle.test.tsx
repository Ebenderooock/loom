import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
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
});
