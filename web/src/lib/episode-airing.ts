/** Actual instants are localized; date-only values must never be shifted. */
export interface AiringDate {
  airDate?: string;
  airDateUtc?: string;
}

export type EpisodeStatus =
  | "downloaded"
  | "grabbed"
  | "missing"
  | "unaired"
  | "unmonitored";

interface AiringEpisode extends AiringDate {
  monitored: boolean;
  hasFile: boolean;
  grabbed?: boolean;
}

export function getEpisodeStatus(
  ep: AiringEpisode,
  now = new Date(),
): EpisodeStatus {
  if (!ep.monitored) return "unmonitored";
  if (ep.hasFile) return "downloaded";
  if (ep.grabbed) return "grabbed";
  return hasEpisodeAired(ep, now) ? "missing" : "unaired";
}

export function getSeasonStats(episodes: AiringEpisode[], now = new Date()) {
  const counts = {
    downloaded: 0,
    grabbed: 0,
    missing: 0,
    unaired: 0,
    unmonitored: 0,
  };
  for (const ep of episodes) counts[getEpisodeStatus(ep, now)]++;
  const aired = counts.downloaded + counts.grabbed + counts.missing;
  return {
    ...counts,
    aired,
    total: episodes.length,
    percent:
      aired > 0
        ? Math.round((counts.downloaded / aired) * 100)
        : episodes.length > 0
          ? 100
          : 0,
  };
}

export function hasEpisodeAired(ep: AiringDate, now = new Date()): boolean {
  if (ep.airDateUtc) {
    return new Date(ep.airDateUtc).getTime() <= now.getTime();
  }
  // Preserve the legacy UTC-day fallback when no airing time is known.
  return !!ep.airDate && ep.airDate <= now.toISOString().slice(0, 10);
}

export function episodeReleaseDay(
  ep: AiringDate,
  timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone,
): string {
  if (!ep.airDateUtc) return ep.airDate ?? "";
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(ep.airDateUtc));
  const part = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((p) => p.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
}

export function formatEpisodeAirDate(ep: AiringDate): string {
  if (!ep.airDateUtc && !ep.airDate) return "—";
  const date = new Date(ep.airDateUtc || `${ep.airDate}T00:00:00`);
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    ...(ep.airDateUtc
      ? { hour: "numeric", minute: "2-digit", timeZoneName: "short" }
      : {}),
  });
}
