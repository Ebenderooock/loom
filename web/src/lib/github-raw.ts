// Helpers for importing a plugin script from a GitHub URL.
//
// Converts a github.com "blob" URL (or an existing raw URL) into a
// raw.githubusercontent.com URL that can be fetched directly. Only public
// repositories are supported (raw.githubusercontent.com permits CORS for those).

export class GitHubUrlError extends Error {}

/**
 * Normalize a GitHub file URL to its raw.githubusercontent.com equivalent.
 *
 * Supported inputs:
 *   - https://github.com/<owner>/<repo>/blob/<ref>/<path...>
 *   - https://raw.githubusercontent.com/<owner>/<repo>/<ref>/<path...> (passthrough)
 *
 * The ref segment may itself contain slashes (e.g. a branch name like
 * "feature/x"); everything after "/blob/" is treated as "<ref>/<path>" and
 * preserved verbatim.
 */
export function toRawGitHubUrl(input: string): string {
  let url: URL;
  try {
    url = new URL(input.trim());
  } catch {
    throw new GitHubUrlError("Enter a valid URL.");
  }

  if (url.protocol !== "https:") {
    throw new GitHubUrlError("Only https URLs are supported.");
  }

  const host = url.hostname.toLowerCase();

  if (host === "raw.githubusercontent.com") {
    return url.toString();
  }

  if (host === "github.com" || host === "www.github.com") {
    const marker = "/blob/";
    const idx = url.pathname.indexOf(marker);
    if (idx === -1) {
      throw new GitHubUrlError(
        "Use the URL of a file on GitHub (it should contain “/blob/”).",
      );
    }
    const repoPath = url.pathname.slice(0, idx); // /<owner>/<repo>
    const refAndFile = url.pathname.slice(idx + marker.length); // <ref>/<path...>
    if (!refAndFile) {
      throw new GitHubUrlError("That GitHub URL has no file path.");
    }
    return `https://raw.githubusercontent.com${repoPath}/${refAndFile}`;
  }

  throw new GitHubUrlError(
    "Enter a github.com or raw.githubusercontent.com URL.",
  );
}

/** Fetch the raw text of a public GitHub file given any supported GitHub URL. */
export async function fetchGitHubScript(input: string): Promise<string> {
  const raw = toRawGitHubUrl(input);
  const res = await fetch(raw, { headers: { Accept: "text/plain" } });
  if (!res.ok) {
    if (res.status === 404) {
      throw new GitHubUrlError(
        "File not found (404). Check the URL and that the repository is public.",
      );
    }
    throw new GitHubUrlError(`Failed to fetch script (HTTP ${res.status}).`);
  }
  return res.text();
}
