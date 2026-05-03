import { describe, expect, it } from "vitest";
import { maskUrlCredentials } from "@/pages/proxies";

describe("maskUrlCredentials", () => {
  it("returns em-dash for empty input", () => {
    expect(maskUrlCredentials("")).toBe("—");
  });

  it("masks user and password in the URL", () => {
    const masked = maskUrlCredentials("http://alice:hunter2@gateway:3128");
    expect(masked).not.toContain("alice");
    expect(masked).not.toContain("hunter2");
    expect(masked).toContain("***");
  });

  it("leaves credential-free URLs untouched", () => {
    expect(maskUrlCredentials("http://gateway:3128")).toBe(
      "http://gateway:3128/",
    );
  });

  it("returns the input verbatim when it cannot be parsed", () => {
    expect(maskUrlCredentials("not a url")).toBe("not a url");
  });
});
