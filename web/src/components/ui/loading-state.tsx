import { Loader2 } from "lucide-react";

interface LoadingStateProps {
  label?: string;
}

export function LoadingState({ label = "Loading…" }: LoadingStateProps) {
  return (
    <div className="animate-fade-in-up text-muted-foreground flex items-center justify-center py-8">
      <div className="relative">
        <Loader2 className="mr-2 h-5 w-5 animate-spin" />
        <div className="animate-glow-pulse absolute inset-0 rounded-full" />
      </div>
      <span className="ml-1">{label}</span>
    </div>
  );
}
