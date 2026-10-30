import { STATUS_CONFIG } from "./types";

export function StatusAccent({ status }: { status: string }) {
  const color = STATUS_CONFIG[status]?.border ?? "#6b7280";
  return (
    <>
      <div
        className="absolute left-0 right-0 top-0 z-10 h-[3px]"
        style={{
          background: `linear-gradient(90deg, ${color}, ${color}cc 60%, transparent)`,
        }}
      />
      <div
        className="pointer-events-none absolute left-0 right-0 top-[3px] z-10 h-3"
        style={{
          background: `linear-gradient(180deg, ${color}25, transparent)`,
        }}
      />
    </>
  );
}

export function MonitoringLabel({ status }: { status?: string }) {
  return status === "unmonitored" ? (
    <span className="text-[10px] uppercase tracking-wider text-gray-500">
      Unmonitored
    </span>
  ) : null;
}

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
