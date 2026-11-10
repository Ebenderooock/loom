import type { ResolvedTheme } from "@/hooks/use-theme";

export interface Palette {
  primary: string;
  secondary: string;
  accent: string;
  background: string;
}

export const PALETTES = {
  Loom: {
    primary: "#7542ad",
    secondary: "#9467c4",
    accent: "#00bfa6",
    background: "#28283b",
  },
  Ocean: {
    primary: "#1879c9",
    secondary: "#607ee0",
    accent: "#20b9c9",
    background: "#172d3d",
  },
  Ember: {
    primary: "#c75d28",
    secondary: "#b95978",
    accent: "#e5aa35",
    background: "#39271f",
  },
  Forest: {
    primary: "#348360",
    secondary: "#788651",
    accent: "#bb9c39",
    background: "#23352b",
  },
} satisfies Record<string, Palette>;

export function validHex(value: string): boolean {
  return /^#[0-9a-f]{6}$/i.test(value);
}

function rgb(hex: string): [number, number, number] {
  return [
    parseInt(hex.slice(1, 3), 16),
    parseInt(hex.slice(3, 5), 16),
    parseInt(hex.slice(5, 7), 16),
  ];
}

function hex(channels: number[]): string {
  return `#${channels.map((value) => Math.round(value).toString(16).padStart(2, "0")).join("")}`;
}

function mix(color: string, target: string, amount: number): string {
  const [r, g, b] = rgb(color);
  const [tr, tg, tb] = rgb(target);
  return hex([
    r + (tr - r) * amount,
    g + (tg - g) * amount,
    b + (tb - b) * amount,
  ]);
}

function luminance(color: string): number {
  const linear = (value: number) => {
    const channel = value / 255;
    return channel <= 0.04045
      ? channel / 12.92
      : ((channel + 0.055) / 1.055) ** 2.4;
  };
  const [r, g, b] = rgb(color);
  return linear(r) * 0.2126 + linear(g) * 0.7152 + linear(b) * 0.0722;
}

export function contrast(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}

function foreground(background: string): string {
  return contrast("#ffffff", background) >= contrast("#000000", background)
    ? "#ffffff"
    : "#000000";
}

function readable(color: string, background: string): string {
  const target = foreground(background);
  for (let step = 0; step <= 20; step++) {
    const candidate = mix(color, target, step / 20);
    if (contrast(candidate, background) >= 4.5) return candidate;
  }
  return target;
}

export function hsl(color: string): string {
  const [red, green, blue] = rgb(color);
  const r = red / 255;
  const g = green / 255;
  const b = blue / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const delta = max - min;
  const lightness = (max + min) / 2;
  let hue = 0;
  if (delta) {
    if (max === r) hue = ((g - b) / delta) % 6;
    else if (max === g) hue = (b - r) / delta + 2;
    else hue = (r - g) / delta + 4;
  }
  const saturation = delta ? delta / (1 - Math.abs(2 * lightness - 1)) : 0;
  return `${((hue * 60 + 360) % 360).toFixed(2)} ${(saturation * 100).toFixed(2)}% ${(lightness * 100).toFixed(2)}%`;
}

export function paletteColors(
  palette: Palette,
  theme: ResolvedTheme,
): Record<string, string> {
  const light = theme === "light";
  const background =
    theme === "amoled"
      ? "#000000"
      : mix(
          palette.background,
          light ? "#ffffff" : "#000000",
          light ? 0.94 : 0.45,
        );
  const card = mix(
    background,
    light ? "#ffffff" : palette.background,
    light ? 0.65 : 0.3,
  );
  const text = readable(light ? "#202020" : "#eeeeee", background);
  const muted = mix(background, text, light ? 0.035 : 0.07);
  return {
    background,
    foreground: text,
    card,
    "card-foreground": readable(text, card),
    popover: card,
    "popover-foreground": readable(text, card),
    primary: palette.primary,
    "primary-readable": readable(palette.primary, card),
    "primary-foreground": foreground(palette.primary),
    secondary: palette.secondary,
    "secondary-readable": readable(palette.secondary, card),
    "secondary-foreground": foreground(palette.secondary),
    accent: readable(palette.accent, card),
    "accent-foreground": foreground(readable(palette.accent, card)),
    muted,
    "muted-foreground": readable(mix(background, text, 0.55), card),
    border: mix(background, text, light ? 0.18 : 0.22),
    input: mix(background, text, light ? 0.18 : 0.22),
    ring: readable(palette.accent, card),
  };
}

export function paletteVariables(
  palette: Palette,
  theme: ResolvedTheme,
): Record<string, string> {
  return Object.fromEntries(
    Object.entries(paletteColors(palette, theme)).map(([key, color]) => [
      `--${key}`,
      hsl(color),
    ]),
  );
}
