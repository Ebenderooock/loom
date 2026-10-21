import { afterEach, expect, it, vi } from "vitest";
import { fetchCalendarEvents } from "./calendar-api";

afterEach(() => vi.restoreAllMocks());

it("sends the browser's IANA timezone alongside the inclusive local dates", async () => {
  const resolved = new Intl.DateTimeFormat().resolvedOptions();
  vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
    ...resolved,
    timeZone: "Pacific/Auckland",
  });
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response("[]"));
  await expect(
    fetchCalendarEvents("2026-10-01", "2026-10-31"),
  ).resolves.toEqual([]);
  const url = new URL(String(fetch.mock.calls[0]![0]), "http://localhost");
  expect(url.searchParams.get("timezone")).toBe("Pacific/Auckland");
  expect(url.searchParams.get("start")).toBe("2026-10-01");
  expect(url.searchParams.get("end")).toBe("2026-10-31");
});
