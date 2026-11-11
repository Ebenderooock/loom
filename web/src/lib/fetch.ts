export interface AcquisitionDecision {
  history_id: string;
  title: string;
  status: string;
  retry_allowed: boolean;
}

type RecoveryHandler = (decision: AcquisitionDecision) => Promise<boolean>;
let recoveryHandler: RecoveryHandler | undefined;

export function registerAcquisitionRecovery(handler: RecoveryHandler) {
  recoveryHandler = handler;
  return () => {
    if (recoveryHandler === handler) recoveryHandler = undefined;
  };
}

/**
 * Project-wide fetch wrapper that always includes credentials.
 * Use this instead of raw fetch() to ensure auth cookies are sent.
 */
export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  // Device approval can be mounted below a reverse-proxy prefix. The rest of
  // the UI retains its existing root deployment behavior.
  if (
    typeof input === "string" &&
    input.startsWith("/api/") &&
    /\/pair\/?$/.test(window.location.pathname)
  ) {
    input = window.location.pathname.replace(/\/pair\/?$/, "") + input;
  }
  const response = await fetch(input, { ...init, credentials: "include" });
  const handleRecovery = recoveryHandler;
  // All grab surfaces share the same explicit recovery decision, including
  // music searches. Replay only JSON requests, never consumed request streams.
  if (
    response.status !== 409 ||
    !handleRecovery ||
    typeof input !== "string" ||
    init?.method?.toUpperCase() !== "POST" ||
    (init.body !== undefined && typeof init.body !== "string")
  ) {
    return response;
  }
  const payload: unknown = await response
    .clone()
    .json()
    .catch(() => null);
  if (handleRecovery !== recoveryHandler) return response;
  if (!payload || typeof payload !== "object" || !("error" in payload)) {
    return response;
  }
  const error = payload.error;
  if (
    !error ||
    typeof error !== "object" ||
    !("code" in error) ||
    error.code !== "acquisition_deferred" ||
    !("details" in error)
  ) {
    return response;
  }
  const details = error.details;
  if (
    !details ||
    typeof details !== "object" ||
    !("history_id" in details) ||
    typeof details.history_id !== "string" ||
    !("title" in details) ||
    typeof details.title !== "string" ||
    !("status" in details) ||
    typeof details.status !== "string" ||
    !("retry_allowed" in details) ||
    typeof details.retry_allowed !== "boolean"
  ) {
    return response;
  }
  if (
    init.signal?.aborted ||
    !(await handleRecovery({
      history_id: details.history_id,
      title: details.title,
      status: details.status,
      retry_allowed: details.retry_allowed,
    })) ||
    init.signal?.aborted
  ) {
    return response;
  }
  const authorization = await fetch("/api/v1/downloads/history/actions", {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids: [details.history_id], action: "retry" }),
    signal: init.signal,
  });
  if (!authorization.ok) return authorization;
  const result: {
    results?: Array<{ id: string; ok: boolean; error?: string }>;
  } = await authorization.json();
  const outcome = result.results?.find(
    (item) => item.id === details.history_id,
  );
  if (!outcome?.ok) {
    return new Response(
      JSON.stringify({
        error: {
          code: "retry_refused",
          message: outcome?.error || "Could not authorize retry.",
        },
      }),
      { status: 409, headers: { "Content-Type": "application/json" } },
    );
  }
  // One replay per decision: another conflict must not create a dialog loop.
  return fetch(input, { ...init, credentials: "include" });
}
