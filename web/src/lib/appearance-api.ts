import { apiFetch } from "@/lib/fetch";
import type { Theme } from "@/hooks/use-theme";

export const ACCENTS = {
  default: { label: "Loom", color: "173 100% 39%", darkColor: "173 100% 39%" },
  teal: { label: "Teal", color: "173 100% 26%", darkColor: "173 80% 55%" },
  blue: { label: "Blue", color: "213 85% 45%", darkColor: "213 90% 70%" },
  rose: { label: "Rose", color: "340 70% 43%", darkColor: "340 85% 70%" },
  amber: { label: "Amber", color: "38 92% 32%", darkColor: "38 92% 65%" },
} as const;

export interface Appearance {
  theme: Theme;
  accent: keyof typeof ACCENTS;
  backgroundUrl: string;
  backgroundDim: number;
}

export const DEFAULT_APPEARANCE: Appearance = {
  theme: "system",
  accent: "default",
  backgroundUrl: "",
  backgroundDim: 70,
};

export function backgroundUrlError(value: string): string | undefined {
  if (!value) return;
  if (value.length > 2048)
    return "Use an image URL of 2048 characters or fewer.";
  try {
    const url = new URL(value);
    if (
      (url.protocol === "https:" || url.protocol === "http:") &&
      !url.username &&
      !url.password
    ) {
      return;
    }
  } catch {
    // Report invalid URLs inline before sending them to the server.
  }
  return "Enter a full HTTP or HTTPS image URL without a username or password.";
}

export async function fetchAppearance(): Promise<Appearance> {
  const res = await apiFetch("/api/v1/appearance");
  if (!res.ok) throw new Error("Failed to load appearance settings.");
  return res.json();
}

export async function saveAppearance(value: Appearance): Promise<Appearance> {
  const res = await apiFetch("/api/v1/appearance", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(value),
  });
  if (!res.ok) {
    throw new Error(
      res.status === 403
        ? "Only admins can change the appearance for this Loom instance."
        : "Failed to save appearance settings. Try again.",
    );
  }
  return res.json();
}
