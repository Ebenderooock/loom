import { describe, expect, it } from "vitest";
import { toRawGitHubUrl, GitHubUrlError } from "@/lib/github-raw";

describe("toRawGitHubUrl", () => {
  it("converts a blob URL to raw", () => {
    expect(
      toRawGitHubUrl(
        "https://github.com/owner/repo/blob/main/plugins/hook.js",
      ),
    ).toBe(
      "https://raw.githubusercontent.com/owner/repo/main/plugins/hook.js",
    );
  });

  it("preserves a branch name containing slashes", () => {
    expect(
      toRawGitHubUrl(
        "https://github.com/owner/repo/blob/feature/new-thing/dir/hook.js",
      ),
    ).toBe(
      "https://raw.githubusercontent.com/owner/repo/feature/new-thing/dir/hook.js",
    );
  });

  it("passes through an already-raw URL", () => {
    const raw =
      "https://raw.githubusercontent.com/owner/repo/main/hook.js";
    expect(toRawGitHubUrl(raw)).toBe(raw);
  });

  it("trims surrounding whitespace", () => {
    expect(
      toRawGitHubUrl("  https://github.com/o/r/blob/main/a.js \n"),
    ).toBe("https://raw.githubusercontent.com/o/r/main/a.js");
  });

  it("rejects a non-URL string", () => {
    expect(() => toRawGitHubUrl("not a url")).toThrow(GitHubUrlError);
  });

  it("rejects a non-GitHub host", () => {
    expect(() =>
      toRawGitHubUrl("https://example.com/owner/repo/blob/main/a.js"),
    ).toThrow(GitHubUrlError);
  });

  it("rejects a github.com URL without a /blob/ segment", () => {
    expect(() =>
      toRawGitHubUrl("https://github.com/owner/repo"),
    ).toThrow(GitHubUrlError);
  });

  it("rejects http (non-https) URLs", () => {
    expect(() =>
      toRawGitHubUrl("http://github.com/o/r/blob/main/a.js"),
    ).toThrow(GitHubUrlError);
  });
});
