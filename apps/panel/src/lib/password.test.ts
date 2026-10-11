// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { generatePassword } from "./password";

describe("generatePassword", () => {
  it("makes a password of the asked length from letters and digits without look-alikes", () => {
    for (const length of [12, 20, 64]) {
      const pw = generatePassword(length);
      expect(pw).toHaveLength(length);
      expect(pw).toMatch(/^[A-HJ-NP-Za-km-z2-9]+$/);
    }
  });

  it("does not repeat", () => {
    const seen = new Set(Array.from({ length: 200 }, () => generatePassword()));
    expect(seen.size).toBe(200);
  });
});
