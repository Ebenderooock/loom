import { SERIES_STATUS_CONFIG } from "./types";

export function SeriesStatusBadge({ status }: { status: string }) {
  const cfg = SERIES_STATUS_CONFIG[status] ?? {
    label: status,
    color: "text-gray-400",
    bg: "bg-gray-500/20",
  };
  return (
    <span
      className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium ${cfg.color} ${cfg.bg}`}
    >
      <span
        className={`h-1.5 w-1.5 rounded-full ${cfg.color.replace("text-", "bg-")}`}
      />
      {cfg.label}
    </span>
  );
}
