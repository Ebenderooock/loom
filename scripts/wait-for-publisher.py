#!/usr/bin/env python3
"""Observe publication without granting source workflows publishing credentials."""

import json
import os
import re
import sys
import time
import urllib.error
import urllib.request


SOURCE_REPOSITORY = "Ebenderooock/loom"
PUBLISHER_RUN = re.compile(
    r"https://github\.com/Ebenderooock/loom-publisher/actions/runs/[1-9][0-9]*",
    re.IGNORECASE,
)
SOURCE_REF = re.compile(
    r"refs/(?:heads/(?:main|(?:prelease|prerelease)/[0-9]+\.[0-9]+\.[0-9]+)"
    r"|tags/v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?"
    r"(?:\+[0-9A-Za-z.-]+)?)"
)


def trusted_status(statuses, context, app_slug):
    for status in statuses:
        creator = status.get("creator")
        if (
            status.get("context") == context
            and isinstance(creator, dict)
            and creator.get("login") == f"{app_slug}[bot]"
            and PUBLISHER_RUN.fullmatch(status.get("target_url") or "")
        ):
            if status.get("state") not in {"pending", "success", "failure", "error"}:
                raise ValueError("Publisher returned an unknown publication state")
            return status
    return None


def fetch_status(sha, token, context, app_slug):
    for page in range(1, 21):
        request = urllib.request.Request(
            f"https://api.github.com/repos/{SOURCE_REPOSITORY}/commits/"
            f"{sha}/statuses?per_page=100&page={page}",
            headers={
                "Authorization": f"Bearer {token}",
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
                "User-Agent": "loom-publishing-observer",
            },
        )
        with urllib.request.urlopen(request, timeout=30) as response:
            statuses = json.load(response)
        if not isinstance(statuses, list) or not all(
            isinstance(status, dict) for status in statuses
        ):
            raise ValueError("GitHub returned an invalid commit-status response")
        status = trusted_status(statuses, context, app_slug)
        if status is not None:
            return status
        if len(statuses) < 100:
            return None
    raise ValueError("Commit-status pagination limit exceeded; publication not verified")


def wait_for_publication(sha, token, context, app_slug, timeout=10800):
    deadline = time.monotonic() + timeout
    consecutive_errors = 0
    print(f"Waiting for {context} on {sha}", flush=True)
    while True:
        try:
            status = fetch_status(sha, token, context, app_slug)
            consecutive_errors = 0
        except (urllib.error.URLError, TimeoutError) as error:
            if isinstance(error, urllib.error.HTTPError) and error.code not in {
                429, 500, 502, 503, 504
            }:
                raise
            consecutive_errors += 1
            if consecutive_errors >= 3:
                raise
            print(
                f"GitHub status read failed; retrying ({consecutive_errors}/3): {error}",
                file=sys.stderr,
                flush=True,
            )
            status = None
        if status is not None:
            state = status["state"]
            if state == "success":
                print(f"Publication succeeded: {status['target_url']}", flush=True)
                return status
            if state in {"failure", "error"}:
                raise ValueError(f"Publication {state}: {status['target_url']}")
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError(
                "Publication was not confirmed before the timeout. Check source CI, "
                "the loom-publisher workflow, and PUBLISHER_APP_SLUG configuration."
            )
        time.sleep(min(30, remaining))


def main():
    token = os.environ.get("GH_TOKEN", "")
    app_slug = os.environ.get("PUBLISHER_APP_SLUG", "")
    sha = os.environ.get("SOURCE_SHA", "")
    ref = os.environ.get("SOURCE_REF", "")
    if not token:
        raise ValueError("GH_TOKEN is required to read publication status")
    if not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*", app_slug):
        raise ValueError(
            "Set repository variable PUBLISHER_APP_SLUG to the publishing "
            "GitHub App slug, without the [bot] suffix"
        )
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("SOURCE_SHA must be a full commit SHA")
    if not SOURCE_REF.fullmatch(ref):
        raise ValueError(f"Unsupported publication ref: {ref}")
    context = f"loom-publisher/{ref.split('/', 2)[2]}"
    status = wait_for_publication(sha, token, context, app_slug)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as output:
            output.write(
                f"Published `{ref}` at `{sha}`.\n\n"
                f"[Trusted publisher run]({status['target_url']})\n"
            )


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as error:
        print(f"Publication could not be verified: {error}", file=sys.stderr)
        sys.exit(1)
