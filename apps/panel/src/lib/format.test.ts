import { describe, expect, it } from "vitest";
import { formatBytes, formatUptime, stepLabel, usedPercent } from "./format";
import { errorMessageFrom } from "../api/client";

describe("formatBytes", () => {
  it("uses binary units with one decimal above bytes", () => {
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
    expect(formatBytes(3 * 1024 ** 3)).toBe("3.0 GB");
  });

  it("rejects impossible values", () => {
    expect(formatBytes(-1)).toBe("—");
    expect(formatBytes(Number.NaN)).toBe("—");
  });
});

describe("formatUptime", () => {
  it("shows days and hours when the server has been up a while", () => {
    expect(formatUptime(90061)).toBe("1d 1h");
  });

  it("shows hours and minutes for shorter uptimes", () => {
    expect(formatUptime(3725)).toBe("1h 2m");
  });
});

describe("usedPercent", () => {
  it("rounds to a whole percent and handles an unknown total", () => {
    expect(usedPercent(1, 3)).toBe(33);
    expect(usedPercent(5, 0)).toBeNull();
  });
});

describe("stepLabel", () => {
  it("turns step identifiers into words", () => {
    expect(stepLabel("create_wp_config")).toBe("create wp config");
  });
});

describe("errorMessageFrom", () => {
  it("prefers the API's message", () => {
    expect(errorMessageFrom({ error: { message: "Domain is taken" } }, "fallback")).toBe(
      "Domain is taken",
    );
  });

  it("falls back when the body has no message", () => {
    expect(errorMessageFrom(null, "fallback")).toBe("fallback");
    expect(errorMessageFrom({ error: {} }, "fallback")).toBe("fallback");
  });
});

describe("describeError", () => {
  it("uses the API's sentence for API errors", async () => {
    const { ApiError, describeError } = await import("../api/client");
    expect(describeError(new ApiError(409, "domain_in_use", "still has 2 live websites"))).toBe(
      "still has 2 live websites",
    );
  });

  it("explains a network failure in plain words", async () => {
    const { describeError } = await import("../api/client");
    expect(describeError(new TypeError("Failed to fetch"))).toMatch(/Cannot reach the panel/);
  });

  it("never shows raw technical text for unknown errors", async () => {
    const { describeError } = await import("../api/client");
    expect(describeError(new Error("ECONNRESET socket 10.0.0.1"))).toBe(
      "Something unexpected happened. Please try again.",
    );
  });
});
