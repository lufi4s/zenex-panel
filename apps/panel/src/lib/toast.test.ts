import { afterEach, describe, expect, it, vi } from "vitest";
import { dismiss, getToasts, subscribe, toast } from "./toast";

afterEach(() => {
  vi.useRealTimers();
  getToasts().forEach((t) => dismiss(t.id));
});

describe("toast store", () => {
  it("adds a message and notifies subscribers", () => {
    const listener = vi.fn();
    const unsubscribe = subscribe(listener);
    toast.error("Domain is in use");
    expect(getToasts().at(-1)?.message).toBe("Domain is in use");
    expect(getToasts().at(-1)?.tone).toBe("error");
    expect(listener).toHaveBeenCalled();
    unsubscribe();
  });

  it("removes a message after its time is up", () => {
    vi.useFakeTimers();
    toast.success("Saved");
    expect(getToasts().some((t) => t.message === "Saved")).toBe(true);
    vi.advanceTimersByTime(4_500);
    expect(getToasts().some((t) => t.message === "Saved")).toBe(false);
  });

  it("keeps at most four messages", () => {
    for (let i = 0; i < 6; i++) toast.info(`m${i}`);
    expect(getToasts().length).toBeLessThanOrEqual(4);
  });
});
