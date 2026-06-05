import { STATUS_CONFIG } from "./types";

export function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? {
    label: status,
    color: "text-gray-400",
    bg: "bg-gray-500/20",
  };
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium ${cfg.color} ${cfg.bg} border-current/10 border shadow-sm`}
    >
      <span
        className={`h-1.5 w-1.5 rounded-full ${cfg.color.replace("text-", "bg-")} animate-pulse`}
      />
      {cfg.label}
    </span>
  );
}
