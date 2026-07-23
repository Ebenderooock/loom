import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle } from "lucide-react";

export function AuthPage() {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { refreshAuth } = useAuth();

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!username.trim() || !password.trim()) {
      setError("Username and password are required");
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const response = await fetch("/api/v1/auth/login", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          username: username.trim(),
          password,
        }),
      });

      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || "Login failed");
      }

      // Refresh auth state to pick up the new session cookie
      await refreshAuth();

      // Redirect to dashboard
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex h-screen w-full items-center justify-center bg-gradient-to-br from-purple-midnight via-neutral-dark to-teal-deep">
      <div className="mx-auto w-full max-w-md px-4">
        <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
          <div className="space-y-4 text-center">
            <img
              src="/loom-logo.png"
              alt="Loom Logo"
              className="mx-auto h-16 object-contain"
            />
            <div className="space-y-2">
              <h1 className="text-3xl font-bold text-neutral-light">
                Welcome back to Loom
              </h1>
              <p className="text-neutral-muted">
                Enter your credentials to continue
              </p>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <form onSubmit={handleLogin} className="space-y-4">
            <div className="space-y-2">
              <label
                htmlFor="username"
                className="text-sm font-medium text-neutral-light"
              >
                Username
              </label>
              <Input
                id="username"
                type="text"
                placeholder="Enter your username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                disabled={isLoading}
                className="border-neutral-800 bg-neutral-900 text-neutral-light placeholder-neutral-muted"
              />
            </div>

            <div className="space-y-2">
              <label
                htmlFor="password"
                className="text-sm font-medium text-neutral-light"
              >
                Password
              </label>
              <Input
                id="password"
                type="password"
                placeholder="Enter your password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={isLoading}
                className="border-neutral-800 bg-neutral-900 text-neutral-light placeholder-neutral-muted"
              />
            </div>

            <Button
              type="submit"
              disabled={isLoading}
              className="h-10 w-full bg-teal-500 text-white hover:bg-teal-600"
            >
              {isLoading ? "Logging in..." : "Login"}
            </Button>
          </form>

          <p className="text-center text-sm text-neutral-muted">
            Make sure you've completed the initial setup first
          </p>
        </div>
      </div>
    </div>
  );
}
