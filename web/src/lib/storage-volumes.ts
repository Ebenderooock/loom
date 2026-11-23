import type { DiskSpace } from "@/lib/libraries-api";

export interface StorageLibrary {
  id: string;
  name: string;
  disk_space?: DiskSpace | null;
  size_bytes?: number;
}

type AvailableLibrary = StorageLibrary & { disk_space: DiskSpace };

export interface StorageVolume {
  libraries: [AvailableLibrary, ...AvailableLibrary[]];
  total: number;
  free: number;
  estimated: boolean;
}

const FREE_SPACE_TOLERANCE = 64 * 1024 * 1024;

function hasSpace(library: StorageLibrary): library is AvailableLibrary {
  const space = library.disk_space;
  return (
    space != null &&
    Number.isFinite(space.total_bytes) &&
    space.total_bytes > 0 &&
    Number.isFinite(space.free_bytes) &&
    space.free_bytes >= 0 &&
    space.free_bytes <= space.total_bytes
  );
}

export function groupStorageVolumes(libraries: StorageLibrary[]) {
  const identified = new Map<string, StorageVolume>();
  const candidates: StorageVolume[] = [];
  const unavailable: StorageLibrary[] = [];

  for (const library of libraries) {
    if (!hasSpace(library)) {
      unavailable.push(library);
      continue;
    }
    const space = library.disk_space;
    const existing = space.volume_id
      ? identified.get(space.volume_id)
      : undefined;
    if (existing) {
      existing.libraries.push(library);
      existing.total = Math.max(existing.total, space.total_bytes);
      existing.free = Math.min(existing.free, space.free_bytes);
    } else {
      const volume: StorageVolume = {
        libraries: [library],
        total: space.total_bytes,
        free: space.free_bytes,
        estimated: false,
      };
      candidates.push(volume);
      if (space.volume_id) identified.set(space.volume_id, volume);
    }
  }

  const volumes: StorageVolume[] = [];
  for (const candidate of candidates) {
    // Compare every snapshot, not just the latest minimum: this prevents
    // tolerance chains from combining unrelated disks.
    const match = volumes.find((volume) =>
      volume.libraries.every((left) =>
        candidate.libraries.every(
          (right) =>
            left.disk_space.total_bytes === right.disk_space.total_bytes &&
            Math.abs(
              left.disk_space.free_bytes - right.disk_space.free_bytes,
            ) <= FREE_SPACE_TOLERANCE,
        ),
      ),
    );
    if (match) {
      match.libraries.push(...candidate.libraries);
      match.free = Math.min(match.free, candidate.free);
      match.estimated = true;
    } else {
      volumes.push(candidate);
    }
  }

  return { volumes, unavailable };
}

export function storageUsage(volume: StorageVolume) {
  const used = volume.total - volume.free;
  const sizes = volume.libraries.map((library) =>
    Number.isFinite(library.size_bytes) && (library.size_bytes ?? 0) > 0
      ? library.size_bytes!
      : 0,
  );
  const libraryBytes = sizes.reduce((sum, size) => sum + size, 0);
  // Logical file sizes can exceed physical usage (hardlinks, compression,
  // overlapping roots or stale scans). Keep the bar within physical usage.
  const scale = libraryBytes > used ? used / libraryBytes : 1;
  return {
    segments: sizes.map((size) => size * scale),
    other: Math.max(0, used - libraryBytes),
    adjusted: libraryBytes > used,
    unknownSizes: volume.libraries.some(
      (library) =>
        library.size_bytes == null ||
        !Number.isFinite(library.size_bytes) ||
        library.size_bytes < 0,
    ),
  };
}
