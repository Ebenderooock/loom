#!/usr/bin/env python3
"""Build unsigned native Prelease archives; never sign or publish from source CI."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import zipfile


ROOT = Path(__file__).resolve().parent.parent
TARGETS = (
    ("linux", "amd64", ""),
    ("linux", "arm64", ""),
    ("linux", "arm", "7"),
    ("darwin", "amd64", ""),
    ("darwin", "arm64", ""),
    ("windows", "amd64", ""),
    ("windows", "arm64", ""),
)


def version_for(ref, sha, timestamp):
    match = re.fullmatch(
        r"(?:prelease|prerelease)/((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))",
        ref,
    )
    if not match or not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("Expected prelease/X.Y.Z or prerelease/X.Y.Z and a full commit SHA")
    date = datetime.fromtimestamp(timestamp, timezone.utc).strftime("%Y%m%d%H%M%S")
    return f"{match[1]}-prelease.{date}.g{sha[:12]}"


def build(ref, output):
    sha = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    timestamp = int(subprocess.check_output(
        ["git", "show", "-s", "--format=%ct", "HEAD"], cwd=ROOT, text=True
    ).strip())
    version = version_for(ref, sha, timestamp)
    if not (ROOT / "web/dist/index.html").is_file():
        raise ValueError("Build web/dist first: cd web && pnpm install --frozen-lockfile && pnpm build")
    output.mkdir(parents=True, exist_ok=True)
    # Refuse stale/mixed outputs rather than publishing an ambiguous directory.
    if any(output.iterdir()):
        raise ValueError("Output directory must be empty")
    artifacts = []
    date = datetime.fromtimestamp(timestamp, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for goos, arch, arm in TARGETS:
        platform = "macos" if goos == "darwin" else goos
        binary_name = "loom.exe" if goos == "windows" else "loom"
        suffix = ".zip" if goos == "windows" else ".tar.gz"
        name = f"loom_{version}_{goos}_{arch}{'v' + arm if arm else ''}{suffix}"
        with tempfile.TemporaryDirectory() as staging:
            binary = Path(staging) / binary_name
            env = dict(os.environ, CGO_ENABLED="0", GOOS=goos, GOARCH=arch)
            env.pop("GOARM", None)
            if arm:
                env["GOARM"] = arm
            prefix = "github.com/ebenderooock/loom/internal/buildinfo"
            subprocess.run(
                ["go", "build", "-trimpath", "-tags", "embed", "-ldflags",
                 f"-s -w -X {prefix}.Version={version} -X {prefix}.Commit={sha} -X {prefix}.Date={date} -X {prefix}.SourceRef=refs/heads/{ref}",
                 "-o", str(binary), "./cmd/loom"],
                cwd=ROOT, env=env, check=True,
            )
            files = [(binary, binary_name)]
            if goos in ("linux", "darwin"):
                helper = Path(staging) / "loom-updater"
                subprocess.run(
                    ["go", "build", "-trimpath", "-o", str(helper), "./cmd/loom-updater"],
                    cwd=ROOT, env=env, check=True,
                )
                files.append((helper, "loom-updater"))
            for filename in ("LICENSE", "README.md", "CHANGELOG.md"):
                files.append((ROOT / filename, filename))
            for directory in (ROOT / "packaging" / platform, ROOT / "docs"):
                for path in sorted(directory.rglob("*")):
                    if path.is_file() and not path.is_symlink():
                        files.append((path, str(path.relative_to(ROOT))))
            archive = output / name
            if goos == "windows":
                with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as zf:
                    for path, dest in files:
                        zf.write(path, dest)
            else:
                with tarfile.open(archive, "w:gz") as tf:
                    for path, dest in files:
                        tf.add(path, dest, recursive=False)
            with archive.open("rb") as stream:
                digest = hashlib.file_digest(stream, "sha256").hexdigest()
            artifacts.append({
                "name": name, "os": goos, "arch": arch,
                **({"arm": arm} if arm else {}),
                "sha256": digest,
                "size": archive.stat().st_size,
            })
    # This descriptor is UNTRUSTED build output, not a signed update manifest.
    # The publisher must independently validate names, sizes, hashes, ref/SHA
    # and version, then construct and sign the publisher contract.
    (output / "native-artifacts.json").write_text(json.dumps({
        "schema": 1, "repository": "Ebenderooock/loom", "channel": "prelease",
        "version": "v" + version, "tag": "v" + version, "artifacts": artifacts,
    }, indent=2) + "\n")
    print(f"Built {len(artifacts)} unsigned native archives for v{version}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True)
    parser.add_argument("--output", type=Path, default=ROOT / "dist/native-prelease")
    args = parser.parse_args()
    build(args.ref, args.output.resolve())


if __name__ == "__main__":
    main()
