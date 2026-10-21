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
  return fetch(input, { ...init, credentials: "include" });
}
