import { afterEach, expect, it, vi } from "vitest";
import * as fetchLib from "@/lib/fetch";
import {
  createRequest,
  listReconciliationHistory,
  reconcileRequests,
} from "./requests-api";

afterEach(() => vi.restoreAllMocks());

it("sends re_request_of when creating an explicit re-request", async () => {
  const apiFetch = vi.spyOn(fetchLib, "apiFetch").mockResolvedValue(
    new Response(
      JSON.stringify({
        id: "new-id",
        user_id: "1",
        username: "alice",
        media_type: "movie",
        tmdb_id: "603",
        title: "The Matrix",
        year: 1999,
        status: "pending",
        created_at: "2026-10-23T00:00:00Z",
        updated_at: "2026-10-23T00:00:00Z",
      }),
      { status: 201, headers: { "Content-Type": "application/json" } },
    ),
  );

  await createRequest({
    media_type: "movie",
    tmdb_id: "603",
    title: "The Matrix",
    re_request_of: "old-request-id",
  });

  const init = apiFetch.mock.calls[0]?.[1] as RequestInit;
  expect(apiFetch.mock.calls[0]?.[0]).toBe("/api/v1/requests");
  expect(init.method).toBe("POST");
  expect(JSON.parse(String(init.body))).toEqual(
    expect.objectContaining({
      media_type: "movie",
      tmdb_id: "603",
      re_request_of: "old-request-id",
    }),
  );
});

it("posts dry_run for reconciliation preview/apply", async () => {
  const apiFetch = vi.spyOn(fetchLib, "apiFetch").mockResolvedValue(
    new Response(
      JSON.stringify({
        dry_run: true,
        outcomes: [],
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ),
  );

  await expect(reconcileRequests(true)).resolves.toEqual({
    dry_run: true,
    outcomes: [],
  });

  const init = apiFetch.mock.calls[0]?.[1] as RequestInit;
  expect(apiFetch.mock.calls[0]?.[0]).toBe("/api/v1/requests/reconcile");
  expect(init.method).toBe("POST");
  expect(JSON.parse(String(init.body))).toEqual({ dry_run: true });
});

it("reads reconciliation run history from the data envelope", async () => {
  vi.spyOn(fetchLib, "apiFetch").mockResolvedValue(
    new Response(
      JSON.stringify({
        data: [
          {
            id: "run-1",
            actor: "admin",
            dry_run: false,
            created_at: "2026-10-23T00:00:00Z",
            outcomes: [
              {
                request_id: "req-1",
                before: "missing",
                after: "present",
                reason: "library match",
                changed: true,
              },
            ],
          },
        ],
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    ),
  );

  await expect(listReconciliationHistory()).resolves.toEqual([
    expect.objectContaining({
      id: "run-1",
      actor: "admin",
      dry_run: false,
      outcomes: [expect.objectContaining({ request_id: "req-1" })],
    }),
  ]);
});
