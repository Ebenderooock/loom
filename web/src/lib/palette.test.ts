import { describe, expect, it } from "vitest";
import {
  PALETTES,
  contrast,
  paletteColors,
  validHex,
  type Palette,
} from "./palette";

describe("coordinated palettes", () => {
  const palettes: Palette[] = [
    ...Object.values(PALETTES),
    {
      primary: "#ffffff",
      secondary: "#000000",
      accent: "#eeeeee",
      background: "#ffffff",
    },
    {
      primary: "#777777",
      secondary: "#ffff00",
      accent: "#000000",
      background: "#000000",
    },
  ];
  for (const theme of ["light", "dark", "amoled"] as const) {
    it(`keeps text and selections readable in ${theme} mode, including extreme colors`, () => {
      for (const palette of palettes) {
        const colors = paletteColors(palette, theme);
        for (const [front, back] of [
          ["foreground", "background"],
          ["card-foreground", "card"],
          ["popover-foreground", "popover"],
          ["primary-foreground", "primary"],
          ["primary-readable", "card"],
          ["secondary-foreground", "secondary"],
          ["secondary-readable", "card"],
          ["accent", "card"],
          ["accent-foreground", "accent"],
          ["muted-foreground", "card"],
        ]) {
          expect(
            contrast(colors[front!]!, colors[back!]!),
            `${front} on ${back}`,
          ).toBeGreaterThanOrEqual(4.5);
        }
        expect(colors.primary).toBe(palette.primary);
        expect(colors.secondary).toBe(palette.secondary);
        if (theme === "amoled") expect(colors.background).toBe("#000000");
      }
    });
  }
  it("changes surfaces, primary, secondary and navigation instead of just the accent", () => {
    const loom = paletteColors(PALETTES.Loom, "dark");
    const ember = paletteColors(PALETTES.Ember, "dark");
    for (const key of [
      "primary",
      "secondary",
      "accent",
      "background",
      "card",
      "border",
      "input",
    ]) {
      expect(ember[key]).not.toBe(loom[key]);
    }
  });
  it("accepts only full hex colors", () => {
    expect(validHex("#Aa12bF")).toBe(true);
    for (const invalid of [
      "red",
      "#abc",
      "#12345678",
      "var(--primary)",
      "url(x)",
      "",
    ]) {
      expect(validHex(invalid)).toBe(false);
    }
  });
});
