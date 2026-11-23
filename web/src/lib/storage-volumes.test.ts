import { describe, expect, it } from "vitest";
import {
  groupStorageVolumes,
  storageUsage,
  type StorageLibrary,
} from "./storage-volumes";

const MiB = 1024 * 1024;

function library(
  id: string,
  volumeId: string | undefined,
  free = 400 * MiB,
  total = 1000 * MiB,
): StorageLibrary {
  return {
    id,
    name: id,
    size_bytes: 100 * MiB,
    disk_space: {
      volume_id: volumeId,
      total_bytes: total,
      free_bytes: free,
      used_bytes: total - free,
    },
  };
}

describe("groupStorageVolumes", () => {
  it("counts shared volume capacity once despite differing snapshots", () => {
    const { volumes } = groupStorageVolumes([
      library("Movies", "disk1"),
      library("TV", "disk1", 300 * MiB),
      library("Anime", "disk1", 200 * MiB),
    ]);
    expect(volumes).toHaveLength(1);
    expect(volumes[0]!.total).toBe(1000 * MiB);
    expect(volumes[0]!.free).toBe(200 * MiB);
    expect(volumes[0]!.estimated).toBe(false);
    expect(volumes[0]!.libraries).toHaveLength(3);
  });

  it("keeps separate volumes with different readings separate", () => {
    const { volumes } = groupStorageVolumes([
      library("Movies", "disk1"),
      library("TV", "disk2", 200 * MiB),
      library("Anime", "disk3", 400 * MiB, 2000 * MiB),
    ]);
    expect(volumes).toHaveLength(3);
    expect(volumes.reduce((sum, volume) => sum + volume.total, 0)).toBe(
      4000 * MiB,
    );
  });

  it("estimates shared NAS mounts using exact capacity and bounded free space", () => {
    const { volumes } = groupStorageVolumes([
      library("Movies", "mount1"),
      library("TV", "mount2", 464 * MiB),
      library("Anime", undefined, 430 * MiB),
    ]);
    expect(volumes).toHaveLength(1);
    expect(volumes[0]!.estimated).toBe(true);
    expect(volumes[0]!.free).toBe(400 * MiB);
  });

  it("does not bridge a tolerance chain or split an exact identity group", () => {
    const { volumes } = groupStorageVolumes([
      library("a", "one", 100 * MiB),
      library("b", "two", 150 * MiB),
      library("c", "three", 200 * MiB),
      library("d", "three", 250 * MiB),
    ]);
    expect(
      volumes.map((volume) => volume.libraries.map((lib) => lib.id)),
    ).toEqual([
      ["a", "b"],
      ["c", "d"],
    ]);
  });

  it("excludes absent, zero and invalid disk readings without hiding libraries", () => {
    const invalid = [
      { id: "missing", name: "Missing" },
      library("zero", undefined, 0, 0),
      library("negative", undefined, -1),
      library("overfull", undefined, 1100 * MiB),
      library("nan", undefined, Number.NaN),
    ];
    const result = groupStorageVolumes([...invalid, library("valid", "one")]);
    expect(result.volumes).toHaveLength(1);
    expect(result.unavailable).toEqual(invalid);
  });
});

describe("storageUsage", () => {
  it("splits physical usage into library sizes and other usage", () => {
    const { volumes } = groupStorageVolumes([
      library("Movies", "one"),
      library("TV", "one"),
    ]);
    expect(storageUsage(volumes[0]!)).toEqual({
      segments: [100 * MiB, 100 * MiB],
      other: 400 * MiB,
      adjusted: false,
      unknownSizes: false,
    });
  });

  it("bounds overlapping or compressed file sizes to physical usage", () => {
    const { volumes } = groupStorageVolumes([
      library("Movies", "one", 950 * MiB),
      library("TV", "one", 950 * MiB),
    ]);
    const usage = storageUsage(volumes[0]!);
    expect(usage.segments).toEqual([25 * MiB, 25 * MiB]);
    expect(usage.other).toBe(0);
    expect(usage.adjusted).toBe(true);
  });

  it("labels unreported sizes and avoids invalid bar widths", () => {
    const lib = library("old server", "one");
    delete lib.size_bytes;
    const { volumes } = groupStorageVolumes([lib]);
    expect(storageUsage(volumes[0]!)).toEqual({
      segments: [0],
      other: 600 * MiB,
      adjusted: false,
      unknownSizes: true,
    });
  });
});
