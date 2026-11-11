import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useAuth } from "@/hooks/use-auth";
import { ThemeProvider, useTheme } from "@/hooks/use-theme";
import { paletteVariables } from "@/lib/palette";
import {
  ACCENTS,
  DEFAULT_APPEARANCE,
  fetchAppearance,
  saveAppearance,
  type Appearance,
} from "@/lib/appearance-api";

const appearanceKey = ["appearance"] as const;

export function useAppearance() {
  const { isAuthenticated } = useAuth();
  return useQuery({
    queryKey: appearanceKey,
    queryFn: fetchAppearance,
    enabled: isAuthenticated,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
  });
}

export function useSaveAppearance() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: saveAppearance,
    onSuccess: (value) => client.setQueryData(appearanceKey, value),
  });
}

export function AppearanceProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const { user, isAuthenticated } = useAuth();
  const { data, error } = useAppearance();
  const { mutate, isPending } = useSaveAppearance();
  const appearance = isAuthenticated
    ? (data ?? DEFAULT_APPEARANCE)
    : DEFAULT_APPEARANCE;

  React.useEffect(() => {
    if (error) toast.error(error.message);
  }, [error]);

  const changeTheme = (theme: Appearance["theme"]) => {
    mutate(
      { ...appearance, theme },
      { onError: (err) => toast.error(err.message) },
    );
  };

  return (
    <ThemeProvider
      theme={appearance.theme}
      onThemeChange={changeTheme}
      canChangeTheme={user?.role === "admin" && !!data && !isPending}
    >
      <AppearanceStyles appearance={appearance} />
      {children}
    </ThemeProvider>
  );
}

function AppearanceStyles({ appearance }: { appearance: Appearance }) {
  const { resolvedTheme } = useTheme();
  React.useEffect(() => {
    const root = document.documentElement;
    const accent =
      appearance.accent === "default" ? undefined : ACCENTS[appearance.accent];
    root.dataset.accent = appearance.accent;
    if (accent) {
      root.style.setProperty("--personal-accent", accent.color);
      root.style.setProperty("--personal-accent-dark", accent.darkColor);
    } else {
      root.style.removeProperty("--personal-accent");
      root.style.removeProperty("--personal-accent-dark");
    }
    root.style.setProperty(
      "--loom-background-image",
      appearance.backgroundUrl
        ? `url(${JSON.stringify(appearance.backgroundUrl)})`
        : "none",
    );
    root.style.setProperty(
      "--loom-background-dim",
      `${appearance.backgroundDim}%`,
    );
    const variables = appearance.palette
      ? paletteVariables(appearance.palette, resolvedTheme)
      : {};
    for (const [key, value] of Object.entries(variables))
      root.style.setProperty(key, value);
    if (appearance.palette) root.dataset.palette = "custom";
    else delete root.dataset.palette;
    return () => {
      for (const key of Object.keys(variables)) root.style.removeProperty(key);
      delete root.dataset.accent;
      delete root.dataset.palette;
      root.style.removeProperty("--personal-accent");
      root.style.removeProperty("--personal-accent-dark");
      root.style.removeProperty("--loom-background-image");
      root.style.removeProperty("--loom-background-dim");
    };
  }, [appearance, resolvedTheme]);
  return null;
}
