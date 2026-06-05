import { useEffect, useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useAuth } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, Loader2 } from "lucide-react";
import { lookupInvite, acceptInvite } from "@/lib/invites-api";
import { ApiError } from "@/lib/users-api";

function ApiErrorMessage(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return fallback;
}

type Phase = "checking" | "invalid" | "ready" | "submitting" | "done";

export function InvitePage() {
  const { token } = useParams({ strict: false }) as { token?: string };
  const { isAuthenticated, user, logout } = useAuth();
  const [phase, setPhase] = useState<Phase>("checking");
  const [email, setEmail] = useState<string | undefined>();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    if (!token) {
      setPhase("invalid");
      return;
    }
    lookupInvite(token)
      .then((res) => {
        if (!active) return;
        if (res.valid) {
          setEmail(res.email);
          setPhase("ready");
        } else {
          setPhase("invalid");
        }
      })
      .catch(() => active && setPhase("invalid"));
    return () => {
      active = false;
    };
  }, [token]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!username.trim()) {
      setError("Choose a username");
      return;
    }
    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }
    if (password !== confirm) {
      setError("Passwords do not match");
      return;
    }
    setPhase("submitting");
    try {
      await acceptInvite(token as string, {
        username: username.trim(),
        password,
      });
      setPhase("done");
      // Full reload so the auth provider re-checks and lands authenticated.
      window.location.href = "/";
    } catch (err) {
      setError(ApiErrorMessage(err, "Could not create your account"));
      setPhase("ready");
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
                Join Loom
              </h1>
              <p className="text-neutral-muted">
                You&apos;ve been invited to create an account.
              </p>
            </div>
          </div>

          {isAuthenticated ? (
            <div className="space-y-4">
              <Alert>
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>
                  You&apos;re already signed in as{" "}
                  <span className="font-medium">{user?.username}</span>.
                  Accepting this invite creates a separate new account.
                </AlertDescription>
              </Alert>
              <div className="flex flex-col gap-2">
                <Button onClick={() => (window.location.href = "/")}>
                  Continue to Loom
                </Button>
                <Button
                  variant="outline"
                  onClick={async () => {
                    await logout();
                    window.location.reload();
                  }}
                >
                  Sign out to accept invite
                </Button>
              </div>
            </div>
          ) : (
            <>
              {phase === "checking" && (
                <div className="flex items-center justify-center gap-2 py-8 text-neutral-muted">
                  <Loader2 className="h-5 w-5 animate-spin" /> Checking your
                  invite…
                </div>
              )}

              {phase === "invalid" && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>
                    This invite link is invalid, has expired, or has already
                    been used. Ask your administrator for a new one.
                  </AlertDescription>
                </Alert>
              )}

              {(phase === "ready" || phase === "submitting") && (
                <form onSubmit={handleSubmit} className="space-y-4">
                  {error && (
                    <Alert variant="destructive">
                      <AlertCircle className="h-4 w-4" />
                      <AlertDescription>{error}</AlertDescription>
                    </Alert>
                  )}
                  {email && (
                    <p className="text-sm text-neutral-muted">
                      Invited as <span className="font-medium">{email}</span>
                    </p>
                  )}
                  <div className="space-y-1.5">
                    <label
                      htmlFor="invite-username"
                      className="text-sm text-neutral-light"
                    >
                      Username
                    </label>
                    <Input
                      id="invite-username"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoComplete="username"
                      disabled={phase === "submitting"}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label
                      htmlFor="invite-password"
                      className="text-sm text-neutral-light"
                    >
                      Password
                    </label>
                    <Input
                      id="invite-password"
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="new-password"
                      disabled={phase === "submitting"}
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label
                      htmlFor="invite-confirm"
                      className="text-sm text-neutral-light"
                    >
                      Confirm password
                    </label>
                    <Input
                      id="invite-confirm"
                      type="password"
                      value={confirm}
                      onChange={(e) => setConfirm(e.target.value)}
                      autoComplete="new-password"
                      disabled={phase === "submitting"}
                    />
                  </div>
                  <Button
                    type="submit"
                    className="w-full"
                    disabled={phase === "submitting"}
                  >
                    {phase === "submitting" ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />{" "}
                        Creating account…
                      </>
                    ) : (
                      "Create account"
                    )}
                  </Button>
                </form>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
