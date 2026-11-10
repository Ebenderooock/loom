import * as React from "react";
import { toast } from "sonner";
import { useAuth } from "@/hooks/use-auth";
import { useAppearance, useSaveAppearance } from "@/hooks/use-appearance";
import {
  ACCENTS,
  DEFAULT_APPEARANCE,
  backgroundUrlError,
  type Appearance,
} from "@/lib/appearance-api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const THEMES: Array<{ value: Appearance["theme"]; label: string }> = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "amoled", label: "AMOLED" },
];
const ACCENT_KEYS: Appearance["accent"][] = [
  "default",
  "teal",
  "blue",
  "rose",
  "amber",
];

export function AppearanceSettings() {
  const { data, isPending, error, refetch } = useAppearance();
  const { user } = useAuth();

  if (isPending) return <p role="status">Loading appearance settings...</p>;
  if (error || !data) {
    return (
      <div role="alert" className="space-y-3">
        <p>Could not load appearance settings.</p>
        <Button variant="outline" onClick={() => void refetch()}>
          Try again
        </Button>
      </div>
    );
  }

  return (
    <AppearanceForm
      key={JSON.stringify(data)}
      initial={data}
      canEdit={user?.role === "admin"}
    />
  );
}

function AppearanceForm({
  initial,
  canEdit,
}: {
  initial: Appearance;
  canEdit: boolean;
}) {
  const [draft, setDraft] = React.useState(initial);
  const [failedImage, setFailedImage] = React.useState<string>();
  const save = useSaveAppearance();
  const url = draft.backgroundUrl.trim();
  const urlError = backgroundUrlError(url);
  const imageFailed = !!url && failedImage === url;
  const dirty = JSON.stringify(draft) !== JSON.stringify(initial);
  const disabled = !canEdit || save.isPending;

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (urlError || disabled) return;
    save.mutate(
      { ...draft, backgroundUrl: url },
      {
        onSuccess: () => toast.success("Appearance saved for everyone."),
        onError: (error) => toast.error(error.message),
      },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Instance appearance</CardTitle>
          <p className="text-sm text-muted-foreground">
            Applies to everyone on this Loom instance, on every device.
            {!canEdit && " Only admins can change these settings."}
          </p>
        </CardHeader>
        <CardContent className="space-y-6">
          <fieldset disabled={disabled} className="space-y-2">
            <legend className="mb-2 text-sm font-medium">Theme</legend>
            <div className="flex flex-wrap gap-2">
              {THEMES.map(({ value, label }) => (
                <Button
                  type="button"
                  key={value}
                  variant={draft.theme === value ? "default" : "outline"}
                  aria-pressed={draft.theme === value}
                  onClick={() => setDraft({ ...draft, theme: value })}
                >
                  {label}
                </Button>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              System follows each device's light or dark preference.
            </p>
          </fieldset>

          <fieldset disabled={disabled} className="space-y-2">
            <legend className="mb-2 text-sm font-medium">Accent color</legend>
            <div className="flex flex-wrap gap-2">
              {ACCENT_KEYS.map((key) => (
                <Button
                  type="button"
                  key={key}
                  variant="outline"
                  aria-pressed={draft.accent === key}
                  className={draft.accent === key ? "ring-2 ring-ring" : ""}
                  onClick={() => setDraft({ ...draft, accent: key })}
                >
                  <span
                    className="h-3 w-3 rounded-full"
                    style={{ backgroundColor: `hsl(${ACCENTS[key].color})` }}
                    aria-hidden="true"
                  />
                  {ACCENTS[key].label}
                </Button>
              ))}
            </div>
          </fieldset>

          <div className="space-y-2">
            <Label htmlFor="appearance-background">Background image URL</Label>
            <Input
              id="appearance-background"
              type="url"
              maxLength={2048}
              placeholder="https://example.com/background.jpg"
              value={draft.backgroundUrl}
              disabled={disabled}
              aria-invalid={!!urlError}
              aria-describedby={
                urlError
                  ? "appearance-background-error appearance-background-help"
                  : "appearance-background-help"
              }
              onChange={(event) =>
                setDraft({ ...draft, backgroundUrl: event.target.value })
              }
            />
            <p
              id="appearance-background-help"
              className="text-xs text-muted-foreground"
            >
              Leave empty for a plain background. Use an image URL reachable
              from every device. Images load directly from that host; use HTTPS
              when Loom is served over HTTPS.
            </p>
            {urlError && (
              <p
                id="appearance-background-error"
                role="alert"
                className="text-sm text-destructive"
              >
                {urlError}
              </p>
            )}
          </div>

          {url && !urlError && (
            <div className="space-y-3">
              <div className="relative flex h-36 items-center justify-center overflow-hidden rounded-md border bg-background">
                <img
                  src={url}
                  alt=""
                  referrerPolicy="no-referrer"
                  className="absolute inset-0 h-full w-full object-cover"
                  onError={() => setFailedImage(url)}
                  onLoad={() => setFailedImage(undefined)}
                />
                <div
                  className="absolute inset-0 bg-background"
                  style={{ opacity: draft.backgroundDim / 100 }}
                />
                <span className="relative rounded-md border bg-card px-4 py-2 text-sm text-card-foreground shadow-sm">
                  Background preview
                </span>
              </div>
              {imageFailed && (
                <p role="alert" className="text-sm text-destructive">
                  This image could not be loaded. Check the URL and image host
                  permissions before saving.
                </p>
              )}
              <div className="space-y-2">
                <Label htmlFor="appearance-dim">
                  Background dimming: {draft.backgroundDim}%
                </Label>
                <input
                  id="appearance-dim"
                  type="range"
                  min={0}
                  max={100}
                  step={1}
                  value={draft.backgroundDim}
                  disabled={disabled}
                  className="block w-full accent-[hsl(var(--accent))]"
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      backgroundDim: Number(event.target.value),
                    })
                  }
                />
                <p className="text-xs text-muted-foreground">
                  Higher dimming keeps text readable. At 100%, the image is
                  hidden.
                </p>
              </div>
            </div>
          )}

          {save.error && (
            <p role="alert" className="text-sm text-destructive">
              {save.error.message}
            </p>
          )}
          {canEdit && (
            <div className="flex flex-wrap justify-between gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={save.isPending}
                onClick={() => setDraft({ ...DEFAULT_APPEARANCE })}
              >
                Reset to defaults
              </Button>
              <Button type="submit" disabled={disabled || !dirty || !!urlError}>
                {save.isPending ? "Saving..." : "Save appearance"}
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  );
}
