import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, CheckCircle2, Lock, Copy, Check } from "lucide-react";

type SetupStep = "welcome" | "credentials" | "display-key" | "complete";

export function SetupPage() {
  const [step, setStep] = useState<SetupStep>("welcome");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [email, setEmail] = useState("");
  const [generatedApiKey, setGeneratedApiKey] = useState("");
  const [copied, setCopied] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { refreshAuth } = useAuth();

  const completeSetup = async () => {
    if (!username.trim() || !password.trim()) {
      setError("Username and password are required");
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      const response = await fetch("/api/v1/auth/initialize", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          username: username.trim(),
          password,
          email: email.trim(),
        }),
      });

      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || "Failed to complete setup");
      }

      const data = await response.json();
      setGeneratedApiKey(data.api_key);
      setStep("display-key");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to complete setup");
    } finally {
      setIsLoading(false);
    }
  };

  const handleCompleteSetup = async () => {
    setStep("complete");
    // Refresh auth state - will pick up the session cookie from initialize
    await refreshAuth();
    // Navigate to dashboard after a short delay for UX
    setTimeout(() => {
      navigate({ to: "/" });
    }, 1500);
  };

  return (
    <div className="w-full h-screen flex items-center justify-center bg-gradient-to-br from-purple-midnight via-neutral-dark to-teal-deep">
      <div className="w-full max-w-md mx-auto px-4">
        {/* Welcome Step */}
        {step === "welcome" && (
          <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
            <div className="text-center space-y-4">
              <img 
                src="/loom-logo.png" 
                alt="Loom Logo" 
                className="h-16 mx-auto object-contain"
              />
              <div className="space-y-2">
                <h1 className="text-3xl font-bold text-neutral-light">Welcome to Loom</h1>
                <p className="text-neutral-muted">Let's get you set up in just a few steps</p>
              </div>
            </div>

            <div className="bg-purple-midnight/20 border border-purple-rich/30 rounded-lg p-4 space-y-3">
              <div className="flex items-start gap-3">
                <CheckCircle2 className="w-5 h-5 text-teal-electric mt-0.5 flex-shrink-0" />
                <div>
                  <p className="font-medium text-neutral-light">Create Admin Account</p>
                  <p className="text-sm text-neutral-muted">Set up your credentials for Loom</p>
                </div>
              </div>
              <div className="flex items-start gap-3">
                <CheckCircle2 className="w-5 h-5 text-teal-electric mt-0.5 flex-shrink-0" />
                <div>
                  <p className="font-medium text-neutral-light">Generate API Key</p>
                  <p className="text-sm text-neutral-muted">Get an API key for integrations</p>
                </div>
              </div>
            </div>

            <Button
              onClick={() => setStep("credentials")}
              className="w-full bg-purple-rich hover:bg-purple-midnight text-white font-medium py-2"
            >
              Get Started
            </Button>
          </div>
        )}

        {/* Credentials Step */}
        {step === "credentials" && (
          <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
            <div className="text-center space-y-2">
              <Lock className="w-8 h-8 text-teal-electric mx-auto" />
              <h2 className="text-2xl font-bold text-neutral-light">Create Your Account</h2>
              <p className="text-neutral-muted text-sm">
                Set up your admin credentials to access Loom
              </p>
            </div>

            {error && (
              <Alert className="border-semantic-error/30 bg-semantic-error/10">
                <AlertCircle className="h-4 w-4 text-semantic-error" />
                <AlertDescription className="text-semantic-error">{error}</AlertDescription>
              </Alert>
            )}

            <div className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium text-neutral-light">Username</label>
                <Input
                  type="text"
                  placeholder="Enter username"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  disabled={isLoading}
                  className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-neutral-light">Password</label>
                <Input
                  type="password"
                  placeholder="Enter password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  disabled={isLoading}
                  className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
                />
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium text-neutral-light">Email (Optional)</label>
                <Input
                  type="email"
                  placeholder="Enter email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  disabled={isLoading}
                  className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
                />
              </div>

              <p className="text-xs text-neutral-muted">
                Your credentials are encrypted and stored securely
              </p>
            </div>

            <div className="flex gap-3">
              <Button
                onClick={() => {
                  setStep("welcome");
                  setError(null);
                }}
                variant="outline"
                className="flex-1"
                disabled={isLoading}
              >
                Back
              </Button>
              <Button
                onClick={completeSetup}
                disabled={isLoading || !username.trim() || !password.trim()}
                className="flex-1 bg-teal-electric hover:bg-teal-ocean text-neutral-dark font-medium"
              >
                {isLoading ? "Setting up..." : "Create Account"}
              </Button>
            </div>
          </div>
        )}

        {/* Display API Key Step */}
        {step === "display-key" && (
          <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
            <div className="text-center space-y-2">
              <div className="flex justify-center">
                <CheckCircle2 className="w-12 h-12 text-semantic-success" />
              </div>
              <h2 className="text-2xl font-bold text-neutral-light">Account Created!</h2>
              <p className="text-neutral-muted text-sm">
                Save your API key for integrations and API access
              </p>
            </div>

            <div className="bg-neutral-dark rounded-lg p-4 border border-teal-electric/30 space-y-3">
              <div className="flex items-center justify-between">
                <p className="text-xs font-medium text-neutral-muted uppercase tracking-wide">API Key</p>
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(generatedApiKey);
                    setCopied(true);
                    setTimeout(() => setCopied(false), 2000);
                  }}
                  className="inline-flex items-center gap-2 text-xs px-2 py-1 rounded bg-teal-electric/10 text-teal-electric hover:bg-teal-electric/20 transition-colors"
                >
                  {copied ? (
                    <>
                      <Check className="w-3 h-3" />
                      Copied!
                    </>
                  ) : (
                    <>
                      <Copy className="w-3 h-3" />
                      Copy
                    </>
                  )}
                </button>
              </div>
              <code className="block text-sm text-neutral-light font-mono break-all bg-neutral-dark/50 p-2 rounded">
                {generatedApiKey}
              </code>
              <p className="text-xs text-semantic-warning">
                ⚠️ Save this key in a secure location. You won't be able to see it again.
              </p>
            </div>

            <Alert className="border-semantic-info/30 bg-semantic-info/10">
              <AlertCircle className="h-4 w-4 text-semantic-info" />
              <AlertDescription className="text-semantic-info text-sm">
                Use this API key as <code className="bg-neutral-dark px-1 rounded">X-API-Key</code> header for API requests
              </AlertDescription>
            </Alert>

            <Button
              onClick={handleCompleteSetup}
              className="w-full bg-teal-electric hover:bg-teal-ocean text-neutral-dark font-medium"
            >
              Continue to Dashboard
            </Button>
          </div>
        )}

        {/* Complete Step */}
        {step === "complete" && (
          <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
            <div className="text-center space-y-2">
              <div className="flex justify-center">
                <CheckCircle2 className="w-12 h-12 text-semantic-success" />
              </div>
              <h2 className="text-2xl font-bold text-neutral-light">All Set!</h2>
              <p className="text-neutral-muted">Your setup is complete. Redirecting...</p>
            </div>

            <div className="w-full bg-neutral-dark rounded-full h-1 overflow-hidden">
              <div className="h-full bg-gradient-to-r from-teal-electric to-purple-rich animate-pulse" />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
