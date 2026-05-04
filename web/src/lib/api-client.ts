import { useAuth } from "@/hooks/use-auth";

const API_BASE_URL = "http://localhost:8989/api/v1";

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public data?: unknown
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function useApiClient() {
  const { isAuthenticated } = useAuth();

  const request = async <T = unknown>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> => {
    if (!isAuthenticated) {
      throw new ApiError("Not authenticated", 401);
    }

    const url = endpoint.startsWith("http") ? endpoint : `${API_BASE_URL}${endpoint}`;

    const headers: HeadersInit = {
      "Content-Type": "application/json",
      ...options.headers,
    };

    try {
      const response = await fetch(url, {
        credentials: "include",
        ...options,
        headers,
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new ApiError(
          `API error: ${response.status} ${response.statusText}`,
          response.status,
          data
        );
      }

      if (response.status === 204) {
        return undefined as T;
      }

      return await response.json();
    } catch (err) {
      if (err instanceof ApiError) {
        throw err;
      }
      throw new ApiError(err instanceof Error ? err.message : "Unknown error", 0);
    }
  };

  return {
    get: <T = unknown>(endpoint: string) =>
      request<T>(endpoint, { method: "GET" }),

    post: <T = unknown>(endpoint: string, body?: unknown) =>
      request<T>(endpoint, {
        method: "POST",
        body: body ? JSON.stringify(body) : undefined,
      }),

    put: <T = unknown>(endpoint: string, body?: unknown) =>
      request<T>(endpoint, {
        method: "PUT",
        body: body ? JSON.stringify(body) : undefined,
      }),

    patch: <T = unknown>(endpoint: string, body?: unknown) =>
      request<T>(endpoint, {
        method: "PATCH",
        body: body ? JSON.stringify(body) : undefined,
      }),

    delete: <T = unknown>(endpoint: string) =>
      request<T>(endpoint, { method: "DELETE" }),
  };
}
