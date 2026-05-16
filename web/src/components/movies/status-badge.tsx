import { STATUS_CONFIG } from "./types";

export function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? { label: status, color: "text-gray-400", bg: "bg-gray-500/20" };
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-xs font-medium ${cfg.color} ${cfg.bg} border border-current/10 shadow-sm`}>
      <span className={`w-1.5 h-1.5 rounded-full ${cfg.color.replace("text-", "bg-")} animate-pulse`} />
      {cfg.label}
    </span>
  );
}
