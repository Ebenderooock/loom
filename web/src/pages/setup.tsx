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
  AlertCircle, CheckCircle2, Lock, Copy, Check,
  FolderOpen, Rss, Download, Trash2, Loader2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { createLibrary, deleteLibrary, type Library, MEDIA_TYPES } from "@/lib/libraries-api";

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
  "libraries": "Libraries",
  indexers: "Indexers",
  "download-clients": "Downloads",
  complete: "Done",
};

const CLIENT_TYPES = ["qbittorrent", "sabnzbd", "nzbget", "deluge", "transmission"] as const;

// ─── Step Indicator ──────────────────────────────────────────────────

function StepIndicator({ current }: { current: SetupStep }) {
  const idx = STEPS.indexOf(current);
  return (
    <div className="flex items-center justify-center gap-1.5 mb-6">
      {STEPS.map((s, i) => (
        <div key={s} className="flex items-center gap-1.5">
          <div
            className={cn(
              "w-2.5 h-2.5 rounded-full transition-colors",
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
    <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
      <div className="text-center space-y-2">
        <FolderOpen className="w-8 h-8 text-teal-electric mx-auto" />
        <h2 className="text-2xl font-bold text-neutral-light">Libraries</h2>
        <p className="text-neutral-muted text-sm">
          Add libraries where your movie/series collections live
        </p>
      </div>

      {error && (
        <Alert className="border-semantic-error/30 bg-semantic-error/10">
          <AlertCircle className="h-4 w-4 text-semantic-error" />
          <AlertDescription className="text-semantic-error">{error}</AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        <div className="flex gap-2">
          <Input
            placeholder="Library name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
          <Select value={mediaType} onValueChange={setMediaType}>
            <SelectTrigger className="w-32 bg-neutral-dark border-purple-rich/30 text-neutral-light">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {MEDIA_TYPES.map(mt => (
                <SelectItem key={mt.value} value={mt.value}>{mt.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex gap-2">
          <Input
            placeholder="/mnt/media/movies"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
            onKeyDown={(e) => { if (e.key === "Enter") addLib(); }}
          />
          <Button
            onClick={addLib}
            disabled={adding || !name.trim() || !path.trim()}
            className="bg-teal-electric hover:bg-teal-ocean text-neutral-dark shrink-0"
          >
            {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : "Add Library"}
          </Button>
        </div>

        {libraries.length > 0 && (
          <div className="space-y-1.5">
            {libraries.map((lib) => (
              <div
                key={lib.id}
                className="flex items-center gap-2 bg-neutral-dark/50 rounded px-3 py-2 text-sm text-neutral-light"
              >
                <FolderOpen className="w-4 h-4 text-teal-electric shrink-0" />
                <span className="flex-1 truncate">{lib.name} — {lib.path}</span>
                <span className="text-xs text-neutral-muted capitalize">{lib.media_type}</span>
                <button
                  onClick={() => removeLib(lib.id)}
                  className="text-neutral-muted hover:text-semantic-error transition-colors"
                >
                  <Trash2 className="w-3.5 h-3.5" />
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
        <Button
          onClick={onNext}
          variant="outline"
          className="flex-1"
        >
          Skip
        </Button>
        <Button
          onClick={onNext}
          className="flex-1 bg-teal-electric hover:bg-teal-ocean text-neutral-dark font-medium"
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
    <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
      <div className="text-center space-y-2">
        <Rss className="w-8 h-8 text-teal-electric mx-auto" />
        <h2 className="text-2xl font-bold text-neutral-light">Indexers</h2>
        <p className="text-neutral-muted text-sm">
          Add indexers to search for releases
        </p>
      </div>

      {error && (
        <Alert className="border-semantic-error/30 bg-semantic-error/10">
          <AlertCircle className="h-4 w-4 text-semantic-error" />
          <AlertDescription className="text-semantic-error">{error}</AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        <Input
          placeholder="Indexer name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Input
          placeholder="URL (e.g. https://indexer.example.com)"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Input
          placeholder="API Key"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Button
          onClick={addIndexer}
          disabled={adding || !name.trim() || !url.trim()}
          className="w-full bg-teal-electric hover:bg-teal-ocean text-neutral-dark"
        >
          {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : "Add Indexer"}
        </Button>

        {indexers.length > 0 && (
          <div className="space-y-1.5">
            {indexers.map((idx) => (
              <div
                key={idx.id}
                className="flex items-center gap-2 bg-neutral-dark/50 rounded px-3 py-2 text-sm text-neutral-light"
              >
                <Rss className="w-4 h-4 text-teal-electric shrink-0" />
                <span className="flex-1 truncate">{idx.name}</span>
                <span className="text-xs text-neutral-muted truncate max-w-[140px]">{idx.url}</span>
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
          className="flex-1 bg-teal-electric hover:bg-teal-ocean text-neutral-dark font-medium"
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
    <div className="bg-neutral-card rounded-lg shadow-lg p-8 space-y-6">
      <div className="text-center space-y-2">
        <Download className="w-8 h-8 text-teal-electric mx-auto" />
        <h2 className="text-2xl font-bold text-neutral-light">Download Client</h2>
        <p className="text-neutral-muted text-sm">
          Configure a download client for grabbing releases
        </p>
      </div>

      {error && (
        <Alert className="border-semantic-error/30 bg-semantic-error/10">
          <AlertCircle className="h-4 w-4 text-semantic-error" />
          <AlertDescription className="text-semantic-error">{error}</AlertDescription>
        </Alert>
      )}

      <div className="space-y-3">
        <Input
          placeholder="Client name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
        />
        <Select value={type} onValueChange={setType}>
          <SelectTrigger className="bg-neutral-dark border-purple-rich/30 text-neutral-light">
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
            className="col-span-2 bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
          <Input
            placeholder="Port"
            type="number"
            value={port}
            onChange={(e) => setPort(e.target.value)}
            className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Input
            placeholder="Username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
          <Input
            placeholder="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="bg-neutral-dark border-purple-rich/30 text-neutral-light placeholder:text-neutral-muted focus:border-teal-electric"
          />
        </div>
        <Button
          onClick={addClient}
          disabled={adding || !name.trim() || !host.trim()}
          className="w-full bg-teal-electric hover:bg-teal-ocean text-neutral-dark"
        >
          {adding ? <Loader2 className="w-4 h-4 animate-spin" /> : "Add Client"}
        </Button>

        {clients.length > 0 && (
          <div className="space-y-1.5">
            {clients.map((c) => (
              <div
                key={c.id}
                className="flex items-center gap-2 bg-neutral-dark/50 rounded px-3 py-2 text-sm text-neutral-light"
              >
                <Download className="w-4 h-4 text-teal-electric shrink-0" />
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
          className="flex-1 bg-teal-electric hover:bg-teal-ocean text-neutral-dark font-medium"
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
    <div className="w-full min-h-screen flex items-center justify-center bg-gradient-to-br from-purple-midnight via-neutral-dark to-teal-deep py-8">
      <div className="w-full max-w-md mx-auto px-4">
        <StepIndicator current={step} />

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
              {[
                ["Create Admin Account", "Set up your credentials for Loom"],
                ["Generate API Key", "Get an API key for integrations"],
                ["Add Libraries", "Point to your media libraries"],
                ["Configure Indexers", "Add sources to search for content"],
                ["Download Client", "Set up a download client"],
              ].map(([title, desc]) => (
                <div key={title} className="flex items-start gap-3">
                  <CheckCircle2 className="w-5 h-5 text-teal-electric mt-0.5 flex-shrink-0" />
                  <div>
                    <p className="font-medium text-neutral-light">{title}</p>
                    <p className="text-sm text-neutral-muted">{desc}</p>
                  </div>
                </div>
              ))}
            </div>

            <Button
              onClick={() => goTo("credentials")}
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
              onClick={() => goTo("libraries")}
              className="w-full bg-teal-electric hover:bg-teal-ocean text-neutral-dark font-medium"
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
