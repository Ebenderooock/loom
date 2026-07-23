import { Loader2 } from "lucide-react";

export function PageLoader() {
  return (
    <div
      className="flex min-h-[50vh] w-full items-center justify-center"
      role="status"
      aria-live="polite"
    >
      <div className="animate-fade-in-up text-muted-foreground flex flex-col items-center gap-4">
        <div className="relative">
          <div className="border-primary/20 bg-primary/10 flex h-12 w-12 items-center justify-center rounded-xl border">
            <span className="gradient-text text-lg font-bold">L</span>
          </div>
          <div className="animate-glow-pulse absolute inset-0 rounded-xl" />
        </div>
        <div className="flex items-center gap-2">
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          <span className="text-sm font-medium">Loading…</span>
        </div>
      </div>
    </div>
  );
}
