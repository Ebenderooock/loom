#!/usr/bin/env python3
"""Determine whether a push or pull request changes code rather than docs only."""

import os
from pathlib import Path
import subprocess
import sys


ZERO_SHA = "0" * 40


def is_code_change(path):
    normalized = path.replace("\\", "/")
    return not (
        normalized.startswith("docs/")
        or normalized.lower().endswith(".md")
    )


def changed_files(event_name, ref, before, base_sha, head_sha):
    if event_name == "push" and ref.startswith("refs/tags/"):
        return None
    if event_name == "pull_request":
        revisions = [f"{base_sha}...{head_sha}"]
    elif event_name == "push":
        if not before or before == ZERO_SHA:
            return None
        revisions = [before, head_sha]
    else:
        return None

    result = subprocess.run(
        ["git", "diff", "--name-only", *revisions],
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout.splitlines()


def main():
    files = changed_files(
        os.environ.get("EVENT_NAME", ""),
        os.environ.get("REF", ""),
        os.environ.get("BEFORE", ""),
        os.environ.get("BASE_SHA", ""),
        os.environ.get("HEAD_SHA", ""),
    )
    code_changed = files is None or any(is_code_change(path) for path in files)

    output_path = os.environ.get("GITHUB_OUTPUT")
    if not output_path:
        raise ValueError("GITHUB_OUTPUT is required")
    with Path(output_path).open("a", encoding="utf-8") as output:
        output.write(f"code={str(code_changed).lower()}\n")


if __name__ == "__main__":
    try:
        main()
    except (OSError, subprocess.CalledProcessError, ValueError) as error:
        print(f"Unable to detect code changes: {error}", file=sys.stderr)
        raise SystemExit(1) from error
