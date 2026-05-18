import { useRouter } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { AlertCircle } from "lucide-react";

export function ErrorFallback({ error, reset }: { error: Error; reset?: () => void }) {
  const router = useRouter();

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <div className="mx-auto max-w-md space-y-6 text-center animate-fade-in-up">
        {/* Decorative background orb */}
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 h-48 w-48 rounded-full bg-destructive/5 blur-3xl pointer-events-none" />
        <div className="flex justify-center">
          <div className="flex h-20 w-20 items-center justify-center rounded-2xl bg-gradient-to-br from-destructive/20 to-destructive/5 border border-destructive/20">
            <AlertCircle className="h-10 w-10 text-destructive" />
          </div>
        </div>
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight text-foreground">
            Something went wrong
          </h1>
          <p className="text-sm text-muted-foreground">
            An unexpected error occurred. You can try again or return to the home page.
          </p>
        </div>
        <pre className="max-h-40 overflow-auto rounded-lg border border-border/50 bg-card/50 backdrop-blur-sm p-4 text-left text-xs text-muted-foreground">
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
