import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useAuth } from "@/hooks/use-auth";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertCircle,
  CheckCircle2,
  Lock,
  Copy,
  Check,
  FolderOpen,
  Rss,
  Download,
  Trash2,
  Loader2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import {
  createLibrary,
  deleteLibrary,
  type Library,
  MEDIA_TYPES,
} from "@/lib/libraries-api";

type SetupStep =
  | "welcome"
  | "credentials"
  | "display-key"
  | "libraries"
  | "indexers"
  | "download-clients"
  | "complete";

const STEPS: SetupStep[] = [
  "welcome",
  "credentials",
  "display-key",
  "libraries",
  "indexers",
  "download-clients",
  "complete",
];

const STEP_LABELS: Record<SetupStep, string> = {
  welcome: "Welcome",
  credentials: "Account",
  "display-key": "API Key",
  libraries: "Libraries",
  indexers: "Indexers",
  "download-clients": "Downloads",
  complete: "Done",
};

const CLIENT_TYPES = [
  "qbittorrent",
  "sabnzbd",
  "nzbget",
  "deluge",
  "transmission",
] as const;

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
                  ? "bg-purple-rich ring-2 ring-purple-rich/40"
                  : "bg-neutral-muted/30",
            )}
            title={STEP_LABELS[s]}
          />
        </div>
      ))}
    </div>
  );
}

// ─── Libraries Step ────────────────────────────────────────────────

function LibrariesStep({
  onNext,
  onBack,
}: {
  onNext: () => void;
  onBack: () => void;
}) {
  const [libraries, setLibraries] = useState<Library[]>([]);
  const [name, setName] = useState("");
  const [path, setPath] = useState("");
  const [mediaType, setMediaType] = useState<string>("movie");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addLib = async () => {
    if (!name.trim() || !path.trim()) return;
    setAdding(true);
    setError(null);
    try {
      const created = await createLibrary({
        name: name.trim(),
        path: path.trim(),
        media_type: mediaType as "movie" | "series" | "music",
      });
      setLibraries((prev) => [...prev, created]);
      setName("");
      setPath("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add library");
    } finally {
      setAdding(false);
    }
  };

  const removeLib = async (id: string) => {
    try {
      await deleteLibrary(id);
      setLibraries((prev) => prev.filter((l) => l.id !== id));
    } catch {
      /* ignore */
    }
  };

  return (
    <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
      <div className="space-y-2 text-center">
        <FolderOpen className="mx-auto h-8 w-8 text-teal-electric" />
        <h2 className="text-2xl font-bold text-neutral-light">Libraries</h2>
        <p className="text-sm text-neutral-muted">
          Add libraries where your movie/series collections live
        </p>
      </div>

      {error && (
        <Alert className="border-semantic-error/30 bg-semantic-error/10">
          <AlertCircle className="h-4 w-4 text-semantic-error" />
          <AlertDescription className="text-semantic-error">
            {error}
          </AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        <div className="flex gap-2">
          <Input
            placeholder="Library name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
          <Select value={mediaType} onValueChange={setMediaType}>
            <SelectTrigger className="w-32 border-purple-rich/30 bg-neutral-dark text-neutral-light">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {MEDIA_TYPES.map((mt) => (
                <SelectItem key={mt.value} value={mt.value}>
                  {mt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex gap-2">
          <Input
            placeholder="/mnt/media/movies"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
            onKeyDown={(e) => {
              if (e.key === "Enter") addLib();
            }}
          />
          <Button
            onClick={addLib}
            disabled={adding || !name.trim() || !path.trim()}
            className="shrink-0 bg-teal-electric text-neutral-dark hover:bg-teal-ocean"
          >
            {adding ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              "Add Library"
            )}
          </Button>
        </div>

        {libraries.length > 0 && (
          <div className="space-y-1.5">
            {libraries.map((lib) => (
              <div
                key={lib.id}
                className="flex items-center gap-2 rounded bg-neutral-dark/50 px-3 py-2 text-sm text-neutral-light"
              >
                <FolderOpen className="h-4 w-4 shrink-0 text-teal-electric" />
                <span className="flex-1 truncate">
                  {lib.name} — {lib.path}
                </span>
                <span className="text-xs capitalize text-neutral-muted">
                  {lib.media_type}
                </span>
                <button
                  onClick={() => removeLib(lib.id)}
                  className="text-neutral-muted transition-colors hover:text-semantic-error"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex gap-3">
        <Button onClick={onBack} variant="outline" className="flex-1">
          Back
        </Button>
        <Button onClick={onNext} variant="outline" className="flex-1">
          Skip
        </Button>
        <Button
          onClick={onNext}
          className="flex-1 bg-teal-electric font-medium text-neutral-dark hover:bg-teal-ocean"
        >
          Next
        </Button>
      </div>
    </div>
  );
}

// ─── Indexers Step ─────────────────────────────────────────────────────

interface IndexerItem {
  id: string;
  name: string;
  url: string;
}

function IndexersStep({
  onNext,
  onBack,
}: {
  onNext: () => void;
  onBack: () => void;
}) {
  const [indexers, setIndexers] = useState<IndexerItem[]>([]);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addIndexer = async () => {
    if (!name.trim() || !url.trim()) return;
    setAdding(true);
    setError(null);
    try {
      const res = await apiFetch("/api/v1/indexers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name.trim(),
          url: url.trim(),
          api_key: apiKey.trim(),
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to add indexer");
      }
      const created = await res.json();
      setIndexers((prev) => [...prev, created]);
      setName("");
      setUrl("");
      setApiKey("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add indexer");
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
      <div className="space-y-2 text-center">
        <Rss className="mx-auto h-8 w-8 text-teal-electric" />
        <h2 className="text-2xl font-bold text-neutral-light">Indexers</h2>
        <p className="text-sm text-neutral-muted">
          Add indexers to search for releases
        </p>
      </div>

      {error && (
        <Alert className="border-semantic-error/30 bg-semantic-error/10">
          <AlertCircle className="h-4 w-4 text-semantic-error" />
          <AlertDescription className="text-semantic-error">
            {error}
          </AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        <Input
          placeholder="Indexer name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Input
          placeholder="URL (e.g. https://indexer.example.com)"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Input
          placeholder="API Key"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Button
          onClick={addIndexer}
          disabled={adding || !name.trim() || !url.trim()}
          className="w-full bg-teal-electric text-neutral-dark hover:bg-teal-ocean"
        >
          {adding ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            "Add Indexer"
          )}
        </Button>

        {indexers.length > 0 && (
          <div className="space-y-1.5">
            {indexers.map((idx) => (
              <div
                key={idx.id}
                className="flex items-center gap-2 rounded bg-neutral-dark/50 px-3 py-2 text-sm text-neutral-light"
              >
                <Rss className="h-4 w-4 shrink-0 text-teal-electric" />
                <span className="flex-1 truncate">{idx.name}</span>
                <span className="max-w-[140px] truncate text-xs text-neutral-muted">
                  {idx.url}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex gap-3">
        <Button onClick={onBack} variant="outline" className="flex-1">
          Back
        </Button>
        <Button onClick={onNext} variant="outline" className="flex-1">
          Skip
        </Button>
        <Button
          onClick={onNext}
          className="flex-1 bg-teal-electric font-medium text-neutral-dark hover:bg-teal-ocean"
        >
          Next
        </Button>
      </div>
    </div>
  );
}

// ─── Download Clients Step ────────────────────────────────────────────

interface DownloadClientItem {
  id: string;
  name: string;
  type: string;
}

function DownloadClientsStep({
  onNext,
  onBack,
}: {
  onNext: () => void;
  onBack: () => void;
}) {
  const [clients, setClients] = useState<DownloadClientItem[]>([]);
  const [name, setName] = useState("");
  const [type, setType] = useState<string>(CLIENT_TYPES[0]);
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [adding, setAdding] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const addClient = async () => {
    if (!name.trim() || !host.trim()) return;
    setAdding(true);
    setError(null);
    try {
      const res = await apiFetch("/api/v1/download-clients", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name.trim(),
          type,
          host: host.trim(),
          port: port ? Number(port) : undefined,
          username: username.trim() || undefined,
          password: password || undefined,
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Failed to add download client");
      }
      const created = await res.json();
      setClients((prev) => [...prev, created]);
      setName("");
      setHost("");
      setPort("");
      setUsername("");
      setPassword("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add client");
    } finally {
      setAdding(false);
    }
  };

  return (
    <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
      <div className="space-y-2 text-center">
        <Download className="mx-auto h-8 w-8 text-teal-electric" />
        <h2 className="text-2xl font-bold text-neutral-light">
          Download Client
        </h2>
        <p className="text-sm text-neutral-muted">
          Configure a download client for grabbing releases
        </p>
      </div>

      {error && (
        <Alert className="border-semantic-error/30 bg-semantic-error/10">
          <AlertCircle className="h-4 w-4 text-semantic-error" />
          <AlertDescription className="text-semantic-error">
            {error}
          </AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        <Input
          placeholder="Client name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Select value={type} onValueChange={setType}>
          <SelectTrigger className="border-purple-rich/30 bg-neutral-dark text-neutral-light">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CLIENT_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="grid grid-cols-3 gap-2">
          <Input
            placeholder="Host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            className="col-span-2 border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
          <Input
            placeholder="Port"
            type="number"
            value={port}
            onChange={(e) => setPort(e.target.value)}
            className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Input
            placeholder="Username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
          <Input
            placeholder="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
        </div>
        <Button
          onClick={addClient}
          disabled={adding || !name.trim() || !host.trim()}
          className="w-full bg-teal-electric text-neutral-dark hover:bg-teal-ocean"
        >
          {adding ? <Loader2 className="h-4 w-4 animate-spin" /> : "Add Client"}
        </Button>

        {clients.length > 0 && (
          <div className="space-y-1.5">
            {clients.map((c) => (
              <div
                key={c.id}
                className="flex items-center gap-2 rounded bg-neutral-dark/50 px-3 py-2 text-sm text-neutral-light"
              >
                <Download className="h-4 w-4 shrink-0 text-teal-electric" />
                <span className="flex-1 truncate">{c.name}</span>
                <span className="text-xs text-neutral-muted">{c.type}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="flex gap-3">
        <Button onClick={onBack} variant="outline" className="flex-1">
          Back
        </Button>
        <Button onClick={onNext} variant="outline" className="flex-1">
          Skip
        </Button>
        <Button
          onClick={onNext}
          className="flex-1 bg-teal-electric font-medium text-neutral-dark hover:bg-teal-ocean"
        >
          Next
        </Button>
      </div>
    </div>
  );
}

// ─── Main Setup Page ──────────────────────────────────────────────────

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
          email: email.trim(),
        }),
      });

      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || "Failed to complete setup");
      }

      const data = await response.json();
      setGeneratedApiKey(data.api_key);
      goTo("display-key");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to complete setup");
    } finally {
      setIsLoading(false);
    }
  };

  const handleFinalComplete = async () => {
    goTo("complete");
    await refreshAuth();
    setTimeout(() => {
      navigate({ to: "/" });
    }, 1500);
  };

  return (
    <div className="flex min-h-screen w-full items-center justify-center bg-gradient-to-br from-purple-midnight via-neutral-dark to-teal-deep py-8">
      <div className="mx-auto w-full max-w-md px-4">
        <StepIndicator current={step} />

        {/* Welcome Step */}
        {step === "welcome" && (
          <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
            <div className="space-y-4 text-center">
              <img
                src="/loom-logo.png"
                alt="Loom Logo"
                className="mx-auto h-16 object-contain"
              />
              <div className="space-y-2">
                <h1 className="text-3xl font-bold text-neutral-light">
                  Welcome to Loom
                </h1>
                <p className="text-neutral-muted">
                  Let's get you set up in just a few steps
                </p>
              </div>
            </div>

            <div className="space-y-3 rounded-lg border border-purple-rich/30 bg-purple-midnight/20 p-4">
              {[
                ["Create Admin Account", "Set up your credentials for Loom"],
                ["Generate API Key", "Get an API key for integrations"],
                ["Add Libraries", "Point to your media libraries"],
                ["Configure Indexers", "Add sources to search for content"],
                ["Download Client", "Set up a download client"],
              ].map(([title, desc]) => (
                <div key={title} className="flex items-start gap-3">
                  <CheckCircle2 className="mt-0.5 h-5 w-5 flex-shrink-0 text-teal-electric" />
                  <div>
                    <p className="font-medium text-neutral-light">{title}</p>
                    <p className="text-sm text-neutral-muted">{desc}</p>
                  </div>
                </div>
              ))}
            </div>

            <Button
              onClick={() => goTo("credentials")}
              className="w-full bg-purple-rich py-2 font-medium text-white hover:bg-purple-midnight"
            >
              Get Started
            </Button>
          </div>
        )}

        {/* Credentials Step */}
        {step === "credentials" && (
          <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
            <div className="space-y-2 text-center">
              <Lock className="mx-auto h-8 w-8 text-teal-electric" />
              <h2 className="text-2xl font-bold text-neutral-light">
                Create Your Account
              </h2>
              <p className="text-sm text-neutral-muted">
                Set up your admin credentials to access Loom
              </p>
            </div>

            {error && (
              <Alert className="border-semantic-error/30 bg-semantic-error/10">
                <AlertCircle className="h-4 w-4 text-semantic-error" />
                <AlertDescription className="text-semantic-error">
                  {error}
                </AlertDescription>
              </Alert>
            )}

            <div className="space-y-4">
              <div className="space-y-2">
                <label
                  htmlFor="setup-username"
                  className="text-sm font-medium text-neutral-light"
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
                  className="text-sm font-medium text-neutral-light"
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

              <div className="space-y-2">
                <label
                  htmlFor="setup-email"
                  className="text-sm font-medium text-neutral-light"
                >
                  Email (Optional)
                </label>
                <Input
                  id="setup-email"
                  type="email"
                  placeholder="Enter email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  disabled={isLoading}
                  className="border-purple-rich/30 bg-neutral-dark text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
                />
              </div>

              <p className="text-xs text-neutral-muted">
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
                className="flex-1 bg-teal-electric font-medium text-neutral-dark hover:bg-teal-ocean"
              >
                {isLoading ? "Setting up..." : "Create Account"}
              </Button>
            </div>
          </div>
        )}

        {/* Display API Key Step */}
        {step === "display-key" && (
          <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
            <div className="space-y-2 text-center">
              <div className="flex justify-center">
                <CheckCircle2 className="h-12 w-12 text-semantic-success" />
              </div>
              <h2 className="text-2xl font-bold text-neutral-light">
                Account Created!
              </h2>
              <p className="text-sm text-neutral-muted">
                Save your API key for integrations and API access
              </p>
            </div>

            <div className="space-y-3 rounded-lg border border-teal-electric/30 bg-neutral-dark p-4">
              <div className="flex items-center justify-between">
                <p className="text-xs font-medium uppercase tracking-wide text-neutral-muted">
                  API Key
                </p>
                <button
                  onClick={() => {
                    navigator.clipboard.writeText(generatedApiKey);
                    setCopied(true);
                    setTimeout(() => setCopied(false), 2000);
                  }}
                  className="inline-flex items-center gap-2 rounded bg-teal-electric/10 px-2 py-1 text-xs text-teal-electric transition-colors hover:bg-teal-electric/20"
                >
                  {copied ? (
                    <>
                      <Check className="h-3 w-3" />
                      Copied!
                    </>
                  ) : (
                    <>
                      <Copy className="h-3 w-3" />
                      Copy
                    </>
                  )}
                </button>
              </div>
              <code className="block break-all rounded bg-neutral-dark/50 p-2 font-mono text-sm text-neutral-light">
                {generatedApiKey}
              </code>
              <p className="text-xs text-semantic-warning">
                ⚠️ Save this key in a secure location. You won't be able to see
                it again.
              </p>
            </div>

            <Alert className="border-semantic-info/30 bg-semantic-info/10">
              <AlertCircle className="h-4 w-4 text-semantic-info" />
              <AlertDescription className="text-sm text-semantic-info">
                Use this API key as{" "}
                <code className="rounded bg-neutral-dark px-1">X-API-Key</code>{" "}
                header for API requests
              </AlertDescription>
            </Alert>

            <Button
              onClick={() => goTo("libraries")}
              className="w-full bg-teal-electric font-medium text-neutral-dark hover:bg-teal-ocean"
            >
              Continue Setup
            </Button>
          </div>
        )}

        {/* Libraries Step */}
        {step === "libraries" && (
          <LibrariesStep
            onNext={() => goTo("indexers")}
            onBack={() => goTo("display-key")}
          />
        )}

        {/* Indexers Step */}
        {step === "indexers" && (
          <IndexersStep
            onNext={() => goTo("download-clients")}
            onBack={() => goTo("libraries")}
          />
        )}

        {/* Download Clients Step */}
        {step === "download-clients" && (
          <DownloadClientsStep
            onNext={handleFinalComplete}
            onBack={() => goTo("indexers")}
          />
        )}

        {/* Complete Step */}
        {step === "complete" && (
          <div className="space-y-6 rounded-lg bg-neutral-card p-8 shadow-lg">
            <div className="space-y-2 text-center">
              <div className="flex justify-center">
                <CheckCircle2 className="h-12 w-12 text-semantic-success" />
              </div>
              <h2 className="text-2xl font-bold text-neutral-light">
                All Set!
              </h2>
              <p className="text-neutral-muted">
                Your setup is complete. Redirecting...
              </p>
            </div>

            <div className="h-1 w-full overflow-hidden rounded-full bg-neutral-dark">
              <div className="h-full animate-pulse bg-gradient-to-r from-teal-electric to-purple-rich" />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
