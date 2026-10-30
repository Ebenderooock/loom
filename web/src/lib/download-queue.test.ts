import { describe, expect, it } from "vitest";
import {
  sortQueueItems,
  type QueueItem,
  type QueueSortKey,
} from "./download-queue";

function item(id: string, overrides: Partial<QueueItem> = {}): QueueItem {
  return {
    id,
    client_id: "client",
    title: id,
    category: "",
    status: "downloading",
    progress: 0.5,
    size_bytes: 100,
    downloaded_bytes: 50,
    eta_seconds: 60,
    download_rate: 10,
    upload_rate: 0,
    ratio: 0,
    message: "",
    save_path: "",
    ...overrides,
  };
}

describe("sortQueueItems", () => {
  it("uses client queue positions, not status or response order", () => {
    const items = [
      item("second", { queue_position: 1 }),
      item("unqueued"),
      item("first", { queue_position: 0, status: "queued" }),
    ];
    expect(sortQueueItems(items, "queue", "asc").map((i) => i.id)).toEqual([
      "first",
      "second",
      "unqueued",
    ]);
    expect(sortQueueItems(items, "queue", "desc").map((i) => i.id)).toEqual([
      "second",
      "first",
      "unqueued",
    ]);
    expect(items[0]?.id).toBe("second");
  });

  it("sorts speed in either direction, including zero", () => {
    const items = [
      item("slow", { download_rate: 1 }),
      item("stopped", { download_rate: 0 }),
      item("fast", { download_rate: 100 }),
    ];
    expect(sortQueueItems(items, "speed", "desc").map((i) => i.id)).toEqual([
      "fast",
      "slow",
      "stopped",
    ]);
    expect(sortQueueItems(items, "speed", "asc").map((i) => i.id)).toEqual([
      "stopped",
      "slow",
      "fast",
    ]);
  });

  it("keeps unavailable ETAs last in both directions", () => {
    const items = [
      item("zero", { eta_seconds: 0 }),
      item("negative", { eta_seconds: -1 }),
      item("infinite", { eta_seconds: 8_640_000 }),
      item("paused", { status: "paused", eta_seconds: 1 }),
      item("seeding", { status: "seeding", eta_seconds: 1 }),
      item("completed", { status: "completed", eta_seconds: 1 }),
      item("failed", { status: "failed", eta_seconds: 1 }),
      item("short", { eta_seconds: 30 }),
      item("long", { eta_seconds: 120 }),
    ];
    expect(
      sortQueueItems(items, "eta", "asc")
        .slice(0, 2)
        .map((i) => i.id),
    ).toEqual(["short", "long"]);
    expect(
      sortQueueItems(items, "eta", "desc")
        .slice(0, 2)
        .map((i) => i.id),
    ).toEqual(["long", "short"]);
  });

  it("groups statuses using the existing lifecycle ordering", () => {
    const statuses = [
      "unknown",
      "failed",
      "completed",
      "seeding",
      "paused",
      "queued",
      "downloading",
    ];
    const items = statuses.map((status) => item(status, { status }));
    expect(sortQueueItems(items, "status", "asc").map((i) => i.status)).toEqual(
      [...statuses].reverse(),
    );
    expect(
      sortQueueItems(items, "status", "desc").map((i) => i.status),
    ).toEqual([
      "failed",
      "completed",
      "seeding",
      "paused",
      "queued",
      "downloading",
      "unknown",
    ]);
  });

  it.each<QueueSortKey>(["queue", "speed", "eta", "status"])(
    "keeps %s ties deterministic when polling changes the response order",
    (key) => {
      const items = [
        item("same", { client_id: "b", queue_position: 0 }),
        item("z", { queue_position: 1 }),
        item("same", { client_id: "a", queue_position: 0 }),
        item("a", { queue_position: 1 }),
      ];
      const expected = sortQueueItems(items, key, "asc");
      expect(sortQueueItems([...items].reverse(), key, "asc")).toEqual(
        expected,
      );
      expect(expected.map((i) => [i.client_id, i.id])).toEqual([
        ["a", "same"],
        ["b", "same"],
        ["client", "a"],
        ["client", "z"],
      ]);
    },
  );

  it("treats invalid and missing queue positions as unavailable", () => {
    const items = [
      item("missing"),
      item("negative", { queue_position: -1 }),
      item("nan", { queue_position: NaN }),
      item("first", { queue_position: 0 }),
    ];
    expect(sortQueueItems(items, "queue", "asc")[0]?.id).toBe("first");
    expect(sortQueueItems(items, "queue", "desc")[0]?.id).toBe("first");
  });
});
