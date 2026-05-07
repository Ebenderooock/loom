import { useRouter } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { AlertCircle } from "lucide-react";

export function ErrorFallback({ error, reset }: { error: Error; reset?: () => void }) {
  const router = useRouter();

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <div className="mx-auto max-w-md space-y-6 text-center">
        <div className="flex justify-center">
          <AlertCircle className="h-16 w-16 text-destructive" />
        </div>
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight text-foreground">
            Something went wrong
          </h1>
          <p className="text-sm text-muted-foreground">
            An unexpected error occurred. You can try again or return to the home page.
          </p>
        </div>
        <pre className="max-h-40 overflow-auto rounded-md border border-zinc-800 bg-zinc-900/50 p-4 text-left text-xs text-muted-foreground">
          {error.message}
        </pre>
        <div className="flex items-center justify-center gap-3">
          <Button
            variant="outline"
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
