import { apiFetch } from "@/lib/fetch";

export type MediaFilesScope =
  | { mediaType: "movie"; id: string; season?: never }
  | { mediaType: "series"; id: string; season?: number };

export type DeletionMode = "recycle" | "permanent";

export interface MediaFile {
  path: string;
  size: number;
  tracked: boolean;
  quality?: string;
  format?: string;
}

export interface MediaFilesResponse {
  files: MediaFile[];
  deletionMode: DeletionMode;
}

function mediaFilesURL(scope: MediaFilesScope, deleting = false) {
  const base = `/api/v1/media-files/${scope.mediaType}/${encodeURIComponent(scope.id)}`;
  const query =
    scope.mediaType === "series" && scope.season !== undefined
      ? `?season=${scope.season}`
      : "";
  return `${base}${deleting ? "/delete" : ""}${query}`;
}

async function readResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(
      body?.error?.message || `File request failed (${response.status})`,
    );
  }
  return response.json();
}

export async function getMediaFiles(
  scope: MediaFilesScope,
  signal?: AbortSignal,
): Promise<MediaFilesResponse> {
  return readResponse(await apiFetch(mediaFilesURL(scope), { signal }));
}

export async function deleteMediaFiles(
  scope: MediaFilesScope,
  paths: string[],
  deletionMode: DeletionMode,
): Promise<{ deleted: string[] }> {
  const uniquePaths = [...new Set(paths)];
  if (uniquePaths.length < 1 || uniquePaths.length > 1000) {
    throw new Error("Select between 1 and 1,000 files per operation.");
  }
  const body = JSON.stringify({ paths: uniquePaths, deletionMode });
  if (new Blob([body]).size > 128 * 1024) {
    throw new Error(
      "Selected paths exceed the 128 KiB request limit. Select fewer files and try again.",
    );
  }
  return readResponse(
    await apiFetch(mediaFilesURL(scope, true), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Confirm-Delete": "true",
      },
      body,
    }),
  );
}
