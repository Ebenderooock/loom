/**
 * Project-wide fetch wrapper that always includes credentials.
 * Use this instead of raw fetch() to ensure auth cookies are sent.
 */
export async function apiFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  return fetch(input, { credentials: "include", ...init });
}
