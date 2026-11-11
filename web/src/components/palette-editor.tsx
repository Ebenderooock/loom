import * as React from "react";
import type { Theme } from "@/hooks/use-theme";
import {
  PALETTES,
  paletteVariables,
  validHex,
  type Palette,
} from "@/lib/palette";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoomLogo } from "@/components/loom-logo";
import { Check } from "lucide-react";

const COLORS: Array<{
  key: keyof Palette;
  label: string;
  description: string;
}> = [
  {
    key: "primary",
    label: "Primary",
    description: "Main buttons and brand highlights",
  },
  {
    key: "secondary",
    label: "Secondary",
    description: "Supporting buttons and details",
  },
  {
    key: "accent",
    label: "Accent",
    description: "Navigation, focus rings, and selections",
  },
  {
    key: "background",
    label: "Surface tint",
    description: "Page, sidebar, cards, and dialogs",
  },
];

function ColorControl({
  value,
  hex,
  label,
  description,
  onChange,
}: {
  value: string;
  hex: string;
  label: string;
  description: string;
  onChange: (value: string) => void;
}) {
  const id = React.useId();
  const invalid = !validHex(hex);
  return (
    <div className="space-y-2">
      <Label htmlFor={`${id}-picker`}>{label}</Label>
      <div className="flex items-center gap-2">
        <input
          id={`${id}-picker`}
          type="color"
          value={value}
          aria-label={`${label} color`}
          className="h-11 w-12 shrink-0 cursor-pointer rounded-md border border-input bg-card p-1"
          onChange={(event) => onChange(event.target.value)}
        />
        <Input
          aria-label={`${label} hex`}
          value={hex}
          maxLength={7}
          spellCheck={false}
          aria-invalid={invalid}
          aria-describedby={`${id}-help`}
          className="h-11 min-w-0 font-mono text-base"
          onChange={(event) => onChange(event.target.value)}
        />
      </div>
      <p id={`${id}-help`} className="text-xs text-muted-foreground">
        {invalid
          ? "Enter a six-digit hex color, such as #1879c9."
          : description}
      </p>
    </div>
  );
}

export function PaletteEditor({
  palette,
  theme,
  disabled,
  colorInputs,
  onColorInputChange,
  onChange,
}: {
  palette: Palette;
  theme: Theme;
  disabled: boolean;
  colorInputs: Partial<Palette>;
  onColorInputChange: (key: keyof Palette, value: string) => void;
  onChange: (palette: Palette) => void;
}) {
  const [systemDark, setSystemDark] = React.useState(
    () => window.matchMedia("(prefers-color-scheme: dark)").matches,
  );
  React.useEffect(() => {
    const query = window.matchMedia("(prefers-color-scheme: dark)");
    const update = () => setSystemDark(query.matches);
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);
  const previewTheme =
    theme === "system" ? (systemDark ? "dark" : "light") : theme;
  const variables = {
    ...paletteVariables(palette, previewTheme),
    "--loom-logo-primary": "initial",
    "--loom-logo-accent": "initial",
  } as React.CSSProperties;
  const selectedPreset = Object.entries(PALETTES).find(([, colors]) =>
    COLORS.every(
      ({ key }) => palette[key].toLowerCase() === colors[key].toLowerCase(),
    ),
  )?.[0];
  return (
    <fieldset disabled={disabled} className="min-w-0 space-y-4">
      <legend className="mb-2 text-base font-semibold">
        2. Choose your colors
      </legend>
      <p className="text-sm text-muted-foreground">
        Pick a ready-made color scheme. You can fine-tune it below, or leave it
        as it is.
      </p>
      <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,8rem),1fr))] gap-3">
        {Object.entries(PALETTES).map(([name, colors]) => (
          <Button
            type="button"
            key={name}
            variant="outline"
            className={
              selectedPreset === name
                ? "h-auto min-h-12 flex-wrap whitespace-normal border-accent px-3 py-3 ring-1 ring-accent"
                : "h-auto min-h-12 flex-wrap whitespace-normal px-3 py-3"
            }
            aria-pressed={selectedPreset === name}
            onClick={() => onChange({ ...colors })}
          >
            <span className="flex shrink-0 -space-x-1" aria-hidden="true">
              {[colors.primary, colors.secondary, colors.accent].map(
                (color, index) => (
                  <span
                    key={index}
                    className="h-3 w-3 rounded-full border border-card"
                    style={{ backgroundColor: color }}
                  />
                ),
              )}
            </span>
            {name}
            {selectedPreset === name && (
              <Check className="h-4 w-4 shrink-0" aria-hidden="true" />
            )}
          </Button>
        ))}
      </div>
      <p className="text-sm text-muted-foreground">
        Selected:{" "}
        <span className="font-medium text-foreground">
          {selectedPreset ?? "Custom colors"}
        </span>
      </p>
      <details className="rounded-lg border border-border">
        <summary className="cursor-pointer rounded-lg px-4 py-3 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
          Customize individual colors
        </summary>
        <div className="space-y-4 border-t border-border p-4">
          <p className="text-sm text-muted-foreground">
            Tap a color swatch to choose a color. Hex codes are optional.
            Surface tint changes the color of pages and cards; text adjusts to
            stay readable.
          </p>
          <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,14rem),1fr))] gap-4">
            {COLORS.map(({ key, label, description }) => (
              <ColorControl
                key={key}
                value={palette[key]}
                hex={colorInputs[key] ?? palette[key]}
                label={label}
                description={description}
                onChange={(value) => onColorInputChange(key, value)}
              />
            ))}
          </div>
        </div>
      </details>
      <div
        role="region"
        aria-label="Palette preview"
        style={variables}
        className="overflow-hidden rounded-lg border border-border bg-background text-foreground"
      >
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-card px-4 py-3">
          <span className="flex items-center gap-2 font-semibold text-primary-readable">
            <LoomLogo className="h-6 w-6" />
            Loom
          </span>
          <span className="text-xs text-muted-foreground">Preview only</span>
        </div>
        <div className="flex">
          <div className="hidden w-28 shrink-0 border-r border-border bg-card p-3 lg:block">
            <span className="block rounded bg-accent/15 px-2 py-1 text-xs text-accent">
              Library
            </span>
            <span className="mt-2 block px-2 text-xs text-muted-foreground">
              Settings
            </span>
          </div>
          <div className="min-w-0 flex-1 p-4">
            <div className="rounded-md border border-border bg-card p-4 text-card-foreground">
              <p className="font-medium">Your media, your colors</p>
              <p className="mb-3 mt-1 text-xs text-muted-foreground">
                Cards, navigation, buttons, and dialogs use this palette.
              </p>
              <div className="flex flex-wrap gap-2">
                <span className="rounded-md bg-primary px-3 py-2 text-xs text-primary-foreground">
                  Primary action
                </span>
                <span className="rounded-md bg-secondary px-3 py-2 text-xs text-secondary-foreground">
                  Secondary action
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </fieldset>
  );
}
