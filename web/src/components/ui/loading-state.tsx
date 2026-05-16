import { Loader2 } from "lucide-react";

interface LoadingStateProps {
  label?: string;
}

export function LoadingState({ label = "Loading…" }: LoadingStateProps) {
  return (
    <div className="flex items-center justify-center py-8 text-muted-foreground animate-fade-in-up">
      <div className="relative">
        <Loader2 className="h-5 w-5 animate-spin mr-2" />
        <div className="absolute inset-0 animate-glow-pulse rounded-full" />
      </div>
      <span className="ml-1">{label}</span>
    </div>
  );
}
