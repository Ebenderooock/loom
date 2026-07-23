import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/hooks/use-auth";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, CheckCircle2, Lock } from "lucide-react";
import { cn } from "@/lib/utils";

type SetupStep = "welcome" | "credentials" | "complete";

const STEPS: SetupStep[] = ["welcome", "credentials", "complete"];

const STEP_LABELS: Record<SetupStep, string> = {
  welcome: "Welcome",
  credentials: "Account",
  complete: "Done",
};

// ─── Step Indicator ──────────────────────────────────────────────────

function StepIndicator({ current }: { current: SetupStep }) {
  const idx = STEPS.indexOf(current);
  return (
    <div className="mb-6 flex items-center justify-center gap-1.5">
      {STEPS.map((s, i) => (
        <div key={s} className="flex items-center gap-1.5">
          <div
            className={cn(
              "h-2.5 w-2.5 rounded-full transition-colors",
              i < idx
                ? "bg-teal-electric"
                : i === idx
                  ? "bg-purple-rich ring-purple-rich/40 ring-2"
                  : "bg-neutral-muted/30",
            )}
            title={STEP_LABELS[s]}
          />
        </div>
      ))}
    </div>
  );
}

// ─── Main Setup Page ──────────────────────────────────────────────────

export function SetupPage() {
  const [step, setStep] = useState<SetupStep>("welcome");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { refreshAuth } = useAuth();

  const goTo = (s: SetupStep) => {
    setError(null);
    setStep(s);
  };

  const completeSetup = async () => {
    if (!username.trim() || !password.trim()) {
      setError("Username and password are required");
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const response = await apiFetch("/api/v1/auth/initialize", {
        method: "POST",
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
        throw new Error(err.error || "Failed to complete setup");
      }

      goTo("complete");
      await refreshAuth();
      setTimeout(() => {
        navigate({ to: "/" });
      }, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to complete setup");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="from-purple-midnight via-neutral-dark to-teal-deep flex min-h-screen w-full items-center justify-center bg-gradient-to-br py-8">
      <div className="mx-auto w-full max-w-md px-4">
        <StepIndicator current={step} />

        {/* Welcome Step */}
        {step === "welcome" && (
          <div className="bg-neutral-card space-y-6 rounded-lg p-8 shadow-lg">
            <div className="space-y-4 text-center">
              <img
                src="/loom-logo.png"
                alt="Loom Logo"
                className="mx-auto h-16 object-contain"
              />
              <div className="space-y-2">
                <h1 className="text-neutral-light text-3xl font-bold">
                  Welcome to Loom
                </h1>
                <p className="text-neutral-muted">
                  Create your admin account to get started
                </p>
              </div>
            </div>

            <div className="border-purple-rich/30 bg-purple-midnight/20 space-y-3 rounded-lg border p-4">
              {[
                ["Create Admin Account", "Set up your credentials for Loom"],
                [
                  "Configure Inside the App",
                  "Add libraries, indexers, and download clients from Settings once you're in",
                ],
              ].map(([title, desc]) => (
                <div key={title} className="flex items-start gap-3">
                  <CheckCircle2 className="text-teal-electric mt-0.5 h-5 w-5 flex-shrink-0" />
                  <div>
                    <p className="text-neutral-light font-medium">{title}</p>
                    <p className="text-neutral-muted text-sm">{desc}</p>
                  </div>
                </div>
              ))}
            </div>

            <Button
              onClick={() => goTo("credentials")}
              className="bg-purple-rich hover:bg-purple-midnight w-full py-2 font-medium text-white"
            >
              Get Started
            </Button>
          </div>
        )}

        {/* Credentials Step */}
        {step === "credentials" && (
          <div className="bg-neutral-card space-y-6 rounded-lg p-8 shadow-lg">
            <div className="space-y-2 text-center">
              <Lock className="text-teal-electric mx-auto h-8 w-8" />
              <h2 className="text-neutral-light text-2xl font-bold">
                Create Your Account
              </h2>
              <p className="text-neutral-muted text-sm">
                Set up your admin credentials to access Loom
              </p>
            </div>

            {error && (
              <Alert className="border-semantic-error/30 bg-semantic-error/10">
                <AlertCircle className="text-semantic-error h-4 w-4" />
                <AlertDescription className="text-semantic-error">
                  {error}
                </AlertDescription>
              </Alert>
            )}

            <div className="space-y-4">
              <div className="space-y-2">
                <label
                  htmlFor="setup-username"
                  className="text-neutral-light text-sm font-medium"
                >
                  Username
                </label>
                <Input
                  id="setup-username"
                  type="text"
                  placeholder="Enter username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={isLoading}
                  className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
                />
              </div>

              <div className="space-y-2">
                <label
                  htmlFor="setup-password"
                  className="text-neutral-light text-sm font-medium"
                >
                  Password
                </label>
                <Input
                  id="setup-password"
                  type="password"
                  placeholder="Enter password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={isLoading}
                  className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
                />
              </div>

              <p className="text-neutral-muted text-xs">
                Your credentials are encrypted and stored securely
              </p>
            </div>

            <div className="flex gap-3">
              <Button
                onClick={() => goTo("welcome")}
                variant="outline"
                className="flex-1"
                disabled={isLoading}
              >
                Back
              </Button>
              <Button
                onClick={completeSetup}
                disabled={isLoading || !username.trim() || !password.trim()}
                className="bg-teal-electric text-neutral-dark hover:bg-teal-ocean flex-1 font-medium"
              >
                {isLoading ? "Setting up..." : "Create Account"}
              </Button>
            </div>
          </div>
        )}

        {/* Complete Step */}
        {step === "complete" && (
          <div className="bg-neutral-card space-y-6 rounded-lg p-8 shadow-lg">
            <div className="space-y-2 text-center">
              <div className="flex justify-center">
                <CheckCircle2 className="text-semantic-success h-12 w-12" />
              </div>
              <h2 className="text-neutral-light text-2xl font-bold">
                All Set!
              </h2>
              <p className="text-neutral-muted">
                Your setup is complete. Redirecting...
              </p>
            </div>

            <div className="bg-neutral-dark h-1 w-full overflow-hidden rounded-full">
              <div className="from-teal-electric to-purple-rich h-full animate-pulse bg-gradient-to-r" />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
