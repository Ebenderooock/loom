import { useRouter } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { AlertCircle } from "lucide-react";

export function ErrorFallback({
  error,
  reset,
}: {
  error: Error;
  reset?: () => void;
}) {
  const router = useRouter();

  return (
    <div className="bg-background flex min-h-screen items-center justify-center p-4">
      <div className="animate-fade-in-up mx-auto max-w-md space-y-6 text-center">
        {/* Decorative background orb */}
        <div className="bg-destructive/5 pointer-events-none absolute top-1/2 left-1/2 h-48 w-48 -translate-x-1/2 -translate-y-1/2 rounded-full blur-3xl" />
        <div className="flex justify-center">
          <div className="border-destructive/20 from-destructive/20 to-destructive/5 flex h-20 w-20 items-center justify-center rounded-2xl border bg-gradient-to-br">
            <AlertCircle className="text-destructive h-10 w-10" />
          </div>
        </div>
        <div className="space-y-2">
          <h1 className="text-foreground text-2xl font-semibold tracking-tight">
            Something went wrong
          </h1>
          <p className="text-muted-foreground text-sm">
            An unexpected error occurred. You can try again or return to the
            home page.
          </p>
        </div>
        <pre className="border-border/50 bg-card/50 text-muted-foreground max-h-40 overflow-auto rounded-lg border p-4 text-left text-xs backdrop-blur-sm">
          {error.message}
        </pre>
        <div className="flex items-center justify-center gap-3">
          <Button
            variant="outline"
            className="border-border/50 hover:border-accent/30"
            onClick={() => {
              if (reset) {
                reset();
              } else {
                router.invalidate();
              }
            }}
          >
            Try Again
          </Button>
          <Button
            onClick={() => {
              window.location.href = "/";
            }}
          >
            Go Home
          </Button>
        </div>
      </div>
    </div>
  );
}
