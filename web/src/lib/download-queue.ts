export interface QueueItem {
  id: string;
  client_id: string;
  title: string;
  category: string;
  status: string;
  progress: number;
  size_bytes: number;
  downloaded_bytes: number;
  eta_seconds: number;
  download_rate: number;
  upload_rate: number;
  ratio: number;
  message: string;
  save_path: string;
  queue_position?: number;
}

export const queueSortOptions = [
  { value: "queue", label: "Queue" },
  { value: "speed", label: "Download speed" },
  { value: "eta", label: "Time to completion" },
  { value: "status", label: "Status" },
] as const;

export type QueueSortKey = (typeof queueSortOptions)[number]["value"];
export type SortDirection = "asc" | "desc";

const statusOrder: Record<string, number> = {
  downloading: 0,
  queued: 1,
  paused: 2,
  seeding: 3,
  completed: 4,
  failed: 5,
};

function sortValue(item: QueueItem, key: QueueSortKey): number | undefined {
  switch (key) {
    case "queue":
      return item.queue_position != null &&
        Number.isFinite(item.queue_position) &&
        item.queue_position >= 0
        ? item.queue_position
        : undefined;
    case "speed":
      return Number.isFinite(item.download_rate) && item.download_rate >= 0
        ? item.download_rate
        : undefined;
    case "eta":
      // Zero/negative ETAs and qBittorrent's infinity sentinel are unknown.
      return item.eta_seconds > 0 &&
        item.eta_seconds < 8_640_000 &&
        item.status !== "paused" &&
        item.status !== "seeding" &&
        item.status !== "completed" &&
        item.status !== "failed"
        ? item.eta_seconds
        : undefined;
    case "status":
      return statusOrder[item.status];
  }
}

function compareIdentity(a: QueueItem, b: QueueItem): number {
  if (a.client_id !== b.client_id) return a.client_id < b.client_id ? -1 : 1;
  return a.id === b.id ? 0 : a.id < b.id ? -1 : 1;
}

export function sortQueueItems(
  items: readonly QueueItem[],
  key: QueueSortKey,
  direction: SortDirection,
): QueueItem[] {
  return [...items].sort((a, b) => {
    const av = sortValue(a, key);
    const bv = sortValue(b, key);
    // Missing values stay last in either direction.
    if (av === undefined && bv !== undefined) return 1;
    if (av !== undefined && bv === undefined) return -1;
    if (av !== undefined && bv !== undefined && av !== bv) {
      return direction === "asc" ? av - bv : bv - av;
    }
    const aq = sortValue(a, "queue");
    const bq = sortValue(b, "queue");
    if (aq !== undefined && bq !== undefined && aq !== bq) return aq - bq;
    if (aq === undefined && bq !== undefined) return 1;
    if (aq !== undefined && bq === undefined) return -1;
    return compareIdentity(a, b);
  });
}
