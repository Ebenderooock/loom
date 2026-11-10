import * as React from "react";

export type Theme = "light" | "dark" | "amoled" | "system";
export type ResolvedTheme = "light" | "dark" | "amoled";

interface ThemeContextValue {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (t: Theme) => void;
  canChangeTheme: boolean;
}

const ThemeContext = React.createContext<ThemeContextValue | null>(null);

const STORAGE_KEY = "loom.theme";

function readSystem(): ResolvedTheme {
  if (typeof window === "undefined") return "light";
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function applyTheme(t: ResolvedTheme) {
  const root = document.documentElement;
  root.dataset.theme = t;
  root.style.colorScheme = t === "light" ? "light" : "dark";
}

export function ThemeProvider({
  children,
  theme: controlledTheme,
  onThemeChange,
  canChangeTheme = true,
}: {
  children: React.ReactNode;
  theme?: Theme;
  onThemeChange?: (theme: Theme) => void;
  canChangeTheme?: boolean;
}) {
  const [theme, setThemeState] = React.useState<Theme>(() => {
    if (typeof window === "undefined") return "system";
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (
      stored === "light" ||
      stored === "dark" ||
      stored === "amoled" ||
      stored === "system"
    ) {
      return stored;
    }
    return "system";
  });

  const [systemTheme, setSystemTheme] = React.useState<ResolvedTheme>(() =>
    readSystem(),
  );

  React.useEffect(() => {
    const mql = window.matchMedia("(prefers-color-scheme: dark)");
    const handler = () => setSystemTheme(mql.matches ? "dark" : "light");
    mql.addEventListener("change", handler);
    return () => mql.removeEventListener("change", handler);
  }, []);

  const activeTheme = controlledTheme ?? theme;
  const resolvedTheme: ResolvedTheme =
    activeTheme === "system" ? systemTheme : activeTheme;

  React.useEffect(() => {
    applyTheme(resolvedTheme);
  }, [resolvedTheme]);

  const setTheme = React.useCallback(
    (t: Theme) => {
      if (!canChangeTheme) return;
      if (controlledTheme !== undefined) {
        onThemeChange?.(t);
        return;
      }
      setThemeState(t);
      window.localStorage.setItem(STORAGE_KEY, t);
    },
    [controlledTheme, onThemeChange, canChangeTheme],
  );

  const value = React.useMemo(
    () => ({
      theme: activeTheme,
      resolvedTheme,
      setTheme,
      canChangeTheme,
    }),
    [activeTheme, resolvedTheme, setTheme, canChangeTheme],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = React.useContext(ThemeContext);
  if (!ctx) {
    throw new Error("useTheme must be used inside <ThemeProvider>");
  }
  return ctx;
}
