export type StatusVariant = "destructive" | "secondary" | "outline" | "default";

export interface StatusConfig {
  label: string;
  variant: StatusVariant;
  className?: string;
}

/** Shared download/queue status config for Badge component */
export function downloadStatusConfig(status: string): StatusConfig {
  switch (status) {
    case "downloading":
      return { label: "Downloading", variant: "outline", className: "border-blue-500/30 bg-blue-500/10 text-blue-400" };
    case "seeding":
      return { label: "Seeding", variant: "outline", className: "border-green-500/30 bg-green-500/10 text-green-400" };
    case "queued":
      return { label: "Queued", variant: "outline", className: "border-yellow-500/30 bg-yellow-500/10 text-yellow-400" };
    case "paused":
      return { label: "Paused", variant: "outline", className: "border-zinc-500/30 bg-zinc-500/10 text-zinc-400" };
    case "completed":
      return { label: "Completed", variant: "outline", className: "border-teal-500/30 bg-teal-500/10 text-teal-400" };
    case "failed": case "error":
      return { label: status === "error" ? "Error" : "Failed", variant: "destructive" };
    case "stalled":
      return { label: "Stalled", variant: "outline", className: "border-orange-500/30 bg-orange-500/10 text-orange-400" };
    default:
      return { label: status || "Unknown", variant: "secondary" };
  }
}

/** Map audit log / event levels to Badge variants */
export function levelVariant(level: string): StatusVariant {
  switch (level) {
    case "error": return "destructive";
    case "warn": return "secondary";
    default: return "outline";
  }
}
