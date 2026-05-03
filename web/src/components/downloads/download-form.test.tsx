import { describe, it, expect } from "vitest";
import {
  validateDownloadForm,
  type DownloadFormValues,
} from "@/components/downloads/download-form";

describe("DownloadForm validation", () => {
  const baseValues: DownloadFormValues = {
    kind: "qbittorrent",
    name: "My qBittorrent",
    protocol: "torrent",
    enabled: true,
    priority: 25,
    host: "localhost",
    port: 6881,
    tls: false,
    username: "",
    password: "",
    category_default: "",
    save_path_default: "",
    remove_completed: false,
    remove_failed: false,
  };

  it("validates a correct form", () => {
    const errors = validateDownloadForm(baseValues);
    expect(Object.keys(errors).length).toBe(0);
  });

  it("rejects empty name", () => {
    const errors = validateDownloadForm({ ...baseValues, name: "" });
    expect(errors.name).toBeDefined();
  });

  it("rejects empty host", () => {
    const errors = validateDownloadForm({ ...baseValues, host: "" });
    expect(errors.host).toBeDefined();
  });

  it("rejects invalid port (too low)", () => {
    const errors = validateDownloadForm({ ...baseValues, port: 0 });
    expect(errors.port).toBeDefined();
  });

  it("rejects invalid port (too high)", () => {
    const errors = validateDownloadForm({ ...baseValues, port: 65536 });
    expect(errors.port).toBeDefined();
  });

  it("rejects invalid priority (too low)", () => {
    const errors = validateDownloadForm({ ...baseValues, priority: -1 });
    expect(errors.priority).toBeDefined();
  });

  it("rejects invalid priority (too high)", () => {
    const errors = validateDownloadForm({ ...baseValues, priority: 101 });
    expect(errors.priority).toBeDefined();
  });

  it("accepts valid priority range", () => {
    const errors = validateDownloadForm({ ...baseValues, priority: 0 });
    expect(errors.priority).toBeUndefined();

    const errors2 = validateDownloadForm({ ...baseValues, priority: 100 });
    expect(errors2.priority).toBeUndefined();

    const errors3 = validateDownloadForm({ ...baseValues, priority: 50 });
    expect(errors3.priority).toBeUndefined();
  });
});
