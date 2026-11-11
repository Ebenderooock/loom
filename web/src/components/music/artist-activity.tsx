import { Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { formatBytes, relativeTime } from "@/lib/utils";
import { useArtistFiles, useArtistHistory } from "@/lib/music-api";

function fileName(path: string): string {
  return path.split(/[\\/]/).pop() || path;
}

export function ArtistFilesList({
  artistId,
  enabled,
}: {
  artistId: string;
  enabled: boolean;
}) {
  const { data: files, isLoading, isError } = useArtistFiles(artistId, enabled);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading files…
      </div>
    );
  }
  if (isError) {
    return (
      <p className="py-2 text-sm text-muted-foreground">
        Could not load files.
      </p>
    );
  }
  if (!files || files.length === 0) {
    return (
      <p className="py-2 text-sm text-muted-foreground">
        No audio files have been imported for this artist.
      </p>
    );
  }

  const totalSize = files.reduce((sum, file) => sum + (file.size || 0), 0);
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        {files.length} files · {formatBytes(totalSize)}
      </p>
      <ul className="divide-y divide-border/60 rounded-md border border-border">
        {files.map((file) => (
          <li key={file.id} className="space-y-0.5 px-3 py-2 text-sm">
            <div className="flex items-center gap-2">
              <span className="min-w-0 flex-1 truncate">
                {file.track_title || fileName(file.file_path)}
              </span>
              {(file.quality || file.format) && (
                <Badge variant="outline" className="shrink-0 text-[10px]">
                  {file.quality || file.format}
                </Badge>
              )}
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                {formatBytes(file.size)}
              </span>
            </div>
            <p
              className="truncate text-xs text-muted-foreground"
              title={file.file_path}
            >
              {file.album_title && `${file.album_title} · `}
              {file.file_path}
            </p>
          </li>
        ))}
      </ul>
    </div>
  );
}

const STATUS_VARIANT: Record<string, "secondary" | "outline" | "destructive"> =
  {
    imported: "secondary",
    acquired: "secondary",
    completed: "secondary",
    failed: "destructive",
    acquisition_failed: "destructive",
  };

function statusLabel(status: string): string {
  return status.replace(/_/g, " ");
}

export function ArtistHistoryList({
  artistId,
  enabled,
}: {
  artistId: string;
  enabled: boolean;
}) {
  const {
    data: entries,
    isLoading,
    isError,
  } = useArtistHistory(artistId, enabled);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 py-3 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading history…
      </div>
    );
  }
  if (isError) {
    return (
      <p className="py-2 text-sm text-muted-foreground">
        Could not load history.
      </p>
    );
  }
  if (!entries || entries.length === 0) {
    return (
      <p className="py-2 text-sm text-muted-foreground">
        No grabs or imports recorded yet.
      </p>
    );
  }

  return (
    <ul className="space-y-2">
      {entries.map((entry) => (
        <li
          key={`${entry.type}-${entry.id}`}
          className="rounded-md border border-border px-3 py-2 text-sm"
        >
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="shrink-0 text-[10px]">
              {entry.type === "grab" ? "Grabbed" : "Import"}
            </Badge>
            <span className="min-w-0 flex-1 truncate font-medium">
              {entry.album_title || "Unknown album"}
            </span>
            <Badge
              variant={STATUS_VARIANT[entry.status] ?? "outline"}
              className="shrink-0 text-[10px] capitalize"
            >
              {statusLabel(entry.status)}
            </Badge>
          </div>
          <p className="mt-1 truncate text-xs text-muted-foreground">
            {[
              relativeTime(entry.date),
              entry.title,
              entry.dest_path || entry.source_path,
            ]
              .filter(Boolean)
              .join(" · ")}
          </p>
          {entry.reason && (
            <p
              className={
                STATUS_VARIANT[entry.status] === "destructive"
                  ? "mt-1 text-xs text-destructive"
                  : "mt-1 text-xs text-muted-foreground"
              }
            >
              {entry.reason}
            </p>
          )}
        </li>
      ))}
    </ul>
  );
}
