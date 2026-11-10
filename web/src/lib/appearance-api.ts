import { apiFetch } from "@/lib/fetch";
import type { Theme } from "@/hooks/use-theme";
import type { Palette } from "@/lib/palette";

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
  palette?: Palette | null;
}

export const DEFAULT_APPEARANCE: Appearance = {
  theme: "system",
  accent: "default",
  backgroundUrl: "",
  backgroundDim: 70,
};

export function backgroundUrlError(value: string): string | undefined {
  if (!value) return;
  if (/^\/api\/v1\/appearance\/background(?:\?v=[a-f0-9]{64})?$/.test(value))
    return;
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

async function appearanceError(
  response: Response,
  fallback: string,
): Promise<Error> {
  if (response.headers.get("content-type")?.includes("application/json")) {
    const body: unknown = await response.json();
    if (
      body &&
      typeof body === "object" &&
      "error" in body &&
      typeof body.error === "string"
    ) {
      return new Error(body.error);
    }
  }
  return new Error(fallback);
}

export async function uploadBackground(file: File): Promise<string> {
  if (file.size > 5 * 1024 * 1024)
    throw new Error("Choose an image smaller than 5 MiB.");
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
    throw new Error("Choose a PNG, JPEG, or WebP image.");
  }
  const form = new FormData();
  form.append("image", file);
  const response = await apiFetch("/api/v1/appearance/background", {
    method: "POST",
    body: form,
  });
  if (!response.ok) {
    throw await appearanceError(
      response,
      response.status === 413
        ? "Choose an image smaller than 5 MiB."
        : "Image upload failed. Check the image format and try again.",
    );
  }
  const body: { url: string } = await response.json();
  return body.url;
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
    throw await appearanceError(
      res,
      res.status === 403
        ? "Only admins can change the appearance for this Loom instance."
        : "Failed to save appearance settings. Try again.",
    );
  }
  return res.json();
}
