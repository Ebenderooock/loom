import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { getEpisodeStatus } from "@/lib/episode-airing";
import { useAiringNow } from "./use-airing-now";

afterEach(() => vi.useRealTimers());

it("updates a mounted view exactly at broadcast time and cleans up", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-22T00:29:59Z"));
  const episodes = [
    {
      airDate: "2026-10-21",
      airDateUtc: "2026-10-22T00:30:00Z",
      monitored: true,
      hasFile: false,
    },
  ];
  const { result, unmount } = renderHook(() =>
    getEpisodeStatus(episodes[0]!, useAiringNow(episodes)),
  );
  expect(result.current).toBe("unaired");
  act(() => vi.advanceTimersByTime(1000));
  expect(result.current).toBe("missing");
  unmount();
  expect(vi.getTimerCount()).toBe(0);
});

it("updates date-only fallback at UTC midnight without shifting the date", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-20T23:59:59Z"));
  const episodes = [{ airDate: "2026-10-21", monitored: true, hasFile: false }];
  const { result, unmount } = renderHook(() =>
    getEpisodeStatus(episodes[0]!, useAiringNow(episodes)),
  );
  expect(result.current).toBe("unaired");
  act(() => vi.advanceTimersByTime(1000));
  expect(result.current).toBe("missing");
  unmount();
});
