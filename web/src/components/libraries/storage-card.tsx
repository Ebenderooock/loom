import { AlertTriangle, FolderOpen } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useLibraries } from "@/lib/libraries-api";
import { groupStorageVolumes, storageUsage } from "@/lib/storage-volumes";
import { cn, formatBytes } from "@/lib/utils";

const colors = [
  "bg-blue-500",
  "bg-emerald-500",
  "bg-violet-500",
  "bg-amber-500",
];

export function StorageCard() {
  const { data, isLoading, isError } = useLibraries();
  const libraries = data ?? [];
  const { volumes, unavailable } = groupStorageVolumes(libraries);
  const total = volumes.reduce((sum, volume) => sum + volume.total, 0);
  const free = volumes.reduce((sum, volume) => sum + volume.free, 0);

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Storage</CardTitle>
        {!isLoading && !isError && volumes.length > 0 && (
          <p className="text-sm text-muted-foreground">
            {formatBytes(free)} free of {formatBytes(total)}
            {volumes.length > 1 && ` across ${volumes.length} volumes`}
            {volumes.some((volume) => volume.estimated) && " (estimated)"}
          </p>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {isLoading ? (
          <>
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </>
        ) : isError ? (
          <p role="alert" className="text-sm text-destructive">
            Unable to load storage information.
          </p>
        ) : (
          <>
            {volumes.map((volume, index) => {
              const usage = storageUsage(volume);
              const used = volume.total - volume.free;
              const percent = Math.round((used / volume.total) * 100);
              const title =
                volumes.length === 1 ? "Storage volume" : `Volume ${index + 1}`;
              const label = `${title}: ${formatBytes(volume.free)} free of ${formatBytes(volume.total)}, ${percent}% used`;
              return (
                <section
                  key={volume.libraries[0].id}
                  aria-label={title}
                  className="space-y-3 rounded-lg border border-border/50 p-3"
                >
                  <div className="flex flex-wrap items-center justify-between gap-1 text-sm">
                    <span className="font-medium">{title}</span>
                    <span className="text-xs text-muted-foreground">
                      {formatBytes(volume.free)} free of{" "}
                      {formatBytes(volume.total)}
                    </span>
                  </div>
                  <div
                    role="img"
                    aria-label={label}
                    className="flex h-3 overflow-hidden rounded-full bg-muted"
                  >
                    {volume.libraries.map((library, i) => (
                      <span
                        key={library.id}
                        aria-hidden="true"
                        className={cn(
                          "h-full shrink-0",
                          colors[i % colors.length],
                        )}
                        style={{
                          width: `${((usage.segments[i] ?? 0) / volume.total) * 100}%`,
                        }}
                      />
                    ))}
                    <span
                      aria-hidden="true"
                      className="h-full shrink-0 bg-muted-foreground/50"
                      style={{
                        width: `${(usage.other / volume.total) * 100}%`,
                      }}
                    />
                  </div>
                  <ul className="space-y-1 text-xs">
                    {volume.libraries.map((library, i) => (
                      <li key={library.id} className="flex items-start gap-2">
                        <span
                          aria-hidden="true"
                          className={cn(
                            "mt-0.5 h-2.5 w-2.5 shrink-0 rounded-sm",
                            colors[i % colors.length],
                          )}
                        />
                        <span className="min-w-0 break-words">
                          {library.name}
                        </span>
                        <span className="ml-auto shrink-0 text-muted-foreground">
                          {library.size_bytes != null &&
                          Number.isFinite(library.size_bytes) &&
                          library.size_bytes >= 0
                            ? formatBytes(library.size_bytes)
                            : "Size unavailable"}
                        </span>
                      </li>
                    ))}
                    <li className="flex items-center gap-2">
                      <span
                        aria-hidden="true"
                        className="h-2.5 w-2.5 rounded-sm bg-muted-foreground/50"
                      />
                      <span>
                        Other usage{usage.unknownSizes && " / unscanned"}
                      </span>
                      <span className="ml-auto text-muted-foreground">
                        {formatBytes(usage.other)}
                      </span>
                    </li>
                  </ul>
                  <p className="text-xs text-muted-foreground">
                    {percent}% used
                  </p>
                  {volume.estimated && (
                    <p className="text-xs text-muted-foreground">
                      Estimated shared storage: matching capacity and free
                      space. Separate drives with similar readings may be
                      grouped.
                    </p>
                  )}
                  {usage.adjusted && (
                    <p className="text-xs text-muted-foreground">
                      Library sizes exceed disk usage; the bar is proportional.
                      Files may be shared, compressed or awaiting a rescan.
                    </p>
                  )}
                  {volume.free / volume.total < 0.1 && (
                    <p className="flex items-center gap-2 text-xs text-amber-500">
                      <AlertTriangle aria-hidden="true" className="h-4 w-4" />
                      Low free space (less than 10%)
                    </p>
                  )}
                </section>
              );
            })}
            {volumes.length === 0 && libraries.length === 0 && (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <FolderOpen aria-hidden="true" className="h-4 w-4" />
                No libraries configured
              </p>
            )}
            {unavailable.length > 0 && (
              <p className="text-sm text-muted-foreground">
                Storage info unavailable:{" "}
                {unavailable.map((library) => library.name).join(", ")}. Not
                included in totals.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
