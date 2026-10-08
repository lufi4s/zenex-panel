import { describe, expect, it } from "vitest";
import { filterTerminalLines, levelFromText, type TerminalLine } from "./Terminal";

const lines: TerminalLine[] = [
  { id: 1, text: "GET /index.php 200", level: "info" },
  { id: 2, text: "PHP Fatal error: memory exhausted", level: "error" },
  { id: 3, text: "cache warmed", level: "success" },
];

describe("filterTerminalLines", () => {
  it("returns everything for an empty filter", () => {
    expect(filterTerminalLines(lines, "  ")).toHaveLength(3);
  });

  it("matches text case-insensitively", () => {
    const hits = filterTerminalLines(lines, "FATAL");
    expect(hits.map((l) => l.id)).toEqual([2]);
  });

  it("also matches the level name", () => {
    expect(filterTerminalLines(lines, "error")).toHaveLength(1);
  });
});

describe("levelFromText", () => {
  it("detects common level words", () => {
    expect(levelFromText("2026/10/08 ERROR upstream failed")).toBe("error");
    expect(levelFromText("WARN slow query")).toBe("warn");
    expect(levelFromText("DEBUG cache miss")).toBe("debug");
  });

  it("leaves ordinary lines unlabelled", () => {
    expect(levelFromText("GET /index.php 200")).toBeUndefined();
  });
});
