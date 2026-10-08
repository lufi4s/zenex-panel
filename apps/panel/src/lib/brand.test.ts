import { describe, expect, it } from "vitest";
import { foregroundFor, luminance, rgbOf } from "./brand";

describe("brand colours", () => {
  it("reads hex colours", () => {
    expect(rgbOf("#3b6fd4")).toEqual([59, 111, 212]);
  });

  it("uses dark text on light accents and white on dark ones", () => {
    expect(foregroundFor("#f5d03b")).toBe("#0b0f14");
    expect(foregroundFor("#1d3a8a")).toBe("#ffffff");
  });

  it("orders black below white", () => {
    expect(luminance("#000000")).toBeLessThan(luminance("#ffffff"));
  });
});
