import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";

export function NotFoundPage() {
  return (
    <div className="relative flex min-h-[60vh] flex-col items-center justify-center gap-6 text-center animate-fade-in-up">
      {/* Decorative background orb */}
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 h-64 w-64 rounded-full bg-accent/5 blur-3xl pointer-events-none" />
      <p className="text-8xl font-bold gradient-text">404</p>
      <h1 className="text-2xl font-semibold tracking-tight">Page not found</h1>
      <p className="text-sm text-muted-foreground max-w-sm">
        The page you're looking for doesn't exist or has been moved.
      </p>
      <Button asChild className="mt-2">
        <Link to="/">Back to dashboard</Link>
      </Button>
    </div>
  );
}
