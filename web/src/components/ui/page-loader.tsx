import { Loader2 } from "lucide-react";

export function PageLoader() {
  return (
    <div
      className="flex min-h-[50vh] w-full items-center justify-center"
      role="status"
      aria-live="polite"
    >
      <div className="flex animate-fade-in-up flex-col items-center gap-4 text-muted-foreground">
        <div className="relative">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl border border-primary/20 bg-primary/10">
            <span className="gradient-text text-lg font-bold">L</span>
          </div>
          <div className="absolute inset-0 animate-glow-pulse rounded-xl" />
        </div>
        <div className="flex items-center gap-2">
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          <span className="text-sm font-medium">Loading…</span>
        </div>
      </div>
    </div>
  );
}
