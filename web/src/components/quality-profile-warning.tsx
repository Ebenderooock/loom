import { AlertTriangle, Pencil } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * True when a movie/series has no usable quality profile. The server's
 * `qualityProfileMissing` flag is authoritative (it also catches dangling
 * IDs). Without that flag, only a missing, blank, or whitespace-only ID is
 * flagged — non-empty IDs are never inferred invalid from client-side data,
 * to avoid false positives.
 */
export function isMissingQualityProfile(item: {
  qualityProfileId?: string | null;
  qualityProfileMissing?: boolean | null;
}) {
  if (item.qualityProfileMissing === true) return true;
  const id = item.qualityProfileId;
  return !id || id.trim() === "";
}

const MISSING_LABEL = "No quality profile";

/** Compact, always-visible badge for cards and table rows. */
export function MissingQualityProfileBadge({
  className,
  onDark = false,
}: {
  className?: string;
  /** Use high-contrast colours for placement over poster artwork. */
  onDark?: boolean;
}) {
  return (
    <span
      title="No quality profile — this item can't be searched until one is set"
      className={cn(
        "inline-flex items-center gap-1 whitespace-nowrap rounded px-1.5 py-0.5 text-[10px] font-medium",
        onDark
          ? "bg-amber-500/90 text-black"
          : "bg-amber-500/15 text-amber-700 ring-1 ring-amber-500/30 dark:text-amber-300",
        className,
      )}
    >
      <AlertTriangle className="h-3 w-3 shrink-0" aria-hidden="true" />
      {MISSING_LABEL}
    </span>
  );
}

/** Detail-sheet banner explaining the problem with a direct fix action. */
export function MissingQualityProfileAlert({
  mediaLabel,
  onFix,
  className,
}: {
  mediaLabel: "movie" | "series";
  onFix: () => void;
  className?: string;
}) {
  return (
    <div
      role="alert"
      className={cn(
        "flex flex-col gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-700 sm:flex-row sm:items-center sm:justify-between dark:text-amber-300",
        className,
      )}
    >
      <div className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <div>
          <p className="font-semibold">{MISSING_LABEL}</p>
          <p className="text-xs opacity-90">
            This {mediaLabel} can&apos;t be searched or upgraded until a quality
            profile is selected.
          </p>
        </div>
      </div>
      <Button
        size="sm"
        variant="outline"
        className="h-9 shrink-0 gap-1.5 border-amber-500/50 text-xs"
        onClick={onFix}
      >
        <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
        Set quality profile
      </Button>
    </div>
  );
}

/**
 * Resolve the default quality profile for a new item: the preferred (e.g.
 * library) profile when it exists, otherwise the built-in "Any" profile.
 * Never falls back to an arbitrary first profile; returns "" when neither is
 * available so the server can apply its own "Any" default.
 */
export function defaultQualityProfileId(
  profiles: { id: string; name: string }[],
  preferredId?: string | null,
) {
  const preferred = preferredId?.trim();
  if (preferred && profiles.some((p) => p.id === preferred)) return preferred;
  return profiles.find((p) => p.name.trim().toLowerCase() === "any")?.id ?? "";
}
