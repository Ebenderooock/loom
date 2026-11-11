import { useCallback, useEffect, useRef, useState } from "react";
import { apiFetch } from "@/lib/fetch";

export interface ScanResult {
  id: string;
  libraryId: string;
  rootFolderPath: string;
  status: "running" | "completed" | "failed";
  totalFiles: number;
  matched: number;
  unmatched: number;
  imported: number;
  errors?: string[];
}

export interface UnmatchedFile {
  id: string;
  scanId: string;
  filePath: string;
  size: number;
  parsedTitle: string;
  parsedYear: number;
  quality: string;
  source: string;
}

export async function importRequest<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const res = await apiFetch(path, init);
  if (!res.ok) {
    const text = await res.text();
    let message = text;
    try {
      const body: unknown = JSON.parse(text);
      if (body && typeof body === "object" && "error" in body) {
        const error = body.error;
        if (typeof error === "string") message = error;
        else if (error && typeof error === "object" && "message" in error) {
          if (typeof error.message === "string") message = error.message;
        }
      }
    } catch (error) {
      if (!(error instanceof SyntaxError)) throw error;
    }
    throw new Error(
      `${res.status} ${res.statusText}: ${message || "Import request failed"}`,
    );
  }
  return res.json();
}

export function useLibraryImport(
  open: boolean,
  endpoint: string,
  onImportComplete: () => void,
) {
  const [scanning, setScanning] = useState(false);
  const [scanResult, setScanResult] = useState<ScanResult | null>(null);
  const [unmatchedFiles, setUnmatchedFiles] = useState<UnmatchedFile[]>([]);
  const [error, setError] = useState<string | null>(null);
  const runRef = useRef(0);
  const controllerRef = useRef<AbortController | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const onCompleteRef = useRef(onImportComplete);

  useEffect(() => {
    onCompleteRef.current = onImportComplete;
  }, [onImportComplete]);

  const cancel = useCallback(() => {
    runRef.current++;
    controllerRef.current?.abort();
    clearTimeout(timerRef.current);
  }, []);

  const reset = useCallback(() => {
    cancel();
    setScanning(false);
    setScanResult(null);
    setUnmatchedFiles([]);
    setError(null);
  }, [cancel]);

  useEffect(() => {
    reset();
    return cancel;
  }, [open, reset, cancel]);

  const startScan = useCallback(
    async (libraryId: string) => {
      reset();
      setScanning(true);
      const run = runRef.current;
      const controller = new AbortController();
      controllerRef.current = controller;
      const active = () => run === runRef.current;
      const fail = (err: unknown) => {
        if (!active()) return;
        setScanning(false);
        setError(err instanceof Error ? err.message : "Import failed");
      };

      try {
        const data = await importRequest<{ scanId: string }>(endpoint, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ libraryId }),
          signal: controller.signal,
        });
        if (!active()) return;
        if (!data || typeof data.scanId !== "string" || !data.scanId) {
          throw new Error("Loom returned an invalid scan ID");
        }
        const poll = async () => {
          try {
            const result = await importRequest<ScanResult>(
              `${endpoint}/${encodeURIComponent(data.scanId)}`,
              { signal: controller.signal },
            );
            if (!active()) return;
            if (
              !result ||
              !["running", "completed", "failed"].includes(result.status)
            ) {
              throw new Error("Loom returned an invalid scan status");
            }
            setScanResult(result);
            if (result.status === "running") {
              timerRef.current = setTimeout(poll, 1500);
            } else {
              setScanning(false);
              if (result.status === "completed") onCompleteRef.current();
              const files = await importRequest<UnmatchedFile[]>(
                `${endpoint}/unmatched`,
                { signal: controller.signal },
              );
              if (!active()) return;
              if (!Array.isArray(files)) {
                throw new Error("Loom returned an invalid unmatched file list");
              }
              setUnmatchedFiles(files);
            }
          } catch (err) {
            fail(err);
          }
        };
        void poll();
      } catch (err) {
        fail(err);
      }
    },
    [endpoint, reset],
  );

  return {
    scanning,
    scanResult,
    unmatchedFiles,
    setUnmatchedFiles,
    error,
    setError,
    startScan,
    reset,
  };
}
