"""Discover and run deterministic race-test shards, or merge their coverage."""

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys


# Relative weights from Go job 133605996611, not elapsed-time predictions.
WEIGHTS = {
    "internal/collections": 320,
    "internal/requests": 293,
    "internal/downloads": 291,
    "internal/imports": 284,
    "internal/music": 253,
    "internal/auth": 247,
    "internal/indexers": 227,
    "internal/plugins": 156,
}


def go_list(pattern):
    result = subprocess.run(
        ["go", "list", "-tags", "disable_libutp", pattern],
        check=True, capture_output=True, text=True,
    )
    packages = result.stdout.splitlines()
    if not packages or len(packages) != len(set(packages)):
        raise ValueError(f"empty or duplicate package discovery for {pattern}")
    return set(packages)


def partition(packages, workflows, count):
    if count < 1:
        raise ValueError("shard count must be positive")
    if not workflows or not workflows <= packages:
        raise ValueError("workflow packages must be a nonempty subset of all packages")
    race = packages - workflows
    if len(race) < count:
        raise ValueError("not enough race packages for nonempty shards")

    def weight(package):
        return next(
            (seconds for suffix, seconds in WEIGHTS.items()
             if package.endswith("/" + suffix)),
            30,
        )

    shards = [[] for _ in range(count)]
    loads = [0] * count
    for package in sorted(race, key=lambda p: (-weight(p), p)):
        index = min(range(count), key=lambda i: (loads[i], i))
        shards[index].append(package)
        loads[index] += weight(package)
    return [sorted(shard) for shard in shards]


def discover(count):
    return partition(
        go_list("./..."), go_list("./internal/workflows/..."), count,
    )


def run_shard(packages, coverage):
    if not packages:
        raise ValueError("refusing to run an empty shard")
    subprocess.run(
        ["go", "test", "-race", "-tags", "disable_libutp", "-count=1",
         f"-coverprofile={coverage}", *packages],
        env={**os.environ, "CGO_ENABLED": "1"},
        check=True,
    )


def merge_coverage(directory, count, output):
    if count < 1:
        raise ValueError("shard count must be positive")
    expected = {f"coverage-{i}.out" for i in range(count)}
    if {p.name for p in directory.glob("*.out")} != expected:
        raise ValueError("missing or unexpected shard coverage profiles")
    blocks = {}
    for name in sorted(expected):
        lines = (directory / name).read_text().splitlines()
        if not lines or lines[0] != "mode: atomic":
            raise ValueError(f"{name}: expected atomic coverage mode")
        for line in lines[1:]:
            location, statements, hits = line.split()
            if location in blocks:
                raise ValueError(f"duplicate coverage block: {location}")
            if int(statements) < 0 or int(hits) < 0:
                raise ValueError(f"invalid coverage counts: {line}")
            blocks[location] = line
    if not blocks:
        raise ValueError("empty combined coverage profile")
    output.write_text(
        "mode: atomic\n" + "".join(blocks[key] + "\n" for key in sorted(blocks))
    )


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["plan", "test", "merge"])
    parser.add_argument("--shards", type=int, default=4)
    parser.add_argument("--shard", type=int)
    parser.add_argument("--directory", type=Path, default=Path("."))
    parser.add_argument("--coverage", type=Path, default=Path("coverage.out"))
    args = parser.parse_args()
    if args.command == "merge":
        merge_coverage(args.directory, args.shards, args.coverage)
    else:
        if args.command == "test" and (
            args.shard is None or not 0 <= args.shard < args.shards
        ):
            parser.error("--shard must be between zero and --shards minus one")
        shards = discover(args.shards)
        if args.command == "plan":
            print(json.dumps(shards, indent=2))
        else:
            packages = shards[args.shard]
            print("\n".join(packages), flush=True)
            run_shard(packages, args.coverage)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(f"Go CI failed: {error}", file=sys.stderr)
        if isinstance(error, subprocess.CalledProcessError) and error.stderr:
            print(error.stderr, file=sys.stderr)
        sys.exit(1)
