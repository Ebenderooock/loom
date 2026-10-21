import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiFetch } from "@/lib/fetch";

interface PairRequest {
  user_code: string;
  device_name: string;
  status: string;
  expires_in: number;
}

// Keep API calls under the same reverse-proxy prefix as the approval page.
export function pairingAPI(path: string): string {
  const base = window.location.pathname.replace(/\/pair\/?$/, "");
  return `${base}/api/v1/auth/pairing/${path}`;
}

export function PairPage() {
  const [code, setCode] = useState(
    () => new URLSearchParams(window.location.search).get("code") ?? "",
  );
  const [request, setRequest] = useState<PairRequest | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState("");

  useEffect(() => {
    // No approval on load: even a valid link requires reviewing the device and
    // typing the exact code displayed by the initiating TV.
    document.title = "Pair your TV · Loom";
  }, []);

  async function lookup(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setRequest(null);
    try {
      const response = await apiFetch(
        pairingAPI(`request?code=${encodeURIComponent(code)}`),
      );
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Unable to find pairing");
      if (data.status !== "pending")
        throw new Error(
          "This request was already decided. Start again on your TV.",
        );
      setRequest(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to find pairing");
    } finally {
      setBusy(false);
    }
  }

  async function decide(decision: "approve" | "deny") {
    if (!request || code !== request.user_code) return;
    setBusy(true);
    setError("");
    try {
      const response = await apiFetch(pairingAPI("decision"), {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Loom-Pairing": "1",
        },
        body: JSON.stringify({ user_code: request.user_code, decision }),
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "Pairing failed");
      setResult(
        decision === "approve"
          ? "Approved. Your TV will finish connecting. You can revoke its Loom TV key in Settings."
          : "Denied. No API key was issued.",
      );
      setRequest(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Pairing failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="mx-auto max-w-lg space-y-6 p-6">
      <h1 className="text-2xl font-semibold">Pair your TV</h1>
      <p className="text-muted-foreground">
        Approve only a request you started on your own TV. This grants that TV
        access to Loom as your account. Never approve a code sent by someone
        else.
      </p>
      {result ? (
        <p role="status">{result}</p>
      ) : (
        <>
          <form onSubmit={lookup} className="space-y-3">
            <label htmlFor="pair-code" className="block font-medium">
              Code shown on your TV
            </label>
            <Input
              id="pair-code"
              value={code}
              onChange={(event) => {
                setCode(event.target.value);
                setRequest(null);
              }}
              autoComplete="off"
              spellCheck={false}
              maxLength={9}
              placeholder="ABCD-2345"
              required
            />
            <Button type="submit" disabled={busy}>
              {busy ? "Please wait…" : "Review request"}
            </Button>
          </form>
          {request && (
            <section className="space-y-4 rounded-lg border p-4">
              <h2 className="text-lg font-semibold">{request.device_name}</h2>
              <p className="font-mono text-2xl">{request.user_code}</p>
              <p>
                Check that this exact code is visible on your TV. Request
                expires in about {Math.ceil(request.expires_in / 60)} minutes.
              </p>
              <div className="flex flex-wrap gap-3">
                <Button disabled={busy} onClick={() => decide("approve")}>
                  Approve my TV
                </Button>
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => decide("deny")}
                >
                  Deny request
                </Button>
              </div>
            </section>
          )}
        </>
      )}
      {error && <p role="alert">{error}</p>}
    </main>
  );
}
