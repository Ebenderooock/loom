import { useEffect, useState } from "react";
import type { AiringDate } from "@/lib/episode-airing";

/** Re-render at the next release instant (or legacy UTC day boundary). */
export function useAiringNow(episodes: AiringDate[]): Date {
  const [now, setNow] = useState(() => new Date());
  // Depend on schedule values, not a possibly freshly allocated result array.
  const airingTimes = episodes.map((ep) => ep.airDateUtc ?? "").join(",");
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    const instants = airingTimes.split(",").map((stamp) => Date.parse(stamp));
    const refresh = () => {
      clearTimeout(timer);
      const current = Date.now();
      setNow(new Date(current));
      const nextDay = new Date(current);
      nextDay.setUTCHours(24, 0, 0, 0);
      const next = Math.min(
        nextDay.getTime(),
        ...instants.filter((instant) => instant > current),
      );
      timer = setTimeout(refresh, next - current);
    };
    refresh();
    window.addEventListener("focus", refresh);
    return () => {
      clearTimeout(timer);
      window.removeEventListener("focus", refresh);
    };
  }, [airingTimes]);
  return now;
}
