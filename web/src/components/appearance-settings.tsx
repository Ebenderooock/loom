import * as React from "react";
import { toast } from "sonner";
import { useAuth } from "@/hooks/use-auth";
import { useAppearance, useSaveAppearance } from "@/hooks/use-appearance";
import {
  DEFAULT_APPEARANCE,
  backgroundUrlError,
  uploadBackground,
  type Appearance,
} from "@/lib/appearance-api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PaletteEditor } from "@/components/palette-editor";
import { PALETTES, validHex, type Palette } from "@/lib/palette";

const THEMES: Array<{ value: Appearance["theme"]; label: string }> = [
  { value: "system", label: "System" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
  { value: "amoled", label: "AMOLED" },
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
  const [colorInputs, setColorInputs] = React.useState<Partial<Palette>>({});
  const [failedImage, setFailedImage] = React.useState<string>();
  const [uploading, setUploading] = React.useState(false);
  const [uploadError, setUploadError] = React.useState<string>();
  const [backgroundSource, setBackgroundSource] = React.useState<
    "url" | "upload"
  >(
    initial.backgroundUrl.startsWith("/api/v1/appearance/background")
      ? "upload"
      : "url",
  );
  const save = useSaveAppearance();
  const url = draft.backgroundUrl.trim();
  const urlError = backgroundUrlError(url);
  const imageFailed = !!url && failedImage === url;
  const invalidColors = Object.values(colorInputs).some(
    (value) => !validHex(value),
  );
  const dirty =
    invalidColors || JSON.stringify(draft) !== JSON.stringify(initial);
  const disabled = !canEdit || save.isPending || uploading;
  const palette = draft.palette ?? PALETTES.Loom;

  const upload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setUploadError(undefined);
    try {
      const uploadedUrl = await uploadBackground(file);
      setDraft((current) => ({ ...current, backgroundUrl: uploadedUrl }));
    } catch (error) {
      setUploadError(
        error instanceof Error ? error.message : "Image upload failed.",
      );
    } finally {
      setUploading(false);
      event.target.value = "";
    }
  };

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (urlError || invalidColors || disabled) return;
    save.mutate(
      { ...draft, backgroundUrl: url },
      {
        onSuccess: () => toast.success("Appearance saved for everyone."),
        onError: (error) => toast.error(error.message),
      },
    );
  };

  return (
    <form onSubmit={submit} className="min-w-0 space-y-4 break-words">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Appearance for everyone</CardTitle>
          <p className="text-sm text-muted-foreground">
            Choose a theme, pick your colors, then add a background if you like.
            Save when you're happy to apply it for everyone on every device.
            {!canEdit && " Only admins can change these settings."}
          </p>
        </CardHeader>
        <CardContent className="space-y-6">
          <fieldset disabled={disabled} className="min-w-0 space-y-2">
            <legend className="mb-2 text-base font-semibold">
              1. Choose a theme
            </legend>
            <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,8rem),1fr))] gap-3">
              {THEMES.map(({ value, label }) => (
                <Button
                  type="button"
                  key={value}
                  variant={draft.theme === value ? "default" : "outline"}
                  aria-pressed={draft.theme === value}
                  className="h-auto min-h-11 whitespace-normal py-3"
                  onClick={() => setDraft({ ...draft, theme: value })}
                >
                  {label}
                </Button>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">
              {draft.theme === "system" &&
                "System automatically matches each device's light or dark setting."}
              {draft.theme === "light" &&
                "Light uses bright backgrounds and dark text."}
              {draft.theme === "dark" &&
                "Dark uses softer, dark backgrounds and light text."}
              {draft.theme === "amoled" &&
                "AMOLED uses a true-black page background."}
            </p>
          </fieldset>

          <PaletteEditor
            palette={palette}
            theme={draft.theme}
            disabled={disabled}
            colorInputs={colorInputs}
            onColorInputChange={(key, value) => {
              setColorInputs((current) => ({ ...current, [key]: value }));
              if (validHex(value)) {
                setDraft((current) => ({
                  ...current,
                  palette: {
                    ...(current.palette ?? PALETTES.Loom),
                    [key]: value,
                  },
                  accent: "default",
                }));
              }
            }}
            onChange={(next) => {
              setColorInputs({});
              setDraft({ ...draft, palette: next, accent: "default" });
            }}
          />

          <div className="space-y-3 border-t border-border pt-6">
            <h3 className="text-base font-semibold">
              3. Add a background{" "}
              <span className="font-normal text-muted-foreground">
                (optional)
              </span>
            </h3>
            <p className="text-sm text-muted-foreground">
              Keep a plain background, upload a photo, or paste a link to an
              image.
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant={backgroundSource === "url" ? "secondary" : "outline"}
                disabled={disabled}
                aria-pressed={backgroundSource === "url"}
                className="h-auto min-h-11 whitespace-normal py-3"
                onClick={() => {
                  setBackgroundSource("url");
                  setUploadError(undefined);
                }}
              >
                Image URL
              </Button>
              <Button
                type="button"
                variant={
                  backgroundSource === "upload" ? "secondary" : "outline"
                }
                disabled={disabled}
                aria-pressed={backgroundSource === "upload"}
                className="h-auto min-h-11 whitespace-normal py-3"
                onClick={() => {
                  setBackgroundSource("upload");
                  setUploadError(undefined);
                }}
              >
                Upload image
              </Button>
            </div>
            {backgroundSource === "url" ? (
              <>
                <Label htmlFor="appearance-background">
                  Background image URL
                </Label>
                <Input
                  id="appearance-background"
                  type="text"
                  inputMode="url"
                  className="h-11 text-base"
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
                  Paste a full image link starting with https://. Leave empty
                  for no image. Everyone's device must be able to open the link.
                </p>
              </>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="appearance-upload">
                  Choose background image
                </Label>
                <Input
                  id="appearance-upload"
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  className="h-11 text-base"
                  disabled={disabled}
                  onChange={(event) => void upload(event)}
                />
                <p className="text-xs text-muted-foreground">
                  PNG, JPEG, or WebP, up to 5 MiB. Loom stores the image for
                  every device. Upload first, then save to apply it.
                </p>
                {uploading && (
                  <p role="status" className="text-sm">
                    Uploading image...
                  </p>
                )}
                {uploadError && (
                  <p role="alert" className="text-sm text-destructive">
                    {uploadError}
                  </p>
                )}
              </div>
            )}
            {url && (
              <Button
                type="button"
                variant="outline"
                className="h-11"
                disabled={disabled}
                onClick={() => setDraft({ ...draft, backgroundUrl: "" })}
              >
                Remove background
              </Button>
            )}
            {urlError && (
              <p
                id="appearance-background-error"
                role="alert"
                className="text-sm text-destructive"
              >
                {urlError}
              </p>
            )}
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
                    className="block h-11 w-full accent-[hsl(var(--accent))]"
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        backgroundDim: Number(event.target.value),
                      })
                    }
                  />
                  <p className="text-xs text-muted-foreground">
                    Slide right to make the image less distracting. At 100%, the
                    image is hidden.
                  </p>
                </div>
              </div>
            )}
          </div>

          {save.error && (
            <p role="alert" className="text-sm text-destructive">
              {save.error.message}
            </p>
          )}
          {invalidColors && (
            <p role="alert" className="text-sm text-destructive">
              Correct the invalid color values under Customize individual colors
              before saving.
            </p>
          )}
          {canEdit && (
            <div className="space-y-3 border-t border-border pt-4">
              <p role="status" className="text-sm text-muted-foreground">
                {dirty
                  ? "Unsaved changes. Save to apply them for everyone."
                  : "No unsaved changes."}
              </p>
              <div className="flex flex-col-reverse flex-wrap gap-3 sm:flex-row sm:items-center sm:justify-between">
                <Button
                  type="button"
                  variant="outline"
                  className="h-auto min-h-11 whitespace-normal py-3"
                  disabled={disabled}
                  onClick={() => {
                    setDraft({ ...DEFAULT_APPEARANCE });
                    setColorInputs({});
                    setBackgroundSource("url");
                    setUploadError(undefined);
                  }}
                >
                  Reset to defaults
                </Button>
                <div className="flex min-w-0 flex-col flex-wrap gap-3 sm:flex-row">
                  {dirty && (
                    <Button
                      type="button"
                      variant="outline"
                      className="h-auto min-h-11 whitespace-normal py-3"
                      disabled={disabled}
                      onClick={() => {
                        setDraft(initial);
                        setColorInputs({});
                        setBackgroundSource(
                          initial.backgroundUrl.startsWith(
                            "/api/v1/appearance/background",
                          )
                            ? "upload"
                            : "url",
                        );
                        setUploadError(undefined);
                        save.reset();
                      }}
                    >
                      Discard changes
                    </Button>
                  )}
                  <Button
                    type="submit"
                    className="h-auto min-h-11 whitespace-normal py-3"
                    disabled={disabled || !dirty || !!urlError || invalidColors}
                  >
                    {save.isPending ? "Saving..." : "Save appearance"}
                  </Button>
                </div>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </form>
  );
}
